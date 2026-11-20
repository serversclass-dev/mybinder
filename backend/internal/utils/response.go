package utils

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ── Standard Response Envelope ────────────────────────────────────────────────

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Meta    *Meta  `json:"meta,omitempty"`
}

type ErrorResponse struct {
	Success bool       `json:"success"`
	Error   ErrorBody  `json:"error"`
}

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Meta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// ── Response Helpers ──────────────────────────────────────────────────────────

// RespondOK sends a 200 success response
func RespondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Message: "OK",
		Data:    data,
	})
}

// RespondCreated sends a 201 created response
func RespondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Response{
		Success: true,
		Message: "Created",
		Data:    data,
	})
}

// RespondList sends a paginated list response
func RespondList(c *gin.Context, data any, total int64, page, perPage int) {
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	c.JSON(http.StatusOK, Response{
		Success: true,
		Message: "OK",
		Data:    data,
		Meta: &Meta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// RespondNoContent sends a 204 no content response
func RespondNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// RespondError sends a structured error response
func RespondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, ErrorResponse{
		Success: false,
		Error: ErrorBody{
			Code:    code,
			Message: message,
		},
	})
}

// RespondValidationError sends a 422 validation error with field details
func RespondValidationError(c *gin.Context, details map[string]any) {
	c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
		Success: false,
		Error: ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "Validation failed",
			Details: details,
		},
	})
}

// RespondNotFound sends a 404 response
func RespondNotFound(c *gin.Context, resource string) {
	RespondError(c, http.StatusNotFound, "NOT_FOUND", resource+" not found")
}

// RespondForbidden sends a 403 response
func RespondForbidden(c *gin.Context) {
	RespondError(c, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions")
}

// RespondUnauthorized sends a 401 response
func RespondUnauthorized(c *gin.Context) {
	RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
}

// RespondConflict sends a 409 response
func RespondConflict(c *gin.Context, message string) {
	RespondError(c, http.StatusConflict, "CONFLICT", message)
}

// RespondInternalError sends a 500 response
func RespondInternalError(c *gin.Context) {
	RespondError(c, http.StatusInternalServerError, "SERVER_ERROR", "An unexpected error occurred")
}

// ── Pagination ────────────────────────────────────────────────────────────────

type PaginationParams struct {
	Page    int
	PerPage int
	Offset  int
	Sort    string
	Order   string
}

const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

// ParsePagination extracts and validates pagination query params
func ParsePagination(c *gin.Context) PaginationParams {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	sort := c.DefaultQuery("sort", "created_at")
	order := c.DefaultQuery("order", "desc")

	if page < 1 {
		page = DefaultPage
	}
	if perPage < 1 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}

	// whitelist order direction
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	offset := (page - 1) * perPage

	return PaginationParams{
		Page:    page,
		PerPage: perPage,
		Offset:  offset,
		Sort:    sort,
		Order:   order,
	}
}

// ── UUID ──────────────────────────────────────────────────────────────────────

// NewUUID generates a new UUID v4 string
func NewUUID() string {
	return uuid.New().String()
}

// IsValidUUID returns true if the string is a valid UUID
func IsValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// ── String helpers ────────────────────────────────────────────────────────────

// StrPtr returns a pointer to a string — useful for optional fields
func StrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// StrVal safely dereferences a string pointer, returning "" if nil
func StrVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ── Numeric helpers ───────────────────────────────────────────────────────────

// RoundToTwo rounds a float to 2 decimal places
func RoundToTwo(v float64) float64 {
	return math.Round(v*100) / 100
}

// Float64Ptr returns a pointer to a float64
func Float64Ptr(f float64) *float64 {
	return &f
}

// IntPtr returns a pointer to an int
func IntPtr(i int) *int {
	return &i
}

// ── IP Address ────────────────────────────────────────────────────────────────

// GetClientIP extracts the real client IP from the request
func GetClientIP(c *gin.Context) string {
	// check X-Forwarded-For first (behind proxy/nginx)
	ip := c.GetHeader("X-Forwarded-For")
	if ip != "" {
		// X-Forwarded-For can be comma-separated list; take first
		parts := splitAndTrim(ip, ",")
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
	}

	ip = c.GetHeader("X-Real-IP")
	if ip != "" {
		return ip
	}

	return c.ClientIP()
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimString(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitString(s, sep string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			parts = append(parts, s[start:i])
			start = i + len(sep)
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimString(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// ── Order Number Generator ────────────────────────────────────────────────────

// These are convenience wrappers; actual sequence is DB-driven.
// The format is ORD-YYYYNNNNN where NNNNN is a zero-padded sequence.

// FormatOrderNumber formats a year + sequence into a master order number
func FormatOrderNumber(year, seq int) string {
	return "ORD-" + strconv.Itoa(year) + zeroPad(seq, 5)
}

// FormatSubOrderNumber appends a letter suffix to a master order number
func FormatSubOrderNumber(masterNumber string, index int) string {
	suffix := string(rune('A' + index))
	return masterNumber + "-" + suffix
}

// FormatComplaintNumber formats a complaint number
func FormatComplaintNumber(year, seq int) string {
	return "CMP-" + strconv.Itoa(year) + zeroPad(seq, 5)
}

// FormatTransactionNumber formats a transaction number
func FormatTransactionNumber(year, seq int) string {
	return "TXN-" + strconv.Itoa(year) + zeroPad(seq, 5)
}

func zeroPad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// ── Validation helpers ────────────────────────────────────────────────────────

// ValidationErrors collects field-level validation messages
type ValidationErrors map[string][]string

// Add appends a message for a field
func (ve ValidationErrors) Add(field, message string) {
	ve[field] = append(ve[field], message)
}

// HasErrors returns true if any errors were added
func (ve ValidationErrors) HasErrors() bool {
	return len(ve) > 0
}

// ToDetails converts to map[string]any for the error response
func (ve ValidationErrors) ToDetails() map[string]any {
	details := make(map[string]any, len(ve))
	for k, v := range ve {
		details[k] = v
	}
	return details
}