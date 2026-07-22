package transport

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	pb "github.com/airport-panel/proto/agent/v1"
)

type Channel interface {
	Name() string
	Priority() int
	Connect(ctx context.Context) error
	Send(msg *pb.AgentMessage) error
	Recv() (<-chan *pb.PanelMessage, error)
	HealthCheck(ctx context.Context, timeout time.Duration) error
	Close() error
	IsConnected() bool
}

type ChannelStatus int

const (
	StatusDisconnected ChannelStatus = iota
	StatusConnecting
	StatusConnected
	StatusDegraded
)

const (
	defaultReconnectBase   = 1 * time.Second
	defaultReconnectMax    = 60 * time.Second
	defaultReconnectJitter = 0.2
)

type ChannelManager struct {
	mu               sync.RWMutex
	channels         []Channel
	active           Channel
	status           map[Channel]ChannelStatus
	failCount        map[Channel]int
	backoff          map[Channel]*backoffState
	sendCh           chan *pb.AgentMessage
	recvCh           chan *pb.PanelMessage
	ctx              context.Context
	cancel           context.CancelFunc
	healthCheck      time.Duration
	failThreshold    int
	upgradeInterval  time.Duration
	reconnectInterval time.Duration
	reconnectBase    time.Duration
	failFast         bool
	logger           interface{}
}

type backoffState struct {
	attempts    int
	nextRetryAt time.Time
}

type ManagerConfig struct {
	Channels         []Channel
	HealthInterval   time.Duration
	FailThreshold    int
	UpgradeEvery     time.Duration
	ReconnectEvery   time.Duration
	ReconnectBase    time.Duration
	FailFast         bool
	Logger           interface{}
}

func NewChannelManager(cfg ManagerConfig) *ChannelManager {
	if cfg.HealthInterval == 0 {
		cfg.HealthInterval = 20 * time.Second
	}
	if cfg.FailThreshold == 0 {
		cfg.FailThreshold = 3
	}
	if cfg.UpgradeEvery == 0 {
		cfg.UpgradeEvery = 60 * time.Second
	}
	if cfg.ReconnectEvery == 0 {
		cfg.ReconnectEvery = 5 * time.Second
	}
	if cfg.ReconnectBase == 0 {
		cfg.ReconnectBase = defaultReconnectBase
	}
	ctx, cancel := context.WithCancel(context.Background())
	cm := &ChannelManager{
		channels:          cfg.Channels,
		status:            make(map[Channel]ChannelStatus),
		failCount:         make(map[Channel]int),
		backoff:           make(map[Channel]*backoffState),
		sendCh:            make(chan *pb.AgentMessage, 64),
		recvCh:            make(chan *pb.PanelMessage, 64),
		ctx:               ctx,
		cancel:            cancel,
		healthCheck:       cfg.HealthInterval,
		failThreshold:     cfg.FailThreshold,
		upgradeInterval:   cfg.UpgradeEvery,
		reconnectInterval: cfg.ReconnectEvery,
		reconnectBase:     cfg.ReconnectBase,
		failFast:          cfg.FailFast,
		logger:            cfg.Logger,
	}
	for _, ch := range cfg.Channels {
		cm.status[ch] = StatusDisconnected
		cm.backoff[ch] = &backoffState{}
	}
	return cm
}

func (cm *ChannelManager) Start(ctx context.Context) error {
	if err := cm.connectBest(ctx); err != nil {
		if cm.failFast {
			return err
		}
		if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
			l.Warn("initial connect failed, will retry in background", "error", err)
		}
	}
	go cm.healthLoop()
	go cm.sendLoop()
	go cm.recvLoop()
	go cm.upgradeLoop()
	go cm.reconnectLoop()
	return nil
}

func (cm *ChannelManager) Send(msg *pb.AgentMessage) error {
	select {
	case cm.sendCh <- msg:
		return nil
	case <-cm.ctx.Done():
		return cm.ctx.Err()
	}
}

func (cm *ChannelManager) Recv() <-chan *pb.PanelMessage {
	return cm.recvCh
}

func (cm *ChannelManager) ActiveChannel() Channel {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.active
}

type HealthStatus struct {
	ActiveChannel string
	State         string
	FailCount     int
}

func (cm *ChannelManager) GetHealthStatus() HealthStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.active == nil {
		return HealthStatus{ActiveChannel: "unknown", State: "unhealthy"}
	}
	name := cm.active.Name()
	switch name {
	case "grpc", "GRPC":
		name = "grpc"
	case "ws", "websocket", "WebSocket":
		name = "ws"
	case "http", "HTTP":
		name = "http"
	}
	state := "healthy"
	if st, ok := cm.status[cm.active]; ok {
		switch st {
		case StatusConnected:
			state = "healthy"
		case StatusDegraded:
			state = "degraded"
		case StatusConnecting:
			state = "degraded"
		case StatusDisconnected:
			state = "unhealthy"
		}
	}
	return HealthStatus{
		ActiveChannel: name,
		State:         state,
		FailCount:     cm.failCount[cm.active],
	}
}

func (cm *ChannelManager) Stop() {
	cm.cancel()
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, ch := range cm.channels {
		ch.Close()
	}
}

func (cm *ChannelManager) SwitchChannel(target string) error {
	target = strings.ToLower(strings.TrimSpace(target))
	var targetCh Channel
	for _, ch := range cm.channels {
		if strings.ToLower(ch.Name()) == target {
			targetCh = ch
			break
		}
	}
	if targetCh == nil {
		return fmt.Errorf("unknown channel %q, available: grpc/ws/http", target)
	}

	cm.mu.RLock()
	alreadyActive := cm.active == targetCh
	cm.mu.RUnlock()
	if alreadyActive {
		if cm.status[targetCh] == StatusConnected {
			return nil
		}
	}

	if !targetCh.IsConnected() {
		if err := cm.tryConnect(cm.ctx, targetCh); err != nil {
			return fmt.Errorf("connect to channel %s: %w", target, err)
		}
	}

	cm.setActive(targetCh)
	if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
		l.Info("channel switched by panel command", "target", target)
	}
	return nil
}

func (cm *ChannelManager) connectBest(ctx context.Context) error {
	for _, ch := range cm.channels {
		if err := cm.tryConnect(ctx, ch); err == nil {
			cm.setActive(ch)
			return nil
		}
	}
	return ErrNoChannelAvailable
}

func (cm *ChannelManager) nextBackoff(ch Channel) time.Duration {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	base := cm.reconnectBase
	if base == 0 {
		base = defaultReconnectBase
	}
	bo, ok := cm.backoff[ch]
	if !ok {
		bo = &backoffState{}
		cm.backoff[ch] = bo
	}
	bo.attempts++
	d := base * time.Duration(1<<uint(bo.attempts-1))
	if d > defaultReconnectMax {
		d = defaultReconnectMax
	}
	jitter := time.Duration(float64(d) * defaultReconnectJitter * (rand.Float64()*2 - 1))
	d += jitter
	if d < 50*time.Millisecond {
		d = 50 * time.Millisecond
	}
	bo.nextRetryAt = time.Now().Add(d)
	return d
}

func (cm *ChannelManager) resetBackoff(ch Channel) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if bo, ok := cm.backoff[ch]; ok {
		bo.attempts = 0
		bo.nextRetryAt = time.Time{}
	}
}

func (cm *ChannelManager) canReconnect(ch Channel) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	bo, ok := cm.backoff[ch]
	if !ok {
		return true
	}
	if bo.attempts == 0 {
		return true
	}
	return time.Now().After(bo.nextRetryAt)
}

func (cm *ChannelManager) tryConnect(ctx context.Context, ch Channel) error {
	cm.setStatus(ch, StatusConnecting)
	if err := ch.Connect(ctx); err != nil {
		cm.setStatus(ch, StatusDisconnected)
		cm.nextBackoff(ch)
		return err
	}
	healthCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := ch.HealthCheck(healthCtx, 3*time.Second); err != nil {
		ch.Close()
		cm.setStatus(ch, StatusDisconnected)
		cm.nextBackoff(ch)
		return err
	}
	cm.setStatus(ch, StatusConnected)
	cm.failCount[ch] = 0
	cm.resetBackoff(ch)
	return nil
}

func (cm *ChannelManager) setActive(ch Channel) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.active != nil && cm.active != ch {
		cm.active.Close()
		cm.status[cm.active] = StatusDisconnected
	}
	cm.active = ch
	cm.status[ch] = StatusConnected
}

func (cm *ChannelManager) setStatus(ch Channel, status ChannelStatus) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.status[ch] = status
}

func (cm *ChannelManager) handleFailure(ch Channel) {
	cm.mu.Lock()
	cm.failCount[ch]++
	fails := cm.failCount[ch]
	cm.mu.Unlock()

	if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
		l.Warn("channel failure detected", "channel", ch.Name(), "fail_count", fails)
	}

	if fails >= cm.failThreshold {
		ch.Close()
		cm.mu.Lock()
		cm.status[ch] = StatusDisconnected
		cm.mu.Unlock()
		cm.nextBackoff(ch)
		if cm.active == ch {
			cm.failover()
		}
	}
}

func (cm *ChannelManager) failover() {
	for _, ch := range cm.channels {
		if ch == cm.active {
			continue
		}
		if cm.status[ch] == StatusConnected {
			cm.mu.Lock()
			cm.active = ch
			cm.mu.Unlock()
			if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
				l.Info("failover to connected channel", "channel", ch.Name())
			}
			return
		}
	}
	for _, ch := range cm.channels {
		if ch == cm.active {
			continue
		}
		if !cm.canReconnect(ch) {
			continue
		}
		if err := cm.tryConnect(cm.ctx, ch); err == nil {
			cm.mu.Lock()
			cm.active = ch
			cm.mu.Unlock()
			if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
				l.Info("failover to reconnected channel", "channel", ch.Name())
			}
			return
		}
	}
	if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
		l.Error("all channels unavailable, waiting for reconnect loop")
	}
}

func (cm *ChannelManager) healthLoop() {
	ticker := time.NewTicker(cm.healthCheck)
	defer ticker.Stop()
	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			cm.mu.RLock()
			active := cm.active
			cm.mu.RUnlock()
			if active == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(cm.ctx, 3*time.Second)
			if err := active.HealthCheck(ctx, 3*time.Second); err != nil {
				cancel()
				cm.handleFailure(active)
			} else {
				cancel()
				cm.mu.Lock()
				cm.failCount[active] = 0
				cm.mu.Unlock()
			}
		}
	}
}

func (cm *ChannelManager) reconnectLoop() {
	ticker := time.NewTicker(cm.reconnectInterval)
	defer ticker.Stop()
	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			cm.reconnectDisconnected()
		}
	}
}

func (cm *ChannelManager) reconnectDisconnected() {
	cm.mu.RLock()
	active := cm.active
	cm.mu.RUnlock()

	for _, ch := range cm.channels {
		if ch.IsConnected() {
			continue
		}
		if cm.status[ch] == StatusConnecting {
			continue
		}
		if !cm.canReconnect(ch) {
			continue
		}
		if active != nil && ch.Priority() >= active.Priority() && !ch.IsConnected() {
			if active.IsConnected() {
				continue
			}
		}
		if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
			l.Info("attempting reconnect", "channel", ch.Name())
		}
		if err := cm.tryConnect(cm.ctx, ch); err != nil {
			if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
				cm.mu.RLock()
				bo := cm.backoff[ch]
				nextIn := time.Until(bo.nextRetryAt)
				cm.mu.RUnlock()
				l.Warn("reconnect failed", "channel", ch.Name(), "error", err, "next_retry_in", nextIn)
			}
			continue
		}
		if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
			l.Info("channel reconnected", "channel", ch.Name())
		}
		if active == nil || !active.IsConnected() {
			cm.setActive(ch)
		}
	}
}

func (cm *ChannelManager) upgradeLoop() {
	ticker := time.NewTicker(cm.upgradeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			cm.tryUpgrade()
		}
	}
}

func (cm *ChannelManager) tryUpgrade() {
	cm.mu.RLock()
	active := cm.active
	cm.mu.RUnlock()

	priority := 0
	if active != nil {
		priority = active.Priority()
	}
	for _, ch := range cm.channels {
		if ch.Priority() >= priority {
			continue
		}
		if ch.IsConnected() {
			cm.setActive(ch)
			if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
				l.Info("upgraded to better channel", "channel", ch.Name())
			}
			return
		}
		if !cm.canReconnect(ch) {
			continue
		}
		if err := cm.tryConnect(cm.ctx, ch); err == nil {
			cm.setActive(ch)
			if l, ok := cm.logger.(*slog.Logger); ok && l != nil {
				l.Info("upgraded to better channel (reconnected)", "channel", ch.Name())
			}
			return
		}
	}
}

func (cm *ChannelManager) sendLoop() {
	for {
		select {
		case <-cm.ctx.Done():
			return
		case msg := <-cm.sendCh:
			cm.mu.RLock()
			active := cm.active
			cm.mu.RUnlock()
			if active == nil {
				select {
				case <-cm.ctx.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}
			if err := active.Send(msg); err != nil {
				cm.handleFailure(active)
			}
		}
	}
}

func (cm *ChannelManager) recvLoop() {
	for {
		select {
		case <-cm.ctx.Done():
			return
		default:
		}
		cm.mu.RLock()
		active := cm.active
		cm.mu.RUnlock()
		if active == nil {
			select {
			case <-cm.ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		msgCh, err := active.Recv()
		if err != nil {
			cm.handleFailure(active)
			select {
			case <-cm.ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		select {
		case <-cm.ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				cm.handleFailure(active)
				select {
				case <-cm.ctx.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}
			select {
			case cm.recvCh <- msg:
			case <-cm.ctx.Done():
				return
			}
		}
	}
}
