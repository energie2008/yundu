// Package cert - P1-2: CertEventBus 证书事件总线
//
// 证书签发/续期/失败时发布事件，订阅者（KernelReconciler/NginxReconciler）
// 收到事件后自动触发 xray SIGUSR1 热重载或 nginx reload，
// 实现"证书续期后 xray 自动接收新 PEM"（P1 验收标准 1）。
package cert

import (
	"sync"
	"time"
)

// CertEventType 证书事件类型
type CertEventType string

const (
	CertEventObtained CertEventType = "obtained" // 首次签发成功
	CertEventRenewed  CertEventType = "renewed"  // 续期成功
	CertEventFailed   CertEventType = "failed"   // 签发/续期失败
	CertEventRevoked  CertEventType = "revoked"  // 证书吊销
	CertEventContent  CertEventType = "content"  // content 模式 PEM 更新（P1-4）
)

// CertEvent 证书事件
type CertEvent struct {
	Type     CertEventType
	Domain   string
	BundleID string      // 证书标识（通常为 domain）
	PEM      *PEMBundle  // 新的 PEM bundle（obtained/renewed/content 时非 nil）
	Error    error       // failed 时的错误
	Time     time.Time
}

// CertEventHandler 证书事件处理函数
type CertEventHandler func(event CertEvent)

// CertEventBus 证书事件总线
// 支持多订阅者，事件发布时异步通知所有订阅者。
type CertEventBus struct {
	mu         sync.RWMutex
	subscribers []CertEventHandler
	logger     Logger
}

// NewCertEventBus 创建证书事件总线
func NewCertEventBus(logger Logger) *CertEventBus {
	if logger == nil {
		logger = noopLogger{}
	}
	return &CertEventBus{
		logger: logger,
	}
}

// Subscribe 订阅证书事件
// 返回取消订阅函数
func (b *CertEventBus) Subscribe(handler CertEventHandler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, handler)
	idx := len(b.subscribers) - 1

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if idx < len(b.subscribers) {
			b.subscribers[idx] = nil // 置 nil 避免索引偏移
		}
	}
}

// Publish 发布证书事件
// 异步通知所有订阅者，不阻塞发布者。
func (b *CertEventBus) Publish(event CertEvent) {
	if event.Time.IsZero() {
		event.Time = time.Now()
	}

	b.mu.RLock()
	handlers := make([]CertEventHandler, 0, len(b.subscribers))
	for _, h := range b.subscribers {
		if h != nil {
			handlers = append(handlers, h)
		}
	}
	b.mu.RUnlock()

	b.logger.Info("cert event published",
		"type", string(event.Type),
		"domain", event.Domain,
		"subscribers", len(handlers))

	// 异步通知，避免阻塞证书签发流程
	for _, handler := range handlers {
		go func(h CertEventHandler) {
			defer func() {
				if r := recover(); r != nil {
					b.logger.Error("cert event handler panicked",
						"type", string(event.Type), "domain", event.Domain, "panic", r)
				}
			}()
			h(event)
		}(handler)
	}
}
