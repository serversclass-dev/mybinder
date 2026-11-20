package db

import (
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/go-sql-driver/mysql"
	"souqiq/config"
)

var instance *sqlx.DB

// Connect initializes the MySQL connection pool
func Connect(cfg *config.Config) *sqlx.DB {
	if instance != nil {
		return instance
	}

	db, err := sqlx.Open("mysql", cfg.Database.DSN)
	if err != nil {
		log.Fatalf("[db] failed to open connection: %v", err)
	}

	// connection pool settings
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(10 * time.Minute)

	// verify connectivity
	if err := db.Ping(); err != nil {
		log.Fatalf("[db] failed to ping MySQL: %v", err)
	}

	log.Printf("[db] connected to MySQL: %s/%s",
		cfg.Database.Host, cfg.Database.Name)

	instance = db
	return instance
}

// Get returns the singleton DB instance (panics if not connected)
func Get() *sqlx.DB {
	if instance == nil {
		log.Fatal("[db] database not connected — call db.Connect() first")
	}
	return instance
}

// Close gracefully closes the connection pool
func Close() {
	if instance != nil {
		if err := instance.Close(); err != nil {
			log.Printf("[db] error closing database: %v", err)
		}
		log.Println("[db] connection pool closed")
	}
}

// Ping checks if DB is reachable
func Ping() error {
	if instance == nil {
		return fmt.Errorf("database not connected")
	}
	return instance.Ping()
}

// MustBegin starts a transaction or panics — use for critical operations
func MustBegin() *sqlx.Tx {
	tx, err := instance.Beginx()
	if err != nil {
		log.Fatalf("[db] failed to begin transaction: %v", err)
	}
	return tx
}

// WithTransaction executes fn inside a transaction.
// Automatically commits on success, rolls back on error or panic.
func WithTransaction(fn func(tx *sqlx.Tx) error) error {
	tx, err := instance.Beginx()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-panic after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx error: %v, rollback error: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}