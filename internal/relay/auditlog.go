package relay

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
)

// AuditLog is an append-only JSONL trade journal.
type AuditLog struct {
	mu   sync.Mutex
	file *os.File
}

// NewAuditLog opens (or creates) a JSONL audit log file with 0600 permissions.
func NewAuditLog(path string) (*AuditLog, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	return &AuditLog{file: f}, nil
}

// RecordTrade appends one JSON line per trade event.
func (a *AuditLog) RecordTrade(e TradeEvent) {
	data, err := json.Marshal(e)
	if err != nil {
		slog.Warn("audit log: marshal error", "error", err)
		return
	}
	data = append(data, '\n')

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.file.Write(data); err != nil {
		slog.Warn("audit log: write error", "error", err)
	}
}

// Close flushes and closes the audit log file.
func (a *AuditLog) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.file.Close()
}

// MultiRecorder fans out trade events to multiple recorders.
type MultiRecorder struct {
	recorders []TradeRecorder
}

// NewMultiRecorder creates a recorder that forwards to all given recorders.
func NewMultiRecorder(recorders ...TradeRecorder) *MultiRecorder {
	return &MultiRecorder{recorders: recorders}
}

// RecordTrade forwards the event to all wrapped recorders.
func (m *MultiRecorder) RecordTrade(e TradeEvent) {
	for _, r := range m.recorders {
		r.RecordTrade(e)
	}
}
