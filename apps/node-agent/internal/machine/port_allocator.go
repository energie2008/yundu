package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type allocationRecord struct {
	ServerCode string `json:"server_code"`
	Port       int    `json:"port"`
	AllocatedAt int64  `json:"allocated_at"`
}

type APIPortAllocator struct {
	mu        sync.Mutex
	baseDir   string
	poolName  string
	portStart int
	portEnd   int
	filePath  string
	records   map[string]allocationRecord
}

func NewAPIPortAllocator(baseDir, poolName string, portStart, portEnd int) *APIPortAllocator {
	os.MkdirAll(baseDir, 0755)
	a := &APIPortAllocator{
		baseDir:   baseDir,
		poolName:  poolName,
		portStart: portStart,
		portEnd:   portEnd,
		filePath:  filepath.Join(baseDir, fmt.Sprintf("port_allocations_%s.json", poolName)),
		records:   make(map[string]allocationRecord),
	}
	a.load()
	return a
}

func (a *APIPortAllocator) load() {
	data, err := os.ReadFile(a.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
		return
	}
	var records []allocationRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return
	}
	for _, r := range records {
		if r.Port >= a.portStart && r.Port <= a.portEnd {
			a.records[r.ServerCode] = r
		}
	}
}

func (a *APIPortAllocator) persist() error {
	records := make([]allocationRecord, 0, len(a.records))
	for _, r := range a.records {
		records = append(records, r)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Port < records[j].Port
	})
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := a.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, a.filePath)
}

func (a *APIPortAllocator) Allocate(serverCode string) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if rec, ok := a.records[serverCode]; ok {
		return rec.Port, nil
	}

	usedPorts := make(map[int]bool)
	for _, r := range a.records {
		usedPorts[r.Port] = true
	}

	for port := a.portStart; port <= a.portEnd; port++ {
		if !usedPorts[port] {
			a.records[serverCode] = allocationRecord{
				ServerCode:  serverCode,
				Port:        port,
				AllocatedAt: 0,
			}
			if err := a.persist(); err != nil {
				delete(a.records, serverCode)
				return 0, fmt.Errorf("persist port allocation: %w", err)
			}
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", a.portStart, a.portEnd)
}

func (a *APIPortAllocator) Release(serverCode string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.records[serverCode]; !ok {
		return nil
	}
	delete(a.records, serverCode)
	return a.persist()
}

func (a *APIPortAllocator) GetAllocated(serverCode string) (int, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rec, ok := a.records[serverCode]
	if !ok {
		return 0, false
	}
	return rec.Port, true
}
