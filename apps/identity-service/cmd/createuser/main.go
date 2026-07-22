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

func main() {
	email := "t***@yundu.local"
	password := "user123"
	if len(os.Args) > 1 {
		email = os.Args[1]
	}
	if len(os.Args) > 2 {
		password = os.Args[2]
	}

	hash, err := pkg.HashPassword(password, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash error: %v\n", err)
		os.Exit(1)
	}

	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "app",
		Password: "app",
		DBName:   "airport",
	}
	pool, err := db.NewPool(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db connect error: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	var userID uuid.UUID
	now := time.Now()

	err = pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
	if err != nil {
		userID = uuid.New()
		_, err = pool.Exec(context.Background(),
			`INSERT INTO users (id, email, username, status, email_verified_at, locale, timezone, password_hash, created_at, updated_at)
			 VALUES ($1, $2, $3, 'active', $4, 'zh-CN', 'Asia/Shanghai', $5, $6, $6)`,
			userID, email, "testuser", now, hash, now,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create user error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Created new user.")
	} else {
		_, err = pool.Exec(context.Background(),
			`UPDATE users SET password_hash = $1, email_verified_at = $2, status = 'active', updated_at = $2 WHERE id = $3`,
			hash, now, userID,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "update user error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Updated existing user password.")
	}

	var planID uuid.UUID
	err = pool.QueryRow(context.Background(), `SELECT id FROM plans WHERE code = 'free' LIMIT 1`).Scan(&planID)
	if err == nil {
		expiresAt := now.AddDate(0, 0, 7)
		_, err = pool.Exec(context.Background(),
			`INSERT INTO user_plan_subscriptions (id, user_id, plan_id, status, started_at, expires_at, traffic_quota_bytes, traffic_used_bytes, created_at, updated_at)
			 VALUES ($1, $2, $3, 'active', $4, $5, 1073741824, 0, $4, $4)
			 ON CONFLICT DO NOTHING`,
			uuid.New(), userID, planID, now, expiresAt,
		)
		if err != nil {
			fmt.Printf("subscription note: %v\n", err)
		} else {
			fmt.Println("Assigned free plan (1GB / 7 days).")
		}
	} else {
		fmt.Println("No free plan found, skipping subscription assignment.")
	}

	rawToken, tokenHash := pkg.GenerateSubscriptionToken()
	preview := rawToken[:4] + "..." + rawToken[len(rawToken)-4:]
	tokenID := uuid.New()
	_, err = pool.Exec(context.Background(),
		`INSERT INTO subscription_tokens (id, user_id, token_hash, token_preview, status, allow_ip_bind, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'active', false, $5, $5)`,
		tokenID, userID, tokenHash, preview, now,
	)
	if err != nil {
		fmt.Printf("token note: %v\n", err)
	} else {
		fmt.Println("Created subscription token.")
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("User email:    %s\n", email)
	fmt.Printf("User password: %s\n", password)
	fmt.Printf("========================================\n")
}
