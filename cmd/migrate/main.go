package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getMigrationsDir() string {
	if dir := os.Getenv("MIGRATIONS_DIR"); dir != "" {
		return dir
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(exeDir, "..", "..", "migrations"),
			filepath.Join(exeDir, "migrations"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					return abs
				}
				return c
			}
		}
	}
	if wd, err := os.Getwd(); err == nil {
		candidates := []string{
			filepath.Join(wd, "migrations"),
			filepath.Join(wd, "..", "migrations"),
			`d:\机场搭建\air\migrations`,
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					return abs
				}
				return c
			}
		}
	}
	return "/app/migrations"
}

type MigrationFile struct {
	Version  int64
	Filename string
	Path     string
}

func main() {
	ctx := context.Background()

	cfg := config.Load()

	fmt.Println("=== 数据库迁移工具 ===")
	fmt.Printf("连接数据库: %s\n", cfg.Database.DSN)

	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	fmt.Println("数据库连接成功")

	if err := ensureVersionTable(ctx, pool); err != nil {
		fmt.Fprintf(os.Stderr, "创建版本表失败: %v\n", err)
		os.Exit(1)
	}

	currentVersion, err := getCurrentVersion(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取当前版本失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("当前数据库版本: %d\n", currentVersion)

	migrations, err := loadMigrations(getMigrationsDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载迁移文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("找到 %d 个迁移文件\n", len(migrations))

	pending := getPendingMigrations(migrations, currentVersion)
	fmt.Printf("待执行迁移: %d 个\n\n", len(pending))

	if len(pending) == 0 {
		fmt.Println("数据库已是最新版本，无需迁移")
		return
	}

	successCount := 0
	var appliedVersions []int64

	for _, m := range pending {
		fmt.Printf("[%d] 执行迁移: %s\n", m.Version, m.Filename)

		if err := runMigration(ctx, pool, m); err != nil {
			fmt.Fprintf(os.Stderr, "[%d] 迁移失败: %v\n", m.Version, err)
			fmt.Printf("\n迁移中断。已成功应用 %d 个迁移: %v\n", successCount, appliedVersions)
			os.Exit(1)
		}

		fmt.Printf("[%d] 迁移成功\n\n", m.Version)
		appliedVersions = append(appliedVersions, m.Version)
		successCount++
	}

	fmt.Println("=== 迁移完成 ===")
	fmt.Printf("成功应用 %d 个迁移\n", successCount)
	fmt.Printf("已应用版本: %v\n", appliedVersions)
}

func ensureVersionTable(ctx context.Context, pool *pgxpool.Pool) error {
	sql := `
	CREATE TABLE IF NOT EXISTS goose_db_version (
		id SERIAL PRIMARY KEY,
		version_id BIGINT NOT NULL,
		is_applied BOOLEAN NOT NULL,
		tstamp TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_goose_db_version_version_id ON goose_db_version(version_id);
	`
	_, err := pool.Exec(ctx, sql)
	return err
}

func getCurrentVersion(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var version int64
	err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = true").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

var versionRegex = regexp.MustCompile(`^(\d{6})_.*\.sql$`)

func loadMigrations(dir string) ([]MigrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var migrations []MigrationFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		matches := versionRegex.FindStringSubmatch(name)
		if matches == nil {
			continue
		}
		var version int64
		if _, err := fmt.Sscanf(matches[1], "%d", &version); err != nil {
			continue
		}
		migrations = append(migrations, MigrationFile{
			Version:  version,
			Filename: name,
			Path:     filepath.Join(dir, name),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func getPendingMigrations(migrations []MigrationFile, currentVersion int64) []MigrationFile {
	var pending []MigrationFile
	for _, m := range migrations {
		if m.Version > currentVersion {
			pending = append(pending, m)
		}
	}
	return pending
}

func parseSQLStatements(content string) []string {
	upMarker := "-- +goose Up"
	downMarker := "-- +goose Down"
	stmtBegin := "-- +goose StatementBegin"
	stmtEnd := "-- +goose StatementEnd"

	upIdx := strings.Index(content, upMarker)
	if upIdx == -1 {
		return nil
	}
	upContent := content[upIdx+len(upMarker):]

	downIdx := strings.Index(upContent, downMarker)
	if downIdx != -1 {
		upContent = upContent[:downIdx]
	}

	var statements []string
	remaining := upContent

	for {
		beginIdx := strings.Index(remaining, stmtBegin)
		if beginIdx == -1 {
			break
		}
		afterBegin := remaining[beginIdx+len(stmtBegin):]
		endIdx := strings.Index(afterBegin, stmtEnd)
		if endIdx == -1 {
			break
		}
		stmt := strings.TrimSpace(afterBegin[:endIdx])
		if stmt != "" {
			statements = append(statements, stmt)
		}
		remaining = afterBegin[endIdx+len(stmtEnd):]
	}

	return statements
}

func runMigration(ctx context.Context, pool *pgxpool.Pool, m MigrationFile) error {
	contentBytes, err := os.ReadFile(m.Path)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	statements := parseSQLStatements(string(contentBytes))
	if len(statements) == 0 {
		return fmt.Errorf("未找到有效SQL语句")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	for i, stmt := range statements {
		stmtPreview := strings.ReplaceAll(strings.TrimSpace(stmt), "\n", " ")
		if len(stmtPreview) > 80 {
			stmtPreview = stmtPreview[:77] + "..."
		}
		fmt.Printf("  执行语句 %d/%d: %s\n", i+1, len(statements), stmtPreview)

		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("执行SQL失败 (语句 %d): %w\nSQL: %s", i+1, err, stmt)
		}
	}

	_, err = tx.Exec(ctx,
		"INSERT INTO goose_db_version(version_id, is_applied, tstamp) VALUES($1, true, now())",
		m.Version,
	)
	if err != nil {
		return fmt.Errorf("插入版本记录失败: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}
