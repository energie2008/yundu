package app

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// runMigrationsOnStart 生产启动时自动执行数据库迁移（MIGRATE_ON_START=true）。
// 迁移二进制默认 /opt/yundu/bin/migrate，可用 MIGRATE_BIN 环境变量覆盖。
// 迁移目录默认 /opt/yundu/migrations，可用 MIGRATIONS_DIR 覆盖并注入子进程环境。
// 迁移失败只记录 ERROR 不阻断启动（生产可用性优先；schema 不一致由版本发布流程保证）。
func runMigrationsOnStart(logger *slog.Logger) {
	if os.Getenv("MIGRATE_ON_START") != "true" {
		return
	}
	bin := os.Getenv("MIGRATE_BIN")
	if bin == "" {
		bin = "/opt/yundu/bin/migrate"
	}
	if _, err := os.Stat(bin); err != nil {
		logger.Error("MIGRATE_ON_START enabled but migrate binary not found", "bin", bin)
		return
	}
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "/opt/yundu/migrations"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "up")
	cmd.Dir = filepath.Dir(bin)
	cmd.Env = append(os.Environ(),
		"MIGRATIONS_DIR="+migrationsDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("migrate up failed, refusing to continue with stale schema",
			"bin", bin, "migrations_dir", migrationsDir, "error", err, "output", tailString(string(out), 300))
		return
	}
	logger.Info("migrate up completed on start",
		"bin", bin, "migrations_dir", migrationsDir, "output_tail", tailString(string(out), 300))
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
