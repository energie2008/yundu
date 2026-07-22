package pipeline

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeployLock_WriteAndRemove(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewPipeline(nil, tmpDir, slog.Default())

	if err := p.WriteDeployLock("v123"); err != nil {
		t.Fatalf("WriteDeployLock failed: %v", err)
	}

	exists, version, err := p.CheckStaleLock()
	if err != nil {
		t.Fatalf("CheckStaleLock failed: %v", err)
	}
	if !exists {
		t.Error("expected lock to exist")
	}
	if version != "v123" {
		t.Errorf("expected version v123, got %s", version)
	}

	p.RemoveDeployLock()

	exists, _, _ = p.CheckStaleLock()
	if exists {
		t.Error("expected lock to be removed")
	}
}

func TestLKG_BackupAndRestore(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewPipeline(nil, tmpDir, slog.Default())

	configPath := filepath.Join(tmpDir, "config", "xray.json")
	os.MkdirAll(filepath.Dir(configPath), 0755)
	os.WriteFile(configPath, []byte(`{"test":true}`), 0644)

	if err := p.BackupLKG(configPath, "xray"); err != nil {
		t.Fatalf("BackupLKG failed: %v", err)
	}

	if !p.HasLKG("xray") {
		t.Error("expected LKG to exist")
	}

	// 删除原配置
	os.Remove(configPath)

	// 从 LKG 恢复
	if err := p.RestoreLKG(configPath, "xray"); err != nil {
		t.Fatalf("RestoreLKG failed: %v", err)
	}

	data, _ := os.ReadFile(configPath)
	if string(data) != `{"test":true}` {
		t.Errorf("restored config mismatch: %s", string(data))
	}
}

func TestCheckStaleLock_NoLock(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewPipeline(nil, tmpDir, slog.Default())

	exists, _, err := p.CheckStaleLock()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected no lock")
	}
}

func TestLKGConfigPath_RuntimeTypes(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewPipeline(nil, tmpDir, slog.Default())

	cases := map[string]string{
		"xray":     filepath.Join(tmpDir, "config", "xray.lkg.json"),
		"sing-box": filepath.Join(tmpDir, "config", "sing-box.lkg.json"),
		"other":    filepath.Join(tmpDir, "config", "config.lkg.json"),
	}
	for rt, want := range cases {
		if got := p.LKGConfigPath(rt); got != want {
			t.Errorf("LKGConfigPath(%q) = %q, want %q", rt, got, want)
		}
	}
}

func TestBackupLKG_NoCurrentConfig(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewPipeline(nil, tmpDir, slog.Default())

	// 当前配置不存在时，BackupLKG 应跳过（返回 nil）且不创建 LKG
	missingPath := filepath.Join(tmpDir, "config", "xray.json")
	if err := p.BackupLKG(missingPath, "xray"); err != nil {
		t.Fatalf("BackupLKG on missing config should not error: %v", err)
	}
	if p.HasLKG("xray") {
		t.Error("expected no LKG when current config is missing")
	}
}

func TestRun_NoValidatorSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewPipeline(nil, tmpDir, slog.Default())

	configPath := filepath.Join(tmpDir, "config", "xray.json")
	// validator 为 nil 时跳过预检，验证事务流程能正常完成
	cfg := []byte(`{"inbounds":[{"port":0}]}`)

	res := p.Run(context.Background(), "v1", cfg, configPath, "xray", DeployCallbacks{})
	if !res.Success {
		t.Fatalf("expected success without validator, got error: %s", res.Error)
	}
	// deploy.lock 应已被清理
	if exists, _, _ := p.CheckStaleLock(); exists {
		t.Error("expected deploy.lock to be removed after Run")
	}
	// 配置应已写入
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("expected config file written: %v", err)
	}
}

func TestRun_ContextCancel(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewPipeline(nil, tmpDir, slog.Default())

	configPath := filepath.Join(tmpDir, "config", "xray.json")
	cfg := []byte(`{}`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	cb := DeployCallbacks{HealthCheck: func(ctx context.Context, _ []byte) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	res := p.Run(ctx, "v1", cfg, configPath, "xray", cb)
	if res.Success {
		t.Error("expected failure due to context cancellation during health wait")
	}
}

func TestExtractPorts_Xray(t *testing.T) {
	cfg := []byte(`{"inbounds":[{"port":10000},{"port":10001}]}`)
	ports, err := extractPorts(cfg)
	if err != nil {
		t.Fatalf("extractPorts failed: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}
	if ports[0] != 10000 || ports[1] != 10001 {
		t.Errorf("unexpected ports: %v", ports)
	}
}

func TestExtractPorts_SingBox(t *testing.T) {
	cfg := []byte(`{"inbounds":[{"listen_port":20000}]}`)
	ports, err := extractPorts(cfg)
	if err != nil {
		t.Fatalf("extractPorts failed: %v", err)
	}
	if len(ports) != 1 || ports[0] != 20000 {
		t.Errorf("unexpected ports: %v", ports)
	}
}

func TestCheckHealth_NoPorts(t *testing.T) {
	h := NewHealthChecker(slog.Default())
	// 没有端口时认为健康
	cfg := []byte(`{"inbounds":[]}`)
	if err := h.CheckHealth(context.Background(), cfg, 0); err != nil {
		t.Errorf("expected nil error when no ports, got: %v", err)
	}
}
