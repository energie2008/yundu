package transport

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "github.com/airport-panel/proto/agent/v1"
)

type mockChannel struct {
	mu            sync.Mutex
	name          string
	priority      int
	connected     bool
	healthErr     error
	connectErr    error
	sendErr       error
	sentMessages  []*pb.AgentMessage
	recvCh        chan *pb.PanelMessage
	connectCount  int
	healthCount   int
	closeCount    int
	healthLatency time.Duration
}

func newMockChannel(name string, priority int) *mockChannel {
	return &mockChannel{
		name:     name,
		priority: priority,
		recvCh:   make(chan *pb.PanelMessage, 10),
	}
}

func (m *mockChannel) Name() string    { return m.name }
func (m *mockChannel) Priority() int   { return m.priority }
func (m *mockChannel) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockChannel) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectCount++
	if m.connectErr != nil {
		return m.connectErr
	}
	m.connected = true
	return nil
}

func (m *mockChannel) Send(msg *pb.AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	if !m.connected {
		return ErrNotConnected
	}
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func (m *mockChannel) Recv() (<-chan *pb.PanelMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return nil, ErrNotConnected
	}
	return m.recvCh, nil
}

func (m *mockChannel) HealthCheck(ctx context.Context, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthCount++
	if m.healthLatency > 0 {
		select {
		case <-time.After(m.healthLatency):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.healthErr
}

func (m *mockChannel) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCount++
	m.connected = false
	return nil
}

func (m *mockChannel) injectRecv(msg *pb.PanelMessage) {
	m.recvCh <- msg
}

func (m *mockChannel) setHealthErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthErr = err
}

func TestChannelManager_ConnectBest(t *testing.T) {
	grpc := newMockChannel("grpc", 0)
	ws := newMockChannel("ws", 1)
	http := newMockChannel("http", 2)

	cm := NewChannelManager(ManagerConfig{
		Channels:      []Channel{grpc, ws, http},
		HealthInterval: 50 * time.Millisecond,
		FailThreshold: 2,
		UpgradeEvery:  100 * time.Millisecond,
	})
	ctx := context.Background()
	if err := cm.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cm.Stop()

	active := cm.ActiveChannel()
	if active.Name() != "grpc" {
		t.Errorf("expected active channel grpc, got %s", active.Name())
	}
	if !grpc.IsConnected() {
		t.Error("expected grpc to be connected")
	}
}

func TestChannelManager_Failover(t *testing.T) {
	grpc := newMockChannel("grpc", 0)
	ws := newMockChannel("ws", 1)
	http := newMockChannel("http", 2)

	cm := NewChannelManager(ManagerConfig{
		Channels:      []Channel{grpc, ws, http},
		HealthInterval: 30 * time.Millisecond,
		FailThreshold: 2,
		UpgradeEvery:  500 * time.Millisecond,
	})
	ctx := context.Background()
	if err := cm.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cm.Stop()

	if cm.ActiveChannel().Name() != "grpc" {
		t.Fatal("expected grpc active initially")
	}

	grpc.setHealthErr(ErrHealthCheckFailed)

	time.Sleep(200 * time.Millisecond)

	active := cm.ActiveChannel()
	if active.Name() != "ws" {
		t.Errorf("expected failover to ws after grpc failures, got %s", active.Name())
	}
}

func TestChannelManager_GRPCHandshakeOKButStreamTimeout(t *testing.T) {
	grpc := newMockChannel("grpc", 0)
	grpc.healthLatency = 5 * time.Second
	ws := newMockChannel("ws", 1)
	http := newMockChannel("http", 2)

	cm := NewChannelManager(ManagerConfig{
		Channels:      []Channel{grpc, ws, http},
		HealthInterval: 50 * time.Millisecond,
		FailThreshold: 1,
		UpgradeEvery:  500 * time.Millisecond,
	})
	ctx := context.Background()
	if err := cm.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cm.Stop()

	time.Sleep(150 * time.Millisecond)

	active := cm.ActiveChannel()
	if active.Name() == "grpc" {
		t.Error("expected failover from grpc due to health check timeout, but grpc still active")
	}
}

func TestChannelManager_UpgradeAfterRecovery(t *testing.T) {
	grpc := newMockChannel("grpc", 0)
	grpc.connectErr = context.DeadlineExceeded
	ws := newMockChannel("ws", 1)
	http := newMockChannel("http", 2)

	cm := NewChannelManager(ManagerConfig{
		Channels:      []Channel{grpc, ws, http},
		HealthInterval: 30 * time.Millisecond,
		FailThreshold: 1,
		UpgradeEvery:  80 * time.Millisecond,
		ReconnectBase: 50 * time.Millisecond,
	})
	ctx := context.Background()
	if err := cm.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cm.Stop()

	if cm.ActiveChannel().Name() != "ws" {
		t.Fatalf("expected ws active when grpc fails to connect, got %s", cm.ActiveChannel().Name())
	}

	grpc.mu.Lock()
	grpc.connectErr = nil
	grpc.mu.Unlock()

	time.Sleep(500 * time.Millisecond)

	active := cm.ActiveChannel()
	if active.Name() != "grpc" {
		t.Errorf("expected upgrade back to grpc after recovery, got %s", active.Name())
	}
}

func TestChannelManager_AllChannelsFail(t *testing.T) {
	grpc := newMockChannel("grpc", 0)
	grpc.connectErr = context.DeadlineExceeded
	ws := newMockChannel("ws", 1)
	ws.connectErr = context.DeadlineExceeded
	http := newMockChannel("http", 2)
	http.connectErr = context.DeadlineExceeded

	cm := NewChannelManager(ManagerConfig{
		Channels:      []Channel{grpc, ws, http},
		HealthInterval: 50 * time.Millisecond,
		FailThreshold: 1,
		UpgradeEvery:  100 * time.Millisecond,
		ReconnectBase: 50 * time.Millisecond,
		FailFast:      true,
	})
	ctx := context.Background()
	err := cm.Start(ctx)
	if err == nil {
		cm.Stop()
		t.Fatal("expected error when all channels fail")
	}
	if err != ErrNoChannelAvailable {
		t.Errorf("expected ErrNoChannelAvailable, got %v", err)
	}
}

func TestChannelManager_SendRecv(t *testing.T) {
	grpc := newMockChannel("grpc", 0)
	cm := NewChannelManager(ManagerConfig{
		Channels:      []Channel{grpc},
		HealthInterval: 100 * time.Millisecond,
		FailThreshold: 3,
		UpgradeEvery:  500 * time.Millisecond,
	})
	ctx := context.Background()
	if err := cm.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cm.Stop()

	testMsg := &pb.AgentMessage{Seq: 1}
	if err := cm.Send(testMsg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	grpc.mu.Lock()
	if len(grpc.sentMessages) != 1 {
		t.Errorf("expected 1 sent message, got %d", len(grpc.sentMessages))
	}
	grpc.mu.Unlock()

	panelMsg := &pb.PanelMessage{Seq: 1}
	grpc.injectRecv(panelMsg)

	select {
	case received := <-cm.Recv():
		if received.Seq != 1 {
			t.Errorf("expected seq 1, got %d", received.Seq)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for recv message")
	}
}
