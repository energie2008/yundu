package transport

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/airport-panel/proto/agent/v1"
)

const (
	logBufferSize    = 500
	logFlushInterval = 2 * time.Second
	logMaxBatchSize  = 50
)

type LogSender interface {
	Send(msg *pb.AgentMessage) error
}

type LogCollector struct {
	mu      sync.Mutex
	buffer  []*pb.LogEntry
	sender  atomic.Value // LogSender
	nextSeq func() int64
	done    chan struct{}
	closed  atomic.Bool
	logger  *slog.Logger
}

func NewLogCollector(logger *slog.Logger) *LogCollector {
	return &LogCollector{
		buffer: make([]*pb.LogEntry, 0, logBufferSize),
		done:   make(chan struct{}),
		logger: logger.With("component", "log-collector"),
	}
}

func (c *LogCollector) SetSender(sender LogSender, nextSeq func() int64) {
	c.sender.Store(sender)
	c.nextSeq = nextSeq
}

func (c *LogCollector) AddLog(source, level, message string, labels map[string]string) {
	if c.closed.Load() {
		return
	}
	entry := &pb.LogEntry{
		Timestamp: time.Now().UnixMilli(),
		Source:    source,
		Level:     level,
		Message:   message,
		Labels:    labels,
	}
	c.mu.Lock()
	c.buffer = append(c.buffer, entry)
	if len(c.buffer) > logBufferSize {
		c.buffer = c.buffer[len(c.buffer)-logBufferSize:]
	}
	c.mu.Unlock()
}

func (c *LogCollector) Info(source, message string, labels map[string]string) {
	c.AddLog(source, "info", message, labels)
}

func (c *LogCollector) Warn(source, message string, labels map[string]string) {
	c.AddLog(source, "warn", message, labels)
}

func (c *LogCollector) Error(source, message string, labels map[string]string) {
	c.AddLog(source, "error", message, labels)
}

func (c *LogCollector) Start() {
	go c.flushLoop()
}

func (c *LogCollector) Stop() {
	c.closed.Store(true)
	close(c.done)
	c.Flush()
}

func (c *LogCollector) Flush() {
	senderVal := c.sender.Load()
	if senderVal == nil {
		return
	}
	sender := senderVal.(LogSender)
	if sender == nil || c.nextSeq == nil {
		return
	}

	c.mu.Lock()
	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}
	entries := make([]*pb.LogEntry, len(c.buffer))
	copy(entries, c.buffer)
	c.buffer = c.buffer[:0]
	c.mu.Unlock()

	for i := 0; i < len(entries); i += logMaxBatchSize {
		end := i + logMaxBatchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]
		msg := &pb.AgentMessage{
			Seq:       c.nextSeq(),
			Timestamp: time.Now().UnixMilli(),
			Payload: &pb.AgentMessage_LogChunk{
				LogChunk: &pb.LogChunk{
					Entries: batch,
				},
			},
		}
		if err := sender.Send(msg); err != nil {
			c.logger.Warn("failed to send log chunk", "entries", len(batch), "error", err)
			c.mu.Lock()
			c.buffer = append(entries[i:], c.buffer...)
			c.mu.Unlock()
			return
		}
	}
	c.logger.Debug("flushed log chunks", "entries", len(entries))
}

func (c *LogCollector) flushLoop() {
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.Flush()
		}
	}
}

type slogHandler struct {
	slog.Handler
	collector *LogCollector
	source    string
}

func (h *slogHandler) Handle(ctx context.Context, r slog.Record) error {
	level := "info"
	switch r.Level {
	case slog.LevelDebug:
		level = "debug"
	case slog.LevelWarn:
		level = "warn"
	case slog.LevelError:
		level = "error"
	}
	msg := r.Message
	r.Attrs(func(a slog.Attr) bool {
		msg += " " + a.Key + "=" + a.Value.String()
		return true
	})
	h.collector.AddLog(h.source, level, msg, nil)
	if h.Handler != nil {
		return h.Handler.Handle(ctx, r)
	}
	return nil
}

func (c *LogCollector) NewSlogHandler(wrapped slog.Handler, source string) slog.Handler {
	return &slogHandler{Handler: wrapped, collector: c, source: source}
}
