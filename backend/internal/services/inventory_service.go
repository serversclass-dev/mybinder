package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"souqiq/internal/models"
	"souqiq/internal/utils"
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrInventoryNotFound    = errors.New("inventory record not found")
	ErrInsufficientStock    = errors.New("insufficient stock")
	ErrBackorderNotAllowed  = errors.New("backorder not allowed for this product")
)

// ── DTOs ──────────────────────────────────────────────────────────────────────

type UpdateStockInput struct {
	ProductID   string `json:"product_id"   validate:"required,uuid"`
	Quantity    int    `json:"quantity"     validate:"required,gt=0"`
	Note        string `json:"note"`
}

type BulkStockUpdate struct {
	Items []UpdateStockInput `json:"items" validate:"required,min=1"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type InventoryService struct {
	db *sqlx.DB
}

func NewInventoryService(db *sqlx.DB) *InventoryService {
	return &InventoryService{db: db}
}

// ── Validation ────────────────────────────────────────────────────────────────

// ValidateStock checks if the requested qty can be fulfilled.
func (s *InventoryService) ValidateStock(ctx context.Context, productID string, qty int) error {
	inv, err := s.getInventory(ctx, productID)
	if err != nil {
		return err
	}
	if !inv.CanFulfill(qty) {
		return fmt.Errorf("%w: product %s (available: %d, requested: %d)",
			ErrInsufficientStock, productID, inv.AvailableForSale(), qty)
	}
	return nil
}

// ── Reservation ───────────────────────────────────────────────────────────────

// Reserve increases the reserved qty when item is added to cart.
func (s *InventoryService) Reserve(ctx context.Context, productID string, qty int) error {
	inv, err := s.getInventory(ctx, productID)
	if err != nil {
		return err
	}
	if !inv.CanFulfill(qty) {
		return ErrInsufficientStock
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx, `
		UPDATE inventory
		SET quantity_reserved = quantity_reserved + @p1, updated_at = @p2
		WHERE product_id = @p3`,
		qty, now, productID,
	)
	if err != nil {
		return fmt.Errorf("reserve stock: %w", err)
	}

	s.writeLog(ctx, productID, models.InvChangeReserved,
		inv.QuantityAvailable, qty, inv.QuantityAvailable, nil, nil, "system")
	return nil
}

// Unreserve decreases the reserved qty when item is removed from cart.
func (s *InventoryService) Unreserve(ctx context.Context, productID string, qty int) error {
	inv, err := s.getInventory(ctx, productID)
	if err != nil {
		return err
	}

	actual := qty
	if inv.QuantityReserved < qty {
		actual = inv.QuantityReserved // never go negative
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx, `
		UPDATE inventory
		SET quantity_reserved = quantity_reserved - @p1, updated_at = @p2
		WHERE product_id = @p3`,
		actual, now, productID,
	)
	if err != nil {
		return fmt.Errorf("unreserve stock: %w", err)
	}

	s.writeLog(ctx, productID, models.InvChangeUnreserved,
		inv.QuantityAvailable, -actual, inv.QuantityAvailable, nil, nil, "system")
	return nil
}

// CommitReservation converts reservation to an actual sale:
// decreases both available and reserved qty.
func (s *InventoryService) CommitReservation(ctx context.Context, tx *sqlx.Tx, productID string, qty int, subOrderID string) error {
	inv, err := s.getInventory(ctx, productID)
	if err != nil {
		return err
	}

	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		UPDATE inventory
		SET quantity_available = quantity_available - @p1,
		    quantity_reserved  = quantity_reserved  - @p1,
		    updated_at         = @p2
		WHERE product_id = @p3`,
		qty, now, productID,
	)
	if err != nil {
		return fmt.Errorf("commit reservation: %w", err)
	}

	newQty := inv.QuantityAvailable - qty
	refID := subOrderID
	s.writeLog(ctx, productID, models.InvChangeSale,
		inv.QuantityAvailable, -qty, newQty, &refID, nil, "system")
	return nil
}

// ReleaseReservation releases all reservations for a cancelled sub-order.
func (s *InventoryService) ReleaseReservation(ctx context.Context, subOrderID string) {
	// load all items in the sub-order
	var items []struct {
		ProductID string `db:"product_id"`
		Quantity  int    `db:"quantity"`
	}
	_ = s.db.SelectContext(ctx, &items,
		`SELECT product_id, quantity FROM order_items WHERE sub_order_id = @p1`, subOrderID,
	)
	for _, item := range items {
		_ = s.Unreserve(ctx, item.ProductID, item.Quantity)
	}
}

// RestockFromReturn adds qty back to available after a return.
func (s *InventoryService) RestockFromReturn(ctx context.Context, productID string, qty int, subOrderID, performedBy string) error {
	inv, err := s.getInventory(ctx, productID)
	if err != nil {
		return err
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx, `
		UPDATE inventory
		SET quantity_available = quantity_available + @p1, updated_at = @p2
		WHERE product_id = @p3`,
		qty, now, productID,
	)
	if err != nil {
		return fmt.Errorf("restock: %w", err)
	}

	newQty := inv.QuantityAvailable + qty
	s.writeLog(ctx, productID, models.InvChangeReturn,
		inv.QuantityAvailable, qty, newQty, &subOrderID, nil, performedBy)
	return nil
}

// ── Manual Stock Updates ──────────────────────────────────────────────────────

// UpdateStock sets the absolute available quantity for a product.
func (s *InventoryService) UpdateStock(ctx context.Context, productID, supplierID, performedBy string, newQty int, note string) error {
	// verify product belongs to supplier
	var count int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM products WHERE id = @p1 AND supplier_id = @p2`,
		productID, supplierID,
	).Scan(&count)
	if count == 0 {
		return ErrProductNotOwned
	}

	inv, err := s.getInventory(ctx, productID)
	if err != nil {
		return err
	}

	change := newQty - inv.QuantityAvailable
	now := time.Now()

	_, err = s.db.ExecContext(ctx, `
		UPDATE inventory
		SET quantity_available = @p1, updated_at = @p2
		WHERE product_id = @p3`,
		newQty, now, productID,
	)
	if err != nil {
		return fmt.Errorf("update stock: %w", err)
	}

	notePtr := utils.StrPtr(note)
	s.writeLog(ctx, productID, models.InvChangeAdjustment,
		inv.QuantityAvailable, change, newQty, nil, notePtr, performedBy)
	return nil
}

// UpdateThreshold sets the low stock alert threshold.
func (s *InventoryService) UpdateThreshold(ctx context.Context, productID string, threshold int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE inventory
		SET low_stock_threshold = @p1, updated_at = @p2
		WHERE product_id = @p3`,
		threshold, time.Now(), productID,
	)
	return err
}

// BulkUpdate processes multiple stock updates from a CSV upload.
func (s *InventoryService) BulkUpdate(ctx context.Context, supplierID, performedBy string, items []UpdateStockInput) []error {
	errs := make([]error, 0)
	for _, item := range items {
		if err := s.UpdateStock(ctx, item.ProductID, supplierID, performedBy, item.Quantity, item.Note); err != nil {
			errs = append(errs, fmt.Errorf("product %s: %w", item.ProductID, err))
		}
	}
	return errs
}

// ── Low Stock Alerts ──────────────────────────────────────────────────────────

type LowStockItem struct {
	ProductID         string `db:"product_id"`
	ProductName       string `db:"product_name"`
	SupplierID        string `db:"supplier_id"`
	QuantityAvailable int    `db:"quantity_available"`
	QuantityReserved  int    `db:"quantity_reserved"`
	LowStockThreshold int    `db:"low_stock_threshold"`
}

// GetLowStockItems returns all products at or below their low stock threshold.
func (s *InventoryService) GetLowStockItems(ctx context.Context, supplierID string) ([]LowStockItem, error) {
	query := `
		SELECT i.product_id,
		       p.name_ar AS product_name,
		       p.supplier_id,
		       i.quantity_available,
		       i.quantity_reserved,
		       i.low_stock_threshold
		FROM inventory i
		JOIN products p ON p.id = i.product_id
		WHERE (i.quantity_available - i.quantity_reserved) <= i.low_stock_threshold
		  AND p.status = 'active'`

	args := []any{}
	if supplierID != "" {
		query += " AND p.supplier_id = @p1"
		args = append(args, supplierID)
	}
	query += " ORDER BY (i.quantity_available - i.quantity_reserved) ASC"

	var items []LowStockItem
	err := s.db.SelectContext(ctx, &items, query, args...)
	return items, err
}

// CheckAndAlertLowStock is called by the background scheduler.
// It finds all low-stock products and queues notifications to suppliers.
func (s *InventoryService) CheckAndAlertLowStock(ctx context.Context, notifService *NotificationService) error {
	items, err := s.GetLowStockItems(ctx, "")
	if err != nil {
		return err
	}

	// group by supplier
	bySupplier := map[string][]LowStockItem{}
	for _, item := range items {
		bySupplier[item.SupplierID] = append(bySupplier[item.SupplierID], item)
	}

	for supplierID, supplierItems := range bySupplier {
		// get supplier user_id for notification
		var userID string
		_ = s.db.QueryRowContext(ctx,
			`SELECT user_id FROM suppliers WHERE id = @p1`, supplierID,
		).Scan(&userID)
		if userID == "" {
			continue
		}

		for _, item := range supplierItems {
			go notifService.LowStockAlert(context.Background(), userID, item.ProductID, item.ProductName,
				item.QuantityAvailable-item.QuantityReserved)
		}
	}
	return nil
}

// ── Cart Expiry Cleanup ───────────────────────────────────────────────────────

// ReleaseExpiredCartReservations unreserves stock for all expired carts.
func (s *InventoryService) ReleaseExpiredCartReservations(ctx context.Context) error {
	// find expired carts
	var expiredCarts []struct {
		ID string `db:"id"`
	}
	err := s.db.SelectContext(ctx, &expiredCarts, `
		SELECT id FROM carts
		WHERE is_active = 1 AND expires_at < @p1`,
		time.Now(),
	)
	if err != nil {
		return err
	}

	for _, cart := range expiredCarts {
		// load cart items
		var items []struct {
			ProductID string `db:"product_id"`
			Quantity  int    `db:"quantity"`
		}
		_ = s.db.SelectContext(ctx, &items,
			`SELECT product_id, quantity FROM cart_items WHERE cart_id = @p1`, cart.ID,
		)

		// unreserve each item
		for _, item := range items {
			_ = s.Unreserve(ctx, item.ProductID, item.Quantity)
		}

		// deactivate cart
		_, _ = s.db.ExecContext(ctx, `
			UPDATE carts SET is_active = 0, updated_at = @p1 WHERE id = @p2`,
			time.Now(), cart.ID,
		)
	}
	return nil
}

// ── Read ──────────────────────────────────────────────────────────────────────

// GetInventory returns the inventory for a product.
func (s *InventoryService) GetInventory(ctx context.Context, productID string) (*models.Inventory, error) {
	return s.getInventory(ctx, productID)
}

// GetInventoryOverview returns paginated inventory for a supplier's products.
func (s *InventoryService) GetInventoryOverview(ctx context.Context, supplierID string, pagination utils.PaginationParams) ([]models.Inventory, int64, error) {
	var total int64
	_ = s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM inventory i
		JOIN products p ON p.id = i.product_id
		WHERE p.supplier_id = @p1`, supplierID,
	).Scan(&total)

	var items []models.Inventory
	err := s.db.SelectContext(ctx, &items, `
		SELECT i.* FROM inventory i
		JOIN products p ON p.id = i.product_id
		WHERE p.supplier_id = @p1
		ORDER BY (i.quantity_available - i.quantity_reserved) ASC
		OFFSET @p2 ROWS FETCH NEXT @p3 ROWS ONLY`,
		supplierID, pagination.Offset, pagination.PerPage,
	)
	return items, total, err
}

// GetInventoryLogs returns stock movement history for a product.
func (s *InventoryService) GetInventoryLogs(ctx context.Context, productID string, pagination utils.PaginationParams) ([]models.InventoryLog, int64, error) {
	var total int64
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM inventory_logs WHERE product_id = @p1`, productID,
	).Scan(&total)

	var logs []models.InventoryLog
	err := s.db.SelectContext(ctx, &logs, `
		SELECT * FROM inventory_logs WHERE product_id = @p1
		ORDER BY created_at DESC
		OFFSET @p2 ROWS FETCH NEXT @p3 ROWS ONLY`,
		productID, pagination.Offset, pagination.PerPage,
	)
	return logs, total, err
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (s *InventoryService) getInventory(ctx context.Context, productID string) (*models.Inventory, error) {
	var inv models.Inventory
	err := s.db.GetContext(ctx, &inv,
		`SELECT * FROM inventory WHERE product_id = @p1`, productID,
	)
	if err != nil {
		return nil, ErrInventoryNotFound
	}
	return &inv, nil
}

func (s *InventoryService) writeLog(
	ctx context.Context,
	productID, changeType string,
	before, change, after int,
	referenceID *string,
	note *string,
	performedBy string,
) {
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO inventory_logs (
			id, product_id, change_type,
			quantity_before, quantity_change, quantity_after,
			reference_id, note, performed_by, created_at
		) VALUES (
			@p1, @p2, @p3,
			@p4, @p5, @p6,
			@p7, @p8, @p9, @p10
		)`,
		utils.NewUUID(), productID, changeType,
		before, change, after,
		referenceID, note, performedBy, time.Now(),
	)
}