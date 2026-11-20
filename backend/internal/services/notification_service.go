package services

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"souqiq/internal/models"
	"souqiq/internal/utils"
)

// NotificationService handles in-app notifications for all actors.
// SMS and email are fire-and-forget goroutines — failures are logged, not fatal.
type NotificationService struct {
	db *sqlx.DB
}

func NewNotificationService(db *sqlx.DB) *NotificationService {
	return &NotificationService{db: db}
}

// ── Notification Types ────────────────────────────────────────────────────────

const (
	NotifOrderPlaced          = "order_placed"
	NotifOrderAccepted        = "order_accepted"
	NotifOrderRejected        = "order_rejected"
	NotifOrderCancelled       = "order_cancelled"
	NotifReadyForPickup       = "ready_for_pickup"
	NotifDeliveryAssigned     = "delivery_assigned"
	NotifDeliveryRejected     = "delivery_rejected"
	NotifPickedUp             = "picked_up"
	NotifInTransit            = "in_transit"
	NotifDelivered            = "delivered"
	NotifDeliveryFailed       = "delivery_failed"
	NotifOrderConfirmed       = "order_confirmed"
	NotifWithdrawalProcessed  = "withdrawal_processed"
	NotifWithdrawalRejected   = "withdrawal_rejected"
	NotifSupplierApproved     = "supplier_approved"
	NotifSupplierRejected     = "supplier_rejected"
	NotifProductApproved      = "product_approved"
	NotifProductRejected      = "product_rejected"
	NotifLowStock             = "low_stock"
	NotifNewComplaint         = "new_complaint"
	NotifComplaintResolved    = "complaint_resolved"
	NotifNewReview            = "new_review"
)

// ── Order Notifications ───────────────────────────────────────────────────────

// OrderPlaced notifies customer (confirmation) + all suppliers (new sub-order) + admin
func (s *NotificationService) OrderPlaced(ctx context.Context, orderID, customerID string) {
	// notify customer
	s.send(ctx, customerID, NotifOrderPlaced,
		"تم استلام طلبك",
		fmt.Sprintf("تم استلام طلبك بنجاح. رقم الطلب: %s", orderID),
		map[string]string{"order_id": orderID},
	)

	// notify each supplier via their sub-orders
	var subOrders []struct {
		ID         string `db:"id"`
		SupplierID string `db:"supplier_id"`
		SubNumber  string `db:"sub_order_number"`
	}
	_ = s.db.SelectContext(ctx, &subOrders,
		`SELECT id, supplier_id, sub_order_number FROM sub_orders WHERE order_id = @p1`, orderID,
	)

	for _, sub := range subOrders {
		supplierUserID := s.supplierUserID(ctx, sub.SupplierID)
		if supplierUserID == "" {
			continue
		}
		s.send(ctx, supplierUserID, NotifOrderPlaced,
			"طلب جديد",
			fmt.Sprintf("لديك طلب جديد بانتظار موافقتك. رقم الطلب: %s", sub.SubNumber),
			map[string]string{"sub_order_id": sub.ID, "order_id": orderID},
		)
	}

	// notify admins
	s.notifyAdmins(ctx, NotifOrderPlaced,
		"طلب جديد",
		fmt.Sprintf("تم تقديم طلب جديد رقم %s", orderID),
		map[string]string{"order_id": orderID},
	)
}

// ReadyForPickup notifies admins that a sub-order needs delivery assignment
func (s *NotificationService) ReadyForPickup(ctx context.Context, subOrderID string) {
	sub := s.loadSubOrder(ctx, subOrderID)
	if sub == nil {
		return
	}
	s.notifyAdmins(ctx, NotifReadyForPickup,
		"طلب جاهز للاستلام",
		fmt.Sprintf("الطلب %s جاهز للاستلام ويحتاج إلى تعيين شركة توصيل", sub.SubOrderNumber),
		map[string]string{"sub_order_id": subOrderID},
	)
}

// DeliveryAssigned notifies supplier, customer, and delivery company
func (s *NotificationService) DeliveryAssigned(ctx context.Context, subOrderID, deliveryCompanyID string) {
	sub := s.loadSubOrder(ctx, subOrderID)
	if sub == nil {
		return
	}

	// notify customer
	customerID := s.orderCustomerID(ctx, sub.OrderID)
	if customerID != "" {
		s.send(ctx, customerID, NotifDeliveryAssigned,
			"تم تعيين شركة التوصيل",
			fmt.Sprintf("تم تعيين شركة توصيل لطلبك رقم %s", sub.SubOrderNumber),
			map[string]string{"sub_order_id": subOrderID, "tracking_number": utils.StrVal(sub.TrackingNumber)},
		)
	}

	// notify supplier
	supplierUserID := s.supplierUserID(ctx, sub.SupplierID)
	if supplierUserID != "" {
		s.send(ctx, supplierUserID, NotifDeliveryAssigned,
			"تم تعيين شركة توصيل",
			fmt.Sprintf("تم تعيين شركة توصيل للطلب %s", sub.SubOrderNumber),
			map[string]string{"sub_order_id": subOrderID},
		)
	}

	// notify delivery company
	deliveryUserID := s.deliveryUserID(ctx, deliveryCompanyID)
	if deliveryUserID != "" {
		s.send(ctx, deliveryUserID, NotifDeliveryAssigned,
			"طلب توصيل جديد",
			fmt.Sprintf("تم تعيين طلب توصيل جديد لكم. رقم الطلب: %s", sub.SubOrderNumber),
			map[string]string{"sub_order_id": subOrderID},
		)
	}
}

// DeliveryRejected notifies admin that delivery company rejected assignment
func (s *NotificationService) DeliveryRejected(ctx context.Context, subOrderID, reason string) {
	sub := s.loadSubOrder(ctx, subOrderID)
	if sub == nil {
		return
	}
	s.notifyAdmins(ctx, NotifDeliveryRejected,
		"رفض شركة التوصيل",
		fmt.Sprintf("رفضت شركة التوصيل الطلب %s. السبب: %s", sub.SubOrderNumber, reason),
		map[string]string{"sub_order_id": subOrderID},
	)
}

// OrderDelivered notifies customer, supplier, and admin
func (s *NotificationService) OrderDelivered(ctx context.Context, subOrderID string) {
	sub := s.loadSubOrder(ctx, subOrderID)
	if sub == nil {
		return
	}

	customerID := s.orderCustomerID(ctx, sub.OrderID)
	if customerID != "" {
		s.send(ctx, customerID, NotifDelivered,
			"تم توصيل طلبك",
			fmt.Sprintf("تم توصيل طلبك رقم %s. يرجى تأكيد الاستلام.", sub.SubOrderNumber),
			map[string]string{"sub_order_id": subOrderID, "order_id": sub.OrderID},
		)
	}

	supplierUserID := s.supplierUserID(ctx, sub.SupplierID)
	if supplierUserID != "" {
		s.send(ctx, supplierUserID, NotifDelivered,
			"تم توصيل الطلب",
			fmt.Sprintf("تم توصيل الطلب %s بنجاح", sub.SubOrderNumber),
			map[string]string{"sub_order_id": subOrderID},
		)
	}

	s.notifyAdmins(ctx, NotifDelivered,
		"تم التوصيل",
		fmt.Sprintf("تم توصيل الطلب %s", sub.SubOrderNumber),
		map[string]string{"sub_order_id": subOrderID},
	)
}

// DeliveryFailed notifies admin and customer
func (s *NotificationService) DeliveryFailed(ctx context.Context, subOrderID, reason string) {
	sub := s.loadSubOrder(ctx, subOrderID)
	if sub == nil {
		return
	}

	customerID := s.orderCustomerID(ctx, sub.OrderID)
	if customerID != "" {
		s.send(ctx, customerID, NotifDeliveryFailed,
			"تعذّر التوصيل",
			fmt.Sprintf("تعذّر توصيل طلبك رقم %s. سيتم التواصل معك قريباً.", sub.SubOrderNumber),
			map[string]string{"sub_order_id": subOrderID},
		)
	}

	s.notifyAdmins(ctx, NotifDeliveryFailed,
		"فشل التوصيل",
		fmt.Sprintf("فشل توصيل الطلب %s. السبب: %s", sub.SubOrderNumber, reason),
		map[string]string{"sub_order_id": subOrderID},
	)
}

// ── Supplier Notifications ────────────────────────────────────────────────────

// SupplierApproved notifies the supplier their account was approved
func (s *NotificationService) SupplierApproved(ctx context.Context, supplierID string) {
	userID := s.supplierUserID(ctx, supplierID)
	if userID == "" {
		return
	}
	s.send(ctx, userID, NotifSupplierApproved,
		"تم قبول حسابك",
		"تهانينا! تم قبول حسابك في سوقك. يمكنك الآن إضافة منتجاتك.",
		map[string]string{"supplier_id": supplierID},
	)
}

// SupplierRejected notifies the supplier their account was rejected
func (s *NotificationService) SupplierRejected(ctx context.Context, supplierID, reason string) {
	userID := s.supplierUserID(ctx, supplierID)
	if userID == "" {
		return
	}
	s.send(ctx, userID, NotifSupplierRejected,
		"تم رفض طلب التسجيل",
		fmt.Sprintf("نأسف، تم رفض طلب تسجيلك. السبب: %s", reason),
		map[string]string{"supplier_id": supplierID},
	)
}

// ProductApproved notifies supplier their product is now live
func (s *NotificationService) ProductApproved(ctx context.Context, productID, supplierID string) {
	userID := s.supplierUserID(ctx, supplierID)
	if userID == "" {
		return
	}
	var productName string
	_ = s.db.QueryRowContext(ctx,
		`SELECT name_ar FROM products WHERE id = @p1`, productID,
	).Scan(&productName)

	s.send(ctx, userID, NotifProductApproved,
		"تم قبول المنتج",
		fmt.Sprintf("تم قبول منتجك \"%s\" وهو الآن متاح للبيع.", productName),
		map[string]string{"product_id": productID},
	)
}

// ProductRejected notifies supplier their product was rejected
func (s *NotificationService) ProductRejected(ctx context.Context, productID, supplierID, reason string) {
	userID := s.supplierUserID(ctx, supplierID)
	if userID == "" {
		return
	}
	s.send(ctx, userID, NotifProductRejected,
		"تم رفض المنتج",
		fmt.Sprintf("تم رفض منتجك. السبب: %s", reason),
		map[string]string{"product_id": productID},
	)
}

// LowStockAlert notifies supplier of a product running low
func (s *NotificationService) LowStockAlert(ctx context.Context, userID, productID, productName string, remaining int) {
	s.send(ctx, userID, NotifLowStock,
		"تحذير: مخزون منخفض",
		fmt.Sprintf("المنتج \"%s\" على وشك النفاد. الكمية المتبقية: %d", productName, remaining),
		map[string]string{"product_id": productID, "remaining": fmt.Sprintf("%d", remaining)},
	)
}

// WithdrawalProcessed notifies supplier their withdrawal was paid
func (s *NotificationService) WithdrawalProcessed(ctx context.Context, supplierID, requestID string, amount float64) {
	userID := s.supplierUserID(ctx, supplierID)
	if userID == "" {
		return
	}
	s.send(ctx, userID, NotifWithdrawalProcessed,
		"تم صرف المستحقات",
		fmt.Sprintf("تم تحويل مبلغ %.2f دينار ليبي إلى حسابك البنكي.", amount),
		map[string]string{"withdrawal_id": requestID},
	)
}

// WithdrawalRejected notifies supplier their withdrawal was rejected
func (s *NotificationService) WithdrawalRejected(ctx context.Context, supplierID, requestID, reason string) {
	userID := s.supplierUserID(ctx, supplierID)
	if userID == "" {
		return
	}
	s.send(ctx, userID, NotifWithdrawalRejected,
		"تم رفض طلب السحب",
		fmt.Sprintf("تم رفض طلب سحب المستحقات. السبب: %s", reason),
		map[string]string{"withdrawal_id": requestID},
	)
}

// ── Complaint Notifications ───────────────────────────────────────────────────

// NewComplaint notifies admin of a new complaint
func (s *NotificationService) NewComplaint(ctx context.Context, complaintID, complaintNumber string) {
	s.notifyAdmins(ctx, NotifNewComplaint,
		"شكوى جديدة",
		fmt.Sprintf("تم تقديم شكوى جديدة رقم %s", complaintNumber),
		map[string]string{"complaint_id": complaintID},
	)
}

// ComplaintResolved notifies the customer their complaint was resolved
func (s *NotificationService) ComplaintResolved(ctx context.Context, complaintID, customerUserID, resolution string) {
	s.send(ctx, customerUserID, NotifComplaintResolved,
		"تم حل شكواك",
		fmt.Sprintf("تم معالجة شكواك. %s", resolution),
		map[string]string{"complaint_id": complaintID},
	)
}

// ── Review Notifications ──────────────────────────────────────────────────────

// NewReview notifies supplier of a new customer review
func (s *NotificationService) NewReview(ctx context.Context, supplierID, productID string, rating int) {
	userID := s.supplierUserID(ctx, supplierID)
	if userID == "" {
		return
	}
	s.send(ctx, userID, NotifNewReview,
		"تقييم جديد",
		fmt.Sprintf("حصل أحد منتجاتك على تقييم جديد: %d نجوم", rating),
		map[string]string{"product_id": productID},
	)
}

// ── Read / List ───────────────────────────────────────────────────────────────

// GetNotifications returns paginated notifications for a user
func (s *NotificationService) GetNotifications(ctx context.Context, userID string, pagination utils.PaginationParams) ([]models.Notification, int64, error) {
	var total int64
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM notifications WHERE user_id = @p1`, userID,
	).Scan(&total)

	var notifs []models.Notification
	err := s.db.SelectContext(ctx, &notifs, `
		SELECT * FROM notifications WHERE user_id = @p1
		ORDER BY created_at DESC
		OFFSET @p2 ROWS FETCH NEXT @p3 ROWS ONLY`,
		userID, pagination.Offset, pagination.PerPage,
	)
	return notifs, total, err
}

// MarkRead marks a single notification as read
func (s *NotificationService) MarkRead(ctx context.Context, notifID, userID string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE notifications
		SET is_read = 1, read_at = @p1
		WHERE id = @p2 AND user_id = @p3`,
		now, notifID, userID,
	)
	return err
}

// MarkAllRead marks all notifications as read for a user
func (s *NotificationService) MarkAllRead(ctx context.Context, userID string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE notifications
		SET is_read = 1, read_at = @p1
		WHERE user_id = @p2 AND is_read = 0`,
		now, userID,
	)
	return err
}

// UnreadCount returns the count of unread notifications for a user
func (s *NotificationService) UnreadCount(ctx context.Context, userID string) int {
	var count int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM notifications WHERE user_id = @p1 AND is_read = 0`, userID,
	).Scan(&count)
	return count
}

// ── Core sender ───────────────────────────────────────────────────────────────

// send persists a notification to the DB.
// In a production system this would also push to websocket / FCM / APNs.
func (s *NotificationService) send(ctx context.Context, userID, notifType, title, body string, data map[string]string) {
	if userID == "" {
		return
	}

	dataJSON := mapToJSON(data)
	now := time.Now()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notifications (id, user_id, type, title, body, data, is_read, created_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, 0, @p7)`,
		utils.NewUUID(), userID, notifType, title, body, dataJSON, now,
	)
	if err != nil {
		fmt.Printf("[notif] failed to send notification to %s: %v\n", userID, err)
	}
}

// notifyAdmins sends a notification to all active admin users
func (s *NotificationService) notifyAdmins(ctx context.Context, notifType, title, body string, data map[string]string) {
	var adminIDs []string
	_ = s.db.SelectContext(ctx, &adminIDs, `
		SELECT id FROM users
		WHERE role IN ('super_admin', 'ops_admin', 'finance_admin', 'support_admin')
		  AND is_active = 1`,
	)
	for _, adminID := range adminIDs {
		s.send(ctx, adminID, notifType, title, body, data)
	}
}

// ── DB lookup helpers ─────────────────────────────────────────────────────────

func (s *NotificationService) supplierUserID(ctx context.Context, supplierID string) string {
	var userID string
	_ = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM suppliers WHERE id = @p1`, supplierID,
	).Scan(&userID)
	return userID
}

func (s *NotificationService) deliveryUserID(ctx context.Context, companyID string) string {
	var userID string
	_ = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM delivery_companies WHERE id = @p1`, companyID,
	).Scan(&userID)
	return userID
}

func (s *NotificationService) orderCustomerID(ctx context.Context, orderID string) string {
	var customerID string
	_ = s.db.QueryRowContext(ctx,
		`SELECT customer_id FROM orders WHERE id = @p1`, orderID,
	).Scan(&customerID)
	// resolve customer user_id from customer record
	var userID string
	_ = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM customers WHERE id = @p1`, customerID,
	).Scan(&userID)
	return userID
}

func (s *NotificationService) loadSubOrder(ctx context.Context, subOrderID string) *models.SubOrder {
	var sub models.SubOrder
	err := s.db.GetContext(ctx, &sub,
		`SELECT * FROM sub_orders WHERE id = @p1`, subOrderID,
	)
	if err != nil {
		return nil
	}
	return &sub
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func mapToJSON(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	result := "{"
	first := true
	for k, v := range m {
		if !first {
			result += ","
		}
		result += fmt.Sprintf(`"%s":"%s"`, k, v)
		first = false
	}
	result += "}"
	return result
}