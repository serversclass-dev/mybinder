package supplier

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
	invSvc     *services.InventoryService
	commSvc    *services.CommissionService
	notifSvc   *services.NotificationService
}

func NewHandler(
	db *sqlx.DB,
	productSvc *services.ProductService,
	orderSvc *services.OrderService,
	invSvc *services.InventoryService,
	commSvc *services.CommissionService,
	notifSvc *services.NotificationService,
) *Handler {
	return &Handler{
		db:         db,
		productSvc: productSvc,
		orderSvc:   orderSvc,
		invSvc:     invSvc,
		commSvc:    commSvc,
		notifSvc:   notifSvc,
	}
}

// RegisterRoutesWithMW mounts all supplier routes under /api/v1/supplier
func RegisterRoutesWithMW(r *gin.RouterGroup, h *Handler) {
	s := r.Group("/supplier")
	s.Use(middleware.RequireRoles(models.RoleSupplier))
	{
		s.GET("/profile",  h.GetProfile)
		s.PATCH("/profile", h.UpdateProfile)

		s.GET("/documents",  h.ListDocuments)
		s.POST("/documents", h.UploadDocument)

		s.GET("/bank-accounts",        h.ListBankAccounts)
		s.POST("/bank-accounts",       h.AddBankAccount)
		s.PATCH("/bank-accounts/:id",  h.UpdateBankAccount)
		s.DELETE("/bank-accounts/:id", h.DeleteBankAccount)

		s.GET("/products",                          h.ListProducts)
		s.POST("/products",                         h.CreateProduct)
		s.GET("/products/:id",                      h.GetProduct)
		s.PATCH("/products/:id",                    h.UpdateProduct)
		s.DELETE("/products/:id",                   h.DeleteProduct)
		s.POST("/products/:id/submit",              h.SubmitProduct)
		s.POST("/products/:id/images",              h.AddProductImage)
		s.DELETE("/products/:id/images/:imgID",     h.RemoveProductImage)
		s.POST("/products/:id/discounts",           h.AddDiscount)
		s.DELETE("/products/:id/discounts/:discID", h.RemoveDiscount)

		s.GET("/inventory",                 h.ListInventory)
		s.GET("/inventory/low-stock",       h.GetLowStock)
		s.PATCH("/inventory/:productID",    h.UpdateStock)
		s.GET("/inventory/:productID/logs", h.GetInventoryLogs)

		s.GET("/orders",                      h.ListOrders)
		s.GET("/orders/:id",                  h.GetOrder)
		s.PATCH("/orders/:id/accept",         h.AcceptOrder)
		s.PATCH("/orders/:id/reject",         h.RejectOrder)
		s.PATCH("/orders/:id/mark-preparing", h.MarkPreparing)
		s.PATCH("/orders/:id/mark-ready",     h.MarkReady)

		s.GET("/finance/overview",     h.FinanceOverview)
		s.GET("/finance/transactions", h.ListTransactions)
		s.GET("/finance/withdrawals",  h.ListWithdrawals)
		s.POST("/finance/withdrawals", h.RequestWithdrawal)

		s.GET("/reviews",            h.ListReviews)
		s.POST("/reviews/:id/reply", h.ReplyToReview)
	}
}

// ── Profile ───────────────────────────────────────────────────────────────────

func (h *Handler) GetProfile(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	if userID == "" {
		return
	}
	var supplier models.Supplier
	if err := h.db.GetContext(c.Request.Context(), &supplier,
		`SELECT * FROM suppliers WHERE user_id = ?`, userID,
	); err != nil {
		utils.RespondNotFound(c, "Supplier profile")
		return
	}
	utils.RespondOK(c, supplier)
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	if userID == "" {
		return
	}
	var body struct {
		BusinessName   *string `json:"business_name"`
		BusinessNameAr *string `json:"business_name_ar"`
		Description    *string `json:"description"`
		Address        *string `json:"address"`
		CityID         *string `json:"city_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	_, err := h.db.ExecContext(c.Request.Context(), `
		UPDATE suppliers
		SET business_name    = COALESCE(?, business_name),
		    business_name_ar = COALESCE(?, business_name_ar),
		    description      = COALESCE(?, description),
		    address          = COALESCE(?, address),
		    city_id          = COALESCE(?, city_id),
		    updated_at       = ?
		WHERE user_id = ?`,
		body.BusinessName, body.BusinessNameAr,
		body.Description, body.Address, body.CityID,
		time.Now(), userID,
	)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, gin.H{"message": "Profile updated"})
}

// ── Documents ─────────────────────────────────────────────────────────────────

func (h *Handler) ListDocuments(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var docs []models.SupplierDocument
	_ = h.db.SelectContext(c.Request.Context(), &docs,
		`SELECT * FROM supplier_documents WHERE supplier_id = ? ORDER BY created_at DESC`, supplierID,
	)
	utils.RespondOK(c, docs)
}

func (h *Handler) UploadDocument(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	if supplierID == "" {
		utils.RespondNotFound(c, "Supplier")
		return
	}
	var body struct {
		DocumentType string `json:"document_type"`
		FileURL      string `json:"file_url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.FileURL == "" {
		utils.RespondValidationError(c, map[string]any{"file_url": "File URL is required"})
		return
	}
	now := time.Now()
	_, err := h.db.ExecContext(c.Request.Context(), `
		INSERT INTO supplier_documents (id, supplier_id, document_type, file_url, verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		utils.NewUUID(), supplierID, body.DocumentType, body.FileURL, now, now,
	)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondCreated(c, gin.H{"message": "Document uploaded"})
}

// ── Bank Accounts ─────────────────────────────────────────────────────────────

func (h *Handler) ListBankAccounts(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var accounts []models.SupplierBankAccount
	_ = h.db.SelectContext(c.Request.Context(), &accounts,
		`SELECT * FROM supplier_bank_accounts WHERE supplier_id = ? ORDER BY is_primary DESC`, supplierID,
	)
	utils.RespondOK(c, accounts)
}

func (h *Handler) AddBankAccount(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var body struct {
		BankName      string  `json:"bank_name"`
		AccountHolder string  `json:"account_holder"`
		AccountNumber string  `json:"account_number"`
		IBAN          *string `json:"iban"`
		IsPrimary     bool    `json:"is_primary"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	ve := utils.ValidationErrors{}
	if body.BankName == "" {
		ve.Add("bank_name", "Bank name is required")
	}
	if body.AccountHolder == "" {
		ve.Add("account_holder", "Account holder is required")
	}
	if body.AccountNumber == "" {
		ve.Add("account_number", "Account number is required")
	}
	if ve.HasErrors() {
		utils.RespondValidationError(c, ve.ToDetails())
		return
	}
	if body.IsPrimary {
		_, _ = h.db.ExecContext(c.Request.Context(),
			`UPDATE supplier_bank_accounts SET is_primary = 0 WHERE supplier_id = ?`, supplierID,
		)
	}
	now := time.Now()
	id := utils.NewUUID()
	_, err := h.db.ExecContext(c.Request.Context(), `
		INSERT INTO supplier_bank_accounts (id, supplier_id, bank_name, account_holder, account_number, iban, is_primary, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, supplierID, body.BankName, body.AccountHolder,
		body.AccountNumber, body.IBAN, body.IsPrimary, now, now,
	)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondCreated(c, gin.H{"id": id, "message": "Bank account added"})
}

func (h *Handler) UpdateBankAccount(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	acctID := c.Param("id")
	var body struct {
		BankName      *string `json:"bank_name"`
		AccountHolder *string `json:"account_holder"`
		AccountNumber *string `json:"account_number"`
		IBAN          *string `json:"iban"`
		IsPrimary     *bool   `json:"is_primary"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	if body.IsPrimary != nil && *body.IsPrimary {
		_, _ = h.db.ExecContext(c.Request.Context(),
			`UPDATE supplier_bank_accounts SET is_primary = 0 WHERE supplier_id = ?`, supplierID,
		)
	}
	_, err := h.db.ExecContext(c.Request.Context(), `
		UPDATE supplier_bank_accounts
		SET bank_name      = COALESCE(?, bank_name),
		    account_holder = COALESCE(?, account_holder),
		    account_number = COALESCE(?, account_number),
		    iban           = COALESCE(?, iban),
		    is_primary     = COALESCE(?, is_primary),
		    updated_at     = ?
		WHERE id = ? AND supplier_id = ?`,
		body.BankName, body.AccountHolder, body.AccountNumber,
		body.IBAN, body.IsPrimary, time.Now(), acctID, supplierID,
	)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, gin.H{"message": "Bank account updated"})
}

func (h *Handler) DeleteBankAccount(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	_, _ = h.db.ExecContext(c.Request.Context(),
		`DELETE FROM supplier_bank_accounts WHERE id = ? AND supplier_id = ?`,
		c.Param("id"), supplierID,
	)
	utils.RespondNoContent(c)
}

// ── Products ──────────────────────────────────────────────────────────────────

func (h *Handler) ListProducts(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	pagination := utils.ParsePagination(c)
	filter := services.ProductListFilter{
		SupplierID:       supplierID,
		Status:           c.Query("status"),
		PaginationParams: pagination,
	}
	products, total, err := h.productSvc.List(c.Request.Context(), filter)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondList(c, products, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) CreateProduct(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var input services.CreateProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	input.SupplierID = supplierID
	product, err := h.productSvc.Create(c.Request.Context(), input)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, "CREATE_FAILED", err.Error())
		return
	}
	utils.RespondCreated(c, product)
}

func (h *Handler) GetProduct(c *gin.Context) {
	product, err := h.productSvc.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		utils.RespondNotFound(c, "Product")
		return
	}
	utils.RespondOK(c, product)
}

func (h *Handler) UpdateProduct(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var input services.UpdateProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	product, err := h.productSvc.Update(c.Request.Context(), c.Param("id"), supplierID, input)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, "UPDATE_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, product)
}

func (h *Handler) DeleteProduct(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	if err := h.productSvc.Delete(c.Request.Context(), c.Param("id"), supplierID); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "DELETE_FAILED", err.Error())
		return
	}
	utils.RespondNoContent(c)
}

func (h *Handler) SubmitProduct(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	if err := h.productSvc.SubmitForReview(c.Request.Context(), c.Param("id"), supplierID); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "SUBMIT_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Product submitted for review"})
}

func (h *Handler) AddProductImage(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var body struct {
		URL       string `json:"url"`
		IsPrimary bool   `json:"is_primary"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.URL == "" {
		utils.RespondValidationError(c, map[string]any{"url": "Image URL is required"})
		return
	}
	img, err := h.productSvc.AddImage(c.Request.Context(), c.Param("id"), supplierID, body.URL, body.IsPrimary)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, "IMAGE_FAILED", err.Error())
		return
	}
	utils.RespondCreated(c, img)
}

func (h *Handler) RemoveProductImage(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	if err := h.productSvc.RemoveImage(c.Request.Context(), c.Param("id"), supplierID, c.Param("imgID")); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "IMAGE_FAILED", err.Error())
		return
	}
	utils.RespondNoContent(c)
}

func (h *Handler) AddDiscount(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var input services.AddDiscountInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	if err := h.productSvc.AddDiscount(c.Request.Context(), c.Param("id"), supplierID, userID, input); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "DISCOUNT_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Discount applied"})
}

func (h *Handler) RemoveDiscount(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	if err := h.productSvc.RemoveDiscount(c.Request.Context(), c.Param("id"), supplierID, c.Param("discID")); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "DISCOUNT_FAILED", err.Error())
		return
	}
	utils.RespondNoContent(c)
}

// ── Inventory ─────────────────────────────────────────────────────────────────

func (h *Handler) ListInventory(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	pagination := utils.ParsePagination(c)
	items, total, err := h.invSvc.GetInventoryOverview(c.Request.Context(), supplierID, pagination)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondList(c, items, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) GetLowStock(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	items, err := h.invSvc.GetLowStockItems(c.Request.Context(), supplierID)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, items)
}

func (h *Handler) UpdateStock(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var body struct {
		Quantity int    `json:"quantity"`
		Note     string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Quantity < 0 {
		utils.RespondValidationError(c, map[string]any{"quantity": "Valid non-negative quantity required"})
		return
	}
	if err := h.invSvc.UpdateStock(c.Request.Context(), c.Param("productID"), supplierID, userID, body.Quantity, body.Note); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "STOCK_UPDATE_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Stock updated"})
}

func (h *Handler) GetInventoryLogs(c *gin.Context) {
	pagination := utils.ParsePagination(c)
	logs, total, err := h.invSvc.GetInventoryLogs(c.Request.Context(), c.Param("productID"), pagination)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondList(c, logs, total, pagination.Page, pagination.PerPage)
}

// ── Orders ────────────────────────────────────────────────────────────────────

func (h *Handler) ListOrders(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	pagination := utils.ParsePagination(c)
	filter := map[string]any{
		"supplier_id": supplierID,
		"status":      c.Query("status"),
	}
	orders, total, err := h.orderSvc.ListSubOrders(c.Request.Context(), filter, pagination)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondList(c, orders, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) GetOrder(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var sub models.SubOrder
	if err := h.db.GetContext(c.Request.Context(), &sub,
		`SELECT * FROM sub_orders WHERE id = ? AND supplier_id = ?`, c.Param("id"), supplierID,
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

func (h *Handler) AcceptOrder(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	if err := h.orderSvc.SupplierAccept(c.Request.Context(), c.Param("id"), supplierID, userID); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Order accepted"})
}

func (h *Handler) RejectOrder(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var body struct{ Reason string `json:"reason"` }
	_ = c.ShouldBindJSON(&body)
	if body.Reason == "" {
		utils.RespondValidationError(c, map[string]any{"reason": "Rejection reason is required"})
		return
	}
	if err := h.orderSvc.SupplierReject(c.Request.Context(), c.Param("id"), supplierID, userID, body.Reason); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Order rejected"})
}

func (h *Handler) MarkPreparing(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	if err := h.orderSvc.SupplierMarkPreparing(c.Request.Context(), c.Param("id"), supplierID, userID); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Order marked as preparing"})
}

func (h *Handler) MarkReady(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	if err := h.orderSvc.SupplierMarkReady(c.Request.Context(), c.Param("id"), supplierID, userID); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
		return
	}
	utils.RespondOK(c, gin.H{"message": "Order marked as ready for pickup"})
}

// ── Finance ───────────────────────────────────────────────────────────────────

func (h *Handler) FinanceOverview(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	wallet, err := h.commSvc.GetWallet(c.Request.Context(), supplierID)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, gin.H{
		"wallet":       wallet,
		"total_earned": wallet.TotalEarned(),
	})
}

func (h *Handler) ListTransactions(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	pagination := utils.ParsePagination(c)
	var settled *bool
	if s := c.Query("settled"); s == "true" {
		v := true
		settled = &v
	} else if s == "false" {
		v := false
		settled = &v
	}
	records, total, err := h.commSvc.GetCommissionRecords(c.Request.Context(), supplierID, settled, pagination)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondList(c, records, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) ListWithdrawals(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	pagination := utils.ParsePagination(c)
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(),
		`SELECT COUNT(1) FROM withdrawal_requests WHERE supplier_id = ?`, supplierID,
	).Scan(&total)
	var withdrawals []models.WithdrawalRequest
	_ = h.db.SelectContext(c.Request.Context(), &withdrawals, `
		SELECT * FROM withdrawal_requests WHERE supplier_id = ?
		ORDER BY requested_at DESC LIMIT ? OFFSET ?`,
		supplierID, pagination.PerPage, pagination.Offset,
	)
	utils.RespondList(c, withdrawals, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) RequestWithdrawal(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	var body struct {
		Amount        float64 `json:"amount"`
		BankAccountID string  `json:"bank_account_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}
	ve := utils.ValidationErrors{}
	if body.Amount <= 0 {
		ve.Add("amount", "Amount must be greater than 0")
	}
	if body.BankAccountID == "" {
		ve.Add("bank_account_id", "Bank account ID is required")
	}
	if ve.HasErrors() {
		utils.RespondValidationError(c, ve.ToDetails())
		return
	}
	wr, err := h.commSvc.RequestWithdrawal(c.Request.Context(), supplierID, body.BankAccountID, body.Amount)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, "WITHDRAWAL_FAILED", err.Error())
		return
	}
	utils.RespondCreated(c, wr)
}

// ── Reviews ───────────────────────────────────────────────────────────────────

func (h *Handler) ListReviews(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	supplierID := h.resolveSupplierID(c, userID)
	pagination := utils.ParsePagination(c)
	var total int64
	_ = h.db.QueryRowContext(c.Request.Context(), `
		SELECT COUNT(1) FROM reviews r
		JOIN products p ON p.id = r.product_id
		WHERE p.supplier_id = ?`, supplierID,
	).Scan(&total)
	var reviews []models.Review
	_ = h.db.SelectContext(c.Request.Context(), &reviews, `
		SELECT r.* FROM reviews r
		JOIN products p ON p.id = r.product_id
		WHERE p.supplier_id = ?
		ORDER BY r.created_at DESC LIMIT ? OFFSET ?`,
		supplierID, pagination.PerPage, pagination.Offset,
	)
	utils.RespondList(c, reviews, total, pagination.Page, pagination.PerPage)
}

func (h *Handler) ReplyToReview(c *gin.Context) {
	var body struct{ Reply string `json:"reply"` }
	if err := c.ShouldBindJSON(&body); err != nil || body.Reply == "" {
		utils.RespondValidationError(c, map[string]any{"reply": "Reply text is required"})
		return
	}
	now := time.Now()
	_, err := h.db.ExecContext(c.Request.Context(), `
		UPDATE reviews
		SET supplier_reply = ?, supplier_replied_at = ?, updated_at = ?
		WHERE id = ?`,
		body.Reply, now, now, c.Param("id"),
	)
	if err != nil {
		utils.RespondInternalError(c)
		return
	}
	utils.RespondOK(c, gin.H{"message": "Reply posted"})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) resolveSupplierID(c *gin.Context, userID string) string {
	if val, exists := c.Get("supplier_id"); exists {
		if id, ok := val.(string); ok && id != "" {
			return id
		}
	}
	var id string
	_ = h.db.QueryRowContext(c.Request.Context(),
		`SELECT id FROM suppliers WHERE user_id = ?`, userID,
	).Scan(&id)
	if id != "" {
		c.Set("supplier_id", id)
	}
	return id
}