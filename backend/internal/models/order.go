package models

import (
	"time"
)

// ── Order Status Constants ────────────────────────────────────────────────────

const (
	// Master order statuses
	OrderStatusPending           = "pending"
	OrderStatusProcessing        = "processing"
	OrderStatusPartiallyComplete = "partially_complete"
	OrderStatusComplete          = "complete"
	OrderStatusCancelled         = "cancelled"

	// Sub-order statuses
	SubOrderStatusPendingSupplier  = "pending_supplier"
	SubOrderStatusAccepted         = "accepted"
	SubOrderStatusRejected         = "rejected"
	SubOrderStatusPreparing        = "preparing"
	SubOrderStatusReadyForPickup   = "ready_for_pickup"
	SubOrderStatusPickedUp         = "picked_up"
	SubOrderStatusInTransit        = "in_transit"
	SubOrderStatusDelivered        = "delivered"
	SubOrderStatusConfirmed        = "confirmed"
	SubOrderStatusFailedDelivery   = "failed_delivery"
	SubOrderStatusReturned         = "returned"
	SubOrderStatusCancelled        = "cancelled"

	// Payment methods
	PaymentMethodCOD          = "cod"
	PaymentMethodBankTransfer = "bank_transfer"
	PaymentMethodOnline       = "online"

	// Payment statuses
	PaymentStatusUnpaid            = "unpaid"
	PaymentStatusPaid              = "paid"
	PaymentStatusRefunded          = "refunded"
	PaymentStatusPartiallyRefunded = "partially_refunded"
)

// SubOrderTerminalStatuses are states from which no further transitions are allowed
var SubOrderTerminalStatuses = map[string]bool{
	SubOrderStatusConfirmed: true,
	SubOrderStatusRejected:  true,
	SubOrderStatusReturned:  true,
	SubOrderStatusCancelled: true,
}

// SubOrderTransitions defines valid state transitions for sub-orders
// map[from][]to
var SubOrderTransitions = map[string][]string{
	SubOrderStatusPendingSupplier: {SubOrderStatusAccepted, SubOrderStatusRejected, SubOrderStatusCancelled},
	SubOrderStatusAccepted:        {SubOrderStatusPreparing, SubOrderStatusCancelled},
	SubOrderStatusPreparing:       {SubOrderStatusReadyForPickup, SubOrderStatusCancelled},
	SubOrderStatusReadyForPickup:  {SubOrderStatusPickedUp, SubOrderStatusCancelled},
	SubOrderStatusPickedUp:        {SubOrderStatusInTransit},
	SubOrderStatusInTransit:       {SubOrderStatusDelivered, SubOrderStatusFailedDelivery},
	SubOrderStatusFailedDelivery:  {SubOrderStatusInTransit, SubOrderStatusReturned},
	SubOrderStatusDelivered:       {SubOrderStatusConfirmed},
}

// CanTransitionTo returns true if the given status transition is valid
func CanTransitionTo(from, to string) bool {
	allowed, ok := SubOrderTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the sub-order status is a terminal state
func IsTerminalStatus(status string) bool {
	return SubOrderTerminalStatuses[status]
}

// ── Master Order ──────────────────────────────────────────────────────────────

type Order struct {
	ID                string     `db:"id"                 json:"id"`
	OrderNumber       string     `db:"order_number"       json:"order_number"`
	CustomerID        string     `db:"customer_id"        json:"customer_id"`
	DeliveryAddressID string     `db:"delivery_address_id" json:"delivery_address_id"`
	Status            string     `db:"status"             json:"status"`
	PaymentMethod     string     `db:"payment_method"     json:"payment_method"`
	PaymentStatus     string     `db:"payment_status"     json:"payment_status"`
	Subtotal          float64    `db:"subtotal"           json:"subtotal"`
	TotalDeliveryFee  float64    `db:"total_delivery_fee" json:"total_delivery_fee"`
	TotalCommission   float64    `db:"total_commission"   json:"total_commission"`
	GrandTotal        float64    `db:"grand_total"        json:"grand_total"`
	Notes             *string    `db:"notes"              json:"notes,omitempty"`
	PaidAt            *time.Time `db:"paid_at"            json:"paid_at,omitempty"`
	CreatedAt         time.Time  `db:"created_at"         json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at"         json:"updated_at"`

	// populated via JOIN
	SubOrders       []SubOrder       `db:"-" json:"sub_orders,omitempty"`
	DeliveryAddress *CustomerAddress `db:"-" json:"delivery_address,omitempty"`
}

// ── Sub-Order ─────────────────────────────────────────────────────────────────

type SubOrder struct {
	ID                string     `db:"id"                   json:"id"`
	SubOrderNumber    string     `db:"sub_order_number"     json:"sub_order_number"`
	OrderID           string     `db:"order_id"             json:"order_id"`
	SupplierID        string     `db:"supplier_id"          json:"supplier_id"`
	DeliveryCompanyID *string    `db:"delivery_company_id"  json:"delivery_company_id,omitempty"`
	Status            string     `db:"status"               json:"status"`
	DeliveryFee       float64    `db:"delivery_fee"         json:"delivery_fee"`
	Subtotal          float64    `db:"subtotal"             json:"subtotal"`
	CommissionRate    float64    `db:"commission_rate"      json:"commission_rate"`
	CommissionAmount  float64    `db:"commission_amount"    json:"commission_amount"`
	SupplierNet       float64    `db:"supplier_net"         json:"supplier_net"`
	TrackingNumber    *string    `db:"tracking_number"      json:"tracking_number,omitempty"`
	SupplierAcceptedAt *time.Time `db:"supplier_accepted_at" json:"supplier_accepted_at,omitempty"`
	ReadyAt           *time.Time `db:"ready_at"             json:"ready_at,omitempty"`
	PickedUpAt        *time.Time `db:"picked_up_at"         json:"picked_up_at,omitempty"`
	DeliveredAt       *time.Time `db:"delivered_at"         json:"delivered_at,omitempty"`
	RejectionReason   *string    `db:"rejection_reason"     json:"rejection_reason,omitempty"`
	DeliveryNotes     *string    `db:"delivery_notes"       json:"delivery_notes,omitempty"`
	AssignedBy        *string    `db:"assigned_by"          json:"assigned_by,omitempty"`
	CreatedAt         time.Time  `db:"created_at"           json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at"           json:"updated_at"`

	// populated via JOIN
	Items           []OrderItem        `db:"-" json:"items,omitempty"`
	StatusLogs      []OrderStatusLog   `db:"-" json:"status_logs,omitempty"`
	Supplier        *Supplier          `db:"-" json:"supplier,omitempty"`
	DeliveryCompany *DeliveryCompany   `db:"-" json:"delivery_company,omitempty"`
}

// IsAssigned returns true if a delivery company has been assigned
func (so *SubOrder) IsAssigned() bool {
	return so.DeliveryCompanyID != nil
}

// IsPendingDeliveryAssignment returns true if sub-order is ready but not yet assigned
func (so *SubOrder) IsPendingDeliveryAssignment() bool {
	return so.Status == SubOrderStatusReadyForPickup && so.DeliveryCompanyID == nil
}

// ── Order Item ────────────────────────────────────────────────────────────────

type OrderItem struct {
	ID                   string    `db:"id"                     json:"id"`
	SubOrderID           string    `db:"sub_order_id"           json:"sub_order_id"`
	OrderID              string    `db:"order_id"               json:"order_id"`
	ProductID            string    `db:"product_id"             json:"product_id"`
	SupplierID           string    `db:"supplier_id"            json:"supplier_id"`
	ProductNameSnapshot  string    `db:"product_name_snapshot"  json:"product_name_snapshot"`
	Quantity             int       `db:"quantity"               json:"quantity"`
	UnitPrice            float64   `db:"unit_price"             json:"unit_price"`
	IsWholesale          bool      `db:"is_wholesale"           json:"is_wholesale"`
	LineTotal            float64   `db:"line_total"             json:"line_total"`
	DiscountApplied      float64   `db:"discount_applied"       json:"discount_applied"`
	CreatedAt            time.Time `db:"created_at"             json:"created_at"`
	UpdatedAt            time.Time `db:"updated_at"             json:"updated_at"`
}

// ── Order Status Log ──────────────────────────────────────────────────────────

type OrderStatusLog struct {
	ID             string    `db:"id"              json:"id"`
	SubOrderID     string    `db:"sub_order_id"    json:"sub_order_id"`
	PreviousStatus string    `db:"previous_status" json:"previous_status"`
	NewStatus      string    `db:"new_status"      json:"new_status"`
	ChangedBy      string    `db:"changed_by"      json:"changed_by"`
	Note           *string   `db:"note"            json:"note,omitempty"`
	ChangedAt      time.Time `db:"changed_at"      json:"changed_at"`
}

// ── Financial Models ──────────────────────────────────────────────────────────

const (
	// Transaction types
	TxTypeOrderPayment    = "order_payment"
	TxTypeCommissionCharge = "commission_charge"
	TxTypeSupplierPayout  = "supplier_payout"
	TxTypeDeliveryPayout  = "delivery_payout"
	TxTypeRefund          = "refund"
	TxTypeAdjustment      = "adjustment"
	TxTypeWithdrawal      = "withdrawal_payment"

	// Transaction directions
	TxDirectionCredit = "credit"
	TxDirectionDebit  = "debit"

	// Transaction statuses
	TxStatusPending   = "pending"
	TxStatusCompleted = "completed"
	TxStatusFailed    = "failed"
	TxStatusReversed  = "reversed"

	// Order types for commission
	OrderTypeRetail    = "retail"
	OrderTypeWholesale = "wholesale"

	// Withdrawal statuses
	WithdrawalStatusPending  = "pending"
	WithdrawalStatusApproved = "approved"
	WithdrawalStatusRejected = "rejected"
	WithdrawalStatusPaid     = "paid"
)

type FinancialTransaction struct {
	ID                string    `db:"id"                json:"id"`
	TransactionNumber string    `db:"transaction_number" json:"transaction_number"`
	Type              string    `db:"type"              json:"type"`
	Direction         string    `db:"direction"         json:"direction"`
	Amount            float64   `db:"amount"            json:"amount"`
	Currency          string    `db:"currency"          json:"currency"`
	ReferenceType     *string   `db:"reference_type"    json:"reference_type,omitempty"`
	ReferenceID       *string   `db:"reference_id"      json:"reference_id,omitempty"`
	Description       *string   `db:"description"       json:"description,omitempty"`
	PerformedBy       *string   `db:"performed_by"      json:"performed_by,omitempty"`
	Status            string    `db:"status"            json:"status"`
	CreatedAt         time.Time `db:"created_at"        json:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"        json:"updated_at"`
}

type CommissionRecord struct {
	ID                    string     `db:"id"                       json:"id"`
	SubOrderID            string     `db:"sub_order_id"             json:"sub_order_id"`
	SupplierID            string     `db:"supplier_id"              json:"supplier_id"`
	OrderType             string     `db:"order_type"               json:"order_type"`
	GrossAmount           float64    `db:"gross_amount"             json:"gross_amount"`
	CommissionRate        float64    `db:"commission_rate"          json:"commission_rate"`
	CommissionAmount      float64    `db:"commission_amount"        json:"commission_amount"`
	PlatformFeeFromBuyer  float64    `db:"platform_fee_from_buyer"  json:"platform_fee_from_buyer"`
	NetSupplierAmount     float64    `db:"net_supplier_amount"      json:"net_supplier_amount"`
	Settled               bool       `db:"settled"                  json:"settled"`
	SettledAt             *time.Time `db:"settled_at"               json:"settled_at,omitempty"`
	CreatedAt             time.Time  `db:"created_at"               json:"created_at"`
	UpdatedAt             time.Time  `db:"updated_at"               json:"updated_at"`
}

type SupplierWallet struct {
	ID               string    `db:"id"                json:"id"`
	SupplierID       string    `db:"supplier_id"       json:"supplier_id"`
	BalancePending   float64   `db:"balance_pending"   json:"balance_pending"`
	BalanceAvailable float64   `db:"balance_available" json:"balance_available"`
	BalanceWithdrawn float64   `db:"balance_withdrawn" json:"balance_withdrawn"`
	LastUpdatedAt    time.Time `db:"last_updated_at"   json:"last_updated_at"`
	CreatedAt        time.Time `db:"created_at"        json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"        json:"updated_at"`
}

// TotalEarned returns total gross earnings (pending + available + withdrawn)
func (w *SupplierWallet) TotalEarned() float64 {
	return roundToTwo(w.BalancePending + w.BalanceAvailable + w.BalanceWithdrawn)
}

type WithdrawalRequest struct {
	ID              string     `db:"id"               json:"id"`
	SupplierID      string     `db:"supplier_id"      json:"supplier_id"`
	Amount          float64    `db:"amount"           json:"amount"`
	BankAccountID   string     `db:"bank_account_id"  json:"bank_account_id"`
	Status          string     `db:"status"           json:"status"`
	RequestedAt     time.Time  `db:"requested_at"     json:"requested_at"`
	ProcessedBy     *string    `db:"processed_by"     json:"processed_by,omitempty"`
	ProcessedAt     *time.Time `db:"processed_at"     json:"processed_at,omitempty"`
	RejectionReason *string    `db:"rejection_reason" json:"rejection_reason,omitempty"`
	ProofURL        *string    `db:"proof_url"        json:"proof_url,omitempty"`
	CreatedAt       time.Time  `db:"created_at"       json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"       json:"updated_at"`
}