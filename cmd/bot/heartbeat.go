package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

// Keep in sync with cmd/healthcheck/main.go.
var defaultHeartbeatPath = filepath.Join(os.TempDir(), "degenbox-heartbeat.json")

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

func writeHeartbeat(connected bool, state *hyperliquid.ClearinghouseState, spotUSDC float64) {
	hb := heartbeat{
		Timestamp: time.Now(),
		Version:   version,
		Connected: connected,
	}
	if state != nil {
		perpEquity, _ := strconv.ParseFloat(state.MarginSummary.AccountValue, 64)
		hb.Equity = fmt.Sprintf("%.2f", perpEquity+spotUSDC)
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
