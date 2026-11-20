package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"souqiq/config"
	"souqiq/internal/cache"
	"souqiq/internal/utils"
)

// Claims represents the JWT payload
type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	JTI    string `json:"jti"` // unique token ID for revocation
	jwt.RegisteredClaims
}

// ctxKey is an unexported type for context keys to avoid collisions
type ctxKey string

const (
	CtxUserID = ctxKey("user_id")
	CtxRole   = ctxKey("role")
	CtxJTI    = ctxKey("jti")
)

// ── Token Generation ──────────────────────────────────────────────────────────

// GenerateAccessToken creates a signed JWT access token
func GenerateAccessToken(userID, role string, cfg *config.Config) (string, string, error) {
	jti := utils.NewUUID()
	now := time.Now()

	claims := Claims{
		UserID: userID,
		Role:   role,
		JTI:    jti,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.JWT.AccessTokenExpiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(cfg.JWT.AccessSecret))
	if err != nil {
		return "", "", err
	}
	return signed, jti, nil
}

// GenerateRefreshToken creates a signed JWT refresh token
func GenerateRefreshToken(userID, role string, cfg *config.Config) (string, string, error) {
	jti := utils.NewUUID()
	now := time.Now()

	claims := Claims{
		UserID: userID,
		Role:   role,
		JTI:    jti,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.JWT.RefreshTokenExpiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(cfg.JWT.RefreshSecret))
	if err != nil {
		return "", "", err
	}
	return signed, jti, nil
}

// ParseAccessToken validates and parses an access token string
func ParseAccessToken(tokenStr string, cfg *config.Config) (*Claims, error) {
	return parseToken(tokenStr, cfg.JWT.AccessSecret)
}

// ParseRefreshToken validates and parses a refresh token string
func ParseRefreshToken(tokenStr string, cfg *config.Config) (*Claims, error) {
	return parseToken(tokenStr, cfg.JWT.RefreshSecret)
}

func parseToken(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

// ── Session store helpers ─────────────────────────────────────────────────────

// sessionKey returns the Redis key for a JTI
func sessionKey(jti string) string {
	return "session:jti:" + jti
}

// StoreSession saves the JTI in Redis to mark it as valid/active
// If Redis is unavailable, logs a warning and continues (dev mode)
func StoreSession(ctx context.Context, jti, userID string, ttl time.Duration) error {
	err := cache.SessionSet(ctx, sessionKey(jti), userID, ttl)
	if err != nil {
		// Redis unavailable — log and skip in dev mode
		log.Printf("[auth] WARNING: could not store session in Redis (Redis unavailable): %v", err)
		return nil
	}
	return nil
}

// RevokeSession removes the JTI from Redis (logout)
func RevokeSession(ctx context.Context, jti string) error {
	err := cache.SessionDel(ctx, sessionKey(jti))
	if err != nil {
		log.Printf("[auth] WARNING: could not revoke session in Redis: %v", err)
	}
	return nil
}

// IsSessionValid checks if the JTI still exists in Redis
// If Redis is unavailable, returns true (allow request) in dev mode
func IsSessionValid(ctx context.Context, jti string) (bool, error) {
	exists, err := cache.SessionExists(ctx, sessionKey(jti))
	if err != nil {
		log.Printf("[auth] WARNING: Redis unavailable, allowing session without validation: %v", err)
		return true, nil
	}
	return exists, nil
}

// ── Middleware ────────────────────────────────────────────────────────────────

// Authenticate is a middleware that validates the JWT and sets user context
func Authenticate(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractBearerToken(c)
		if tokenStr == "" {
			utils.RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or malformed authorization token")
			c.Abort()
			return
		}

		claims, err := ParseAccessToken(tokenStr, cfg)
		if err != nil {
			utils.RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired token")
			c.Abort()
			return
		}

		// check if session is still active in Redis (not revoked)
		valid, err := IsSessionValid(c.Request.Context(), claims.JTI)
		if err != nil || !valid {
			utils.RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Session expired or revoked")
			c.Abort()
			return
		}

		// set claims in Gin context
		c.Set(string(CtxUserID), claims.UserID)
		c.Set(string(CtxRole), claims.Role)
		c.Set(string(CtxJTI), claims.JTI)

		c.Next()
	}
}

// RequireRoles returns a middleware that enforces role-based access
func RequireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}

	return func(c *gin.Context) {
		role, exists := c.Get(string(CtxRole))
		if !exists {
			utils.RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Not authenticated")
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok || !allowed[roleStr] {
			utils.RespondError(c, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// OptionalAuth tries to authenticate but does NOT abort if no token present.
// Used for public routes that have optional auth (e.g., storefront with wishlist state).
func OptionalAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractBearerToken(c)
		if tokenStr == "" {
			c.Next()
			return
		}

		claims, err := ParseAccessToken(tokenStr, cfg)
		if err != nil {
			c.Next()
			return
		}

		valid, err := IsSessionValid(c.Request.Context(), claims.JTI)
		if err != nil || !valid {
			c.Next()
			return
		}

		c.Set(string(CtxUserID), claims.UserID)
		c.Set(string(CtxRole), claims.Role)
		c.Set(string(CtxJTI), claims.JTI)

		c.Next()
	}
}

// ── Context Helpers ───────────────────────────────────────────────────────────

// GetUserID retrieves the authenticated user ID from Gin context
func GetUserID(c *gin.Context) (string, bool) {
	val, exists := c.Get(string(CtxUserID))
	if !exists {
		return "", false
	}
	id, ok := val.(string)
	return id, ok
}

// MustGetUserID retrieves user ID or aborts with 401
func MustGetUserID(c *gin.Context) string {
	id, ok := GetUserID(c)
	if !ok || id == "" {
		utils.RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Not authenticated")
		c.Abort()
		return ""
	}
	return id
}

// GetRole retrieves the authenticated user role from Gin context
func GetRole(c *gin.Context) (string, bool) {
	val, exists := c.Get(string(CtxRole))
	if !exists {
		return "", false
	}
	role, ok := val.(string)
	return role, ok
}

// GetJTI retrieves the token JTI from Gin context
func GetJTI(c *gin.Context) (string, bool) {
	val, exists := c.Get(string(CtxJTI))
	if !exists {
		return "", false
	}
	jti, ok := val.(string)
	return jti, ok
}

// IsAdmin returns true if the current user has any admin role
func IsAdmin(c *gin.Context) bool {
	role, ok := GetRole(c)
	if !ok {
		return false
	}
	switch role {
	case "super_admin", "finance_admin", "ops_admin", "support_admin":
		return true
	}
	return false
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// extractBearerToken pulls the token from the Authorization header
func extractBearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}