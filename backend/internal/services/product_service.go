package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"souqiq/internal/cache"
	"souqiq/internal/models"
	"souqiq/internal/utils"
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrProductNotFound      = errors.New("product not found")
	ErrCategoryNotFound     = errors.New("category not found")
	ErrProductNotOwned      = errors.New("product does not belong to this supplier")
	ErrProductNotDraft      = errors.New("product must be in draft status to submit")
	ErrProductAlreadyActive = errors.New("product is already active")
	ErrCannotDeleteProduct  = errors.New("cannot delete product with order history")
)

// ── DTOs ──────────────────────────────────────────────────────────────────────

type CreateProductInput struct {
	SupplierID      string              `json:"-"` // set from auth context
	CategoryID      string              `json:"category_id"       validate:"required,uuid"`
	Name            string              `json:"name"              validate:"required,min=2,max=255"`
	NameAr          string              `json:"name_ar"           validate:"required,min=2,max=255"`
	Description     *string             `json:"description"`
	DescriptionAr   *string             `json:"description_ar"`
	SKU             *string             `json:"sku"`
	RetailPrice     float64             `json:"retail_price"      validate:"required,gt=0"`
	WholesalePrice  *float64            `json:"wholesale_price"`
	WholesaleMinQty *int                `json:"wholesale_min_qty"`
	WeightKg        *float64            `json:"weight_kg"`
	QuantityAvail   int                 `json:"quantity_available" validate:"gte=0"`
	LowStockThresh  *int                `json:"low_stock_threshold"`
	AllowBackorder  bool                `json:"allow_backorder"`
	Attributes      []AttributeInput    `json:"attributes"`
}

type AttributeInput struct {
	Name  string `json:"attribute_name"  validate:"required"`
	Value string `json:"attribute_value" validate:"required"`
}

type UpdateProductInput struct {
	CategoryID      *string          `json:"category_id"`
	Name            *string          `json:"name"`
	NameAr          *string          `json:"name_ar"`
	Description     *string          `json:"description"`
	DescriptionAr   *string          `json:"description_ar"`
	SKU             *string          `json:"sku"`
	RetailPrice     *float64         `json:"retail_price"`
	WholesalePrice  *float64         `json:"wholesale_price"`
	WholesaleMinQty *int             `json:"wholesale_min_qty"`
	WeightKg        *float64         `json:"weight_kg"`
	Attributes      []AttributeInput `json:"attributes"`
}

type AddDiscountInput struct {
	DiscountType  string     `json:"discount_type"  validate:"required,oneof=percentage fixed"`
	DiscountValue float64    `json:"discount_value" validate:"required,gt=0"`
	StartAt       time.Time  `json:"start_at"       validate:"required"`
	EndAt         *time.Time `json:"end_at"`
}

type ProductListFilter struct {
	SupplierID  string
	CategoryID  string
	Status      string
	MinPrice    *float64
	MaxPrice    *float64
	InStock     bool
	HasWholesale bool
	Search      string
	CityID      string
	IsFeatured  bool
	utils.PaginationParams
}

// ── Service ───────────────────────────────────────────────────────────────────

type ProductService struct {
	db *sqlx.DB
}

func NewProductService(db *sqlx.DB) *ProductService {
	return &ProductService{db: db}
}

// ── Create ────────────────────────────────────────────────────────────────────

func (s *ProductService) Create(ctx context.Context, input CreateProductInput) (*models.Product, error) {
	// verify category exists
	if err := s.validateCategory(ctx, input.CategoryID); err != nil {
		return nil, err
	}

	productID := utils.NewUUID()
	inventoryID := utils.NewUUID()
	now := time.Now()

	// resolve low stock threshold
	lowStock := 5 // default
	if input.LowStockThresh != nil {
		lowStock = *input.LowStockThresh
	}

	err := withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO products (
				id, supplier_id, category_id, name, name_ar,
				description, description_ar, sku, status,
				is_featured, retail_price, wholesale_price, wholesale_min_qty,
				weight_kg, created_at, updated_at
			) VALUES (
				@p1, @p2, @p3, @p4, @p5,
				@p6, @p7, @p8, @p9,
				0, @p10, @p11, @p12,
				@p13, @p14, @p15
			)`,
			productID, input.SupplierID, input.CategoryID,
			input.Name, input.NameAr,
			input.Description, input.DescriptionAr, input.SKU,
			models.ProductStatusDraft,
			input.RetailPrice, input.WholesalePrice, input.WholesaleMinQty,
			input.WeightKg, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert product: %w", err)
		}

		// create inventory record
		_, err = tx.ExecContext(ctx, `
			INSERT INTO inventory (id, product_id, quantity_available, quantity_reserved, low_stock_threshold, allow_backorder, created_at, updated_at)
			VALUES (@p1, @p2, @p3, 0, @p4, @p5, @p6, @p7)`,
			inventoryID, productID, input.QuantityAvail,
			lowStock, input.AllowBackorder, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert inventory: %w", err)
		}

		// insert attributes
		for _, attr := range input.Attributes {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO product_attributes (id, product_id, attribute_name, attribute_value, created_at, updated_at)
				VALUES (@p1, @p2, @p3, @p4, @p5, @p6)`,
				utils.NewUUID(), productID, attr.Name, attr.Value, now, now,
			)
			if err != nil {
				return fmt.Errorf("insert attribute: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(ctx, productID)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (s *ProductService) Update(ctx context.Context, productID, supplierID string, input UpdateProductInput) (*models.Product, error) {
	product, err := s.getProductOwned(ctx, productID, supplierID)
	if err != nil {
		return nil, err
	}

	// cannot edit active product without re-review
	setClauses := []string{"updated_at = @p1"}
	args := []any{time.Now()}
	idx := 2

	if input.CategoryID != nil {
		if err := s.validateCategory(ctx, *input.CategoryID); err != nil {
			return nil, err
		}
		setClauses = append(setClauses, fmt.Sprintf("category_id = @p%d", idx))
		args = append(args, *input.CategoryID)
		idx++
	}
	if input.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = @p%d", idx))
		args = append(args, *input.Name)
		idx++
	}
	if input.NameAr != nil {
		setClauses = append(setClauses, fmt.Sprintf("name_ar = @p%d", idx))
		args = append(args, *input.NameAr)
		idx++
	}
	if input.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = @p%d", idx))
		args = append(args, *input.Description)
		idx++
	}
	if input.DescriptionAr != nil {
		setClauses = append(setClauses, fmt.Sprintf("description_ar = @p%d", idx))
		args = append(args, *input.DescriptionAr)
		idx++
	}
	if input.SKU != nil {
		setClauses = append(setClauses, fmt.Sprintf("sku = @p%d", idx))
		args = append(args, *input.SKU)
		idx++
	}
	if input.RetailPrice != nil {
		setClauses = append(setClauses, fmt.Sprintf("retail_price = @p%d", idx))
		args = append(args, *input.RetailPrice)
		idx++
	}
	if input.WholesalePrice != nil {
		setClauses = append(setClauses, fmt.Sprintf("wholesale_price = @p%d", idx))
		args = append(args, *input.WholesalePrice)
		idx++
	}
	if input.WholesaleMinQty != nil {
		setClauses = append(setClauses, fmt.Sprintf("wholesale_min_qty = @p%d", idx))
		args = append(args, *input.WholesaleMinQty)
		idx++
	}
	if input.WeightKg != nil {
		setClauses = append(setClauses, fmt.Sprintf("weight_kg = @p%d", idx))
		args = append(args, *input.WeightKg)
		idx++
	}

	args = append(args, productID)
	query := fmt.Sprintf("UPDATE products SET %s WHERE id = @p%d",
		strings.Join(setClauses, ", "), idx)

	_, err = s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}

	// replace attributes if provided
	if input.Attributes != nil {
		if err := s.replaceAttributes(ctx, productID, input.Attributes); err != nil {
			return nil, err
		}
	}

	// if product was active, move back to pending_review
	if product.Status == models.ProductStatusActive {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE products SET status = @p1 WHERE id = @p2`,
			models.ProductStatusPendingReview, productID,
		)
	}

	_ = cache.Del(ctx, cache.ProductKey(productID))
	return s.GetByID(ctx, productID)
}

// ── Submit for Review ─────────────────────────────────────────────────────────

func (s *ProductService) SubmitForReview(ctx context.Context, productID, supplierID string) error {
	product, err := s.getProductOwned(ctx, productID, supplierID)
	if err != nil {
		return err
	}
	if product.Status != models.ProductStatusDraft && product.Status != models.ProductStatusRejected {
		return ErrProductNotDraft
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE products SET status = @p1, rejection_reason = NULL, updated_at = @p2 WHERE id = @p3`,
		models.ProductStatusPendingReview, time.Now(), productID,
	)
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return err
}

// ── Admin: Approve / Reject ───────────────────────────────────────────────────

func (s *ProductService) Approve(ctx context.Context, productID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE products SET status = @p1, rejection_reason = NULL, updated_at = @p2 WHERE id = @p3`,
		models.ProductStatusActive, time.Now(), productID,
	)
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return err
}

func (s *ProductService) Reject(ctx context.Context, productID, reason string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE products SET status = @p1, rejection_reason = @p2, updated_at = @p3 WHERE id = @p4`,
		models.ProductStatusRejected, reason, time.Now(), productID,
	)
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return err
}

func (s *ProductService) Deactivate(ctx context.Context, productID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE products SET status = @p1, updated_at = @p2 WHERE id = @p3`,
		models.ProductStatusArchived, time.Now(), productID,
	)
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return err
}

func (s *ProductService) SetFeatured(ctx context.Context, productID string, featured bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE products SET is_featured = @p1, updated_at = @p2 WHERE id = @p3`,
		featured, time.Now(), productID,
	)
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return err
}

func (s *ProductService) Delete(ctx context.Context, productID, supplierID string) error {
	// check no order history
	var count int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM order_items WHERE product_id = @p1`, productID,
	).Scan(&count)
	if count > 0 {
		return ErrCannotDeleteProduct
	}

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM products WHERE id = @p1 AND supplier_id = @p2`,
		productID, supplierID,
	)
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return err
}

// ── Images ────────────────────────────────────────────────────────────────────

func (s *ProductService) AddImage(ctx context.Context, productID, supplierID, url string, isPrimary bool) (*models.ProductImage, error) {
	if _, err := s.getProductOwned(ctx, productID, supplierID); err != nil {
		return nil, err
	}

	// if setting as primary, unset others
	if isPrimary {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE product_images SET is_primary = 0 WHERE product_id = @p1`, productID,
		)
	}

	// get next sort order
	var maxSort int
	_ = s.db.QueryRowContext(ctx,
		`SELECT ISNULL(MAX(sort_order), 0) FROM product_images WHERE product_id = @p1`, productID,
	).Scan(&maxSort)

	img := &models.ProductImage{
		ID:        utils.NewUUID(),
		ProductID: productID,
		URL:       url,
		IsPrimary: isPrimary,
		SortOrder: maxSort + 1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO product_images (id, product_id, url, is_primary, sort_order, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7)`,
		img.ID, img.ProductID, img.URL, img.IsPrimary, img.SortOrder, img.CreatedAt, img.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert image: %w", err)
	}
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return img, nil
}

func (s *ProductService) RemoveImage(ctx context.Context, productID, supplierID, imageID string) error {
	if _, err := s.getProductOwned(ctx, productID, supplierID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM product_images WHERE id = @p1 AND product_id = @p2`,
		imageID, productID,
	)
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return err
}

// ── Discounts ─────────────────────────────────────────────────────────────────

func (s *ProductService) AddDiscount(ctx context.Context, productID, supplierID, createdBy string, input AddDiscountInput) error {
	if _, err := s.getProductOwned(ctx, productID, supplierID); err != nil {
		return err
	}

	// deactivate existing discounts
	_, _ = s.db.ExecContext(ctx,
		`UPDATE product_discounts SET is_active = 0 WHERE product_id = @p1`, productID,
	)

	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO product_discounts (id, product_id, discount_type, discount_value, start_at, end_at, is_active, created_by, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, 1, @p7, @p8, @p9)`,
		utils.NewUUID(), productID,
		input.DiscountType, input.DiscountValue,
		input.StartAt, input.EndAt,
		createdBy, now, now,
	)
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return err
}

func (s *ProductService) RemoveDiscount(ctx context.Context, productID, supplierID, discountID string) error {
	if _, err := s.getProductOwned(ctx, productID, supplierID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM product_discounts WHERE id = @p1 AND product_id = @p2`,
		discountID, productID,
	)
	_ = cache.Del(ctx, cache.ProductKey(productID))
	return err
}

// ── Get / List ────────────────────────────────────────────────────────────────

func (s *ProductService) GetByID(ctx context.Context, productID string) (*models.Product, error) {
	// try cache first
	var cached models.Product
	found, err := cache.GetJSON(ctx, cache.ProductKey(productID), &cached)
	if err == nil && found {
		return &cached, nil
	}

	var p models.Product
	err = s.db.GetContext(ctx, &p, `
		SELECT p.*,
		       ISNULL(AVG(CAST(r.rating AS FLOAT)), 0) AS average_rating,
		       COUNT(r.id) AS review_count
		FROM products p
		LEFT JOIN reviews r ON r.product_id = p.id AND r.is_published = 1
		WHERE p.id = @p1
		GROUP BY p.id, p.supplier_id, p.category_id, p.name, p.name_ar,
		         p.description, p.description_ar, p.sku, p.status,
		         p.rejection_reason, p.is_featured, p.retail_price,
		         p.wholesale_price, p.wholesale_min_qty, p.weight_kg,
		         p.commission_rate_override, p.created_at, p.updated_at`,
		productID,
	)
	if err != nil {
		return nil, ErrProductNotFound
	}

	// load images
	_ = s.db.SelectContext(ctx, &p.Images,
		`SELECT * FROM product_images WHERE product_id = @p1 ORDER BY sort_order`, productID,
	)

	// load attributes
	_ = s.db.SelectContext(ctx, &p.Attributes,
		`SELECT * FROM product_attributes WHERE product_id = @p1`, productID,
	)

	// load active discount
	var disc models.ProductDiscount
	discErr := s.db.GetContext(ctx, &disc,
		`SELECT * FROM product_discounts WHERE product_id = @p1 AND is_active = 1`, productID,
	)
	if discErr == nil && disc.IsCurrentlyActive() {
		p.Discount = &disc
	}

	// load inventory
	var inv models.Inventory
	if invErr := s.db.GetContext(ctx, &inv,
		`SELECT * FROM inventory WHERE product_id = @p1`, productID,
	); invErr == nil {
		p.Inventory = &inv
	}

	// cache it
	_ = cache.SetJSON(ctx, cache.ProductKey(productID), p, cache.TTLProduct)
	return &p, nil
}

func (s *ProductService) List(ctx context.Context, filter ProductListFilter) ([]models.Product, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.SupplierID != "" {
		where = append(where, fmt.Sprintf("p.supplier_id = @p%d", idx))
		args = append(args, filter.SupplierID)
		idx++
	}
	if filter.CategoryID != "" {
		where = append(where, fmt.Sprintf("p.category_id = @p%d", idx))
		args = append(args, filter.CategoryID)
		idx++
	}
	if filter.Status != "" {
		where = append(where, fmt.Sprintf("p.status = @p%d", idx))
		args = append(args, filter.Status)
		idx++
	} else {
		// default: only show active products on storefront
		where = append(where, fmt.Sprintf("p.status = @p%d", idx))
		args = append(args, models.ProductStatusActive)
		idx++
	}
	if filter.MinPrice != nil {
		where = append(where, fmt.Sprintf("p.retail_price >= @p%d", idx))
		args = append(args, *filter.MinPrice)
		idx++
	}
	if filter.MaxPrice != nil {
		where = append(where, fmt.Sprintf("p.retail_price <= @p%d", idx))
		args = append(args, *filter.MaxPrice)
		idx++
	}
	if filter.InStock {
		where = append(where, "i.quantity_available > i.quantity_reserved")
	}
	if filter.HasWholesale {
		where = append(where, "p.wholesale_price IS NOT NULL")
	}
	if filter.IsFeatured {
		where = append(where, "p.is_featured = 1")
	}
	if filter.Search != "" {
		where = append(where, fmt.Sprintf(
			"(p.name LIKE @p%d OR p.name_ar LIKE @p%d OR p.description LIKE @p%d)",
			idx, idx, idx,
		))
		args = append(args, "%"+filter.Search+"%")
		idx++
	}

	whereStr := strings.Join(where, " AND ")

	// count
	var total int64
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT p.id)
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE %s`, whereStr)
	_ = s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)

	// data
	allowedSorts := map[string]string{
		"created_at":  "p.created_at",
		"retail_price": "p.retail_price",
		"name":        "p.name",
	}
	sortCol, ok := allowedSorts[filter.Sort]
	if !ok {
		sortCol = "p.created_at"
	}
	orderDir := "DESC"
	if filter.Order == "asc" {
		orderDir = "ASC"
	}

	dataQuery := fmt.Sprintf(`
		SELECT p.*,
		       ISNULL(AVG(CAST(r.rating AS FLOAT)), 0) AS average_rating,
		       COUNT(DISTINCT r.id) AS review_count
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		LEFT JOIN reviews r ON r.product_id = p.id AND r.is_published = 1
		WHERE %s
		GROUP BY p.id, p.supplier_id, p.category_id, p.name, p.name_ar,
		         p.description, p.description_ar, p.sku, p.status,
		         p.rejection_reason, p.is_featured, p.retail_price,
		         p.wholesale_price, p.wholesale_min_qty, p.weight_kg,
		         p.commission_rate_override, p.created_at, p.updated_at
		ORDER BY %s %s
		OFFSET @p%d ROWS FETCH NEXT @p%d ROWS ONLY`,
		whereStr, sortCol, orderDir, idx, idx+1,
	)
	args = append(args, filter.Offset, filter.PerPage)

	var products []models.Product
	if err := s.db.SelectContext(ctx, &products, dataQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}

	// attach primary image to each product
	for i := range products {
		var img models.ProductImage
		_ = s.db.GetContext(ctx, &img,
			`SELECT TOP 1 * FROM product_images WHERE product_id = @p1 AND is_primary = 1`,
			products[i].ID,
		)
		if img.ID != "" {
			products[i].Images = []models.ProductImage{img}
		}
	}

	return products, total, nil
}

// ── Category ──────────────────────────────────────────────────────────────────

func (s *ProductService) GetCategoryTree(ctx context.Context) ([]models.Category, error) {
	var cached []models.Category
	found, _ := cache.GetJSON(ctx, cache.CategoryTreeKey(), &cached)
	if found {
		return cached, nil
	}

	var all []models.Category
	if err := s.db.SelectContext(ctx, &all,
		`SELECT * FROM categories WHERE is_active = 1 ORDER BY sort_order`,
	); err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	tree := buildCategoryTree(all)
	_ = cache.SetJSON(ctx, cache.CategoryTreeKey(), tree, cache.TTLCategory)
	return tree, nil
}

func (s *ProductService) CreateCategory(ctx context.Context, name, nameAr, slug string, parentID *string, sortOrder int) (*models.Category, error) {
	cat := &models.Category{
		ID:        utils.NewUUID(),
		ParentID:  parentID,
		Name:      name,
		NameAr:    nameAr,
		Slug:      slug,
		SortOrder: sortOrder,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO categories (id, parent_id, name, name_ar, slug, sort_order, is_active, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, 1, @p7, @p8)`,
		cat.ID, cat.ParentID, cat.Name, cat.NameAr, cat.Slug,
		cat.SortOrder, cat.CreatedAt, cat.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert category: %w", err)
	}
	_ = cache.Del(ctx, cache.CategoryTreeKey())
	return cat, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (s *ProductService) getProductOwned(ctx context.Context, productID, supplierID string) (*models.Product, error) {
	var p models.Product
	err := s.db.GetContext(ctx, &p,
		`SELECT id, supplier_id, status FROM products WHERE id = @p1`,
		productID,
	)
	if err != nil {
		return nil, ErrProductNotFound
	}
	if p.SupplierID != supplierID {
		return nil, ErrProductNotOwned
	}
	return &p, nil
}

func (s *ProductService) validateCategory(ctx context.Context, categoryID string) error {
	var count int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM categories WHERE id = @p1 AND is_active = 1`, categoryID,
	).Scan(&count)
	if count == 0 {
		return ErrCategoryNotFound
	}
	return nil
}

func (s *ProductService) replaceAttributes(ctx context.Context, productID string, attrs []AttributeInput) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM product_attributes WHERE product_id = @p1`, productID,
	)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, attr := range attrs {
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO product_attributes (id, product_id, attribute_name, attribute_value, created_at, updated_at)
			VALUES (@p1, @p2, @p3, @p4, @p5, @p6)`,
			utils.NewUUID(), productID, attr.Name, attr.Value, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert attribute: %w", err)
		}
	}
	return nil
}

func buildCategoryTree(all []models.Category) []models.Category {
	byID := make(map[string]*models.Category, len(all))
	for i := range all {
		byID[all[i].ID] = &all[i]
	}

	var roots []models.Category
	for i := range all {
		cat := &all[i]
		if cat.ParentID == nil {
			roots = append(roots, all[i])
		} else {
			if parent, ok := byID[*cat.ParentID]; ok {
				parent.Children = append(parent.Children, &all[i])
			}
		}
	}
	return roots
}