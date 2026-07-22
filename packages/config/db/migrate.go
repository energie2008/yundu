package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func openDB(pool *pgxpool.Pool) *sql.DB {
	return stdlib.OpenDBFromPool(pool)
}

func MigrateUp(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	goose.SetDialect("postgres")
	db := openDB(pool)
	defer db.Close()
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("goose up %s: %w", dir, err)
	}
	return nil
}

func MigrateDown(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	goose.SetDialect("postgres")
	db := openDB(pool)
	defer db.Close()
	if err := goose.DownContext(ctx, db, dir); err != nil {
		return fmt.Errorf("goose down %s: %w", dir, err)
	}
	return nil
}

func MigrateStatus(ctx context.Context, pool *pgxpool.Pool, dir string) (int64, error) {
	goose.SetDialect("postgres")
	db := openDB(pool)
	defer db.Close()
	version, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("goose status %s: %w", dir, err)
	}
	return version, nil
}

func MigrateUpTo(ctx context.Context, pool *pgxpool.Pool, dir string, version int64) error {
	goose.SetDialect("postgres")
	db := openDB(pool)
	defer db.Close()
	if err := goose.UpToContext(ctx, db, dir, version); err != nil {
		return fmt.Errorf("goose up-to %d %s: %w", version, dir, err)
	}
	return nil
}

func MigrateUpFS(ctx context.Context, pool *pgxpool.Pool, migrationsFS fs.FS, dir string) error {
	goose.SetBaseFS(migrationsFS)
	goose.SetDialect("postgres")
	db := openDB(pool)
	defer db.Close()
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("goose up fs %s: %w", dir, err)
	}
	return nil
}
