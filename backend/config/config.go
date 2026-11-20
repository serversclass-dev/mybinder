package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Storage  StorageConfig
	Mail     MailConfig
	SMS      SMSConfig
}

type AppConfig struct {
	Env      string
	Port     string
	Name     string
	URL      string
	Currency string
	Language string
	Debug    bool
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	DSN      string
}

type RedisConfig struct {
	Addr           string
	Password       string
	DB             int
	SessionDB      int
	CacheDB        int
	RateLimitDB    int
	CeleryBrokerDB int
}

type JWTConfig struct {
	AccessSecret       string
	RefreshSecret      string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
}

type StorageConfig struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
	CDNBaseURL      string
}

type MailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
}

type SMSConfig struct {
	Provider   string
	AccountSID string
	AuthToken  string
	FromNumber string
}

var cfg *Config

// Load reads .env and returns the singleton Config
func Load() *Config {
	if cfg != nil {
		return cfg
	}

	// load .env if present (ignored in production where env vars are injected)
	if err := godotenv.Load(); err != nil {
		log.Println("[config] no .env file found, reading from environment")
	}

	accessExpiry, _ := strconv.Atoi(getEnv("JWT_ACCESS_TOKEN_EXPIRES_SECONDS", "3600"))
	refreshExpiry, _ := strconv.Atoi(getEnv("JWT_REFRESH_TOKEN_EXPIRES_SECONDS", "2592000"))
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	mailPort, _ := strconv.Atoi(getEnv("MAIL_PORT", "587"))
	mailTLS, _ := strconv.ParseBool(getEnv("MAIL_USE_TLS", "true"))
	debug, _ := strconv.ParseBool(getEnv("DEBUG", "false"))

	cfg = &Config{
		App: AppConfig{
			Env:      getEnv("APP_ENV", "development"),
			Port:     getEnv("APP_PORT", "8080"),
			Name:     getEnv("PLATFORM_NAME", "SOUQIQ"),
			URL:      getEnv("PLATFORM_URL", "http://localhost:8080"),
			Currency: getEnv("DEFAULT_CURRENCY", "LYD"),
			Language: getEnv("DEFAULT_LANGUAGE", "ar"),
			Debug:    debug,
		},
		Database: buildDatabaseConfig(),
		Redis: RedisConfig{
			Addr:           getEnv("REDIS_ADDR", "localhost:6379"),
			Password:       getEnv("REDIS_PASSWORD", ""),
			DB:             redisDB,
			SessionDB:      0,
			CacheDB:        1,
			RateLimitDB:    2,
			CeleryBrokerDB: 3,
		},
		JWT: JWTConfig{
			AccessSecret:       mustEnv("JWT_ACCESS_SECRET"),
			RefreshSecret:      mustEnv("JWT_REFRESH_SECRET"),
			AccessTokenExpiry:  time.Duration(accessExpiry) * time.Second,
			RefreshTokenExpiry: time.Duration(refreshExpiry) * time.Second,
		},
		Storage: StorageConfig{
			Endpoint:        getEnv("S3_ENDPOINT", ""),
			AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", ""),
			SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
			Bucket:          getEnv("AWS_S3_BUCKET", "souqiq-uploads"),
			Region:          getEnv("AWS_S3_REGION", "us-east-1"),
			CDNBaseURL:      getEnv("CDN_BASE_URL", ""),
		},
		Mail: MailConfig{
			Host:     getEnv("MAIL_HOST", "smtp.gmail.com"),
			Port:     mailPort,
			Username: getEnv("MAIL_USERNAME", ""),
			Password: getEnv("MAIL_PASSWORD", ""),
			From:     getEnv("MAIL_FROM", "noreply@souqiq.ly"),
			UseTLS:   mailTLS,
		},
		SMS: SMSConfig{
			Provider:   getEnv("SMS_PROVIDER", "twilio"),
			AccountSID: getEnv("TWILIO_ACCOUNT_SID", ""),
			AuthToken:  getEnv("TWILIO_AUTH_TOKEN", ""),
			FromNumber: getEnv("TWILIO_FROM_NUMBER", ""),
		},
	}

	return cfg
}

// Get returns the loaded config (panics if Load() was not called first)
func Get() *Config {
	if cfg == nil {
		log.Fatal("[config] config not loaded — call config.Load() first")
	}
	return cfg
}

func buildDatabaseConfig() DatabaseConfig {
	host     := getEnv("DB_HOST", "localhost")
	port     := getEnv("DB_PORT", "3306")
	user     := getEnv("DB_USER", "root")
	password := getEnv("DB_PASSWORD", "")
	name     := getEnv("DB_NAME", "souqiq")

	// MySQL DSN format for go-sql-driver/mysql
	dsn := user + ":" + password +
		"@tcp(" + host + ":" + port + ")/" + name +
		"?charset=utf8mb4&parseTime=True&loc=UTC"

	return DatabaseConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		Name:     name,
		DSN:      dsn,
	}
}

// getEnv returns env var value or fallback
func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

// mustEnv panics if the env var is not set — for required secrets
func mustEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("[config] required environment variable %q is not set", key)
	}
	return val
}