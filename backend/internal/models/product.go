package models

import (
	"time"
)

// ── Category ──────────────────────────────────────────────────────────────────

type Category struct {
	ID        string     `db:"id"         json:"id"`
	ParentID  *string    `db:"parent_id"  json:"parent_id,omitempty"`
	Name      string     `db:"name"       json:"name"`
	NameAr    string     `db:"name_ar"    json:"name_ar"`
	Slug      string     `db:"slug"       json:"slug"`
	IconURL   *string    `db:"icon_url"   json:"icon_url,omitempty"`
	SortOrder int        `db:"sort_order" json:"sort_order"`
	IsActive  bool       `db:"is_active"  json:"is_active"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
	Children  []*Category `db:"-"         json:"children,omitempty"`
}

// ── Product Status Constants ───────────────────────────────────────────────────

const (
	ProductStatusDraft         = "draft"
	ProductStatusPendingReview = "pending_review"
	ProductStatusActive        = "active"
	ProductStatusRejected      = "rejected"
	ProductStatusArchived      = "archived"
)

// ── Product ───────────────────────────────────────────────────────────────────

type Product struct {
	ID                     string     `db:"id"                       json:"id"`
	SupplierID             string     `db:"supplier_id"              json:"supplier_id"`
	CategoryID             string     `db:"category_id"              json:"category_id"`
	Name                   string     `db:"name"                     json:"name"`
	NameAr                 string     `db:"name_ar"                  json:"name_ar"`
	Description            *string    `db:"description"              json:"description,omitempty"`
	DescriptionAr          *string    `db:"description_ar"           json:"description_ar,omitempty"`
	SKU                    *string    `db:"sku"                      json:"sku,omitempty"`
	Status                 string     `db:"status"                   json:"status"`
	RejectionReason        *string    `db:"rejection_reason"         json:"rejection_reason,omitempty"`
	IsFeatured             bool       `db:"is_featured"              json:"is_featured"`
	RetailPrice            float64    `db:"retail_price"             json:"retail_price"`
	WholesalePrice         *float64   `db:"wholesale_price"          json:"wholesale_price,omitempty"`
	WholesaleMinQty        *int       `db:"wholesale_min_qty"        json:"wholesale_min_qty,omitempty"`
	WeightKg               *float64   `db:"weight_kg"                json:"weight_kg,omitempty"`
	CommissionRateOverride *float64   `db:"commission_rate_override" json:"commission_rate_override,omitempty"`
	CreatedAt              time.Time  `db:"created_at"               json:"created_at"`
	UpdatedAt              time.Time  `db:"updated_at"               json:"updated_at"`

	// populated via JOIN
	Images     []ProductImage     `db:"-" json:"images,omitempty"`
	Attributes []ProductAttribute `db:"-" json:"attributes,omitempty"`
	Discount   *ProductDiscount   `db:"-" json:"active_discount,omitempty"`
	Inventory  *Inventory         `db:"-" json:"inventory,omitempty"`

	// computed
	EffectivePrice float64 `db:"-" json:"effective_price"`
	IsWholesale    bool    `db:"-" json:"is_wholesale"`
	AverageRating  float64 `db:"average_rating" json:"average_rating"`
	ReviewCount    int     `db:"review_count"   json:"review_count"`
}

// IsWholesaleQty returns true if qty meets wholesale threshold
func (p *Product) IsWholesaleQty(qty int) bool {
	if p.WholesaleMinQty == nil || p.WholesalePrice == nil {
		return false
	}
	return qty >= *p.WholesaleMinQty
}

// PriceForQty returns unit price and wholesale flag for given qty
func (p *Product) PriceForQty(qty int) (price float64, isWholesale bool) {
	if p.IsWholesaleQty(qty) {
		return *p.WholesalePrice, true
	}
	return p.RetailPrice, false
}

// ── Product Image ─────────────────────────────────────────────────────────────

type ProductImage struct {
	ID        string    `db:"id"         json:"id"`
	ProductID string    `db:"product_id" json:"product_id"`
	URL       string    `db:"url"        json:"url"`
	IsPrimary bool      `db:"is_primary" json:"is_primary"`
	SortOrder int       `db:"sort_order" json:"sort_order"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ── Product Attribute ─────────────────────────────────────────────────────────

type ProductAttribute struct {
	ID             string    `db:"id"              json:"id"`
	ProductID      string    `db:"product_id"      json:"product_id"`
	AttributeName  string    `db:"attribute_name"  json:"attribute_name"`
	AttributeValue string    `db:"attribute_value" json:"attribute_value"`
	CreatedAt      time.Time `db:"created_at"      json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"      json:"updated_at"`
}

// ── Discount Type Constants ───────────────────────────────────────────────────

const (
	DiscountTypePercentage = "percentage"
	DiscountTypeFixed      = "fixed"
)

// ── Product Discount ──────────────────────────────────────────────────────────

type ProductDiscount struct {
	ID            string     `db:"id"             json:"id"`
	ProductID     string     `db:"product_id"     json:"product_id"`
	DiscountType  string     `db:"discount_type"  json:"discount_type"`
	DiscountValue float64    `db:"discount_value" json:"discount_value"`
	StartAt       time.Time  `db:"start_at"       json:"start_at"`
	EndAt         *time.Time `db:"end_at"         json:"end_at,omitempty"`
	IsActive      bool       `db:"is_active"      json:"is_active"`
	CreatedBy     string     `db:"created_by"     json:"created_by"`
	CreatedAt     time.Time  `db:"created_at"     json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"     json:"updated_at"`
}

// Apply returns price after discount
func (d *ProductDiscount) Apply(price float64) float64 {
	if !d.IsCurrentlyActive() {
		return price
	}
	switch d.DiscountType {
	case DiscountTypePercentage:
		result := price - (price * d.DiscountValue / 100)
		if result < 0 {
			return 0
		}
		return roundToTwo(result)
	case DiscountTypeFixed:
		result := price - d.DiscountValue
		if result < 0 {
			return 0
		}
		return roundToTwo(result)
	}
	return price
}

// IsCurrentlyActive returns true if discount is valid now
func (d *ProductDiscount) IsCurrentlyActive() bool {
	if !d.IsActive {
		return false
	}
	now := time.Now()
	if now.Before(d.StartAt) {
		return false
	}
	if d.EndAt != nil && now.After(*d.EndAt) {
		return false
	}
	return true
}

// ── Inventory ─────────────────────────────────────────────────────────────────

type Inventory struct {
	ID                NVARCHAR36 `db:"id"                   json:"id"`
	ProductID         string     `db:"product_id"           json:"product_id"`
	QuantityAvailable int        `db:"quantity_available"   json:"quantity_available"`
	QuantityReserved  int        `db:"quantity_reserved"    json:"quantity_reserved"`
	LowStockThreshold int        `db:"low_stock_threshold"  json:"low_stock_threshold"`
	AllowBackorder    bool       `db:"allow_backorder"      json:"allow_backorder"`
	CreatedAt         time.Time  `db:"created_at"           json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at"           json:"updated_at"`
}

// AvailableForSale returns purchasable qty
func (inv *Inventory) AvailableForSale() int {
	avail := inv.QuantityAvailable - inv.QuantityReserved
	if avail < 0 {
		return 0
	}
	return avail
}

// IsLowStock returns true if at or below threshold
func (inv *Inventory) IsLowStock() bool {
	return inv.AvailableForSale() <= inv.LowStockThreshold
}

// CanFulfill returns true if qty can be fulfilled
func (inv *Inventory) CanFulfill(qty int) bool {
	if inv.AllowBackorder {
		return true
	}
	return inv.AvailableForSale() >= qty
}

// ── Inventory Change Types ────────────────────────────────────────────────────

const (
	InvChangeRestock    = "restock"
	InvChangeSale       = "sale"
	InvChangeReturn     = "return"
	InvChangeAdjustment = "adjustment"
	InvChangeReserved   = "reserved"
	InvChangeUnreserved = "unreserved"
)

// ── Inventory Log ─────────────────────────────────────────────────────────────

type InventoryLog struct {
	ID             string    `db:"id"               json:"id"`
	ProductID      string    `db:"product_id"       json:"product_id"`
	ChangeType     string    `db:"change_type"      json:"change_type"`
	QuantityBefore int       `db:"quantity_before"  json:"quantity_before"`
	QuantityChange int       `db:"quantity_change"  json:"quantity_change"`
	QuantityAfter  int       `db:"quantity_after"   json:"quantity_after"`
	ReferenceID    *string   `db:"reference_id"     json:"reference_id,omitempty"`
	Note           *string   `db:"note"             json:"note,omitempty"`
	PerformedBy    string    `db:"performed_by"     json:"performed_by"`
	CreatedAt      time.Time `db:"created_at"       json:"created_at"`
}

// ── Cart ──────────────────────────────────────────────────────────────────────

type Cart struct {
	ID         string     `db:"id"          json:"id"`
	CustomerID string     `db:"customer_id" json:"customer_id"`
	IsActive   bool       `db:"is_active"   json:"is_active"`
	ExpiresAt  *time.Time `db:"expires_at"  json:"expires_at,omitempty"`
	CreatedAt  time.Time  `db:"created_at"  json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"  json:"updated_at"`
	Items      []CartItem `db:"-"           json:"items,omitempty"`
}

// ── Cart Item ─────────────────────────────────────────────────────────────────

type CartItem struct {
	ID            string    `db:"id"              json:"id"`
	CartID        string    `db:"cart_id"         json:"cart_id"`
	ProductID     string    `db:"product_id"      json:"product_id"`
	SupplierID    string    `db:"supplier_id"     json:"supplier_id"`
	Quantity      int       `db:"quantity"        json:"quantity"`
	UnitPrice     float64   `db:"unit_price"      json:"unit_price"`
	IsWholesale   bool      `db:"is_wholesale"    json:"is_wholesale"`
	PriceSnapshot *string   `db:"price_snapshot"  json:"price_snapshot,omitempty"`
	CreatedAt     time.Time `db:"created_at"      json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"      json:"updated_at"`
	Product       *Product  `db:"-"               json:"product,omitempty"`
}

// LineTotal returns total for this cart item
func (ci *CartItem) LineTotal() float64 {
	return roundToTwo(ci.UnitPrice * float64(ci.Quantity))
}

// ── Wishlist ──────────────────────────────────────────────────────────────────

type WishlistItem struct {
	ID         string    `db:"id"          json:"id"`
	CustomerID string    `db:"customer_id" json:"customer_id"`
	ProductID  string    `db:"product_id"  json:"product_id"`
	CreatedAt  time.Time `db:"created_at"  json:"created_at"`
	Product    *Product  `db:"-"           json:"product,omitempty"`
}

// ── Review ────────────────────────────────────────────────────────────────────

type Review struct {
	ID                string     `db:"id"                  json:"id"`
	CustomerID        string     `db:"customer_id"         json:"customer_id"`
	ProductID         string     `db:"product_id"          json:"product_id"`
	SubOrderID        string     `db:"sub_order_id"        json:"sub_order_id"`
	Rating            int        `db:"rating"              json:"rating"`
	Title             *string    `db:"title"               json:"title,omitempty"`
	Body              *string    `db:"body"                json:"body,omitempty"`
	IsPublished       bool       `db:"is_published"        json:"is_published"`
	SupplierReply     *string    `db:"supplier_reply"      json:"supplier_reply,omitempty"`
	SupplierRepliedAt *time.Time `db:"supplier_replied_at" json:"supplier_replied_at,omitempty"`
	CreatedAt         time.Time  `db:"created_at"          json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at"          json:"updated_at"`
	CustomerName      *string    `db:"customer_name"       json:"customer_name,omitempty"`
}

// ── Complaint Constants ───────────────────────────────────────────────────────

const (
	ComplaintStatusOpen        = "open"
	ComplaintStatusUnderReview = "under_review"
	ComplaintStatusResolved    = "resolved"
	ComplaintStatusClosed      = "closed"

	ComplaintTypeWrongItem    = "wrong_item"
	ComplaintTypeNotDelivered = "not_delivered"
	ComplaintTypeDamaged      = "damaged"
	ComplaintTypeLateDelivery = "late_delivery"
	ComplaintTypeFraud        = "fraud"
	ComplaintTypeOther        = "other"

	ComplaintRaiserCustomer = "customer"
	ComplaintRaiserSupplier = "supplier"
)

// ── Complaint ─────────────────────────────────────────────────────────────────

type Complaint struct {
	ID              string     `db:"id"               json:"id"`
	ComplaintNumber string     `db:"complaint_number" json:"complaint_number"`
	RaisedBy        string     `db:"raised_by"        json:"raised_by"`
	RaiserRole      string     `db:"raiser_role"      json:"raiser_role"`
	SubOrderID      string     `db:"sub_order_id"     json:"sub_order_id"`
	Type            string     `db:"type"             json:"type"`
	Description     string     `db:"description"      json:"description"`
	Status          string     `db:"status"           json:"status"`
	Resolution      *string    `db:"resolution"       json:"resolution,omitempty"`
	ResolvedBy      *string    `db:"resolved_by"      json:"resolved_by,omitempty"`
	ResolvedAt      *time.Time `db:"resolved_at"      json:"resolved_at,omitempty"`
	CreatedAt       time.Time  `db:"created_at"       json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"       json:"updated_at"`

	Attachments []ComplaintAttachment `db:"-" json:"attachments,omitempty"`
}

// ── Complaint Attachment ──────────────────────────────────────────────────────

type ComplaintAttachment struct {
	ID          string    `db:"id"           json:"id"`
	ComplaintID string    `db:"complaint_id" json:"complaint_id"`
	FileURL     string    `db:"file_url"     json:"file_url"`
	UploadedBy  string    `db:"uploaded_by"  json:"uploaded_by"`
	CreatedAt   time.Time `db:"created_at"   json:"created_at"`
}

// ── Type alias to avoid confusion ─────────────────────────────────────────────

type NVARCHAR36 = string

// ── Helpers ───────────────────────────────────────────────────────────────────

func roundToTwo(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}