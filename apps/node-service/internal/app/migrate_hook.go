package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// runMigrationsOnStart 生产启动时自动执行数据库迁移（MIGRATE_ON_START=true）。
// 迁移二进制默认 /opt/yundu/bin/migrate，可用 MIGRATE_BIN 环境变量覆盖。
// 迁移失败直接 panic（fail fast），避免旧 schema 上运行新代码。
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "up")
	cmd.Dir = filepath.Dir(bin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("migrate up failed, refusing to continue with stale schema",
			"bin", bin, "error", err, "output", tailString(string(out), 300))
		panic(fmt.Sprintf("migrate up failed: %v", err))
	}
	logger.Info("migrate up completed on start", "bin", bin, "output_tail", tailString(string(out), 300))
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
