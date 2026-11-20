package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
	"souqiq/config"

	"souqiq/internal/middleware"
	"souqiq/internal/models"
	"souqiq/internal/utils"
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountInactive    = errors.New("account is inactive or suspended")
	ErrAccountUnverified  = errors.New("account phone not verified")
	ErrPhoneExists        = errors.New("phone number already registered")
	ErrEmailExists        = errors.New("email address already registered")
	ErrInvalidOTP         = errors.New("invalid or expired OTP")
	ErrTokenInvalid       = errors.New("invalid or expired token")
)

// ── DTOs ──────────────────────────────────────────────────────────────────────

type RegisterCustomerInput struct {
	FullName string  `json:"full_name"  validate:"required,min=2,max=100"`
	Phone    *string `json:"phone"`
	Email    string  `json:"email"      validate:"required"`
	Password string  `json:"password"   validate:"required,min=8"`
	CityID   string  `json:"city_id"    validate:"required,uuid"`
}

type RegisterSupplierInput struct {
	BusinessName   string  `json:"business_name"    validate:"required,min=2,max=255"`
	BusinessNameAr string  `json:"business_name_ar" validate:"required,min=2,max=255"`
	Phone          *string `json:"phone"`
	Email          string  `json:"email"            validate:"required"`
	Password       string  `json:"password"         validate:"required,min=8"`
	CityID         string  `json:"city_id"          validate:"required,uuid"`
	CommercialReg  *string `json:"commercial_register"`
}

type RegisterDeliveryInput struct {
	CompanyName   string  `json:"company_name"    validate:"required,min=2,max=255"`
	CompanyNameAr string  `json:"company_name_ar" validate:"required,min=2,max=255"`
	Phone         *string `json:"phone"`
	Email         string  `json:"email"           validate:"required"`
	Password      string  `json:"password"        validate:"required,min=8"`
}

type LoginInput struct {
	Identifier string `json:"identifier" validate:"required"` // email (phone optional in future)
	Password   string `json:"password"   validate:"required"`
}

type AuthResult struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int         `json:"expires_in"`
	User         *UserPublic `json:"user"`
}

type UserPublic struct {
	ID         string  `json:"id"`
	Role       string  `json:"role"`
	Phone      string  `json:"phone"`
	Email      *string `json:"email,omitempty"`
	FullName   string  `json:"full_name,omitempty"`
	IsVerified bool    `json:"is_verified"`
}

type VerifyOTPInput struct {
	Phone string `json:"phone" validate:"required"`
	OTP   string `json:"otp"   validate:"required,len=4"`
}

type ResetPasswordInput struct {
	Phone    string `json:"phone"    validate:"required"`
	OTP      string `json:"otp"      validate:"required,len=4"`
	Password string `json:"password" validate:"required,min=8"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type AuthService struct {
	db  *sqlx.DB
	cfg *config.Config
}

func NewAuthService(db *sqlx.DB, cfg *config.Config) *AuthService {
	return &AuthService{db: db, cfg: cfg}
}

// ── Registration ──────────────────────────────────────────────────────────────

func (s *AuthService) RegisterCustomer(ctx context.Context, input RegisterCustomerInput, ip string) error {
	if err := s.checkEmailUnique(ctx, input.Email); err != nil {
		return err
	}

	hash, err := hashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	userID := utils.NewUUID()
	customerID := utils.NewUUID()
	now := time.Now()

	return withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, phone, email, password_hash, role, is_active, is_verified, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, 1, ?, ?)`,
			userID, input.Phone, input.Email, hash,
			models.RoleCustomer, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert user: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO customers (id, user_id, full_name, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)`,
			customerID, userID, input.FullName, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert customer: %w", err)
		}
		return nil
	})
}

func (s *AuthService) RegisterSupplier(ctx context.Context, input RegisterSupplierInput, ip string) error {
	if err := s.checkEmailUnique(ctx, input.Email); err != nil {
		return err
	}

	hash, err := hashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	userID := utils.NewUUID()
	supplierID := utils.NewUUID()
	walletID := utils.NewUUID()
	now := time.Now()

	return withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, phone, email, password_hash, role, is_active, is_verified, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, 1, ?, ?)`,
			userID, input.Phone, input.Email, hash,
			models.RoleSupplier, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert user: %w", err)
		}

	_, err = tx.ExecContext(ctx, `
			INSERT INTO suppliers (id, user_id, business_name, business_name_ar, commercial_register, city_id, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			supplierID, userID,
			input.BusinessName, input.BusinessNameAr,
			input.CommercialReg, input.CityID,
			models.SupplierStatusPending, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert supplier: %w", err)
		}

		// create empty wallet
		_, err = tx.ExecContext(ctx, `
			INSERT INTO supplier_wallets (id, supplier_id, balance_pending, balance_available, balance_withdrawn, last_updated_at, created_at, updated_at)
			VALUES (?, ?, 0, 0, 0, ?, ?, ?)`,
			walletID, supplierID, now, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert wallet: %w", err)
		}
		return nil
	})
}

func (s *AuthService) RegisterDelivery(ctx context.Context, input RegisterDeliveryInput, ip string) error {
	if err := s.checkEmailUnique(ctx, input.Email); err != nil {
		return err
	}

	hash, err := hashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	userID := utils.NewUUID()
	companyID := utils.NewUUID()
	now := time.Now()

	return withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, phone, email, password_hash, role, is_active, is_verified, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, 1, ?, ?)`,
			userID, input.Phone, input.Email, hash,
			models.RoleDelivery, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert user: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO delivery_companies (id, user_id, company_name, company_name_ar, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			companyID, userID,
			input.CompanyName, input.CompanyNameAr,
			models.DeliveryStatusPending, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert delivery company: %w", err)
		}
		return nil
	})
}

// ── OTP ───────────────────────────────────────────────────────────────────────

func (s *AuthService) SendOTP(ctx context.Context, identifier string) error {
	// placeholder — OTP via phone disabled, email verification used instead
	return nil
}

func (s *AuthService) VerifyOTP(ctx context.Context, input VerifyOTPInput) error {
	// OTP via phone disabled — users are auto-verified on registration
	return nil
}

// ── Login ─────────────────────────────────────────────────────────────────────

func (s *AuthService) Login(ctx context.Context, input LoginInput, ip, userAgent string) (*AuthResult, error) {
	user, err := s.findUserByIdentifier(ctx, input.Identifier)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.IsActive {
		return nil, ErrAccountInactive
	}
	if !user.IsVerified {
		return nil, ErrAccountUnverified
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueTokenPair(ctx, user, ip, userAgent)
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenStr, ip, userAgent string) (*AuthResult, error) {
	claims, err := middleware.ParseRefreshToken(refreshTokenStr, s.cfg)
	if err != nil {
		return nil, ErrTokenInvalid
	}

	// check refresh token session is still valid
	valid, err := middleware.IsSessionValid(ctx, claims.JTI)
	if err != nil || !valid {
		return nil, ErrTokenInvalid
	}

	// revoke old refresh token
	_ = middleware.RevokeSession(ctx, claims.JTI)

	user, err := s.findUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	if !user.IsActive {
		return nil, ErrAccountInactive
	}

	return s.issueTokenPair(ctx, user, ip, userAgent)
}

// ── Logout ────────────────────────────────────────────────────────────────────

func (s *AuthService) Logout(ctx context.Context, accessJTI, refreshJTI string) error {
	_ = middleware.RevokeSession(ctx, accessJTI)
	if refreshJTI != "" {
		_ = middleware.RevokeSession(ctx, refreshJTI)
	}
	return nil
}

// ── Password Reset ────────────────────────────────────────────────────────────

func (s *AuthService) ForgotPassword(ctx context.Context, email string) error {
	// TODO: send password reset link via email
	fmt.Printf("[auth] password reset requested for: %s\n", email)
	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, input ResetPasswordInput) error {
	// For now reset by email+OTP — OTP check skipped (phone disabled)
	hash, err := hashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE email = ?`,
		hash, time.Now(), input.Phone, // input.Phone holds the email identifier here
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// ── Change Password ───────────────────────────────────────────────────────────

func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	user, err := s.findUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		hash, time.Now(), userID,
	)
	return err
}

// ── HTTP Status mapper ────────────────────────────────────────────────────────

// HTTPStatus maps service errors to HTTP status codes
func AuthHTTPStatus(err error) int {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, ErrAccountInactive):
		return http.StatusForbidden
	case errors.Is(err, ErrAccountUnverified):
		return http.StatusForbidden
	case errors.Is(err, ErrPhoneExists), errors.Is(err, ErrEmailExists):
		return http.StatusConflict
	case errors.Is(err, ErrInvalidOTP):
		return http.StatusUnprocessableEntity
	case errors.Is(err, ErrTokenInvalid):
		return http.StatusUnauthorized
	case errors.Is(err, ErrUserNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (s *AuthService) issueTokenPair(ctx context.Context, user *models.User, ip, userAgent string) (*AuthResult, error) {
	accessToken, accessJTI, err := middleware.GenerateAccessToken(user.ID, user.Role, s.cfg)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, refreshJTI, err := middleware.GenerateRefreshToken(user.ID, user.Role, s.cfg)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// store both JTIs in Redis (non-fatal if Redis is unavailable in dev mode)
	_ = middleware.StoreSession(ctx, accessJTI, user.ID, s.cfg.JWT.AccessTokenExpiry)
	_ = middleware.StoreSession(ctx, refreshJTI, user.ID, s.cfg.JWT.RefreshTokenExpiry)

	// update last login
	_, _ = s.db.ExecContext(ctx,
		`UPDATE users SET last_login_at = ? WHERE id = ?`,
		time.Now(), user.ID,
	)

	pub := &UserPublic{
		ID:         user.ID,
		Role:       user.Role,
		Phone:      user.Phone,
		Email:      user.Email,
		IsVerified: user.IsVerified,
	}

	// attach profile name based on role
	s.attachProfileName(ctx, pub)

	return &AuthResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.cfg.JWT.AccessTokenExpiry.Seconds()),
		User:         pub,
	}, nil
}

func (s *AuthService) attachProfileName(ctx context.Context, pub *UserPublic) {
	switch pub.Role {
	case models.RoleCustomer:
		var name string
		_ = s.db.QueryRowContext(ctx,
			`SELECT full_name FROM customers WHERE user_id = ?`, pub.ID,
		).Scan(&name)
		pub.FullName = name
	case models.RoleSupplier:
		var name string
		_ = s.db.QueryRowContext(ctx,
			`SELECT business_name FROM suppliers WHERE user_id = ?`, pub.ID,
		).Scan(&name)
		pub.FullName = name
	case models.RoleDelivery:
		var name string
		_ = s.db.QueryRowContext(ctx,
			`SELECT company_name FROM delivery_companies WHERE user_id = ?`, pub.ID,
		).Scan(&name)
		pub.FullName = name
	}
}

func (s *AuthService) findUserByIdentifier(ctx context.Context, identifier string) (*models.User, error) {
	var user models.User
	err := s.db.GetContext(ctx, &user,
		`SELECT id, COALESCE(phone,'') as phone, email, password_hash, role, is_active, is_verified, last_login_at, created_at, updated_at
		 FROM users
		 WHERE email = ?`,
		identifier,
	)
	if err != nil {
		log.Printf("[auth] findUserByIdentifier error for %q: %v", identifier, err)
		return nil, ErrUserNotFound
	}
	log.Printf("[auth] found user id=%s role=%s is_active=%v is_verified=%v hash_prefix=%s",
		user.ID, user.Role, user.IsActive, user.IsVerified, user.PasswordHash[:10])
	return &user, nil
}

func (s *AuthService) findUserByID(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	err := s.db.GetContext(ctx, &user,
		`SELECT id, phone, email, password_hash, role, is_active, is_verified, last_login_at, created_at, updated_at
		 FROM users WHERE id = ?`,
		id,
	)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return &user, nil
}

func (s *AuthService) checkEmailUnique(ctx context.Context, email string) error {
	var count int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM users WHERE email = ?`, email,
	).Scan(&count)
	if count > 0 {
		return ErrEmailExists
	}
	return nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func generateOTP() string {
	//nolint:gosec // OTP does not need crypto/rand
	n := rand.Intn(9000) + 1000
	return fmt.Sprintf("%04d", n)
}

// withTx is a local transaction helper
func withTx(ctx context.Context, db *sqlx.DB, fn func(tx *sqlx.Tx) error) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}