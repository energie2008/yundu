package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	goredis "github.com/redis/go-redis/v9"
)

const (
	TopicUserBanned    = "user:banned"
	TopicUserUnbanned  = "user:unbanned"
	TopicTokenRevoked  = "token:revoked"
	TopicTrafficReset  = "user:traffic_reset"
	TopicPlanChanged   = "user:plan_changed"
	TopicConfigChanged = "node:config_changed"
)

type Event struct {
	Topic string          `json:"topic"`
	Data  json.RawMessage `json:"data"`
}

type UserEvent struct {
	UserID   string `json:"user_id"`
	Reason   string `json:"reason,omitempty"`
	Operator string `json:"operator,omitempty"`
}

type TokenEvent struct {
	UserID  string `json:"user_id"`
	TokenID string `json:"token_id,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type NodeConfigEvent struct {
	NodeID string `json:"node_id"`
	Action string `json:"action"`
}

type Bus struct {
	client *goredis.Client
	logger *slog.Logger
	mu     sync.RWMutex
	subs   map[string][]func(Event)
	stopCh chan struct{}
}

func NewBus(client *goredis.Client, logger *slog.Logger) *Bus {
	return &Bus{
		client: client,
		logger: logger.With("component", "events"),
		subs:   make(map[string][]func(Event)),
		stopCh: make(chan struct{}),
	}
}

func NewNopBus(logger *slog.Logger) *Bus {
	return &Bus{
		client: nil,
		logger: logger.With("component", "events"),
		subs:   make(map[string][]func(Event)),
		stopCh: make(chan struct{}),
	}
}

func (b *Bus) Publish(ctx context.Context, topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	evt := Event{Topic: topic, Data: data}

	b.mu.RLock()
	localHandlers := b.subs[topic]
	b.mu.RUnlock()
	for _, h := range localHandlers {
		go h(evt)
	}

	if b.client == nil {
		return nil
	}

	raw, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	if err := b.client.Publish(ctx, topic, raw).Err(); err != nil {
		return fmt.Errorf("publish %s: %w", topic, err)
	}
	b.logger.Debug("event published", "topic", topic)
	return nil
}

func (b *Bus) Subscribe(topic string, handler func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[topic] = append(b.subs[topic], handler)
}

func (b *Bus) Start(ctx context.Context) error {
	if b.client == nil {
		b.logger.Info("redis not configured, event bus running in local-only mode")
		return nil
	}

	b.mu.RLock()
	topics := make([]string, 0, len(b.subs))
	for t := range b.subs {
		topics = append(topics, t)
	}
	b.mu.RUnlock()

	if len(topics) == 0 {
		b.logger.Info("no event subscriptions, bus idle")
		return nil
	}

	b.logger.Info("subscribing to event topics", "topics", topics)
	sub := b.client.Subscribe(ctx, topics...)
	go func() {
		defer sub.Close()
		ch := sub.Channel()
		for {
			select {
			case <-b.stopCh:
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var evt Event
				if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
					b.logger.Warn("failed to unmarshal event", "error", err, "topic", msg.Channel)
					continue
				}
				b.dispatch(evt)
			}
		}
	}()
	return nil
}

func (b *Bus) dispatch(evt Event) {
	b.mu.RLock()
	handlers := b.subs[evt.Topic]
	b.mu.RUnlock()
	for _, h := range handlers {
		go h(evt)
	}
}

func (b *Bus) Stop() {
	close(b.stopCh)
}
