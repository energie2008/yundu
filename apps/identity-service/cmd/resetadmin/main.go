package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/db"
	"github.com/airport-panel/identity-service/internal/pkg"
	"github.com/google/uuid"
)

// 重置 super_admin 密码的工具
// 用法：./resetadmin [user_id] [new_password]
// 不传参数时默认重置 a0000000-0000-0000-0000-000000000001 的密码为 admin123
func main() {
	userIDStr := "a0000000-0000-0000-0000-000000000001"
	password := "admin123"
	if len(os.Args) > 1 {
		userIDStr = os.Args[1]
	}
	if len(os.Args) > 2 {
		password = os.Args[2]
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid user id: %v\n", err)
		os.Exit(1)
	}

	hash, err := pkg.HashPassword(password, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash error: %v\n", err)
		os.Exit(1)
	}

	// VPS190 PostgreSQL 配置（从 .env 读取的实际值）
	cfg := config.DatabaseConfig{
		Host:     "127.0.0.1",
		Port:     5433,
		User:     "app",
		Password: "YunDuProd2026Secure",
		DBName:   "airport",
	}
	pool, err := db.NewPool(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db connect error: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	now := time.Now()
	res, err := pool.Exec(context.Background(),
		`UPDATE users SET password_hash = $1, email_verified_at = $2, status = 'active', updated_at = $2 WHERE id = $3`,
		hash, now, userID,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update user error: %v\n", err)
		os.Exit(1)
	}
	if res.RowsAffected() == 0 {
		fmt.Fprintf(os.Stderr, "no user found with id %s\n", userID)
		os.Exit(1)
	}

	// 查询更新后的 email 用于确认
	var email string
	err = pool.QueryRow(context.Background(), `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if err == nil {
		fmt.Printf("Password updated for user %s (email: %s)\n", userID, email)
	} else {
		fmt.Printf("Password updated for user %s\n", userID)
	}
	fmt.Printf("New password: %s\n", password)
}
