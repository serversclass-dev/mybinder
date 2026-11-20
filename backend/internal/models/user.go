package models

import (
	"time"
)

// Role constants
const (
	RoleSuperAdmin    = "super_admin"
	RoleFinanceAdmin  = "finance_admin"
	RoleOpsAdmin      = "ops_admin"
	RoleSupplier      = "supplier"
	RoleCustomer      = "customer"
	RoleDelivery      = "delivery_staff"
)

// User is the base account for all actors in the system
type User struct {
	ID           string     `db:"id"            json:"id"`
	Email        *string    `db:"email"         json:"email,omitempty"`
	Phone        string     `db:"phone"         json:"phone"`
	PasswordHash string     `db:"password_hash" json:"-"`
	Role         string     `db:"role"          json:"role"`
	IsActive     bool       `db:"is_active"     json:"is_active"`
	IsVerified   bool       `db:"is_verified"   json:"is_verified"`
	LastLoginAt  *time.Time `db:"last_login_at" json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"    json:"updated_at"`
}

// UserSession tracks active JWT sessions for revocation
type UserSession struct {
	ID        string    `db:"id"         json:"id"`
	UserID    string    `db:"user_id"    json:"user_id"`
	TokenHash string    `db:"token_hash" json:"-"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`
	IPAddress string    `db:"ip_address" json:"ip_address"`
	UserAgent string    `db:"user_agent" json:"user_agent"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// ── Supplier ─────────────────────────────────────────────────────────────────

// SupplierStatus constants
const (
	SupplierStatusPending   = "pending"
	SupplierStatusApproved  = "approved"
	SupplierStatusRejected  = "rejected"
	SupplierStatusSuspended = "suspended"
)

type Supplier struct {
	ID                       string     `db:"id"                          json:"id"`
	UserID                   string     `db:"user_id"                     json:"user_id"`
	BusinessName             string     `db:"business_name"               json:"business_name"`
	BusinessNameAr           string     `db:"business_name_ar"            json:"business_name_ar"`
	Description              *string    `db:"description"                 json:"description,omitempty"`
	LogoURL                  *string    `db:"logo_url"                    json:"logo_url,omitempty"`
	CoverURL                 *string    `db:"cover_url"                   json:"cover_url,omitempty"`
	CommercialRegister       *string    `db:"commercial_register"         json:"commercial_register,omitempty"`
	TaxNumber                *string    `db:"tax_number"                  json:"tax_number,omitempty"`
	Status                   string     `db:"status"                      json:"status"`
	RejectionReason          *string    `db:"rejection_reason"            json:"rejection_reason,omitempty"`
	ApprovedBy               *string    `db:"approved_by"                 json:"approved_by,omitempty"`
	ApprovedAt               *time.Time `db:"approved_at"                 json:"approved_at,omitempty"`
	CommissionRateOverride   *float64   `db:"commission_rate_override"    json:"commission_rate_override,omitempty"`
	CityID                   *string    `db:"city_id"                     json:"city_id,omitempty"`
	Address                  *string    `db:"address"                     json:"address,omitempty"`
	CreatedAt                time.Time  `db:"created_at"                  json:"created_at"`
	UpdatedAt                time.Time  `db:"updated_at"                  json:"updated_at"`
}

type SupplierBankAccount struct {
	ID             string    `db:"id"              json:"id"`
	SupplierID     string    `db:"supplier_id"     json:"supplier_id"`
	BankName       string    `db:"bank_name"       json:"bank_name"`
	AccountHolder  string    `db:"account_holder"  json:"account_holder"`
	AccountNumber  string    `db:"account_number"  json:"account_number"`
	IBAN           *string   `db:"iban"            json:"iban,omitempty"`
	IsPrimary      bool      `db:"is_primary"      json:"is_primary"`
	CreatedAt      time.Time `db:"created_at"      json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"      json:"updated_at"`
}

type SupplierDocument struct {
	ID           string    `db:"id"            json:"id"`
	SupplierID   string    `db:"supplier_id"   json:"supplier_id"`
	DocumentType string    `db:"document_type" json:"document_type"`
	FileURL      string    `db:"file_url"      json:"file_url"`
	Verified     bool      `db:"verified"      json:"verified"`
	CreatedAt    time.Time `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"    json:"updated_at"`
}

// ── Customer ──────────────────────────────────────────────────────────────────

type Customer struct {
	ID          string     `db:"id"           json:"id"`
	UserID      string     `db:"user_id"      json:"user_id"`
	FullName    string     `db:"full_name"    json:"full_name"`
	AvatarURL   *string    `db:"avatar_url"   json:"avatar_url,omitempty"`
	DateOfBirth *time.Time `db:"date_of_birth" json:"date_of_birth,omitempty"`
	CreatedAt   time.Time  `db:"created_at"   json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"   json:"updated_at"`
}

type CustomerAddress struct {
	ID          string    `db:"id"           json:"id"`
	CustomerID  string    `db:"customer_id"  json:"customer_id"`
	Label       string    `db:"label"        json:"label"`
	FullAddress string    `db:"full_address" json:"full_address"`
	CityID      string    `db:"city_id"      json:"city_id"`
	District    *string   `db:"district"     json:"district,omitempty"`
	Latitude    *float64  `db:"latitude"     json:"latitude,omitempty"`
	Longitude   *float64  `db:"longitude"    json:"longitude,omitempty"`
	IsDefault   bool      `db:"is_default"   json:"is_default"`
	CreatedAt   time.Time `db:"created_at"   json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"   json:"updated_at"`
}

// ── Delivery Company ──────────────────────────────────────────────────────────

const (
	DeliveryStatusPending   = "pending"
	DeliveryStatusApproved  = "approved"
	DeliveryStatusSuspended = "suspended"
)

type DeliveryCompany struct {
	ID           string     `db:"id"            json:"id"`
	UserID       string     `db:"user_id"       json:"user_id"`
	CompanyName  string     `db:"company_name"  json:"company_name"`
	CompanyNameAr string    `db:"company_name_ar" json:"company_name_ar"`
	LogoURL      *string    `db:"logo_url"      json:"logo_url,omitempty"`
	Status       string     `db:"status"        json:"status"`
	ApprovedBy   *string    `db:"approved_by"   json:"approved_by,omitempty"`
	ApprovedAt   *time.Time `db:"approved_at"   json:"approved_at,omitempty"`
	ContactEmail *string    `db:"contact_email" json:"contact_email,omitempty"`
	ContactPhone *string    `db:"contact_phone" json:"contact_phone,omitempty"`
	CreatedAt    time.Time  `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"    json:"updated_at"`
}

type DeliveryZone struct {
	ID                string    `db:"id"                   json:"id"`
	DeliveryCompanyID string    `db:"delivery_company_id"  json:"delivery_company_id"`
	CityID            string    `db:"city_id"              json:"city_id"`
	District          *string   `db:"district"             json:"district,omitempty"`
	BasePrice         float64   `db:"base_price"           json:"base_price"`
	EstimatedHours    int       `db:"estimated_hours"      json:"estimated_hours"`
	IsActive          bool      `db:"is_active"            json:"is_active"`
	CreatedAt         time.Time `db:"created_at"           json:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"           json:"updated_at"`
}

// ── City ──────────────────────────────────────────────────────────────────────

type City struct {
	ID          string    `db:"id"          json:"id"`
	Name        string    `db:"name"        json:"name"`
	NameAr      string    `db:"name_ar"     json:"name_ar"`
	CountryCode string    `db:"country_code" json:"country_code"`
	IsActive    bool      `db:"is_active"   json:"is_active"`
	CreatedAt   time.Time `db:"created_at"  json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"  json:"updated_at"`
}

// ── Notification ──────────────────────────────────────────────────────────────

type Notification struct {
	ID        string     `db:"id"         json:"id"`
	UserID    string     `db:"user_id"    json:"user_id"`
	Type      string     `db:"type"       json:"type"`
	Title     string     `db:"title"      json:"title"`
	Body      string     `db:"body"       json:"body"`
	Data      *string    `db:"data"       json:"data,omitempty"` // JSON string
	IsRead    bool       `db:"is_read"    json:"is_read"`
	ReadAt    *time.Time `db:"read_at"    json:"read_at,omitempty"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
}

// ── Audit Log ─────────────────────────────────────────────────────────────────

type AuditLog struct {
	ID           string    `db:"id"            json:"id"`
	ActorID      string    `db:"actor_id"      json:"actor_id"`
	ActorRole    string    `db:"actor_role"    json:"actor_role"`
	Action       string    `db:"action"        json:"action"`
	TargetType   string    `db:"target_type"   json:"target_type"`
	TargetID     string    `db:"target_id"     json:"target_id"`
	BeforeState  *string   `db:"before_state"  json:"before_state,omitempty"`  // JSON string
	AfterState   *string   `db:"after_state"   json:"after_state,omitempty"`   // JSON string
	IPAddress    *string   `db:"ip_address"    json:"ip_address,omitempty"`
	PerformedAt  time.Time `db:"performed_at"  json:"performed_at"`
}

// ── Platform Setting ──────────────────────────────────────────────────────────

type PlatformSetting struct {
	ID          string    `db:"id"          json:"id"`
	Key         string    `db:"key"         json:"key"`
	Value       string    `db:"value"       json:"value"`
	ValueType   string    `db:"value_type"  json:"value_type"`
	Description *string   `db:"description" json:"description,omitempty"`
	UpdatedBy   *string   `db:"updated_by"  json:"updated_by,omitempty"`
	CreatedAt   time.Time `db:"created_at"  json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"  json:"updated_at"`
}

// Setting keys — used as constants to avoid string typos
const (
	SettingDefaultCommissionRate         = "default_commission_rate"
	SettingWholesaleCommissionRate       = "wholesale_commission_rate"
	SettingWholesaleBuyerFeeRate         = "wholesale_buyer_fee_rate"
	SettingMinWithdrawalAmount           = "min_withdrawal_amount"
	SettingOrderCancellationWindowMins   = "order_cancellation_window_minutes"
	SettingAutoCompleteOrderHours        = "auto_complete_order_hours"
	SettingSupplierAcceptanceTimeoutHours = "supplier_acceptance_timeout_hours"
	SettingLowStockThresholdDefault      = "low_stock_threshold_default"
	SettingMaxDeliveryAttempts           = "max_delivery_attempts"
)