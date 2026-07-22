package executor

import (
	"time"
)

// backoffDuration calculates exponential backoff for process restart.
// Sequence: 1s, 2s, 4s, 8s, 16s, 30s, 30s, ...
func backoffDuration(restartCount int64) time.Duration {
	if restartCount <= 0 {
		return 1 * time.Second
	}
	d := time.Duration(1<<uint(restartCount-1)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// B48 清理：移除 crashState 和 resetIfStable 死代码
// crashState 从未被任何执行器使用（xray.go 和 singbox.go 各自内联实现了等价逻辑）
