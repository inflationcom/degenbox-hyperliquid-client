package main

import (
	"sync"
	"time"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/relay"
)

type TradeRecord struct {
	Time     time.Time
	Market   string
	Action   string
	Success  bool
	Error    string
	SignalID string
}

type TradeStore struct {
	mu      sync.Mutex
	records []TradeRecord
	cap     int
	cursor  int
	count   int
}

func NewTradeStore(capacity int) *TradeStore {
	return &TradeStore{
		records: make([]TradeRecord, capacity),
		cap:     capacity,
	}
}

func (s *TradeStore) Add(r TradeRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[s.cursor] = r
	s.cursor = (s.cursor + 1) % s.cap
	if s.count < s.cap {
		s.count++
	}
}

// RecordTrade implements relay.TradeRecorder.
func (s *TradeStore) RecordTrade(e relay.TradeEvent) {
	s.Add(TradeRecord{
		Time:     e.Time,
		Market:   e.Market,
		Action:   e.Action,
		Success:  e.Success,
		Error:    e.Error,
		SignalID: e.SignalID,
	})
}

func (s *TradeStore) Recent(n int) []TradeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	if n > s.count {
		n = s.count
	}
	if n == 0 {
		return nil
	}

	result := make([]TradeRecord, n)
	start := (s.cursor - n + s.cap) % s.cap
	for i := 0; i < n; i++ {
		result[i] = s.records[(start+i)%s.cap]
	}
	return result
}
