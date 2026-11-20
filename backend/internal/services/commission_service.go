package services

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"souqiq/internal/models"
	"souqiq/internal/utils"
)

// CommissionService handles rate resolution, commission calculation,
// wallet crediting, and settlement operations.
type CommissionService struct {
	db *sqlx.DB
}

func NewCommissionService(db *sqlx.DB) *CommissionService {
	return &CommissionService{db: db}
}

// ── Rate Resolution ───────────────────────────────────────────────────────────

// ResolveRate returns the commission rate for a given product + supplier + order type.
// Priority: product override → supplier override → platform default
func (s *CommissionService) ResolveRate(ctx context.Context, productID, supplierID string, isWholesale bool) float64 {
	// 1. product-level override
	var productRate *float64
	_ = s.db.QueryRowContext(ctx,
		`SELECT commission_rate_override FROM products WHERE id = @p1`,
		productID,
	).Scan(&productRate)
	if productRate != nil {
		return *productRate
	}

	// 2. supplier-level override
	var supplierRate *float64
	_ = s.db.QueryRowContext(ctx,
		`SELECT commission_rate_override FROM suppliers WHERE id = @p1`,
		supplierID,
	).Scan(&supplierRate)
	if supplierRate != nil {
		return *supplierRate
	}

	// 3. platform default
	return s.platformRate(ctx, isWholesale)
}

func (s *CommissionService) platformRate(ctx context.Context, isWholesale bool) float64 {
	key := models.SettingDefaultCommissionRate
	if isWholesale {
		key = models.SettingWholesaleCommissionRate
	}
	var val float64
	_ = s.db.QueryRowContext(ctx,
		`SELECT CAST(value AS FLOAT) FROM platform_settings WHERE [key] = @p1`, key,
	).Scan(&val)
	if val == 0 {
		if isWholesale {
			return 3.0
		}
		return 5.0
	}
	return val
}

func (s *CommissionService) wholesaleBuyerFeeRate(ctx context.Context) float64 {
	var val float64
	_ = s.db.QueryRowContext(ctx,
		`SELECT CAST(value AS FLOAT) FROM platform_settings WHERE [key] = @p1`,
		models.SettingWholesaleBuyerFeeRate,
	).Scan(&val)
	return val
}

// ── Calculate & Credit ────────────────────────────────────────────────────────

// CalculateAndCredit computes commission for a confirmed sub-order
// and credits the supplier's pending wallet balance.
// Called after sub-order reaches "confirmed" status.
func (s *CommissionService) CalculateAndCredit(ctx context.Context, subOrderID string) error {
	// load sub-order
	var sub models.SubOrder
	err := s.db.GetContext(ctx, &sub,
		`SELECT * FROM sub_orders WHERE id = @p1`, subOrderID,
	)
	if err != nil {
		return fmt.Errorf("load sub-order: %w", err)
	}

	// load order items
	var items []models.OrderItem
	if err := s.db.SelectContext(ctx, &items,
		`SELECT * FROM order_items WHERE sub_order_id = @p1`, subOrderID,
	); err != nil {
		return fmt.Errorf("load order items: %w", err)
	}

	// check if commission record already exists (idempotent)
	var existingCount int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM commission_records WHERE sub_order_id = @p1`, subOrderID,
	).Scan(&existingCount)
	if existingCount > 0 {
		return nil // already processed
	}

	// determine order type (retail or wholesale) from items
	orderType := models.OrderTypeRetail
	for _, item := range items {
		if item.IsWholesale {
			orderType = models.OrderTypeWholesale
			break
		}
	}

	// resolve rate
	rate := s.ResolveRate(ctx, items[0].ProductID, sub.SupplierID, orderType == models.OrderTypeWholesale)
	buyerFeeRate := 0.0
	if orderType == models.OrderTypeWholesale {
		buyerFeeRate = s.wholesaleBuyerFeeRate(ctx)
	}

	grossAmount := sub.Subtotal
	commissionAmount := utils.RoundToTwo(grossAmount * rate / 100)
	platformFeeFromBuyer := utils.RoundToTwo(grossAmount * buyerFeeRate / 100)
	netSupplierAmount := utils.RoundToTwo(grossAmount - commissionAmount)

	now := time.Now()
	recordID := utils.NewUUID()
	txnID := utils.NewUUID()
	txnNumber, _ := s.nextTransactionNumber(ctx)

	return withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		// insert commission record
		_, err := tx.ExecContext(ctx, `
			INSERT INTO commission_records (
				id, sub_order_id, supplier_id, order_type,
				gross_amount, commission_rate, commission_amount,
				platform_fee_from_buyer, net_supplier_amount,
				settled, created_at, updated_at
			) VALUES (
				@p1, @p2, @p3, @p4,
				@p5, @p6, @p7,
				@p8, @p9,
				0, @p10, @p11
			)`,
			recordID, subOrderID, sub.SupplierID, orderType,
			grossAmount, rate, commissionAmount,
			platformFeeFromBuyer, netSupplierAmount,
			now, now,
		)
		if err != nil {
			return fmt.Errorf("insert commission record: %w", err)
		}

		// record financial transaction (commission charge)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO financial_transactions (
				id, transaction_number, type, direction, amount, currency,
				reference_type, reference_id, description, status,
				created_at, updated_at
			) VALUES (
				@p1, @p2, @p3, @p4, @p5, @p6,
				@p7, @p8, @p9, @p10,
				@p11, @p12
			)`,
			txnID, txnNumber,
			models.TxTypeCommissionCharge, models.TxDirectionCredit,
			commissionAmount, "LYD",
			"sub_order", subOrderID,
			fmt.Sprintf("Commission on sub-order %s", sub.SubOrderNumber),
			models.TxStatusCompleted,
			now, now,
		)
		if err != nil {
			return fmt.Errorf("insert commission transaction: %w", err)
		}

		// credit supplier pending wallet
		_, err = tx.ExecContext(ctx, `
			UPDATE supplier_wallets
			SET balance_pending = balance_pending + @p1,
			    last_updated_at = @p2,
			    updated_at = @p2
			WHERE supplier_id = @p3`,
			netSupplierAmount, now, sub.SupplierID,
		)
		if err != nil {
			return fmt.Errorf("credit supplier wallet: %w", err)
		}

		return nil
	})
}

// ── Settlement ────────────────────────────────────────────────────────────────

// SettleSupplier moves a supplier's pending balance to available balance
// for a batch of unsettled commission records.
func (s *CommissionService) SettleSupplier(ctx context.Context, supplierID, adminID string) (float64, error) {
	// load unsettled commissions
	var records []models.CommissionRecord
	err := s.db.SelectContext(ctx, &records, `
		SELECT * FROM commission_records
		WHERE supplier_id = @p1 AND settled = 0`,
		supplierID,
	)
	if err != nil {
		return 0, fmt.Errorf("load commissions: %w", err)
	}
	if len(records) == 0 {
		return 0, nil
	}

	var totalNet float64
	for _, r := range records {
		totalNet += r.NetSupplierAmount
	}
	totalNet = utils.RoundToTwo(totalNet)

	now := time.Now()

	return totalNet, withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		// mark all records as settled
		_, err := tx.ExecContext(ctx, `
			UPDATE commission_records
			SET settled = 1, settled_at = @p1, updated_at = @p1
			WHERE supplier_id = @p2 AND settled = 0`,
			now, supplierID,
		)
		if err != nil {
			return fmt.Errorf("settle commission records: %w", err)
		}

		// move pending → available in wallet
		_, err = tx.ExecContext(ctx, `
			UPDATE supplier_wallets
			SET balance_pending   = balance_pending - @p1,
			    balance_available = balance_available + @p1,
			    last_updated_at   = @p2,
			    updated_at        = @p2
			WHERE supplier_id = @p3`,
			totalNet, now, supplierID,
		)
		if err != nil {
			return fmt.Errorf("update wallet settlement: %w", err)
		}

		// record financial transaction
		txnNumber, _ := s.nextTransactionNumber(ctx)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO financial_transactions (
				id, transaction_number, type, direction, amount, currency,
				reference_type, reference_id, description, performed_by, status,
				created_at, updated_at
			) VALUES (
				@p1, @p2, @p3, @p4, @p5, @p6,
				@p7, @p8, @p9, @p10, @p11,
				@p12, @p13
			)`,
			utils.NewUUID(), txnNumber,
			"settlement", models.TxDirectionCredit,
			totalNet, "LYD",
			"supplier", supplierID,
			fmt.Sprintf("Settlement batch for supplier %s", supplierID),
			adminID, models.TxStatusCompleted,
			now, now,
		)
		return err
	})
}

// ── Withdrawal ────────────────────────────────────────────────────────────────

// RequestWithdrawal creates a withdrawal request if funds are available.
func (s *CommissionService) RequestWithdrawal(ctx context.Context, supplierID, bankAccountID string, amount float64) (*models.WithdrawalRequest, error) {
	// validate minimum amount
	minAmount := s.minWithdrawalAmount(ctx)
	if amount < minAmount {
		return nil, fmt.Errorf("minimum withdrawal amount is %.2f LYD", minAmount)
	}

	// check available balance
	var available float64
	err := s.db.QueryRowContext(ctx,
		`SELECT balance_available FROM supplier_wallets WHERE supplier_id = @p1`, supplierID,
	).Scan(&available)
	if err != nil {
		return nil, fmt.Errorf("load wallet: %w", err)
	}
	if amount > available {
		return nil, fmt.Errorf("insufficient available balance (%.2f LYD)", available)
	}

	// check no pending withdrawal exists
	var pending int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM withdrawal_requests WHERE supplier_id = @p1 AND status = 'pending'`,
		supplierID,
	).Scan(&pending)
	if pending > 0 {
		return nil, fmt.Errorf("a withdrawal request is already pending")
	}

	wr := &models.WithdrawalRequest{
		ID:            utils.NewUUID(),
		SupplierID:    supplierID,
		Amount:        amount,
		BankAccountID: bankAccountID,
		Status:        models.WithdrawalStatusPending,
		RequestedAt:   time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO withdrawal_requests (
			id, supplier_id, amount, bank_account_id, status,
			requested_at, created_at, updated_at
		) VALUES (
			@p1, @p2, @p3, @p4, @p5,
			@p6, @p7, @p8
		)`,
		wr.ID, wr.SupplierID, wr.Amount, wr.BankAccountID, wr.Status,
		wr.RequestedAt, wr.CreatedAt, wr.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert withdrawal request: %w", err)
	}
	return wr, nil
}

// ApproveWithdrawal approves a withdrawal request (admin action).
func (s *CommissionService) ApproveWithdrawal(ctx context.Context, requestID, adminID string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE withdrawal_requests
		SET status = @p1, processed_by = @p2, processed_at = @p3, updated_at = @p3
		WHERE id = @p4 AND status = 'pending'`,
		models.WithdrawalStatusApproved, adminID, now, requestID,
	)
	return err
}

// RejectWithdrawal rejects a withdrawal request (admin action).
func (s *CommissionService) RejectWithdrawal(ctx context.Context, requestID, adminID, reason string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE withdrawal_requests
		SET status = @p1, processed_by = @p2, processed_at = @p3,
		    rejection_reason = @p4, updated_at = @p3
		WHERE id = @p5 AND status = 'pending'`,
		models.WithdrawalStatusRejected, adminID, now, reason, requestID,
	)
	return err
}

// MarkWithdrawalPaid marks a withdrawal as paid and deducts from wallet.
func (s *CommissionService) MarkWithdrawalPaid(ctx context.Context, requestID, adminID, proofURL string) error {
	var wr models.WithdrawalRequest
	err := s.db.GetContext(ctx, &wr,
		`SELECT * FROM withdrawal_requests WHERE id = @p1`, requestID,
	)
	if err != nil {
		return fmt.Errorf("load withdrawal request: %w", err)
	}
	if wr.Status != models.WithdrawalStatusApproved {
		return fmt.Errorf("withdrawal must be approved before marking paid")
	}

	now := time.Now()
	txnNumber, _ := s.nextTransactionNumber(ctx)

	return withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		// update withdrawal request
		_, err := tx.ExecContext(ctx, `
			UPDATE withdrawal_requests
			SET status = @p1, proof_url = @p2, processed_by = @p3,
			    processed_at = @p4, updated_at = @p4
			WHERE id = @p5`,
			models.WithdrawalStatusPaid, proofURL, adminID, now, requestID,
		)
		if err != nil {
			return fmt.Errorf("update withdrawal: %w", err)
		}

		// deduct from available balance
		_, err = tx.ExecContext(ctx, `
			UPDATE supplier_wallets
			SET balance_available = balance_available - @p1,
			    balance_withdrawn = balance_withdrawn + @p1,
			    last_updated_at   = @p2,
			    updated_at        = @p2
			WHERE supplier_id = @p3`,
			wr.Amount, now, wr.SupplierID,
		)
		if err != nil {
			return fmt.Errorf("deduct wallet: %w", err)
		}

		// record financial transaction
		_, err = tx.ExecContext(ctx, `
			INSERT INTO financial_transactions (
				id, transaction_number, type, direction, amount, currency,
				reference_type, reference_id, description, performed_by, status,
				created_at, updated_at
			) VALUES (
				@p1, @p2, @p3, @p4, @p5, @p6,
				@p7, @p8, @p9, @p10, @p11,
				@p12, @p13
			)`,
			utils.NewUUID(), txnNumber,
			models.TxTypeWithdrawal, models.TxDirectionDebit,
			wr.Amount, "LYD",
			"withdrawal_request", requestID,
			fmt.Sprintf("Withdrawal payout to supplier %s", wr.SupplierID),
			adminID, models.TxStatusCompleted,
			now, now,
		)
		return err
	})
}

// ── Refund Commission Reversal ────────────────────────────────────────────────

// ReverseCommission voids a commission record and adjusts the supplier wallet.
// Called when a sub-order is refunded/returned.
func (s *CommissionService) ReverseCommission(ctx context.Context, subOrderID string) error {
	var record models.CommissionRecord
	err := s.db.GetContext(ctx, &record,
		`SELECT * FROM commission_records WHERE sub_order_id = @p1`, subOrderID,
	)
	if err != nil {
		return nil // no commission record exists — nothing to reverse
	}

	now := time.Now()
	txnNumber, _ := s.nextTransactionNumber(ctx)

	return withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		// void commission record
		_, err := tx.ExecContext(ctx, `
			UPDATE commission_records
			SET settled = 0, settled_at = NULL, updated_at = @p1
			WHERE sub_order_id = @p2`,
			now, subOrderID,
		)
		if err != nil {
			return fmt.Errorf("void commission record: %w", err)
		}

		// reverse wallet credit
		// if settled → deduct from available; if pending → deduct from pending
		if record.Settled {
			_, err = tx.ExecContext(ctx, `
				UPDATE supplier_wallets
				SET balance_available = balance_available - @p1,
				    last_updated_at = @p2, updated_at = @p2
				WHERE supplier_id = @p3`,
				record.NetSupplierAmount, now, record.SupplierID,
			)
		} else {
			_, err = tx.ExecContext(ctx, `
				UPDATE supplier_wallets
				SET balance_pending = balance_pending - @p1,
				    last_updated_at = @p2, updated_at = @p2
				WHERE supplier_id = @p3`,
				record.NetSupplierAmount, now, record.SupplierID,
			)
		}
		if err != nil {
			return fmt.Errorf("reverse wallet: %w", err)
		}

		// record reversal transaction
		_, err = tx.ExecContext(ctx, `
			INSERT INTO financial_transactions (
				id, transaction_number, type, direction, amount, currency,
				reference_type, reference_id, description, status,
				created_at, updated_at
			) VALUES (
				@p1, @p2, @p3, @p4, @p5, @p6,
				@p7, @p8, @p9, @p10,
				@p11, @p12
			)`,
			utils.NewUUID(), txnNumber,
			models.TxTypeAdjustment, models.TxDirectionDebit,
			record.NetSupplierAmount, "LYD",
			"sub_order", subOrderID,
			fmt.Sprintf("Commission reversal for sub-order %s", subOrderID),
			models.TxStatusCompleted,
			now, now,
		)
		return err
	})
}

// ── Wallet Read ───────────────────────────────────────────────────────────────

// GetWallet returns the supplier's current wallet state.
func (s *CommissionService) GetWallet(ctx context.Context, supplierID string) (*models.SupplierWallet, error) {
	var w models.SupplierWallet
	err := s.db.GetContext(ctx, &w,
		`SELECT * FROM supplier_wallets WHERE supplier_id = @p1`, supplierID,
	)
	if err != nil {
		return nil, fmt.Errorf("load wallet: %w", err)
	}
	return &w, nil
}

// GetCommissionRecords returns paginated commission records for a supplier.
func (s *CommissionService) GetCommissionRecords(ctx context.Context, supplierID string, settled *bool, pagination utils.PaginationParams) ([]models.CommissionRecord, int64, error) {
	where := "supplier_id = @p1"
	args := []any{supplierID}
	idx := 2

	if settled != nil {
		settledVal := 0
		if *settled {
			settledVal = 1
		}
		where += fmt.Sprintf(" AND settled = @p%d", idx)
		args = append(args, settledVal)
		idx++
	}

	var total int64
	_ = s.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(1) FROM commission_records WHERE %s`, where), args...,
	).Scan(&total)

	args = append(args, pagination.Offset, pagination.PerPage)
	var records []models.CommissionRecord
	err := s.db.SelectContext(ctx, &records, fmt.Sprintf(`
		SELECT * FROM commission_records WHERE %s
		ORDER BY created_at DESC
		OFFSET @p%d ROWS FETCH NEXT @p%d ROWS ONLY`,
		where, idx, idx+1,
	), args...)
	if err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (s *CommissionService) minWithdrawalAmount(ctx context.Context) float64 {
	var val float64
	_ = s.db.QueryRowContext(ctx,
		`SELECT CAST(value AS FLOAT) FROM platform_settings WHERE [key] = @p1`,
		models.SettingMinWithdrawalAmount,
	).Scan(&val)
	if val == 0 {
		return 50.0
	}
	return val
}

func (s *CommissionService) nextTransactionNumber(ctx context.Context) (string, error) {
	year := time.Now().Year()
	var seq int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) + 1 FROM financial_transactions
		WHERE YEAR(created_at) = @p1`, year,
	).Scan(&seq)
	if err != nil {
		return utils.FormatTransactionNumber(year, 1), nil
	}
	return utils.FormatTransactionNumber(year, seq), nil
}