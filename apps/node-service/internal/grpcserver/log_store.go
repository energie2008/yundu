package grpcserver

import (
	"sync"
	"time"

	pb "github.com/airport-panel/proto/agent/v1"
)

const maxLogEntriesPerMachine = 2000

type machineLogBuffer struct {
	mu      sync.RWMutex
	entries []*pb.LogEntry
}

func newMachineLogBuffer() *machineLogBuffer {
	return &machineLogBuffer{
		entries: make([]*pb.LogEntry, 0, maxLogEntriesPerMachine),
	}
}

func (b *machineLogBuffer) append(entries ...*pb.LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, entries...)
	if len(b.entries) > maxLogEntriesPerMachine {
		b.entries = b.entries[len(b.entries)-maxLogEntriesPerMachine:]
	}
}

func (b *machineLogBuffer) query(since time.Time, level string, limit int) []*pb.LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	sinceMs := since.UnixMilli()
	var result []*pb.LogEntry
	for i := len(b.entries) - 1; i >= 0 && len(result) < limit; i-- {
		e := b.entries[i]
		if e.Timestamp < sinceMs {
			break
		}
		if level != "" && e.Level != level {
			continue
		}
		result = append(result, e)
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

type LogStore struct {
	mu       sync.RWMutex
	machines map[string]*machineLogBuffer
}

func NewLogStore() *LogStore {
	return &LogStore{
		machines: make(map[string]*machineLogBuffer),
	}
}

func (s *LogStore) Append(machineID string, entries ...*pb.LogEntry) {
	if len(entries) == 0 {
		return
	}
	s.mu.Lock()
	buf, ok := s.machines[machineID]
	if !ok {
		buf = newMachineLogBuffer()
		s.machines[machineID] = buf
	}
	s.mu.Unlock()
	buf.append(entries...)
}

func (s *LogStore) QueryRaw(machineID string, since time.Time, level string, limit int) []*pb.LogEntry {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	s.mu.RLock()
	buf, ok := s.machines[machineID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	return buf.query(since, level, limit)
}

func (s *LogStore) ListMachines() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.machines))
	for id := range s.machines {
		ids = append(ids, id)
	}
	return ids
}
