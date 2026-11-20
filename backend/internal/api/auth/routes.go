package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"souqiq/config"
	"souqiq/internal/middleware"
	"souqiq/internal/services"
	"souqiq/internal/utils"
)

type Handler struct {
	authSvc *services.AuthService
	cfg     *config.Config
}

func NewHandler(authSvc *services.AuthService, cfg *config.Config) *Handler {
	return &Handler{authSvc: authSvc, cfg: cfg}
}

// RegisterRoutes mounts all auth routes under /api/v1/auth
func RegisterRoutes(r *gin.RouterGroup, h *Handler, cfg *config.Config) {
	auth := r.Group("/auth")
	{
		auth.POST("/register/customer", h.RegisterCustomer)
		auth.POST("/register/supplier", h.RegisterSupplier)
		auth.POST("/register/delivery", h.RegisterDelivery)
		auth.POST("/verify-otp",        h.VerifyOTP)
		auth.POST("/login",             h.Login)
		auth.POST("/refresh",           h.Refresh)
		auth.POST("/forgot-password",   h.ForgotPassword)
		auth.POST("/reset-password",    h.ResetPassword)

		protected := auth.Group("")
		protected.Use(middleware.Authenticate(cfg))
		{
			protected.POST("/logout",          h.Logout)
			protected.POST("/change-password", h.ChangePassword)
		}
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (h *Handler) RegisterCustomer(c *gin.Context) {
	var input services.RegisterCustomerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}

	ve := utils.ValidationErrors{}
	if input.FullName == "" {
		ve.Add("full_name", "Full name is required")
	}
	if input.Email == "" {
		ve.Add("email", "Email is required")
	}
	if len(input.Password) < 8 {
		ve.Add("password", "Password must be at least 8 characters")
	}
	if input.CityID == "" {
		ve.Add("city_id", "City ID is required")
	}
	if ve.HasErrors() {
		utils.RespondValidationError(c, ve.ToDetails())
		return
	}

	ip := utils.GetClientIP(c)
	if err := h.authSvc.RegisterCustomer(c.Request.Context(), input, ip); err != nil {
		status := services.AuthHTTPStatus(err)
		utils.RespondError(c, status, errorCode(err), err.Error())
		return
	}

	utils.RespondCreated(c, gin.H{
		"message": "Account created successfully. Please log in.",
		"email":   input.Email,
	})
}

func (h *Handler) RegisterSupplier(c *gin.Context) {
	var input services.RegisterSupplierInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}

	ve := utils.ValidationErrors{}
	if input.BusinessName == "" {
		ve.Add("business_name", "Business name is required")
	}
	if input.BusinessNameAr == "" {
		ve.Add("business_name_ar", "Arabic business name is required")
	}
	if input.Email == "" {
		ve.Add("email", "Email is required")
	}
	if len(input.Password) < 8 {
		ve.Add("password", "Password must be at least 8 characters")
	}
	if input.CityID == "" {
		ve.Add("city_id", "City ID is required")
	}
	if ve.HasErrors() {
		utils.RespondValidationError(c, ve.ToDetails())
		return
	}

	ip := utils.GetClientIP(c)
	if err := h.authSvc.RegisterSupplier(c.Request.Context(), input, ip); err != nil {
		status := services.AuthHTTPStatus(err)
		utils.RespondError(c, status, errorCode(err), err.Error())
		return
	}

	utils.RespondCreated(c, gin.H{
		"message": "Supplier account created and pending admin review.",
		"email":   input.Email,
	})
}

func (h *Handler) RegisterDelivery(c *gin.Context) {
	var input services.RegisterDeliveryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}

	ve := utils.ValidationErrors{}
	if input.CompanyName == "" {
		ve.Add("company_name", "Company name is required")
	}
	if input.CompanyNameAr == "" {
		ve.Add("company_name_ar", "Arabic company name is required")
	}
	if input.Email == "" {
		ve.Add("email", "Email is required")
	}
	if len(input.Password) < 8 {
		ve.Add("password", "Password must be at least 8 characters")
	}
	if ve.HasErrors() {
		utils.RespondValidationError(c, ve.ToDetails())
		return
	}

	ip := utils.GetClientIP(c)
	if err := h.authSvc.RegisterDelivery(c.Request.Context(), input, ip); err != nil {
		status := services.AuthHTTPStatus(err)
		utils.RespondError(c, status, errorCode(err), err.Error())
		return
	}

	utils.RespondCreated(c, gin.H{
		"message": "Delivery company account created and pending admin review.",
		"email":   input.Email,
	})
}

func (h *Handler) VerifyOTP(c *gin.Context) {
	var input services.VerifyOTPInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}

	ve := utils.ValidationErrors{}
	if input.Phone == "" {
		ve.Add("phone", "Phone number is required")
	}
	if len(input.OTP) != 4 {
		ve.Add("otp", "OTP must be 4 digits")
	}
	if ve.HasErrors() {
		utils.RespondValidationError(c, ve.ToDetails())
		return
	}

	if err := h.authSvc.VerifyOTP(c.Request.Context(), input); err != nil {
		utils.RespondError(c, http.StatusUnprocessableEntity, "INVALID_OTP", err.Error())
		return
	}

	utils.RespondOK(c, gin.H{"message": "Phone number verified successfully"})
}

func (h *Handler) Login(c *gin.Context) {
	var input services.LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}

	ve := utils.ValidationErrors{}
	if input.Identifier == "" {
		ve.Add("identifier", "Phone or email is required")
	}
	if input.Password == "" {
		ve.Add("password", "Password is required")
	}
	if ve.HasErrors() {
		utils.RespondValidationError(c, ve.ToDetails())
		return
	}

	ip := utils.GetClientIP(c)
	userAgent := c.GetHeader("User-Agent")

	result, err := h.authSvc.Login(c.Request.Context(), input, ip, userAgent)
	if err != nil {
		status := services.AuthHTTPStatus(err)
		utils.RespondError(c, status, errorCode(err), err.Error())
		return
	}

	utils.RespondOK(c, result)
}

func (h *Handler) Refresh(c *gin.Context) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.RefreshToken == "" {
		utils.RespondValidationError(c, map[string]any{"refresh_token": "Refresh token is required"})
		return
	}

	ip := utils.GetClientIP(c)
	userAgent := c.GetHeader("User-Agent")

	result, err := h.authSvc.RefreshToken(c.Request.Context(), body.RefreshToken, ip, userAgent)
	if err != nil {
		utils.RespondError(c, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid or expired refresh token")
		return
	}

	utils.RespondOK(c, result)
}

func (h *Handler) Logout(c *gin.Context) {
	accessJTI, _ := middleware.GetJTI(c)

	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.ShouldBindJSON(&body)

	refreshJTI := ""
	if body.RefreshToken != "" {
		claims, err := middleware.ParseRefreshToken(body.RefreshToken, h.cfg)
		if err == nil {
			refreshJTI = claims.JTI
		}
	}

	_ = h.authSvc.Logout(c.Request.Context(), accessJTI, refreshJTI)
	utils.RespondOK(c, gin.H{"message": "Logged out successfully"})
}

func (h *Handler) ForgotPassword(c *gin.Context) {
	var body struct {
		Phone string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Phone == "" {
		utils.RespondValidationError(c, map[string]any{"phone": "Phone number is required"})
		return
	}

	_ = h.authSvc.ForgotPassword(c.Request.Context(), body.Phone)
	utils.RespondOK(c, gin.H{"message": "If this phone number is registered, an OTP has been sent."})
}

func (h *Handler) ResetPassword(c *gin.Context) {
	var input services.ResetPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}

	ve := utils.ValidationErrors{}
	if input.Phone == "" {
		ve.Add("phone", "Phone number is required")
	}
	if len(input.OTP) != 4 {
		ve.Add("otp", "OTP must be 4 digits")
	}
	if len(input.Password) < 8 {
		ve.Add("password", "Password must be at least 8 characters")
	}
	if ve.HasErrors() {
		utils.RespondValidationError(c, ve.ToDetails())
		return
	}

	if err := h.authSvc.ResetPassword(c.Request.Context(), input); err != nil {
		utils.RespondError(c, http.StatusUnprocessableEntity, "INVALID_OTP", err.Error())
		return
	}

	utils.RespondOK(c, gin.H{"message": "Password reset successfully"})
}

func (h *Handler) ChangePassword(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	if userID == "" {
		return
	}

	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondValidationError(c, map[string]any{"body": err.Error()})
		return
	}

	ve := utils.ValidationErrors{}
	if body.OldPassword == "" {
		ve.Add("old_password", "Current password is required")
	}
	if len(body.NewPassword) < 8 {
		ve.Add("new_password", "New password must be at least 8 characters")
	}
	if ve.HasErrors() {
		utils.RespondValidationError(c, ve.ToDetails())
		return
	}

	if err := h.authSvc.ChangePassword(c.Request.Context(), userID, body.OldPassword, body.NewPassword); err != nil {
		status := services.AuthHTTPStatus(err)
		utils.RespondError(c, status, errorCode(err), err.Error())
		return
	}

	utils.RespondOK(c, gin.H{"message": "Password changed successfully"})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func errorCode(err error) string {
	switch err {
	case services.ErrInvalidCredentials:
		return "INVALID_CREDENTIALS"
	case services.ErrAccountInactive:
		return "ACCOUNT_INACTIVE"
	case services.ErrAccountUnverified:
		return "ACCOUNT_UNVERIFIED"
	case services.ErrPhoneExists:
		return "PHONE_EXISTS"
	case services.ErrEmailExists:
		return "EMAIL_EXISTS"
	case services.ErrInvalidOTP:
		return "INVALID_OTP"
	case services.ErrTokenInvalid:
		return "INVALID_TOKEN"
	case services.ErrUserNotFound:
		return "NOT_FOUND"
	default:
		return "SERVER_ERROR"
	}
}