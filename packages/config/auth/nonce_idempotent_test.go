package auth

import (
	"context"
	"testing"
	"time"
)

// TestNonceStoreDoubleStop 验证 MemoryNonceStore.Stop() 重复调用不会 panic。
// Bug-B: 当前实现直接 close(stop)，无 sync.Once 保护，第二次调用会 panic:
// "close of closed channel"。短生命周期场景(如测试/临时实例)会触发此缺陷。
func TestNonceStoreDoubleStop(t *testing.T) {
	store := NewMemoryNonceStore()

	// 第一次 Stop 应正常关闭清理 goroutine
	store.Stop()

	// 第二次 Stop 不应 panic —— 必须幂等
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("second Stop() should be idempotent, but panicked: %v", r)
		}
	}()
	store.Stop()
}

// TestNonceStoreStopThenCheckAndStore 验证 Stop 后仍可安全调用 CheckAndStore
// (不会因 stop channel 状态产生未定义行为)。
func TestNonceStoreStopThenCheckAndStore(t *testing.T) {
	store := NewMemoryNonceStore()
	store.Stop()

	ctx := context.Background()
	// Stop 后调用不应 panic，仍能正常工作 (cleanupLoop 已退出，不影响 map 操作)
	ok, err := store.CheckAndStore(ctx, "post-stop-nonce", 5*time.Second)
	if err != nil {
		t.Fatalf("CheckAndStore after Stop returned error: %v", err)
	}
	if !ok {
		t.Fatal("CheckAndStore after Stop should accept new nonce")
	}
}
