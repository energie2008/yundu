package warp

import (
	"log/slog"
	"os"
	"strings"
)

// 编译期断言：确保三种实现都满足 Manager 接口。
var (
	_ Manager = (*WarpManager)(nil)
	_ Manager = (*MockManager)(nil)
	_ Manager = (*WireProxyManager)(nil)
)

// NewManager 是 Manager 接口的工厂：根据环境变量 WARP_MODE 选择实现。
//
//   - WARP_MODE=mock（或 "" 且未安装 warp-cli 时自动回退）：返回 MockManager，
//     不调用任何真实 exec，适合本地开发 / 沙箱 / CI。
//   - WARP_MODE=real（默认）：返回真实 WarpManager，调用 warp-cli 系统服务（30-50MB）。
//   - WARP_MODE=wireproxy：返回 WireProxyManager，管理 wireproxy 子进程（~4MB），
//     作为 sing-box 原生 wireguard 的轻量级 fallback，符合 ≤150MB 内存约束。
//
// reporter 与 logger 可为 nil（MockManager 不依赖它们；真实实现会回退到 slog.Default()）。
func NewManager(reporter PanelReporter, logger *slog.Logger) Manager {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("WARP_MODE")))
	if mode == "" {
		mode = "real"
	}

	switch mode {
	case "mock":
		if logger != nil {
			logger.Info("warp manager: using MockManager (WARP_MODE=mock)")
		}
		return NewMockManager()
	case "wireproxy":
		if logger != nil {
			logger.Info("warp manager: using WireProxyManager (WARP_MODE=wireproxy, ~4MB RAM)")
		}
		return NewWireProxyManager(reporter, logger)
	case "real":
		if logger != nil {
			logger.Info("warp manager: using real WarpManager (WARP_MODE=real)")
		}
		return NewWarpManager(reporter, logger)
	default:
		if logger != nil {
			logger.Warn("warp manager: unknown WARP_MODE, falling back to real", "mode", mode)
		}
		return NewWarpManager(reporter, logger)
	}
}
