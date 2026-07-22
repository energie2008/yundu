package service

// binary_spec_service.go 实现 P2-4：二进制升级规格管理
//
// 提供 admin 设置目标二进制版本 + agent 拉取期望规格的能力。
// 当前实现为 in-memory（简单场景），未来可持久化到 DB。

import (
	"sync"
	"time"
)

// BinarySpecTarget P2-4: 目标二进制规格（admin 设置，agent 拉取）
type BinarySpecTarget struct {
	RuntimeType string    `json:"runtime_type"`
	Version     string    `json:"version"`
	DownloadURL string    `json:"download_url"`
	Checksum    string    `json:"checksum"`
	Strategy    string    `json:"strategy"` // canary / rolling / all_at_once
	Force       bool      `json:"force"`
	SetAt       time.Time `json:"set_at"`
	// ServerCodeMask 限定哪些 server_code 应用此升级（空=所有）
	ServerCodeMask string `json:"server_code_mask,omitempty"`
}

// BinarySpecService P2-4: 二进制升级规格服务（in-memory）
type BinarySpecService struct {
	mu   sync.RWMutex
	spec *BinarySpecTarget
}

func NewBinarySpecService() *BinarySpecService {
	return &BinarySpecService{}
}

// SetTarget 设置目标二进制规格（admin 调用）
func (s *BinarySpecService) SetTarget(spec *BinarySpecTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if spec.SetAt.IsZero() {
		spec.SetAt = time.Now()
	}
	s.spec = spec
}

// GetTarget 获取目标二进制规格（agent 调用）
// serverCode 用于灰度过滤（canary 策略下仅匹配 ServerCodeMask 的 server 返回规格）
func (s *BinarySpecService) GetTarget(serverCode string) *BinarySpecTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.spec == nil {
		return nil
	}
	// 灰度过滤：如果设置了 ServerCodeMask，仅匹配的 server 返回规格
	if s.spec.ServerCodeMask != "" {
		if s.spec.Strategy == "canary" && serverCode != s.spec.ServerCodeMask {
			return nil
		}
	}
	// 返回副本
	copy := *s.spec
	return &copy
}

// ClearTarget 清除目标规格（取消升级任务）
func (s *BinarySpecService) ClearTarget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spec = nil
}
