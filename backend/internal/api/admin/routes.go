package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"souqiq/internal/middleware"
	"souqiq/internal/models"
	"souqiq/internal/services"
	"souqiq/internal/utils"
)

type Handler struct {
	db         *sqlx.DB
	productSvc *services.ProductService
	orderSvc   *services.OrderService
	commSvc    *services.CommissionService
	notifSvc   *services.NotificationService
}

func NewHandler(
	db *sqlx.DB,
	productSvc *services.ProductService,
	orderSvc *services.OrderService,
	commSvc *services.CommissionService,
	notifSvc *services.NotificationService,
) *Handler {
	return &Handler{
		db:         db,
		productSvc: productSvc,
		orderSvc:   orderSvc,
		commSvc:    commSvc,
		notifSvc:   notifSvc,
	}
}

// RegisterRoutes mounts all admin routes under /api/v1/admin
func RegisterRoutes(r *gin.RouterGroup, h *Handler) {
	a := r.Group("/admin")
	a.Use(middleware.RequireRoles(
		models.RoleSuperAdmin,
		models.RoleOpsAdmin,
		models.RoleFinanceAdmin,
		"support_admin",
	))
	{
		a.GET("/dashboard", h.Dashboard)

		a.GET("/suppliers",                  h.ListSuppliers)
		a.GET("/suppliers/pending",          h.PendingSuppliers)
		a.GET("/suppliers/:id",              h.GetSupplier)
		a.PATCH("/suppliers/:id/approve",    h.ApproveSupplier)
		a.PATCH("/suppliers/:id/reject",     h.RejectSupplier)
		a.PATCH("/suppliers/:id/suspend",    h.SuspendSupplier)
		a.PATCH("/suppliers/:id/reactivate", h.ReactivateSupplier)
		a.PATCH("/suppliers/:id/commission", h.SetSupplierCommission)
		a.GET("/suppliers/:id/products",     h.SupplierProducts)
		a.GET("/suppliers/:id/orders",       h.SupplierOrders)
		a.GET("/suppliers/:id/financials",   h.SupplierFinancials)

		a.GET("/customers",                  h.ListCustomers)
		a.GET("/customers/:id",              h.GetCustomer)
		a.GET("/customers/:id/orders",       h.CustomerOrders)
		a.PATCH("/customers/:id/suspend",    h.SuspendCustomer)
		a.PATCH("/customers/:id/reactivate", h.ReactivateCustomer)

		a.GET("/products",                   h.ListProducts)
		a.GET("/products/pending",           h.PendingProducts)
		a.GET("/products/:id",               h.GetProduct)
		a.PATCH("/products/:id/approve",     h.ApproveProduct)
		a.PATCH("/products/:id/reject",      h.RejectProduct)
		a.PATCH("/products/:id/deactivate",  h.DeactivateProduct)
		a.PATCH("/products/:id/feature",     h.FeatureProduct)
		a.DELETE("/products/:id",            h.DeleteProduct)

		a.GET("/categories",        h.ListCategories)
		a.POST("/categories",       h.CreateCategory)
		a.PATCH("/categories/:id",  h.UpdateCategory)
		a.DELETE("/categories/:id", h.DeleteCategory)

		a.GET("/orders",                             h.ListOrders)
		a.GET("/orders/:id",                         h.GetOrder)
		a.GET("/sub-orders",                         h.ListSubOrders)
		a.GET("/sub-orders/ready-for-pickup",        h.ReadyForPickup)
		a.GET("/sub-orders/:id",                     h.GetSubOrder)
		a.PATCH("/sub-orders/:id/assign-delivery",   h.AssignDelivery)
		a.PATCH("/sub-orders/:id/reassign-delivery", h.ReassignDelivery)
		a.PATCH("/sub-orders/:id/cancel",            h.CancelSubOrder)
		a.PATCH("/sub-orders/:id/force-complete",    h.ForceComplete)

		a.GET("/delivery-companies",                  h.ListDeliveryCompanies)
		a.GET("/delivery-companies/pending",          h.PendingDeliveryCompanies)
		a.GET("/delivery-companies/:id",              h.GetDeliveryCompany)
		a.PATCH("/delivery-companies/:id/approve",    h.ApproveDelivery)
		a.PATCH("/delivery-companies/:id/reject",     h.RejectDelivery)
		a.PATCH("/delivery-companies/:id/suspend",    h.SuspendDelivery)
		a.PATCH("/delivery-companies/:id/reactivate", h.ReactivateDelivery)
		a.GET("/delivery-companies/:id/orders",       h.DeliveryOrders)
		a.GET("/delivery-companies/:id/performance",  h.DeliveryPerformance)

		finance := a.Group("/finance")
		finance.Use(middleware.RequireRoles(models.RoleSuperAdmin, models.RoleFinanceAdmin))
		{
			finance.GET("/overview",                    h.FinanceOverview)
			finance.GET("/transactions",                h.ListTransactions)
			finance.GET("/commissions",                 h.ListCommissions)
			finance.POST("/commissions/settle",         h.SettleCommissions)
			finance.GET("/withdrawals",                 h.ListWithdrawals)
			finance.PATCH("/withdrawals/:id/approve",   h.ApproveWithdrawal)
			finance.PATCH("/withdrawals/:id/reject",    h.RejectWithdrawal)
			finance.PATCH("/withdrawals/:id/mark-paid", h.MarkWithdrawalPaid)
			finance.GET("/supplier-wallets",            h.ListSupplierWallets)
		}

		settings := a.Group("/settings")
		settings.Use(middleware.RequireRoles(models.RoleSuperAdmin))
		{
			settings.GET("",   h.GetSettings)
			settings.PATCH("", h.UpdateSettings)
		}

		a.GET("/audit-logs", h.ListAuditLogs)

		a.GET("/complaints",               h.ListComplaints)
		a.GET("/complaints/:id",           h.GetComplaint)
		a.PATCH("/complaints/:id/resolve", h.ResolveComplaint)
		a.PATCH("/complaints/:id/close",   h.CloseComplaint)

		a.GET("/reviews",                 h.ListReviews)
		a.PATCH("/reviews/:id/unpublish", h.UnpublishReview)
		a.PATCH("/reviews/:id/publish",   h.PublishReview)

		a.GET("/cities",       h.ListCities)
		a.POST("/cities",      h.CreateCity)
		a.PATCH("/cities/:id", h.UpdateCity)
	}
}

// ── Dashboard ─────────────────────────────────────────────────────────────────

func (h *Handler) Dashboard(c *gin.Context) {
	ctx := c.Request.Context()
	type kpi struct {
		TotalOrders        int     `json:"total_orders"`
		ActiveOrders       int     `json:"active_orders"`
		CompletedOrders    int     `json:"completed_orders"`
		CancelledOrders    int     `json:"cancelled_orders"`
		TotalSuppliers     int     `json:"total_suppliers"`
		PendingSuppliers   int     `json:"pending_suppliers"`
		TotalCustomers     int     `json:"total_customers"`
		TotalDeliveryComp  int     `json:"total_delivery_companies"`
		PlatformRevenue    float64 `json:"platform_revenue"`
		PendingSettlements float64 `json:"pending_settlements"`
		PendingWithdrawals int     `json:"pending_withdrawals"`
	}
	var k kpi
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM orders`).Scan(&k.TotalOrders)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM orders WHERE status IN ('pending','processing','partially_complete')`).Scan(&k.ActiveOrders)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM orders WHERE status = 'complete'`).Scan(&k.CompletedOrders)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM orders WHERE status = 'cancelled'`).Scan(&k.CancelledOrders)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM suppliers`).Scan(&k.TotalSuppliers)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM suppliers WHERE status = 'pending'`).Scan(&k.PendingSuppliers)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM customers`).Scan(&k.TotalCustomers)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM delivery_companies WHERE status = 'approved'`).Scan(&k.TotalDeliveryComp)
	_ = h.db.QueryRowContext(ctx, `SELECT IFNULL(SUM(commission_amount),0) FROM commission_records`).Scan(&k.PlatformRevenue)
	_ = h.db.QueryRowContext(ctx, `SELECT IFNULL(SUM(balance_pending),0) FROM supplier_wallets`).Scan(&k.PendingSettlements)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM withdrawal_requests WHERE status = 'pending'`).Scan(&k.PendingWithdrawals)
	utils.RespondOK(c, k)
}

// ── Suppliers ─────────────────────────────────────────────────────────────────

func (h *Handler) ListSuppliers(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	status := c.Query("status")
	search := c.Query("search")
	where := "1=1"
	args := []any{}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if search != "" {
		where += " AND (business_name LIKE ? OR business_name_ar LIKE ?)"
		args = append(args, "%"+search+"%", "%"+search+"%")
	}
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM suppliers WHERE `+where, args...).Scan(&total)
	listArgs := append(args, pagination.PerPage, pagination.Offset)
	type SupplierWithContact struct {
		models.Supplier
		Email *string `db:"email" json:"email,omitempty"`
		Phone string  `db:"phone" json:"phone,omitempty"`
	}
	var suppliers []SupplierWithContact
	_ = h.db.SelectContext(c.Request.Context(), &suppliers,
		`SELECT s.*, u.email, u.phone FROM suppliers s
		 LEFT JOIN users u ON u.id = s.user_id
		 WHERE `+where+` ORDER BY s.created_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	utils.RespondList(c, suppliers, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) PendingSuppliers(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM suppliers WHERE status = 'pending'`).Scan(&total)
	var suppliers []models.Supplier
	_ = h.db.SelectContext(c.Request.Context(), &suppliers, `
		SELECT * FROM suppliers WHERE status = 'pending'
		ORDER BY created_at ASC LIMIT ? OFFSET ?`,
		pagination.PerPage, pagination.Offset,
	)
	utils.RespondList(c, suppliers, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) GetSupplier(c *gin.Context) {
	var s models.Supplier
	if err := h.db.GetContext(c.Request.Context(), &s,
		`SELECT * FROM suppliers WHERE id = ?`, c.Param("id"),
	); err != nil {
		utils.RespondNotFound(c, "Supplier")
		return
	}
	utils.RespondOK(c, s)
}

func (h *Handler) ApproveSupplier(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	now := time.Now()
	_, err := h.db.ExecContext(c.Request.Context(), `
		UPDATE suppliers
		SET status = 'approved', approved_by = ?, approved_at = ?,
		    rejection_reason = NULL, updated_at = ?
		WHERE id = ? AND status = 'pending'`,
		actorID, now, now, id,
	)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	h.audit(c, actorID, "supplier.approved", "supplier", id)
	go h.notifSvc.SupplierApproved(c.Request.Context(), id)
	utils.RespondOK(c, gin.H{"message": "Supplier approved"})
}

func (h *Handler) RejectSupplier(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	var body struct{ Reason string `json:"reason"` }
	_ = c.ShouldBindJSON(&body)
	if body.Reason == "" {
		utils.RespondValidationError(c, map[string]any{"reason": "Required"})
		return
	}
	_, _ = h.db.ExecContext(c.Request.Context(), `
		UPDATE suppliers SET status = 'rejected', rejection_reason = ?, updated_at = ? WHERE id = ?`,
		body.Reason, time.Now(), id,
	)
	h.audit(c, actorID, "supplier.rejected", "supplier", id)
	go h.notifSvc.SupplierRejected(c.Request.Context(), id, body.Reason)
	utils.RespondOK(c, gin.H{"message": "Supplier rejected"})
}

func (h *Handler) SuspendSupplier(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	_, _ = h.db.ExecContext(c.Request.Context(),
		`UPDATE suppliers SET status = 'suspended', updated_at = ? WHERE id = ?`, time.Now(), id,
	)
	h.audit(c, actorID, "supplier.suspended", "supplier", id)
	utils.RespondOK(c, gin.H{"message": "Supplier suspended"})
}

func (h *Handler) ReactivateSupplier(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	_, _ = h.db.ExecContext(c.Request.Context(),
		`UPDATE suppliers SET status = 'approved', updated_at = ? WHERE id = ?`, time.Now(), id,
	)
	h.audit(c, actorID, "supplier.reactivated", "supplier", id)
	utils.RespondOK(c, gin.H{"message": "Supplier reactivated"})
}

func (h *Handler) SetSupplierCommission(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	var body struct{ Rate float64 `json:"commission_rate"` }
	if err := c.ShouldBindJSON(&body); err != nil || body.Rate < 0 {
		utils.RespondValidationError(c, map[string]any{"commission_rate": "Valid rate required"})
		return
	}
	_, _ = h.db.ExecContext(c.Request.Context(),
		`UPDATE suppliers SET commission_rate_override = ?, updated_at = ? WHERE id = ?`,
		body.Rate, time.Now(), id,
	)
	h.audit(c, actorID, "supplier.commission_set", "supplier", id)
	utils.RespondOK(c, gin.H{"message": "Commission rate updated"})
}

func (h *Handler) SupplierProducts(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	filter := services.ProductListFilter{SupplierID: c.Param("id"), PaginationParams: pagination}
	products, total, _ := h.productSvc.List(c.Request.Context(), filter)
	utils.RespondList(c, products, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) SupplierOrders(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	orders, total, _ := h.orderSvc.ListSubOrders(c.Request.Context(),
		map[string]any{"supplier_id": c.Param("id")}, pagination)
	utils.RespondList(c, orders, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) SupplierFinancials(c *gin.Context) {
	wallet, err := h.commSvc.GetWallet(c.Request.Context(), c.Param("id"))
	if err != nil {
		utils.RespondNotFound(c, "Wallet")
		return
	}
	utils.RespondOK(c, wallet)
}

// ── Customers ─────────────────────────────────────────────────────────────────

func (h *Handler) ListCustomers(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	search := c.Query("search")
	where := "1=1"
	args := []any{}
	if search != "" {
		where += " AND full_name LIKE ?"
		args = append(args, "%"+search+"%")
	}
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM customers WHERE `+where, args...).Scan(&total)
	listArgs := append(args, pagination.PerPage, pagination.Offset)
	var customers []models.Customer
	_ = h.db.SelectContext(c.Request.Context(), &customers,
		`SELECT * FROM customers WHERE `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	utils.RespondList(c, customers, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) GetCustomer(c *gin.Context) {
	var cust models.Customer
	if err := h.db.GetContext(c.Request.Context(), &cust,
		`SELECT * FROM customers WHERE id = ?`, c.Param("id"),
	); err != nil {
		utils.RespondNotFound(c, "Customer")
		return
	}
	utils.RespondOK(c, cust)
}

func (h *Handler) CustomerOrders(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	orders, total, _ := h.orderSvc.ListOrders(c.Request.Context(),
		map[string]any{"customer_id": c.Param("id")}, pagination)
	utils.RespondList(c, orders, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) SuspendCustomer(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	_, _ = h.db.ExecContext(c.Request.Context(), `
		UPDATE users SET is_active = 0, updated_at = ?
		WHERE id = (SELECT user_id FROM customers WHERE id = ?)`,
		time.Now(), c.Param("id"),
	)
	h.audit(c, actorID, "customer.suspended", "customer", c.Param("id"))
	utils.RespondOK(c, gin.H{"message": "Customer suspended"})
}

func (h *Handler) ReactivateCustomer(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	_, _ = h.db.ExecContext(c.Request.Context(), `
		UPDATE users SET is_active = 1, updated_at = ?
		WHERE id = (SELECT user_id FROM customers WHERE id = ?)`,
		time.Now(), c.Param("id"),
	)
	h.audit(c, actorID, "customer.reactivated", "customer", c.Param("id"))
	utils.RespondOK(c, gin.H{"message": "Customer reactivated"})
}

// ── Products ──────────────────────────────────────────────────────────────────

func (h *Handler) ListProducts(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	filter := services.ProductListFilter{
		Status:           c.Query("status"),
		CategoryID:       c.Query("category_id"),
		Search:           c.Query("search"),
		PaginationParams: pagination,
	}
	products, total, _ := h.productSvc.List(c.Request.Context(), filter)
	utils.RespondList(c, products, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) PendingProducts(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	filter := services.ProductListFilter{
		Status:           models.ProductStatusPendingReview,
		PaginationParams: pagination,
	}
	products, total, _ := h.productSvc.List(c.Request.Context(), filter)
	utils.RespondList(c, products, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) GetProduct(c *gin.Context) {
	product, err := h.productSvc.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		utils.RespondNotFound(c, "Product")
		return
	}
	utils.RespondOK(c, product)
}

func (h *Handler) ApproveProduct(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	var supplierID string
	_ = h.db.QueryRowContext(c.Request.Context(),
		`SELECT supplier_id FROM products WHERE id = ?`, id,
	).Scan(&supplierID)
	if err := h.productSvc.Approve(c.Request.Context(), id); err != nil {
		utils.RespondInternalError(c)
		return
	}
	h.audit(c, actorID, "product.approved", "product", id)
	go h.notifSvc.ProductApproved(c.Request.Context(), id, supplierID)
	utils.RespondOK(c, gin.H{"message": "Product approved"})
}

func (h *Handler) RejectProduct(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	var body struct{ Reason string `json:"reason"` }
	_ = c.ShouldBindJSON(&body)
	if body.Reason == "" {
		utils.RespondValidationError(c, map[string]any{"reason": "Required"})
		return
	}
	var supplierID string
	_ = h.db.QueryRowContext(c.Request.Context(),
		`SELECT supplier_id FROM products WHERE id = ?`, id,
	).Scan(&supplierID)
	if err := h.productSvc.Reject(c.Request.Context(), id, body.Reason); err != nil {
		utils.RespondInternalError(c)
		return
	}
	h.audit(c, actorID, "product.rejected", "product", id)
	go h.notifSvc.ProductRejected(c.Request.Context(), id, supplierID, body.Reason)
	utils.RespondOK(c, gin.H{"message": "Product rejected"})
}

func (h *Handler) DeactivateProduct(c *gin.Context) {
	if err := h.productSvc.Deactivate(c.Request.Context(), c.Param("id")); err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, gin.H{"message": "Product deactivated"})
}

func (h *Handler) FeatureProduct(c *gin.Context) {
	var body struct{ Featured bool `json:"featured"` }
	_ = c.ShouldBindJSON(&body)
	if err := h.productSvc.SetFeatured(c.Request.Context(), c.Param("id"), body.Featured); err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, gin.H{"message": "Featured status updated"})
}

func (h *Handler) DeleteProduct(c *gin.Context) {
	if err := h.productSvc.Delete(c.Request.Context(), c.Param("id"), ""); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "DELETE_FAILED", err.Error())
		return
	}
	utils.RespondNoContent(c)
}

// ── Categories ────────────────────────────────────────────────────────────────

func (h *Handler) ListCategories(c *gin.Context) {
	tree, _ := h.productSvc.GetCategoryTree(c.Request.Context())
	utils.RespondOK(c, tree)
}

func (h *Handler) CreateCategory(c *gin.Context) {
	var body struct {
		Name      string  `json:"name"`
		NameAr    string  `json:"name_ar"`
		Slug      string  `json:"slug"`
		ParentID  *string `json:"parent_id"`
		SortOrder int     `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	cat, err := h.productSvc.CreateCategory(c.Request.Context(), body.Name, body.NameAr, body.Slug, body.ParentID, body.SortOrder)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondCreated(c, cat)
}

func (h *Handler) UpdateCategory(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Name      *string `json:"name"`
		NameAr    *string `json:"name_ar"`
		IsActive  *bool   `json:"is_active"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	_, _ = h.db.ExecContext(c.Request.Context(), `
		UPDATE categories
		SET name       = COALESCE(?, name),
		    name_ar    = COALESCE(?, name_ar),
		    is_active  = COALESCE(?, is_active),
		    sort_order = COALESCE(?, sort_order),
		    updated_at = ?
		WHERE id = ?`,
		body.Name, body.NameAr, body.IsActive, body.SortOrder, time.Now(), id,
	)
	utils.RespondOK(c, gin.H{"message": "Category updated"})
}

func (h *Handler) DeleteCategory(c *gin.Context) {
	_, _ = h.db.ExecContext(c.Request.Context(),
		`DELETE FROM categories WHERE id = ?`, c.Param("id"),
	)
	utils.RespondNoContent(c)
}

// ── Orders ────────────────────────────────────────────────────────────────────

func (h *Handler) ListOrders(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	orders, total, _ := h.orderSvc.ListOrders(c.Request.Context(),
		map[string]any{"status": c.Query("status")}, pagination)
	utils.RespondList(c, orders, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) GetOrder(c *gin.Context) {
	order, err := h.orderSvc.GetOrderByID(c.Request.Context(), c.Param("id"), "", models.RoleSuperAdmin)
	if err != nil {
		utils.RespondNotFound(c, "Order")
		return
	}
	utils.RespondOK(c, order)
}

func (h *Handler) ListSubOrders(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	filter := map[string]any{"status": c.Query("status")}
	subs, total, _ := h.orderSvc.ListSubOrders(c.Request.Context(), filter, pagination)
	utils.RespondList(c, subs, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) ReadyForPickup(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	filter := map[string]any{"status": models.SubOrderStatusReadyForPickup}
	subs, total, _ := h.orderSvc.ListSubOrders(c.Request.Context(), filter, pagination)
	utils.RespondList(c, subs, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) GetSubOrder(c *gin.Context) {
	var sub models.SubOrder
	if err := h.db.GetContext(c.Request.Context(), &sub,
		`SELECT * FROM sub_orders WHERE id = ?`, c.Param("id"),
	); err != nil {
		utils.RespondNotFound(c, "Sub-order")
		return
	}
	_ = h.db.SelectContext(c.Request.Context(), &sub.Items,
		`SELECT * FROM order_items WHERE sub_order_id = ?`, sub.ID,
	)
	_ = h.db.SelectContext(c.Request.Context(), &sub.StatusLogs,
		`SELECT * FROM order_status_logs WHERE sub_order_id = ? ORDER BY changed_at`, sub.ID,
	)
	utils.RespondOK(c, sub)
}

func (h *Handler) AssignDelivery(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	var input services.AssignDeliveryInput
	if err := c.ShouldBindJSON(&input); err != nil || input.DeliveryCompanyID == "" {
		utils.RespondValidationError(c, map[string]any{"delivery_company_id": "Required"})
		return
	}
	if err := h.orderSvc.AdminAssignDelivery(c.Request.Context(), id, actorID, input); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "ASSIGN_FAILED", err.Error())
		return
	}
	h.audit(c, actorID, "suborder.delivery_assigned", "sub_order", id)
	utils.RespondOK(c, gin.H{"message": "Delivery company assigned"})
}

func (h *Handler) ReassignDelivery(c *gin.Context) { h.AssignDelivery(c) }

func (h *Handler) CancelSubOrder(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	var body struct{ Reason string `json:"reason"` }
	_ = c.ShouldBindJSON(&body)
	if err := h.orderSvc.AdminCancelSubOrder(c.Request.Context(), id, actorID, body.Reason); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "CANCEL_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Sub-order cancelled"})
}

func (h *Handler) ForceComplete(c *gin.Context) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	if err := h.orderSvc.AdminForceComplete(c.Request.Context(), id, actorID); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "COMPLETE_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Sub-order force completed"})
}

// ── Delivery Companies ────────────────────────────────────────────────────────

func (h *Handler) ListDeliveryCompanies(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	status := c.Query("status")
	where := "1=1"
	args := []any{}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM delivery_companies WHERE `+where, args...).Scan(&total)
	listArgs := append(args, pagination.PerPage, pagination.Offset)
	var companies []models.DeliveryCompany
	_ = h.db.SelectContext(c.Request.Context(), &companies,
		`SELECT * FROM delivery_companies WHERE `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	utils.RespondList(c, companies, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) PendingDeliveryCompanies(c *gin.Context) {
	var companies []models.DeliveryCompany
	_ = h.db.SelectContext(c.Request.Context(), &companies,
		`SELECT * FROM delivery_companies WHERE status = 'pending' ORDER BY created_at ASC`,
	)
	utils.RespondOK(c, companies)
}

func (h *Handler) GetDeliveryCompany(c *gin.Context) {
	var co models.DeliveryCompany
	if err := h.db.GetContext(c.Request.Context(), &co,
		`SELECT * FROM delivery_companies WHERE id = ?`, c.Param("id"),
	); err != nil {
		utils.RespondNotFound(c, "Delivery company")
		return
	}
	utils.RespondOK(c, co)
}

func (h *Handler) setDeliveryStatus(c *gin.Context, status, action string) {
	id := c.Param("id")
	actorID := middleware.MustGetUserID(c)
	_, _ = h.db.ExecContext(c.Request.Context(), `
		UPDATE delivery_companies
		SET status = ?, approved_by = ?, approved_at = ?, updated_at = ?
		WHERE id = ?`,
		status, actorID, time.Now(), time.Now(), id,
	)
	h.audit(c, actorID, "delivery."+action, "delivery_company", id)
	utils.RespondOK(c, gin.H{"message": "Delivery company " + action})
}

func (h *Handler) ApproveDelivery(c *gin.Context)    { h.setDeliveryStatus(c, "approved", "approved") }
func (h *Handler) RejectDelivery(c *gin.Context)     { h.setDeliveryStatus(c, "rejected", "rejected") }
func (h *Handler) SuspendDelivery(c *gin.Context)    { h.setDeliveryStatus(c, "suspended", "suspended") }
func (h *Handler) ReactivateDelivery(c *gin.Context) { h.setDeliveryStatus(c, "approved", "reactivated") }

func (h *Handler) DeliveryOrders(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	subs, total, _ := h.orderSvc.ListSubOrders(c.Request.Context(),
		map[string]any{"delivery_company_id": c.Param("id")}, pagination)
	utils.RespondList(c, subs, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) DeliveryPerformance(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	type perf struct {
		TotalAssigned int     `json:"total_assigned"`
		Delivered     int     `json:"delivered"`
		Failed        int     `json:"failed"`
		SuccessRate   float64 `json:"success_rate"`
	}
	var p perf
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sub_orders WHERE delivery_company_id = ?`, id).Scan(&p.TotalAssigned)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sub_orders WHERE delivery_company_id = ? AND status IN ('delivered','confirmed')`, id).Scan(&p.Delivered)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sub_orders WHERE delivery_company_id = ? AND status IN ('failed_delivery','returned')`, id).Scan(&p.Failed)
	if p.TotalAssigned > 0 {
		p.SuccessRate = utils.RoundToTwo(float64(p.Delivered) / float64(p.TotalAssigned) * 100)
	}
	utils.RespondOK(c, p)
}

// ── Finance ───────────────────────────────────────────────────────────────────

func (h *Handler) FinanceOverview(c *gin.Context) {
	ctx := c.Request.Context()
	type ov struct {
		TotalCommissions   float64 `json:"total_commissions"`
		PendingSettlements float64 `json:"pending_settlements"`
		TotalWithdrawn     float64 `json:"total_withdrawn"`
		PendingWithdrawals int     `json:"pending_withdrawal_requests"`
	}
	var o ov
	_ = h.db.QueryRowContext(ctx, `SELECT IFNULL(SUM(commission_amount),0) FROM commission_records`).Scan(&o.TotalCommissions)
	_ = h.db.QueryRowContext(ctx, `SELECT IFNULL(SUM(balance_pending),0) FROM supplier_wallets`).Scan(&o.PendingSettlements)
	_ = h.db.QueryRowContext(ctx, `SELECT IFNULL(SUM(balance_withdrawn),0) FROM supplier_wallets`).Scan(&o.TotalWithdrawn)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM withdrawal_requests WHERE status = 'pending'`).Scan(&o.PendingWithdrawals)
	utils.RespondOK(c, o)
}

func (h *Handler) ListTransactions(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM financial_transactions`).Scan(&total)
	var txns []models.FinancialTransaction
	_ = h.db.SelectContext(c.Request.Context(), &txns, `
		SELECT * FROM financial_transactions ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pagination.PerPage, pagination.Offset,
	)
	utils.RespondList(c, txns, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) ListCommissions(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	where := "1=1"
	if s := c.Query("settled"); s == "true" {
		where += " AND settled = 1"
	} else if s == "false" {
		where += " AND settled = 0"
	}
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM commission_records WHERE `+where).Scan(&total)
	var records []models.CommissionRecord
	_ = h.db.SelectContext(c.Request.Context(), &records, `
		SELECT * FROM commission_records WHERE `+where+`
		ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pagination.PerPage, pagination.Offset,
	)
	utils.RespondList(c, records, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) SettleCommissions(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	var body struct{ SupplierID string `json:"supplier_id"` }
	if err := c.ShouldBindJSON(&body); err != nil || body.SupplierID == "" {
		utils.RespondValidationError(c, map[string]any{"supplier_id": "Required"})
		return
	}
	amount, err := h.commSvc.SettleSupplier(c.Request.Context(), body.SupplierID, actorID)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, gin.H{"message": "Settlement processed", "amount": amount})
}

func (h *Handler) ListWithdrawals(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	status := c.Query("status")
	where := "1=1"
	args := []any{}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM withdrawal_requests WHERE `+where, args...).Scan(&total)
	listArgs := append(args, pagination.PerPage, pagination.Offset)
	var withdrawals []models.WithdrawalRequest
	_ = h.db.SelectContext(c.Request.Context(), &withdrawals, `
		SELECT * FROM withdrawal_requests WHERE `+where+`
		ORDER BY requested_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	utils.RespondList(c, withdrawals, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) ApproveWithdrawal(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	if err := h.commSvc.ApproveWithdrawal(c.Request.Context(), c.Param("id"), actorID); err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, gin.H{"message": "Withdrawal approved"})
}

func (h *Handler) RejectWithdrawal(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	var body struct{ Reason string `json:"reason"` }
	_ = c.ShouldBindJSON(&body)
	if err := h.commSvc.RejectWithdrawal(c.Request.Context(), c.Param("id"), actorID, body.Reason); err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, gin.H{"message": "Withdrawal rejected"})
}

func (h *Handler) MarkWithdrawalPaid(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	var body struct{ ProofURL string `json:"proof_url"` }
	_ = c.ShouldBindJSON(&body)
	if err := h.commSvc.MarkWithdrawalPaid(c.Request.Context(), c.Param("id"), actorID, body.ProofURL); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "PAYOUT_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Withdrawal marked as paid"})
}

func (h *Handler) ListSupplierWallets(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM supplier_wallets`).Scan(&total)
	var wallets []models.SupplierWallet
	_ = h.db.SelectContext(c.Request.Context(), &wallets, `
		SELECT * FROM supplier_wallets ORDER BY balance_pending DESC LIMIT ? OFFSET ?`,
		pagination.PerPage, pagination.Offset,
	)
	utils.RespondList(c, wallets, total, pagination.Page, pagination.PerPage)
}

// ── Settings ──────────────────────────────────────────────────────────────────

func (h *Handler) GetSettings(c *gin.Context) {
	var settings []models.PlatformSetting
	_ = h.db.SelectContext(c.Request.Context(), &settings, "SELECT * FROM platform_settings ORDER BY `key`")
	utils.RespondOK(c, settings)
}

func (h *Handler) UpdateSettings(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	var body map[string]string
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	now := time.Now()
	for key, val := range body {
		_, _ = h.db.ExecContext(c.Request.Context(),
			"UPDATE platform_settings SET value = ?, updated_by = ?, updated_at = ? WHERE `key` = ?",
			val, actorID, now, key,
		)
	}
	utils.RespondOK(c, gin.H{"message": "Settings updated"})
}

// ── Audit Logs ────────────────────────────────────────────────────────────────

func (h *Handler) ListAuditLogs(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM audit_logs`).Scan(&total)
	var logs []models.AuditLog
	_ = h.db.SelectContext(c.Request.Context(), &logs, `
		SELECT * FROM audit_logs ORDER BY performed_at DESC LIMIT ? OFFSET ?`,
		pagination.PerPage, pagination.Offset,
	)
	utils.RespondList(c, logs, total, pagination.Page, pagination.PerPage)
}

// ── Complaints ────────────────────────────────────────────────────────────────

func (h *Handler) ListComplaints(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	status := c.Query("status")
	where := "1=1"
	args := []any{}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM complaints WHERE `+where, args...).Scan(&total)
	listArgs := append(args, pagination.PerPage, pagination.Offset)
	var complaints []models.Complaint
	_ = h.db.SelectContext(c.Request.Context(), &complaints, `
		SELECT * FROM complaints WHERE `+where+`
		ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	utils.RespondList(c, complaints, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) GetComplaint(c *gin.Context) {
	var complaint models.Complaint
	if err := h.db.GetContext(c.Request.Context(), &complaint,
		`SELECT * FROM complaints WHERE id = ?`, c.Param("id"),
	); err != nil {
		utils.RespondNotFound(c, "Complaint")
		return
	}
	_ = h.db.SelectContext(c.Request.Context(), &complaint.Attachments,
		`SELECT * FROM complaint_attachments WHERE complaint_id = ?`, complaint.ID,
	)
	utils.RespondOK(c, complaint)
}

func (h *Handler) ResolveComplaint(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	var body struct{ Resolution string `json:"resolution"` }
	_ = c.ShouldBindJSON(&body)
	now := time.Now()
	_, _ = h.db.ExecContext(c.Request.Context(), `
		UPDATE complaints
		SET status = 'resolved', resolution = ?, resolved_by = ?, resolved_at = ?, updated_at = ?
		WHERE id = ?`,
		body.Resolution, actorID, now, now, c.Param("id"),
	)
	utils.RespondOK(c, gin.H{"message": "Complaint resolved"})
}

func (h *Handler) CloseComplaint(c *gin.Context) {
	_, _ = h.db.ExecContext(c.Request.Context(),
		`UPDATE complaints SET status = 'closed', updated_at = ? WHERE id = ?`,
		time.Now(), c.Param("id"),
	)
	utils.RespondOK(c, gin.H{"message": "Complaint closed"})
}

// ── Reviews ───────────────────────────────────────────────────────────────────

func (h *Handler) ListReviews(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(1) FROM reviews`).Scan(&total)
	var reviews []models.Review
	_ = h.db.SelectContext(c.Request.Context(), &reviews, `
		SELECT * FROM reviews ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pagination.PerPage, pagination.Offset,
	)
	utils.RespondList(c, reviews, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) UnpublishReview(c *gin.Context) {
	_, _ = h.db.ExecContext(c.Request.Context(),
		`UPDATE reviews SET is_published = 0, updated_at = ? WHERE id = ?`, time.Now(), c.Param("id"),
	)
	utils.RespondOK(c, gin.H{"message": "Review unpublished"})
}

func (h *Handler) PublishReview(c *gin.Context) {
	_, _ = h.db.ExecContext(c.Request.Context(),
		`UPDATE reviews SET is_published = 1, updated_at = ? WHERE id = ?`, time.Now(), c.Param("id"),
	)
	utils.RespondOK(c, gin.H{"message": "Review published"})
}

// ── Cities ────────────────────────────────────────────────────────────────────

func (h *Handler) ListCities(c *gin.Context) {
	var cities []models.City
	_ = h.db.SelectContext(c.Request.Context(), &cities,
		`SELECT * FROM cities WHERE is_active = 1 ORDER BY name_ar`,
	)
	utils.RespondOK(c, cities)
}

func (h *Handler) CreateCity(c *gin.Context) {
	var body struct {
		Name        string `json:"name"`
		NameAr      string `json:"name_ar"`
		CountryCode string `json:"country_code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	now := time.Now()
	id := utils.NewUUID()
	_, _ = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO cities (id, name, name_ar, country_code, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)`,
		id, body.Name, body.NameAr, body.CountryCode, now, now,
	)
	utils.RespondCreated(c, gin.H{"id": id, "message": "City created"})
}

func (h *Handler) UpdateCity(c *gin.Context) {
	var body struct {
		Name     *string `json:"name"`
		NameAr   *string `json:"name_ar"`
		IsActive *bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	_, _ = h.db.ExecContext(c.Request.Context(), `
		UPDATE cities
		SET name       = COALESCE(?, name),
		    name_ar    = COALESCE(?, name_ar),
		    is_active  = COALESCE(?, is_active),
		    updated_at = ?
		WHERE id = ?`,
		body.Name, body.NameAr, body.IsActive, time.Now(), c.Param("id"),
	)
	utils.RespondOK(c, gin.H{"message": "City updated"})
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (h *Handler) audit(c *gin.Context, actorID, action, targetType, targetID string) {
	role, _ := middleware.GetRole(c)
	ip := utils.GetClientIP(c)
	_, _ = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO audit_logs (id, actor_id, actor_role, action, target_type, target_id, ip_address, performed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		utils.NewUUID(), actorID, role, action, targetType, targetID, ip, time.Now(),
	)
}