package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/airport-panel/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBuildDSNFromComponents(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "app",
		Password: "secret",
		DBName:   "testdb",
	}
	dsn := BuildDSN(cfg)
	expected := "postgres://app:secret@localhost:5432/testdb?sslmode=disable"
	if dsn != expected {
		t.Errorf("BuildDSN() = %q, want %q", dsn, expected)
	}
}

func TestBuildDSNWithSSLMode(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "db.example.com",
		Port:     5432,
		User:     "prod",
		Password: "p@ss",
		DBName:   "airport",
	}
	dsn := BuildDSNWithSSL(cfg, "require")
	expected := "postgres://prod:p%40ss@db.example.com:5432/airport?sslmode=require"
	if dsn != expected {
		t.Errorf("BuildDSNWithSSL() = %q, want %q", dsn, expected)
	}
}

func TestNewPoolFailsWithInvalidDSN(t *testing.T) {
	cfg := config.DatabaseConfig{DSN: "postgres://invalid:5432/nodb"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, cfg)
	if err == nil {
		pool.Close()
		t.Fatal("expected error with invalid DSN, got nil")
	}
}

func TestPoolConfigParse(t *testing.T) {
	cfg := config.DatabaseConfig{DSN: "postgres://app:app@localhost:5432/airport?sslmode=disable&pool_max_conns=10"}
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if poolCfg.ConnConfig.Database != "airport" {
		t.Errorf("expected db=airport, got %s", poolCfg.ConnConfig.Database)
	}
}

func TestBuildDSNPreservesExistingDSN(t *testing.T) {
	cfg := config.DatabaseConfig{
		DSN:      "postgres://custom:custom@custom:5432/custom?sslmode=disable",
		Host:     "other",
		Port:     9999,
		User:     "other",
		Password: "other",
		DBName:   "other",
	}
	dsn := ResolveDSN(cfg)
	if dsn != cfg.DSN {
		t.Errorf("ResolveDSN should prefer existing DSN, got %q", dsn)
	}
}

func TestResolveDSNBuildsWhenEmpty(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "app",
		Password: "app",
		DBName:   "airport",
	}
	dsn := ResolveDSN(cfg)
	expected := "postgres://app:app@localhost:5432/airport?sslmode=disable"
	if dsn != expected {
		t.Errorf("ResolveDSN() = %q, want %q", dsn, expected)
	}
}

func TestConnectionStringEscaping(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "user",
		Password: "p@ss:w/ord",
		DBName:   "db",
	}
	dsn := BuildDSN(cfg)
	expected := "postgres://user:p%40ss%3Aw%2Ford@localhost:5432/db?sslmode=disable"
	if dsn != expected {
		t.Errorf("password not escaped properly: got %q, want %q", dsn, expected)
	}
	if _, err := fmt.Println(dsn); err != nil {
		t.Fatal(err)
	}
}
