package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	adminapi    "souqiq/internal/api/admin"
	authapi     "souqiq/internal/api/auth"
	supplierapi "souqiq/internal/api/supplier"
	"souqiq/config"
	"souqiq/internal/cache"
	"souqiq/internal/db"
	"souqiq/internal/middleware"
	"souqiq/internal/models"
	"souqiq/internal/services"
	"souqiq/internal/utils"
)

func main() {
	// ── Config ────────────────────────────────────────────────────────────────
	cfg := config.Load()

	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// ── Database ──────────────────────────────────────────────────────────────
	database := db.Connect(cfg)
	defer db.Close()

	// ── Redis Cache ───────────────────────────────────────────────────────────
	cache.Connect(cfg)
	defer cache.Close()

	// ── Services ──────────────────────────────────────────────────────────────
	notifSvc   := services.NewNotificationService(database)
	invSvc     := services.NewInventoryService(database)
	commSvc    := services.NewCommissionService(database)
	productSvc := services.NewProductService(database)
	orderSvc   := services.NewOrderService(database, commSvc, invSvc, notifSvc)
	authSvc    := services.NewAuthService(database, cfg)

	// ── Router ────────────────────────────────────────────────────────────────
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept-Language"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	// inject db into gin context for handlers that need it directly
	r.Use(func(c *gin.Context) {
		c.Set("db", database)
		c.Next()
	})

	// supplier ID middleware — cache supplier_id per request for supplier routes
	supplierIDMiddleware := func(c *gin.Context) {
		userID, exists := c.Get(string(middleware.CtxUserID))
		if !exists {
			c.Next()
			return
		}
		role, _ := c.Get(string(middleware.CtxRole))
		if role != models.RoleSupplier {
			c.Next()
			return
		}
		var supplierID string
		_ = database.QueryRowContext(c.Request.Context(),
			`SELECT id FROM suppliers WHERE user_id = ?`, userID,
		).Scan(&supplierID)
		if supplierID != "" {
			c.Set("supplier_id", supplierID)
		}
		c.Next()
	}

	// ── Health check ──────────────────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		dbErr := db.Ping()
		status := "ok"
		code := http.StatusOK
		if dbErr != nil {
			status = "degraded"
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, gin.H{
			"status":    status,
			"service":   cfg.App.Name,
			"timestamp": time.Now().UTC(),
		})
	})

	// ── API v1 ────────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")

	// ── Auth routes (public) ──────────────────────────────────────────────────
	authHandler := authapi.NewHandler(authSvc, cfg)
	authapi.RegisterRoutes(v1, authHandler, cfg)

	// ── Shared public routes ──────────────────────────────────────────────────
	registerSharedRoutes(v1, database, cfg, productSvc)

	// ── Protected routes (require auth) ───────────────────────────────────────
	protected := v1.Group("")
	protected.Use(middleware.Authenticate(cfg))
	protected.Use(supplierIDMiddleware)

	// Supplier routes
	supplierHandler := supplierapi.NewHandler(database, productSvc, orderSvc, invSvc, commSvc, notifSvc)
	supplierapi.RegisterRoutesWithMW(protected, supplierHandler)

	// Admin routes
	adminHandler := adminapi.NewHandler(database, productSvc, orderSvc, commSvc, notifSvc)
	adminapi.RegisterRoutes(protected, adminHandler)

	// Delivery routes
	registerDeliveryRoutes(protected, database, orderSvc, invSvc, cfg)

	// Customer routes
	registerCustomerRoutes(protected, database, orderSvc, productSvc, commSvc, notifSvc, cfg)

	// Shared authenticated routes (notifications, account)
	registerSharedAuthRoutes(protected, database, notifSvc)

	// ── Upload stubs (require auth) ───────────────────────────────────────────
	protected.POST("/upload/image", func(c *gin.Context) {
		utils.RespondError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "File upload not configured — set up S3/storage first")
	})
	protected.POST("/upload/document", func(c *gin.Context) {
		utils.RespondError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "File upload not configured — set up S3/storage first")
	})

	// ── 404 handler ───────────────────────────────────────────────────────────
	r.NoRoute(func(c *gin.Context) {
		utils.RespondError(c, http.StatusNotFound, "NOT_FOUND", "Endpoint not found")
	})

	// ── Server ────────────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.App.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("[server] SOUQIQ API starting on port %s (env: %s)", cfg.App.Port, cfg.App.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[server] listen error: %v", err)
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[server] shutdown signal received...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[server] forced shutdown: %v", err)
	}
	log.Println("[server] shutdown complete")
}

// ── Shared public routes ──────────────────────────────────────────────────────

func registerSharedRoutes(r *gin.RouterGroup, database *sqlx.DB, cfg *config.Config, productSvc *services.ProductService) {
	// cities
	r.GET("/cities", func(c *gin.Context) {
		var cities []models.City
		_ = database.SelectContext(c.Request.Context(), &cities,
			`SELECT * FROM cities WHERE is_active = 1 ORDER BY name_ar`,
		)
		utils.RespondOK(c, cities)
	})

	// categories
	r.GET("/categories", func(c *gin.Context) {
		tree, _ := productSvc.GetCategoryTree(c.Request.Context())
		utils.RespondOK(c, tree)
	})

	// storefront
	sf := r.Group("/storefront")
	sf.Use(middleware.OptionalAuth(cfg))
	{
		sf.GET("/homepage", func(c *gin.Context) {
			type homepage struct {
				FeaturedProducts []models.Product  `json:"featured_products"`
				Categories       []models.Category `json:"categories"`
				NewArrivals      []models.Product  `json:"new_arrivals"`
			}

			featured, _, _ := productSvc.List(c.Request.Context(), services.ProductListFilter{
				IsFeatured:       true,
				PaginationParams: utils.PaginationParams{Page: 1, PerPage: 8, Offset: 0, Sort: "created_at", Order: "desc"},
			})
			categories, _ := productSvc.GetCategoryTree(c.Request.Context())
			newest, _, _ := productSvc.List(c.Request.Context(), services.ProductListFilter{
				PaginationParams: utils.PaginationParams{Page: 1, PerPage: 12, Offset: 0, Sort: "created_at", Order: "desc"},
			})

			utils.RespondOK(c, homepage{
				FeaturedProducts: featured,
				Categories:       categories,
				NewArrivals:      newest,
			})
		})

		sf.GET("/products", func(c *gin.Context) {
			pagination := utils.ParsePagination(c)
			filter := services.ProductListFilter{
				CategoryID:       c.Query("category_id"),
				SupplierID:       c.Query("supplier_id"),
				Search:           c.Query("q"),
				InStock:          c.Query("in_stock") == "true",
				HasWholesale:     c.Query("has_wholesale") == "true",
				PaginationParams: pagination,
			}
			products, total, _ := productSvc.List(c.Request.Context(), filter)
			utils.RespondList(c, products, total, pagination.Page, pagination.PerPage)
		})

		sf.GET("/products/:id", func(c *gin.Context) {
			product, err := productSvc.GetByID(c.Request.Context(), c.Param("id"))
			if err != nil {
				utils.RespondNotFound(c, "Product")
				return
			}
			utils.RespondOK(c, product)
		})

		sf.GET("/search", func(c *gin.Context) {
			pagination := utils.ParsePagination(c)
			q := c.Query("q")
			if q == "" {
				utils.RespondValidationError(c, map[string]any{"q": "Search query is required"})
				return
			}
			filter := services.ProductListFilter{
				Search:           q,
				PaginationParams: pagination,
			}
			products, total, _ := productSvc.List(c.Request.Context(), filter)
			utils.RespondList(c, products, total, pagination.Page, pagination.PerPage)
		})
	}
}

// ── Delivery routes ───────────────────────────────────────────────────────────

func registerDeliveryRoutes(r *gin.RouterGroup, database *sqlx.DB, orderSvc *services.OrderService, invSvc *services.InventoryService, cfg *config.Config) {
	d := r.Group("/delivery")
	d.Use(middleware.RequireRoles(models.RoleDelivery))
	{
		// profile
		d.GET("/profile", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var company models.DeliveryCompany
			if err := database.GetContext(c.Request.Context(), &company,
				`SELECT * FROM delivery_companies WHERE user_id = ?`, userID,
			); err != nil {
				utils.RespondNotFound(c, "Delivery company")
				return
			}
			utils.RespondOK(c, company)
		})

		d.PATCH("/profile", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var body struct {
				CompanyName  *string `json:"company_name"`
				ContactEmail *string `json:"contact_email"`
				ContactPhone *string `json:"contact_phone"`
			}
			_ = c.ShouldBindJSON(&body)
			_, _ = database.ExecContext(c.Request.Context(), `
				UPDATE delivery_companies
				SET company_name   = COALESCE(?, company_name),
				    contact_email  = COALESCE(?, contact_email),
				    contact_phone  = COALESCE(?, contact_phone),
				    updated_at     = ?
				WHERE user_id = ?`,
				body.CompanyName, body.ContactEmail, body.ContactPhone,
				time.Now(), userID,
			)
			utils.RespondOK(c, gin.H{"message": "Profile updated"})
		})

		// orders list
		d.GET("/orders", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			pagination := utils.ParsePagination(c)
			filter := map[string]any{
				"delivery_company_id": companyID,
				"status":              c.Query("status"),
			}
			orders, total, _ := orderSvc.ListSubOrders(c.Request.Context(), filter, pagination)
			utils.RespondList(c, orders, total, pagination.Page, pagination.PerPage)
		})

		// single order
		d.GET("/orders/:id", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			order, err := orderSvc.GetOrderByID(c.Request.Context(), c.Param("id"), companyID, models.RoleDelivery)
			if err != nil {
				utils.RespondNotFound(c, "Order")
				return
			}
			utils.RespondOK(c, order)
		})

		d.PATCH("/orders/:id/accept", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			actorID := middleware.MustGetUserID(c)
			if err := orderSvc.DeliveryAccept(c.Request.Context(), c.Param("id"), companyID, actorID); err != nil {
				utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
				return
			}
			utils.RespondOK(c, gin.H{"message": "Order accepted"})
		})

		d.PATCH("/orders/:id/reject", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			actorID := middleware.MustGetUserID(c)
			var body struct{ Reason string `json:"reason"` }
			_ = c.ShouldBindJSON(&body)
			if err := orderSvc.DeliveryReject(c.Request.Context(), c.Param("id"), companyID, actorID, body.Reason); err != nil {
				utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
				return
			}
			utils.RespondOK(c, gin.H{"message": "Order rejected"})
		})

		d.PATCH("/orders/:id/mark-picked-up", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			actorID := middleware.MustGetUserID(c)
			if err := orderSvc.DeliveryMarkPickedUp(c.Request.Context(), c.Param("id"), companyID, actorID); err != nil {
				utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
				return
			}
			utils.RespondOK(c, gin.H{"message": "Marked as picked up"})
		})

		d.PATCH("/orders/:id/mark-in-transit", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			actorID := middleware.MustGetUserID(c)
			if err := orderSvc.DeliveryMarkInTransit(c.Request.Context(), c.Param("id"), companyID, actorID); err != nil {
				utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
				return
			}
			utils.RespondOK(c, gin.H{"message": "Marked as in transit"})
		})

		d.PATCH("/orders/:id/mark-delivered", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			actorID := middleware.MustGetUserID(c)
			if err := orderSvc.DeliveryMarkDelivered(c.Request.Context(), c.Param("id"), companyID, actorID); err != nil {
				utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
				return
			}
			utils.RespondOK(c, gin.H{"message": "Marked as delivered"})
		})

		d.PATCH("/orders/:id/report-failure", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			actorID := middleware.MustGetUserID(c)
			var body struct{ Reason string `json:"reason"` }
			_ = c.ShouldBindJSON(&body)
			if err := orderSvc.DeliveryReportFailure(c.Request.Context(), c.Param("id"), companyID, actorID, body.Reason); err != nil {
				utils.RespondError(c, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
				return
			}
			utils.RespondOK(c, gin.H{"message": "Failure reported"})
		})

		// zones
		d.GET("/zones", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			var zones []models.DeliveryZone
			_ = database.SelectContext(c.Request.Context(), &zones,
				`SELECT * FROM delivery_zones WHERE delivery_company_id = ? ORDER BY created_at`, companyID,
			)
			utils.RespondOK(c, zones)
		})

		d.POST("/zones", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			var body struct {
				CityID         string  `json:"city_id"`
				District       *string `json:"district"`
				BasePrice      float64 `json:"base_price"`
				EstimatedHours int     `json:"estimated_hours"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				utils.RespondValidationError(c, map[string]any{"body": err.Error()})
				return
			}
			now := time.Now()
			id := utils.NewUUID()
			_, _ = database.ExecContext(c.Request.Context(), `
				INSERT INTO delivery_zones (id, delivery_company_id, city_id, district, base_price, estimated_hours, is_active, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`,
				id, companyID, body.CityID, body.District, body.BasePrice, body.EstimatedHours, now, now,
			)
			utils.RespondCreated(c, gin.H{"id": id})
		})

		d.PATCH("/zones/:id", func(c *gin.Context) {
			var body struct {
				BasePrice      *float64 `json:"base_price"`
				EstimatedHours *int     `json:"estimated_hours"`
				IsActive       *bool    `json:"is_active"`
			}
			_ = c.ShouldBindJSON(&body)
			_, _ = database.ExecContext(c.Request.Context(), `
				UPDATE delivery_zones
				SET base_price      = COALESCE(?, base_price),
				    estimated_hours = COALESCE(?, estimated_hours),
				    is_active       = COALESCE(?, is_active),
				    updated_at      = ?
				WHERE id = ?`,
				body.BasePrice, body.EstimatedHours, body.IsActive, time.Now(), c.Param("id"),
			)
			utils.RespondOK(c, gin.H{"message": "Zone updated"})
		})

		// earnings
		d.GET("/finance/overview", func(c *gin.Context) {
			companyID := getDeliveryCompanyID(c, database)
			var total float64
			_ = database.QueryRowContext(c.Request.Context(), `
				SELECT IFNULL(SUM(delivery_fee), 0) FROM sub_orders
				WHERE delivery_company_id = ? AND status IN ('delivered', 'confirmed')`,
				companyID,
			).Scan(&total)
			utils.RespondOK(c, gin.H{"total_earnings": total})
		})
	}
}

// ── Customer routes ───────────────────────────────────────────────────────────

func registerCustomerRoutes(
	r *gin.RouterGroup,
	database *sqlx.DB,
	orderSvc *services.OrderService,
	productSvc *services.ProductService,
	commSvc *services.CommissionService,
	notifSvc *services.NotificationService,
	cfg *config.Config,
) {
	c2 := r.Group("/customer")
	c2.Use(middleware.RequireRoles(models.RoleCustomer))
	{
		// profile
		c2.GET("/profile", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var cust models.Customer
			_ = database.GetContext(c.Request.Context(), &cust,
				`SELECT * FROM customers WHERE user_id = ?`, userID,
			)
			utils.RespondOK(c, cust)
		})

		c2.PATCH("/profile", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var body struct {
				FullName *string `json:"full_name"`
				CityID   *string `json:"city_id"`
			}
			_ = c.ShouldBindJSON(&body)
			_, _ = database.ExecContext(c.Request.Context(), `
				UPDATE customers
				SET full_name  = COALESCE(?, full_name),
				    updated_at = ?
				WHERE user_id = ?`,
				body.FullName, time.Now(), userID,
			)
			utils.RespondOK(c, gin.H{"message": "Profile updated"})
		})

		// addresses
		c2.GET("/addresses", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			var addrs []models.CustomerAddress
			_ = database.SelectContext(c.Request.Context(), &addrs,
				`SELECT * FROM customer_addresses WHERE customer_id = ? ORDER BY is_default DESC`, custID,
			)
			utils.RespondOK(c, addrs)
		})

		c2.POST("/addresses", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)

			var body models.CustomerAddress
			if err := c.ShouldBindJSON(&body); err != nil {
				utils.RespondValidationError(c, map[string]any{"body": err.Error()})
				return
			}
			body.ID = utils.NewUUID()
			body.CustomerID = custID
			now := time.Now()
			_, _ = database.ExecContext(c.Request.Context(), `
				INSERT INTO customer_addresses (id, customer_id, label, full_address, city_id, district, latitude, longitude, is_default, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				body.ID, body.CustomerID, body.Label, body.FullAddress, body.CityID,
				body.District, body.Latitude, body.Longitude, body.IsDefault, now, now,
			)
			utils.RespondCreated(c, body)
		})

		c2.PATCH("/addresses/:id", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			var body struct {
				Label       *string  `json:"label"`
				FullAddress *string  `json:"full_address"`
				CityID      *string  `json:"city_id"`
				District    *string  `json:"district"`
				Latitude    *float64 `json:"latitude"`
				Longitude   *float64 `json:"longitude"`
				IsDefault   *bool    `json:"is_default"`
			}
			_ = c.ShouldBindJSON(&body)
			_, _ = database.ExecContext(c.Request.Context(), `
				UPDATE customer_addresses
				SET label        = COALESCE(?, label),
				    full_address = COALESCE(?, full_address),
				    city_id      = COALESCE(?, city_id),
				    district     = COALESCE(?, district),
				    is_default   = COALESCE(?, is_default),
				    updated_at   = ?
				WHERE id = ? AND customer_id = ?`,
				body.Label, body.FullAddress, body.CityID, body.District,
				body.IsDefault, time.Now(), c.Param("id"), custID,
			)
			utils.RespondOK(c, gin.H{"message": "Address updated"})
		})

		c2.DELETE("/addresses/:id", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			_, _ = database.ExecContext(c.Request.Context(),
				`DELETE FROM customer_addresses WHERE id = ? AND customer_id = ?`,
				c.Param("id"), custID,
			)
			utils.RespondNoContent(c)
		})

		// cart
		c2.GET("/cart", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)

			var cart models.Cart
			_ = database.GetContext(c.Request.Context(), &cart,
				`SELECT * FROM carts WHERE customer_id = ? AND is_active = 1`, custID,
			)
			if cart.ID != "" {
				_ = database.SelectContext(c.Request.Context(), &cart.Items,
					`SELECT * FROM cart_items WHERE cart_id = ?`, cart.ID,
				)
			}
			utils.RespondOK(c, cart)
		})

		c2.POST("/cart/items", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)

			var body struct {
				ProductID string `json:"product_id"`
				Quantity  int    `json:"quantity"`
			}
			if err := c.ShouldBindJSON(&body); err != nil || body.ProductID == "" || body.Quantity < 1 {
				utils.RespondValidationError(c, map[string]any{"quantity": "Must be >= 1", "product_id": "Required"})
				return
			}

			// get or create active cart
			var cartID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM carts WHERE customer_id = ? AND is_active = 1`, custID,
			).Scan(&cartID)

			now := time.Now()
			if cartID == "" {
				cartID = utils.NewUUID()
				expires := now.Add(24 * time.Hour)
				_, _ = database.ExecContext(c.Request.Context(), `
					INSERT INTO carts (id, customer_id, is_active, expires_at, created_at, updated_at)
					VALUES (?, ?, 1, ?, ?, ?)`,
					cartID, custID, expires, now, now,
				)
			}

			// get product pricing
			product, err := productSvc.GetByID(c.Request.Context(), body.ProductID)
			if err != nil {
				utils.RespondNotFound(c, "Product")
				return
			}

			unitPrice, isWholesale := product.PriceForQty(body.Quantity)

			_, _ = database.ExecContext(c.Request.Context(), `
				INSERT INTO cart_items (id, cart_id, product_id, supplier_id, quantity, unit_price, is_wholesale, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				utils.NewUUID(), cartID, body.ProductID, product.SupplierID,
				body.Quantity, unitPrice, isWholesale, now, now,
			)
			utils.RespondCreated(c, gin.H{"message": "Item added to cart"})
		})

		c2.PATCH("/cart/items/:id", func(c *gin.Context) {
			var body struct {
				Quantity int `json:"quantity"`
			}
			if err := c.ShouldBindJSON(&body); err != nil || body.Quantity < 1 {
				utils.RespondValidationError(c, map[string]any{"quantity": "Must be >= 1"})
				return
			}
			_, _ = database.ExecContext(c.Request.Context(),
				`UPDATE cart_items SET quantity = ?, updated_at = ? WHERE id = ?`,
				body.Quantity, time.Now(), c.Param("id"),
			)
			utils.RespondOK(c, gin.H{"message": "Cart item updated"})
		})

		c2.DELETE("/cart/items/:id", func(c *gin.Context) {
			_, _ = database.ExecContext(c.Request.Context(),
				`DELETE FROM cart_items WHERE id = ?`, c.Param("id"),
			)
			utils.RespondNoContent(c)
		})

		c2.DELETE("/cart", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			_, _ = database.ExecContext(c.Request.Context(), `
				DELETE ci FROM cart_items ci
				JOIN carts ca ON ca.id = ci.cart_id
				WHERE ca.customer_id = ? AND ca.is_active = 1`,
				custID,
			)
			utils.RespondNoContent(c)
		})

		// orders
		c2.POST("/orders", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)

			var input services.PlaceOrderInput
			if err := c.ShouldBindJSON(&input); err != nil {
				utils.RespondValidationError(c, map[string]any{"body": err.Error()})
				return
			}
			input.CustomerID = custID

			order, err := orderSvc.PlaceOrder(c.Request.Context(), input)
			if err != nil {
				utils.RespondError(c, http.StatusBadRequest, "ORDER_FAILED", err.Error())
				return
			}
			utils.RespondCreated(c, order)
		})

		c2.GET("/orders", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			pagination := utils.ParsePagination(c)
			orders, total, _ := orderSvc.ListOrders(c.Request.Context(),
				map[string]any{"customer_id": custID}, pagination)
			utils.RespondList(c, orders, total, pagination.Page, pagination.PerPage)
		})

		c2.GET("/orders/:id", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			order, err := orderSvc.GetOrderByID(c.Request.Context(), c.Param("id"), custID, models.RoleCustomer)
			if err != nil {
				utils.RespondNotFound(c, "Order")
				return
			}
			utils.RespondOK(c, order)
		})

		c2.PATCH("/orders/:orderID/sub-orders/:id/cancel", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			if err := orderSvc.CustomerCancelSubOrder(c.Request.Context(), c.Param("id"), custID, userID); err != nil {
				utils.RespondError(c, http.StatusBadRequest, "CANCEL_FAILED", err.Error())
				return
			}
			utils.RespondOK(c, gin.H{"message": "Sub-order cancelled"})
		})

		c2.PATCH("/orders/:orderID/sub-orders/:id/confirm", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			if err := orderSvc.CustomerConfirmDelivery(c.Request.Context(), c.Param("id"), custID, userID); err != nil {
				utils.RespondError(c, http.StatusBadRequest, "CONFIRM_FAILED", err.Error())
				return
			}
			utils.RespondOK(c, gin.H{"message": "Delivery confirmed"})
		})

		// reviews
		c2.GET("/reviews", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			pagination := utils.ParsePagination(c)
			var total int64
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT COUNT(1) FROM reviews WHERE customer_id = ?`, custID,
			).Scan(&total)
			type reviewRow struct {
				ID         string    `db:"id"          json:"id"`
				ProductID  string    `db:"product_id"  json:"product_id"`
				SubOrderID string    `db:"sub_order_id" json:"sub_order_id"`
				Rating     int       `db:"rating"      json:"rating"`
				Title      *string   `db:"title"       json:"title,omitempty"`
				Body       *string   `db:"body"        json:"body,omitempty"`
				CreatedAt  time.Time `db:"created_at"  json:"created_at"`
			}
			var reviews []reviewRow
			_ = database.SelectContext(c.Request.Context(), &reviews, `
				SELECT id, product_id, sub_order_id, rating, title, body, created_at
				FROM reviews WHERE customer_id = ?
				ORDER BY created_at DESC
				LIMIT ? OFFSET ?`,
				custID, pagination.PerPage, pagination.Offset,
			)
			utils.RespondList(c, reviews, total, pagination.Page, pagination.PerPage)
		})

		c2.POST("/reviews", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)

			var body struct {
				SubOrderID string  `json:"sub_order_id"`
				ProductID  string  `json:"product_id"`
				Rating     int     `json:"rating"`
				Title      *string `json:"title"`
				Body       *string `json:"body"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				utils.RespondValidationError(c, map[string]any{"body": err.Error()})
				return
			}
			if body.Rating < 1 || body.Rating > 5 {
				utils.RespondValidationError(c, map[string]any{"rating": "Must be between 1 and 5"})
				return
			}

			// verify purchase
			var count int
			_ = database.QueryRowContext(c.Request.Context(), `
				SELECT COUNT(1) FROM order_items oi
				JOIN sub_orders so ON so.id = oi.sub_order_id
				WHERE oi.sub_order_id = ? AND oi.product_id = ?
				  AND so.status = 'confirmed'
				  AND so.order_id IN (SELECT id FROM orders WHERE customer_id = ?)`,
				body.SubOrderID, body.ProductID, custID,
			).Scan(&count)
			if count == 0 {
				utils.RespondError(c, http.StatusForbidden, "NOT_ELIGIBLE", "You can only review products you have purchased and received")
				return
			}

			now := time.Now()
			_, err := database.ExecContext(c.Request.Context(), `
				INSERT INTO reviews (id, customer_id, product_id, sub_order_id, rating, title, body, is_published, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
				utils.NewUUID(), custID, body.ProductID, body.SubOrderID,
				body.Rating, body.Title, body.Body, now, now,
			)
			if err != nil {
				utils.RespondInternalError(c)
				return
			}
			utils.RespondCreated(c, gin.H{"message": "Review submitted"})
		})

		// complaints
		c2.POST("/complaints", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var body struct {
				SubOrderID  string `json:"sub_order_id"`
				Type        string `json:"type"`
				Description string `json:"description"`
			}
			if err := c.ShouldBindJSON(&body); err != nil || body.SubOrderID == "" {
				utils.RespondValidationError(c, map[string]any{"body": err.Error()})
				return
			}
			year := time.Now().Year()
			var seq int
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT COUNT(1)+1 FROM complaints WHERE YEAR(created_at) = ?`, year,
			).Scan(&seq)
			cmpNumber := utils.FormatComplaintNumber(year, seq)
			now := time.Now()
			cmpID := utils.NewUUID()
			_, _ = database.ExecContext(c.Request.Context(), `
				INSERT INTO complaints (id, complaint_number, raised_by, raiser_role, sub_order_id, type, description, status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, 'open', ?, ?)`,
				cmpID, cmpNumber, userID, models.ComplaintRaiserCustomer,
				body.SubOrderID, body.Type, body.Description, now, now,
			)
			go notifSvc.NewComplaint(context.Background(), cmpID, cmpNumber)
			utils.RespondCreated(c, gin.H{"complaint_number": cmpNumber, "id": cmpID})
		})

		c2.GET("/complaints", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			pagination := utils.ParsePagination(c)
			var total int64
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT COUNT(1) FROM complaints WHERE raised_by = ?`, userID,
			).Scan(&total)
			var complaints []models.Complaint
			_ = database.SelectContext(c.Request.Context(), &complaints, `
				SELECT * FROM complaints WHERE raised_by = ?
				ORDER BY created_at DESC
				LIMIT ? OFFSET ?`,
				userID, pagination.PerPage, pagination.Offset,
			)
			utils.RespondList(c, complaints, total, pagination.Page, pagination.PerPage)
		})

		c2.GET("/complaints/:id", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var complaint models.Complaint
			if err := database.GetContext(c.Request.Context(), &complaint,
				`SELECT * FROM complaints WHERE id = ? AND raised_by = ?`,
				c.Param("id"), userID,
			); err != nil {
				utils.RespondNotFound(c, "Complaint")
				return
			}
			utils.RespondOK(c, complaint)
		})

		// wishlist
		c2.GET("/wishlist", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			var items []models.WishlistItem
			_ = database.SelectContext(c.Request.Context(), &items,
				`SELECT * FROM wishlist_items WHERE customer_id = ? ORDER BY created_at DESC`, custID,
			)
			utils.RespondOK(c, items)
		})

		c2.POST("/wishlist", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			var body struct{ ProductID string `json:"product_id"` }
			_ = c.ShouldBindJSON(&body)
			_, _ = database.ExecContext(c.Request.Context(), `
				INSERT INTO wishlist_items (id, customer_id, product_id, created_at)
				VALUES (?, ?, ?, ?)`,
				utils.NewUUID(), custID, body.ProductID, time.Now(),
			)
			utils.RespondCreated(c, gin.H{"message": "Added to wishlist"})
		})

		c2.DELETE("/wishlist/:productID", func(c *gin.Context) {
			userID := middleware.MustGetUserID(c)
			var custID string
			_ = database.QueryRowContext(c.Request.Context(),
				`SELECT id FROM customers WHERE user_id = ?`, userID,
			).Scan(&custID)
			_, _ = database.ExecContext(c.Request.Context(),
				`DELETE FROM wishlist_items WHERE customer_id = ? AND product_id = ?`,
				custID, c.Param("productID"),
			)
			utils.RespondNoContent(c)
		})
	}
}

// ── Shared auth routes ────────────────────────────────────────────────────────

func registerSharedAuthRoutes(r *gin.RouterGroup, database *sqlx.DB, notifSvc *services.NotificationService) {
	r.GET("/notifications", func(c *gin.Context) {
		userID := middleware.MustGetUserID(c)
		pagination := utils.ParsePagination(c)
		notifs, total, _ := notifSvc.GetNotifications(c.Request.Context(), userID, pagination)
		utils.RespondList(c, notifs, total, pagination.Page, pagination.PerPage)
	})

	r.PATCH("/notifications/:id/read", func(c *gin.Context) {
		userID := middleware.MustGetUserID(c)
		_ = notifSvc.MarkRead(c.Request.Context(), c.Param("id"), userID)
		utils.RespondOK(c, gin.H{"message": "Marked as read"})
	})

	r.PATCH("/notifications/read-all", func(c *gin.Context) {
		userID := middleware.MustGetUserID(c)
		_ = notifSvc.MarkAllRead(c.Request.Context(), userID)
		utils.RespondOK(c, gin.H{"message": "All notifications marked as read"})
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func getDeliveryCompanyID(c *gin.Context, database *sqlx.DB) string {
	userID := middleware.MustGetUserID(c)
	var id string
	_ = database.QueryRowContext(c.Request.Context(),
		`SELECT id FROM delivery_companies WHERE user_id = ?`, userID,
	).Scan(&id)
	return id
}