package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	password := "Admin@123456"
	email    := "admin@souqiq.ly"
	dsn      := "root:@tcp(localhost:3306)/souqiq?charset=utf8mb4&parseTime=True&loc=UTC"

	// Generate hash using Go's bcrypt (always $2a$)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		log.Fatalf("hash error: %v", err)
	}
	fmt.Printf("Generated hash: %s\n", string(hash))

	// Verify it works before writing
	if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil {
		log.Fatalf("verification failed: %v", err)
	}
	fmt.Println("Hash verified OK in Go ✅")

	// Connect and update
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	res, err := db.Exec(
		`UPDATE users SET password_hash = ?, is_active = 1, is_verified = 1, updated_at = NOW() WHERE email = ?`,
		string(hash), email,
	)
	if err != nil {
		log.Fatalf("update error: %v", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		log.Fatalf("no rows updated — user %s not found!", email)
	}

	fmt.Printf("Updated %d row(s) for %s ✅\n", rows, email)
	fmt.Println("Now restart the backend and login should work!")
}