package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

// Keep in sync with cmd/healthcheck/main.go.
const defaultHeartbeatPath = "/tmp/degenbox-heartbeat.json"

type heartbeat struct {
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Connected bool      `json:"connected"`
	Equity    string    `json:"equity,omitempty"`
	Positions int       `json:"positions,omitempty"`
}

func heartbeatPath() string {
	if v := os.Getenv("HL_HEARTBEAT_PATH"); v != "" {
		return v
	}
	return defaultHeartbeatPath
}

func writeHeartbeat(connected bool, state *hyperliquid.ClearinghouseState) {
	hb := heartbeat{
		Timestamp: time.Now(),
		Version:   version,
		Connected: connected,
	}
	if state != nil {
		hb.Equity = state.MarginSummary.AccountValue
		hb.Positions = countPositions(state.AssetPositions)
	}

	data, err := json.Marshal(hb)
	if err != nil {
		return
	}
	if err := os.WriteFile(heartbeatPath(), data, 0600); err != nil {
		slog.Debug("heartbeat write failed", "error", err)
	}
}
