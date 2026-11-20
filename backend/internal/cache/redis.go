package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"souqiq/config"
)

// clients holds separate Redis clients per purpose
type clients struct {
	Session   *redis.Client // DB 0 — JWT session/jti store
	Cache     *redis.Client // DB 1 — general app cache
	RateLimit *redis.Client // DB 2 — rate limiting counters
}

var store *clients

// Connect initializes all Redis clients
func Connect(cfg *config.Config) {
	if store != nil {
		return
	}

	store = &clients{
		Session:   newClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.SessionDB),
		Cache:     newClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.CacheDB),
		RateLimit: newClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.RateLimitDB),
	}

	// verify all connections
	ctx := context.Background()
	allOK := true
	for name, client := range map[string]*redis.Client{
		"session":    store.Session,
		"cache":      store.Cache,
		"rate-limit": store.RateLimit,
	} {
		if err := client.Ping(ctx).Err(); err != nil {
			log.Printf("[cache] WARNING: failed to connect to Redis (%s): %v", name, err)
			allOK = false
		}
	}

	if allOK {
		log.Printf("[cache] connected to Redis at %s (3 clients)", cfg.Redis.Addr)
	} else {
		log.Printf("[cache] WARNING: Redis unavailable at %s — running without cache (dev mode)", cfg.Redis.Addr)
	}
}

func newClient(addr, password string, db int) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     20,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
}

// Close gracefully closes all Redis clients
func Close() {
	if store == nil {
		return
	}
	_ = store.Session.Close()
	_ = store.Cache.Close()
	_ = store.RateLimit.Close()
	log.Println("[cache] all Redis clients closed")
}

// ── Session store (JWT jti blacklist / active sessions) ──────────────────────

// SessionSet stores a key with expiry in the session store
func SessionSet(ctx context.Context, key, value string, ttl time.Duration) error {
	return store.Session.Set(ctx, key, value, ttl).Err()
}

// SessionGet retrieves a value from the session store
func SessionGet(ctx context.Context, key string) (string, error) {
	val, err := store.Session.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // key not found — not an error
	}
	return val, err
}

// SessionDel removes a key from the session store
func SessionDel(ctx context.Context, key string) error {
	return store.Session.Del(ctx, key).Err()
}

// SessionExists checks if a key exists in the session store
func SessionExists(ctx context.Context, key string) (bool, error) {
	n, err := store.Session.Exists(ctx, key).Result()
	return n > 0, err
}

// ── General cache ─────────────────────────────────────────────────────────────

// Set stores a string value with expiry
func Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return store.Cache.Set(ctx, key, value, ttl).Err()
}

// Get retrieves a string value; returns ("", nil) if not found
func Get(ctx context.Context, key string) (string, error) {
	val, err := store.Cache.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// Del removes one or more keys from the general cache
func Del(ctx context.Context, keys ...string) error {
	return store.Cache.Del(ctx, keys...).Err()
}

// Exists returns true if the key exists in the general cache
func Exists(ctx context.Context, key string) (bool, error) {
	n, err := store.Cache.Exists(ctx, key).Result()
	return n > 0, err
}

// SetJSON marshals v to JSON and stores it with expiry
func SetJSON(ctx context.Context, key string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("cache.SetJSON marshal: %w", err)
	}
	return store.Cache.Set(ctx, key, b, ttl).Err()
}

// GetJSON retrieves a JSON value and unmarshals into dest.
// Returns (false, nil) if key does not exist.
func GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	val, err := store.Cache.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(val, dest); err != nil {
		return false, fmt.Errorf("cache.GetJSON unmarshal: %w", err)
	}
	return true, nil
}

// Invalidate deletes all keys matching a pattern (use sparingly)
func Invalidate(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := store.Cache.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := store.Cache.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// ── OTP store (uses session client with short TTL) ───────────────────────────

const otpTTL = 5 * time.Minute

// OTPSet stores an OTP for a phone number
func OTPSet(ctx context.Context, phone, otp string) error {
	key := otpKey(phone)
	return store.Session.Set(ctx, key, otp, otpTTL).Err()
}

// OTPGet retrieves the OTP for a phone number
func OTPGet(ctx context.Context, phone string) (string, error) {
	val, err := store.Session.Get(ctx, otpKey(phone)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// OTPDel deletes the OTP after successful verification
func OTPDel(ctx context.Context, phone string) error {
	return store.Session.Del(ctx, otpKey(phone)).Err()
}

func otpKey(phone string) string {
	return "otp:" + phone
}

// ── Rate limit client accessor ───────────────────────────────────────────────

// RateLimitClient returns the rate limit Redis client
// (used by the rate limiter middleware directly)
func RateLimitClient() *redis.Client {
	if store == nil {
		log.Fatal("[cache] cache not connected — call cache.Connect() first")
	}
	return store.RateLimit
}

// ── Cache key helpers ─────────────────────────────────────────────────────────

// Key prefixes — centralised to avoid collisions
const (
	PrefixProduct      = "product:"
	PrefixSupplier     = "supplier:"
	PrefixCategory     = "category:"
	PrefixCart         = "cart:"
	PrefixOrder        = "order:"
	PrefixUser         = "user:"
	PrefixHomepage     = "homepage:"
	PrefixSearch       = "search:"
	PrefixSettings     = "settings:"
)

// ProductKey returns the cache key for a single product
func ProductKey(id string) string { return PrefixProduct + id }

// SupplierKey returns the cache key for a single supplier profile
func SupplierKey(id string) string { return PrefixSupplier + id }

// CategoryTreeKey returns the cache key for the full category tree
func CategoryTreeKey() string { return PrefixCategory + "tree" }

// CartKey returns the cache key for a customer's cart
func CartKey(customerID string) string { return PrefixCart + customerID }

// HomepageKey returns the cache key for homepage data
func HomepageKey() string { return PrefixHomepage + "data" }

// SearchKey returns the cache key for a search query result
func SearchKey(query string) string { return PrefixSearch + query }

// SettingsKey returns the cache key for platform settings
func SettingsKey() string { return PrefixSettings + "all" }

// TTL constants for different data types
const (
	TTLProduct   = 10 * time.Minute
	TTLSupplier  = 10 * time.Minute
	TTLCategory  = 30 * time.Minute
	TTLCart      = 24 * time.Hour
	TTLHomepage  = 5 * time.Minute
	TTLSearch    = 2 * time.Minute
	TTLSettings  = 15 * time.Minute
	TTLShort     = 1 * time.Minute
	TTLSession   = 24 * time.Hour * 30 // 30 days for refresh token
)