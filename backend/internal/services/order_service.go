package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"souqiq/internal/cache"
	"souqiq/internal/models"
	"souqiq/internal/utils"
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrOrderNotFound          = errors.New("order not found")
	ErrSubOrderNotFound       = errors.New("sub-order not found")
	ErrSubOrderNotOwned       = errors.New("sub-order does not belong to this supplier")
	ErrInvalidTransition      = errors.New("invalid status transition")
	ErrTerminalStatus         = errors.New("sub-order is in a terminal state")
	ErrCancellationWindowOver = errors.New("cancellation window has closed")
	ErrCartEmpty              = errors.New("cart is empty")
	ErrStockInsufficient      = errors.New("insufficient stock for one or more items")
	ErrAddressNotFound        = errors.New("delivery address not found")
)

// ── DTOs ──────────────────────────────────────────────────────────────────────

type PlaceOrderInput struct {
	CustomerID        string `json:"-"` // from auth context
	DeliveryAddressID string `json:"delivery_address_id" validate:"required,uuid"`
	PaymentMethod     string `json:"payment_method"      validate:"required,oneof=cod bank_transfer online"`
	Notes             string `json:"notes"`
}

type SubOrderGroup struct {
	SupplierID   string
	Items        []CartItemSnapshot
	Subtotal     float64
	DeliveryFee  float64
	CommissionRate float64
	CommissionAmount float64
	SupplierNet  float64
}

type CartItemSnapshot struct {
	ProductID           string
	ProductNameSnapshot string
	SupplierID          string
	Quantity            int
	UnitPrice           float64
	IsWholesale         bool
	LineTotal           float64
	DiscountApplied     float64
}

type OrderStatusUpdateInput struct {
	Note   string `json:"note"`
	Reason string `json:"reason"`
}

type AssignDeliveryInput struct {
	DeliveryCompanyID string `json:"delivery_company_id" validate:"required,uuid"`
	Note              string `json:"note"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type OrderService struct {
	db          *sqlx.DB
	commService *CommissionService
	invService  *InventoryService
	notifService *NotificationService
}

func NewOrderService(db *sqlx.DB, commService *CommissionService, invService *InventoryService, notifService *NotificationService) *OrderService {
	return &OrderService{
		db:           db,
		commService:  commService,
		invService:   invService,
		notifService: notifService,
	}
}

// ── Place Order ───────────────────────────────────────────────────────────────

func (s *OrderService) PlaceOrder(ctx context.Context, input PlaceOrderInput) (*models.Order, error) {
	// 1. load active cart
	cart, err := s.loadActiveCart(ctx, input.CustomerID)
	if err != nil || len(cart.Items) == 0 {
		return nil, ErrCartEmpty
	}

	// 2. validate delivery address belongs to customer
	if err := s.validateAddress(ctx, input.DeliveryAddressID, input.CustomerID); err != nil {
		return nil, err
	}

	// 3. validate stock for all items
	for _, item := range cart.Items {
		if err := s.invService.ValidateStock(ctx, item.ProductID, item.Quantity); err != nil {
			return nil, fmt.Errorf("%w: product %s", ErrStockInsufficient, item.ProductID)
		}
	}

	// 4. group items by supplier
	groups := s.groupBySupplier(ctx, cart.Items, input.DeliveryAddressID)

	// 5. calculate totals
	var totalSubtotal, totalDelivery, totalCommission float64
	for _, g := range groups {
		totalSubtotal += g.Subtotal
		totalDelivery += g.DeliveryFee
		totalCommission += g.CommissionAmount
	}
	grandTotal := utils.RoundToTwo(totalSubtotal + totalDelivery)

	// 6. generate order number
	orderNumber, err := s.nextOrderNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate order number: %w", err)
	}

	orderID := utils.NewUUID()
	now := time.Now()

	err = withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		// insert master order
		_, err := tx.ExecContext(ctx, `
			INSERT INTO orders (
				id, order_number, customer_id, delivery_address_id,
				status, payment_method, payment_status,
				subtotal, total_delivery_fee, total_commission, grand_total,
				notes, created_at, updated_at
			) VALUES (
				@p1, @p2, @p3, @p4,
				@p5, @p6, @p7,
				@p8, @p9, @p10, @p11,
				@p12, @p13, @p14
			)`,
			orderID, orderNumber, input.CustomerID, input.DeliveryAddressID,
			models.OrderStatusPending, input.PaymentMethod, models.PaymentStatusUnpaid,
			totalSubtotal, totalDelivery, totalCommission, grandTotal,
			utils.StrPtr(input.Notes), now, now,
		)
		if err != nil {
			return fmt.Errorf("insert master order: %w", err)
		}

		// insert sub-orders
		for i, group := range groups {
			subOrderID := utils.NewUUID()
			subOrderNumber := utils.FormatSubOrderNumber(orderNumber, i)

			_, err = tx.ExecContext(ctx, `
				INSERT INTO sub_orders (
					id, sub_order_number, order_id, supplier_id,
					status, delivery_fee, subtotal,
					commission_rate, commission_amount, supplier_net,
					created_at, updated_at
				) VALUES (
					@p1, @p2, @p3, @p4,
					@p5, @p6, @p7,
					@p8, @p9, @p10,
					@p11, @p12
				)`,
				subOrderID, subOrderNumber, orderID, group.SupplierID,
				models.SubOrderStatusPendingSupplier,
				group.DeliveryFee, group.Subtotal,
				group.CommissionRate, group.CommissionAmount, group.SupplierNet,
				now, now,
			)
			if err != nil {
				return fmt.Errorf("insert sub-order: %w", err)
			}

			// insert order items
			for _, item := range group.Items {
				_, err = tx.ExecContext(ctx, `
					INSERT INTO order_items (
						id, sub_order_id, order_id, product_id, supplier_id,
						product_name_snapshot, quantity, unit_price,
						is_wholesale, line_total, discount_applied,
						created_at, updated_at
					) VALUES (
						@p1, @p2, @p3, @p4, @p5,
						@p6, @p7, @p8,
						@p9, @p10, @p11,
						@p12, @p13
					)`,
					utils.NewUUID(), subOrderID, orderID, item.ProductID, item.SupplierID,
					item.ProductNameSnapshot, item.Quantity, item.UnitPrice,
					item.IsWholesale, item.LineTotal, item.DiscountApplied,
					now, now,
				)
				if err != nil {
					return fmt.Errorf("insert order item: %w", err)
				}

				// commit inventory reservation → confirmed sale
				if err := s.invService.CommitReservation(ctx, tx, item.ProductID, item.Quantity, subOrderID); err != nil {
					return fmt.Errorf("commit inventory: %w", err)
				}
			}

			// log initial status
			s.writeStatusLog(ctx, tx, subOrderID, "", models.SubOrderStatusPendingSupplier, input.CustomerID, "Order placed")
		}

		// deactivate cart
		_, err = tx.ExecContext(ctx,
			`UPDATE carts SET is_active = 0, updated_at = @p1 WHERE id = @p2`,
			now, cart.ID,
		)
		return err
	})
	if err != nil {
		return nil, err
	}

	// invalidate cart cache
	_ = cache.Del(ctx, cache.CartKey(input.CustomerID))

	// send notifications async (best-effort)
	go s.notifService.OrderPlaced(context.Background(), orderID, input.CustomerID)

	return s.GetOrderByID(ctx, orderID, input.CustomerID, models.RoleCustomer)
}

// ── Supplier Order Actions ────────────────────────────────────────────────────

func (s *OrderService) SupplierAccept(ctx context.Context, subOrderID, supplierID, actorID string) error {
	sub, err := s.getSubOrderForSupplier(ctx, subOrderID, supplierID)
	if err != nil {
		return err
	}
	return s.transitionSubOrder(ctx, sub, models.SubOrderStatusAccepted, actorID, "")
}

func (s *OrderService) SupplierReject(ctx context.Context, subOrderID, supplierID, actorID, reason string) error {
	sub, err := s.getSubOrderForSupplier(ctx, subOrderID, supplierID)
	if err != nil {
		return err
	}
	if err := s.transitionSubOrder(ctx, sub, models.SubOrderStatusRejected, actorID, reason); err != nil {
		return err
	}
	// release inventory
	go s.invService.ReleaseReservation(context.Background(), subOrderID)
	return nil
}

func (s *OrderService) SupplierMarkPreparing(ctx context.Context, subOrderID, supplierID, actorID string) error {
	sub, err := s.getSubOrderForSupplier(ctx, subOrderID, supplierID)
	if err != nil {
		return err
	}
	return s.transitionSubOrder(ctx, sub, models.SubOrderStatusPreparing, actorID, "")
}

func (s *OrderService) SupplierMarkReady(ctx context.Context, subOrderID, supplierID, actorID string) error {
	sub, err := s.getSubOrderForSupplier(ctx, subOrderID, supplierID)
	if err != nil {
		return err
	}
	if err := s.transitionSubOrder(ctx, sub, models.SubOrderStatusReadyForPickup, actorID, ""); err != nil {
		return err
	}
	// notify admin to assign delivery
	go s.notifService.ReadyForPickup(context.Background(), subOrderID)
	return nil
}

// ── Admin Actions ─────────────────────────────────────────────────────────────

func (s *OrderService) AdminAssignDelivery(ctx context.Context, subOrderID, actorID string, input AssignDeliveryInput) error {
	sub, err := s.getSubOrder(ctx, subOrderID)
	if err != nil {
		return err
	}
	if sub.Status != models.SubOrderStatusReadyForPickup {
		return ErrInvalidTransition
	}

	now := time.Now()
	trackingNumber := "TRK-" + utils.NewUUID()[:8]

	_, err = s.db.ExecContext(ctx, `
		UPDATE sub_orders
		SET delivery_company_id = @p1, tracking_number = @p2, assigned_by = @p3, updated_at = @p4
		WHERE id = @p5`,
		input.DeliveryCompanyID, trackingNumber, actorID, now, subOrderID,
	)
	if err != nil {
		return fmt.Errorf("assign delivery: %w", err)
	}

	go s.notifService.DeliveryAssigned(context.Background(), subOrderID, input.DeliveryCompanyID)
	return nil
}

func (s *OrderService) AdminCancelSubOrder(ctx context.Context, subOrderID, actorID, reason string) error {
	sub, err := s.getSubOrder(ctx, subOrderID)
	if err != nil {
		return err
	}
	if models.IsTerminalStatus(sub.Status) {
		return ErrTerminalStatus
	}
	if err := s.transitionSubOrder(ctx, sub, models.SubOrderStatusCancelled, actorID, reason); err != nil {
		return err
	}
	go s.invService.ReleaseReservation(context.Background(), subOrderID)
	return nil
}

func (s *OrderService) AdminForceComplete(ctx context.Context, subOrderID, actorID string) error {
	sub, err := s.getSubOrder(ctx, subOrderID)
	if err != nil {
		return err
	}
	if sub.Status != models.SubOrderStatusDelivered {
		return ErrInvalidTransition
	}
	return s.confirmSubOrder(ctx, sub, actorID)
}

// ── Delivery Company Actions ──────────────────────────────────────────────────

func (s *OrderService) DeliveryAccept(ctx context.Context, subOrderID, companyID, actorID string) error {
	sub, err := s.getSubOrderForDelivery(ctx, subOrderID, companyID)
	if err != nil {
		return err
	}
	_ = sub
	// acceptance is implicit — first status update is picked_up
	return nil
}

func (s *OrderService) DeliveryReject(ctx context.Context, subOrderID, companyID, actorID, reason string) error {
	_, err := s.getSubOrderForDelivery(ctx, subOrderID, companyID)
	if err != nil {
		return err
	}
	// unassign delivery company + notify admin
	now := time.Now()
	_, err = s.db.ExecContext(ctx, `
		UPDATE sub_orders
		SET delivery_company_id = NULL, tracking_number = NULL, assigned_by = NULL, updated_at = @p1
		WHERE id = @p2`,
		now, subOrderID,
	)
	go s.notifService.DeliveryRejected(context.Background(), subOrderID, reason)
	return err
}

func (s *OrderService) DeliveryMarkPickedUp(ctx context.Context, subOrderID, companyID, actorID string) error {
	sub, err := s.getSubOrderForDelivery(ctx, subOrderID, companyID)
	if err != nil {
		return err
	}
	if err := s.transitionSubOrder(ctx, sub, models.SubOrderStatusPickedUp, actorID, ""); err != nil {
		return err
	}
	now := time.Now()
	_, _ = s.db.ExecContext(ctx,
		`UPDATE sub_orders SET picked_up_at = @p1 WHERE id = @p2`, now, subOrderID,
	)
	return nil
}

func (s *OrderService) DeliveryMarkInTransit(ctx context.Context, subOrderID, companyID, actorID string) error {
	sub, err := s.getSubOrderForDelivery(ctx, subOrderID, companyID)
	if err != nil {
		return err
	}
	return s.transitionSubOrder(ctx, sub, models.SubOrderStatusInTransit, actorID, "")
}

func (s *OrderService) DeliveryMarkDelivered(ctx context.Context, subOrderID, companyID, actorID string) error {
	sub, err := s.getSubOrderForDelivery(ctx, subOrderID, companyID)
	if err != nil {
		return err
	}
	if err := s.transitionSubOrder(ctx, sub, models.SubOrderStatusDelivered, actorID, ""); err != nil {
		return err
	}
	now := time.Now()
	_, _ = s.db.ExecContext(ctx,
		`UPDATE sub_orders SET delivered_at = @p1 WHERE id = @p2`, now, subOrderID,
	)
	go s.notifService.OrderDelivered(context.Background(), subOrderID)
	return nil
}

func (s *OrderService) DeliveryReportFailure(ctx context.Context, subOrderID, companyID, actorID, reason string) error {
	sub, err := s.getSubOrderForDelivery(ctx, subOrderID, companyID)
	if err != nil {
		return err
	}
	if err := s.transitionSubOrder(ctx, sub, models.SubOrderStatusFailedDelivery, actorID, reason); err != nil {
		return err
	}
	go s.notifService.DeliveryFailed(context.Background(), subOrderID, reason)
	return nil
}

// ── Customer Actions ──────────────────────────────────────────────────────────

func (s *OrderService) CustomerCancelSubOrder(ctx context.Context, subOrderID, customerID, actorID string) error {
	sub, err := s.getSubOrderForCustomer(ctx, subOrderID, customerID)
	if err != nil {
		return err
	}

	// only allowed in pending_supplier within cancellation window
	if sub.Status != models.SubOrderStatusPendingSupplier {
		return ErrCancellationWindowOver
	}

	windowMins := s.getCancellationWindowMins(ctx)
	if time.Since(sub.CreatedAt) > time.Duration(windowMins)*time.Minute {
		return ErrCancellationWindowOver
	}

	if err := s.transitionSubOrder(ctx, sub, models.SubOrderStatusCancelled, actorID, "Cancelled by customer"); err != nil {
		return err
	}
	go s.invService.ReleaseReservation(context.Background(), subOrderID)
	return nil
}

func (s *OrderService) CustomerConfirmDelivery(ctx context.Context, subOrderID, customerID, actorID string) error {
	sub, err := s.getSubOrderForCustomer(ctx, subOrderID, customerID)
	if err != nil {
		return err
	}
	if sub.Status != models.SubOrderStatusDelivered {
		return ErrInvalidTransition
	}
	return s.confirmSubOrder(ctx, sub, actorID)
}

// ── Auto-Confirm (called by background job) ───────────────────────────────────

func (s *OrderService) AutoConfirmDelivered(ctx context.Context) error {
	windowHours := s.getAutoCompleteHours(ctx)
	cutoff := time.Now().Add(-time.Duration(windowHours) * time.Hour)

	var subOrders []models.SubOrder
	err := s.db.SelectContext(ctx, &subOrders, `
		SELECT * FROM sub_orders
		WHERE status = @p1 AND delivered_at < @p2`,
		models.SubOrderStatusDelivered, cutoff,
	)
	if err != nil {
		return err
	}

	for _, sub := range subOrders {
		subCopy := sub
		_ = s.confirmSubOrder(ctx, &subCopy, "system")
	}
	return nil
}

// ── Get / List ────────────────────────────────────────────────────────────────

func (s *OrderService) GetOrderByID(ctx context.Context, orderID, actorID, role string) (*models.Order, error) {
	var order models.Order
	err := s.db.GetContext(ctx, &order,
		`SELECT * FROM orders WHERE id = @p1`, orderID,
	)
	if err != nil {
		return nil, ErrOrderNotFound
	}

	// customer can only see their own orders
	if role == models.RoleCustomer && order.CustomerID != actorID {
		return nil, ErrOrderNotFound
	}

	// load sub-orders
	err = s.db.SelectContext(ctx, &order.SubOrders,
		`SELECT * FROM sub_orders WHERE order_id = @p1 ORDER BY created_at`, orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("load sub-orders: %w", err)
	}

	// load items for each sub-order
	for i := range order.SubOrders {
		_ = s.db.SelectContext(ctx, &order.SubOrders[i].Items,
			`SELECT * FROM order_items WHERE sub_order_id = @p1`, order.SubOrders[i].ID,
		)
	}

	return &order, nil
}

func (s *OrderService) ListOrders(ctx context.Context, filter map[string]any, pagination utils.PaginationParams) ([]models.Order, int64, error) {
	where := "1=1"
	args := []any{}
	idx := 1

	if customerID, ok := filter["customer_id"].(string); ok && customerID != "" {
		where += fmt.Sprintf(" AND customer_id = @p%d", idx)
		args = append(args, customerID)
		idx++
	}
	if status, ok := filter["status"].(string); ok && status != "" {
		where += fmt.Sprintf(" AND status = @p%d", idx)
		args = append(args, status)
		idx++
	}

	var total int64
	_ = s.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(1) FROM orders WHERE %s`, where), args...,
	).Scan(&total)

	args = append(args, pagination.Offset, pagination.PerPage)
	var orders []models.Order
	err := s.db.SelectContext(ctx, &orders, fmt.Sprintf(`
		SELECT * FROM orders WHERE %s
		ORDER BY created_at DESC
		OFFSET @p%d ROWS FETCH NEXT @p%d ROWS ONLY`,
		where, idx, idx+1,
	), args...)
	if err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

func (s *OrderService) ListSubOrders(ctx context.Context, filter map[string]any, pagination utils.PaginationParams) ([]models.SubOrder, int64, error) {
	where := "1=1"
	args := []any{}
	idx := 1

	if supplierID, ok := filter["supplier_id"].(string); ok && supplierID != "" {
		where += fmt.Sprintf(" AND supplier_id = @p%d", idx)
		args = append(args, supplierID)
		idx++
	}
	if deliveryID, ok := filter["delivery_company_id"].(string); ok && deliveryID != "" {
		where += fmt.Sprintf(" AND delivery_company_id = @p%d", idx)
		args = append(args, deliveryID)
		idx++
	}
	if status, ok := filter["status"].(string); ok && status != "" {
		where += fmt.Sprintf(" AND status = @p%d", idx)
		args = append(args, status)
		idx++
	}

	var total int64
	_ = s.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(1) FROM sub_orders WHERE %s`, where), args...,
	).Scan(&total)

	args = append(args, pagination.Offset, pagination.PerPage)
	var subs []models.SubOrder
	err := s.db.SelectContext(ctx, &subs, fmt.Sprintf(`
		SELECT * FROM sub_orders WHERE %s
		ORDER BY created_at DESC
		OFFSET @p%d ROWS FETCH NEXT @p%d ROWS ONLY`,
		where, idx, idx+1,
	), args...)
	if err != nil {
		return nil, 0, err
	}
	return subs, total, nil
}

// ── Core state machine transition ─────────────────────────────────────────────

func (s *OrderService) transitionSubOrder(ctx context.Context, sub *models.SubOrder, newStatus, actorID, note string) error {
	if models.IsTerminalStatus(sub.Status) {
		return ErrTerminalStatus
	}
	if !models.CanTransitionTo(sub.Status, newStatus) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, sub.Status, newStatus)
	}

	now := time.Now()
	setClauses := "status = @p1, updated_at = @p2"
	args := []any{newStatus, now}
	idx := 3

	// set timestamp fields
	switch newStatus {
	case models.SubOrderStatusAccepted:
		setClauses += fmt.Sprintf(", supplier_accepted_at = @p%d", idx)
		args = append(args, now)
		idx++
	case models.SubOrderStatusReadyForPickup:
		setClauses += fmt.Sprintf(", ready_at = @p%d", idx)
		args = append(args, now)
		idx++
	case models.SubOrderStatusRejected, models.SubOrderStatusCancelled:
		if note != "" {
			setClauses += fmt.Sprintf(", rejection_reason = @p%d", idx)
			args = append(args, note)
			idx++
		}
	}

	args = append(args, sub.ID)
	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`UPDATE sub_orders SET %s WHERE id = @p%d`, setClauses, idx),
		args...,
	)
	if err != nil {
		return fmt.Errorf("update sub-order status: %w", err)
	}

	err = withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		s.writeStatusLog(ctx, tx, sub.ID, sub.Status, newStatus, actorID, note)
		return nil
	})
	if err != nil {
		// non-fatal — log but don't fail
		fmt.Printf("[order] failed to write status log: %v\n", err)
	}

	// derive and update master order status
	go s.deriveMasterStatus(context.Background(), sub.OrderID)

	return nil
}

func (s *OrderService) confirmSubOrder(ctx context.Context, sub *models.SubOrder, actorID string) error {
	if err := s.transitionSubOrder(ctx, sub, models.SubOrderStatusConfirmed, actorID, ""); err != nil {
		return err
	}
	// trigger commission calculation
	go s.commService.CalculateAndCredit(context.Background(), sub.ID)
	return nil
}

// ── Master order status derivation ───────────────────────────────────────────

func (s *OrderService) deriveMasterStatus(ctx context.Context, orderID string) {
	var statuses []string
	_ = s.db.SelectContext(ctx, &statuses,
		`SELECT status FROM sub_orders WHERE order_id = @p1`, orderID,
	)
	if len(statuses) == 0 {
		return
	}

	newStatus := deriveMasterOrderStatus(statuses)
	_, _ = s.db.ExecContext(ctx,
		`UPDATE orders SET status = @p1, updated_at = @p2 WHERE id = @p3`,
		newStatus, time.Now(), orderID,
	)
}

func deriveMasterOrderStatus(subStatuses []string) string {
	allTerminal := true
	allCancelledOrRejected := true
	anyComplete := false

	for _, s := range subStatuses {
		if !models.IsTerminalStatus(s) {
			allTerminal = false
		}
		if s != models.SubOrderStatusCancelled && s != models.SubOrderStatusRejected {
			allCancelledOrRejected = false
		}
		if s == models.SubOrderStatusConfirmed {
			anyComplete = true
		}
	}

	if !allTerminal {
		if anyComplete {
			return models.OrderStatusPartiallyComplete
		}
		return models.OrderStatusProcessing
	}
	if allCancelledOrRejected {
		return models.OrderStatusCancelled
	}
	if anyComplete {
		return models.OrderStatusComplete
	}
	return models.OrderStatusComplete
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (s *OrderService) loadActiveCart(ctx context.Context, customerID string) (*models.Cart, error) {
	var cart models.Cart
	err := s.db.GetContext(ctx, &cart,
		`SELECT * FROM carts WHERE customer_id = @p1 AND is_active = 1`, customerID,
	)
	if err != nil {
		return nil, ErrCartEmpty
	}
	err = s.db.SelectContext(ctx, &cart.Items,
		`SELECT * FROM cart_items WHERE cart_id = @p1`, cart.ID,
	)
	if err != nil {
		return nil, err
	}
	return &cart, nil
}

func (s *OrderService) validateAddress(ctx context.Context, addressID, customerID string) error {
	var count int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM customer_addresses WHERE id = @p1 AND customer_id = @p2`,
		addressID, customerID,
	).Scan(&count)
	if count == 0 {
		return ErrAddressNotFound
	}
	return nil
}

func (s *OrderService) groupBySupplier(ctx context.Context, items []models.CartItem, addressID string) []SubOrderGroup {
	grouped := map[string]*SubOrderGroup{}

	for _, item := range items {
		g, ok := grouped[item.SupplierID]
		if !ok {
			rate := s.commService.ResolveRate(ctx, item.ProductID, item.SupplierID, item.IsWholesale)
			deliveryFee := s.resolveDeliveryFee(ctx, item.SupplierID, addressID)
			g = &SubOrderGroup{
				SupplierID:     item.SupplierID,
				CommissionRate: rate,
				DeliveryFee:    deliveryFee,
			}
			grouped[item.SupplierID] = g
		}

		snapshot := CartItemSnapshot{
			ProductID:           item.ProductID,
			ProductNameSnapshot: s.getProductName(ctx, item.ProductID),
			SupplierID:          item.SupplierID,
			Quantity:            item.Quantity,
			UnitPrice:           item.UnitPrice,
			IsWholesale:         item.IsWholesale,
			LineTotal:           item.LineTotal(),
		}
		g.Items = append(g.Items, snapshot)
		g.Subtotal = utils.RoundToTwo(g.Subtotal + snapshot.LineTotal)
	}

	result := make([]SubOrderGroup, 0, len(grouped))
	for _, g := range grouped {
		g.CommissionAmount = utils.RoundToTwo(g.Subtotal * g.CommissionRate / 100)
		g.SupplierNet = utils.RoundToTwo(g.Subtotal - g.CommissionAmount)
		result = append(result, *g)
	}
	return result
}

func (s *OrderService) resolveDeliveryFee(ctx context.Context, supplierID, addressID string) float64 {
	var cityID string
	_ = s.db.QueryRowContext(ctx,
		`SELECT city_id FROM customer_addresses WHERE id = @p1`, addressID,
	).Scan(&cityID)

	if cityID == "" {
		return 0
	}

	var fee float64
	_ = s.db.QueryRowContext(ctx, `
		SELECT TOP 1 dz.base_price
		FROM delivery_zones dz
		JOIN delivery_companies dc ON dc.id = dz.delivery_company_id
		WHERE dz.city_id = @p1 AND dz.is_active = 1 AND dc.status = 'approved'
		ORDER BY dz.base_price ASC`,
		cityID,
	).Scan(&fee)
	return fee
}

func (s *OrderService) getProductName(ctx context.Context, productID string) string {
	var name string
	_ = s.db.QueryRowContext(ctx,
		`SELECT name_ar FROM products WHERE id = @p1`, productID,
	).Scan(&name)
	return name
}

func (s *OrderService) getSubOrder(ctx context.Context, subOrderID string) (*models.SubOrder, error) {
	var sub models.SubOrder
	err := s.db.GetContext(ctx, &sub, `SELECT * FROM sub_orders WHERE id = @p1`, subOrderID)
	if err != nil {
		return nil, ErrSubOrderNotFound
	}
	return &sub, nil
}

func (s *OrderService) getSubOrderForSupplier(ctx context.Context, subOrderID, supplierID string) (*models.SubOrder, error) {
	sub, err := s.getSubOrder(ctx, subOrderID)
	if err != nil {
		return nil, err
	}
	if sub.SupplierID != supplierID {
		return nil, ErrSubOrderNotOwned
	}
	return sub, nil
}

func (s *OrderService) getSubOrderForDelivery(ctx context.Context, subOrderID, companyID string) (*models.SubOrder, error) {
	sub, err := s.getSubOrder(ctx, subOrderID)
	if err != nil {
		return nil, err
	}
	if sub.DeliveryCompanyID == nil || *sub.DeliveryCompanyID != companyID {
		return nil, ErrSubOrderNotOwned
	}
	return sub, nil
}

func (s *OrderService) getSubOrderForCustomer(ctx context.Context, subOrderID, customerID string) (*models.SubOrder, error) {
	sub, err := s.getSubOrder(ctx, subOrderID)
	if err != nil {
		return nil, err
	}
	// verify customer owns master order
	var count int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM orders WHERE id = @p1 AND customer_id = @p2`,
		sub.OrderID, customerID,
	).Scan(&count)
	if count == 0 {
		return nil, ErrSubOrderNotFound
	}
	return sub, nil
}

func (s *OrderService) writeStatusLog(ctx context.Context, tx *sqlx.Tx, subOrderID, prev, next, changedBy, note string) {
	_, _ = tx.ExecContext(ctx, `
		INSERT INTO order_status_logs (id, sub_order_id, previous_status, new_status, changed_by, note, changed_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7)`,
		utils.NewUUID(), subOrderID, prev, next, changedBy, utils.StrPtr(note), time.Now(),
	)
}

func (s *OrderService) nextOrderNumber(ctx context.Context) (string, error) {
	year := time.Now().Year()
	var seq int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) + 1 FROM orders
		WHERE YEAR(created_at) = @p1`, year,
	).Scan(&seq)
	if err != nil {
		return "", err
	}
	return utils.FormatOrderNumber(year, seq), nil
}

func (s *OrderService) getCancellationWindowMins(ctx context.Context) int {
	var val int
	_ = s.db.QueryRowContext(ctx,
		`SELECT CAST(value AS INT) FROM platform_settings WHERE [key] = @p1`,
		models.SettingOrderCancellationWindowMins,
	).Scan(&val)
	if val == 0 {
		return 30
	}
	return val
}

func (s *OrderService) getAutoCompleteHours(ctx context.Context) int {
	var val int
	_ = s.db.QueryRowContext(ctx,
		`SELECT CAST(value AS INT) FROM platform_settings WHERE [key] = @p1`,
		models.SettingAutoCompleteOrderHours,
	).Scan(&val)
	if val == 0 {
		return 72
	}
	return val
}