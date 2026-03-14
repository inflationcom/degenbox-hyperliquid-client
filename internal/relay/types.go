package relay

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

// TradeRecorder receives trade execution events for external consumption (e.g. TUI display).
type TradeRecorder interface {
	RecordTrade(TradeEvent)
}

type TradeEvent struct {
	Time     time.Time
	Market   string
	Action   string
	Success  bool
	Error    string
	SignalID string
}

type relayMsg struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

type authMsg struct {
	relayMsg
	Role     string `json:"role"`
	APIKey   string `json:"api_key"`
	ClientID int    `json:"client_id,omitempty"`
	Version  string `json:"version,omitempty"`
}

type authResultMsg struct {
	relayMsg
	Success         bool     `json:"success"`
	Error           string   `json:"error,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	Callers         []string `json:"callers,omitempty"`
	CopytradeTarget string   `json:"copytrade_target,omitempty"`
}

type ExecutionInstruction struct {
	InstructionID string          `json:"instruction_id"`
	SignalID      string          `json:"signal_id,omitempty"`
	IntentID      string          `json:"intent_id,omitempty"`
	Market        string          `json:"market"`
	Steps         []ExecutionStep `json:"steps"`
	Signature     string          `json:"signature,omitempty"` // Ed25519 hex signature from server
}

type ExecutionStep struct {
	Action string `json:"action"`

	Orders        []hyperliquid.OrderWire      `json:"orders,omitempty"`
	Grouping      string                       `json:"grouping,omitempty"`
	Cancels       []hyperliquid.CancelCloidSpec `json:"cancels,omitempty"`
	Asset         int                          `json:"asset,omitempty"`
	Leverage      int                          `json:"leverage,omitempty"`
	IsCross       bool                         `json:"is_cross,omitempty"`
	Modifications []hyperliquid.ModifySpec      `json:"modifications,omitempty"`
}

type instructionRelayMsg struct {
	relayMsg
	ExecutionInstruction
}

type StepResult struct {
	Action   string                    `json:"action"`
	Success  bool                      `json:"success"`
	Response *hyperliquid.OrderResponse `json:"response,omitempty"`
	Error    string                    `json:"error,omitempty"`
}

type instructionResultMsg struct {
	relayMsg
	InstructionID string       `json:"instruction_id"`
	Results       []StepResult `json:"results"`
}

type clientLogMsg struct {
	relayMsg
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Attrs   map[string]string `json:"attrs,omitempty"`
}

type ConfigUpdateMsg struct {
	Name   string `json:"name,omitempty"`
	Paused *bool  `json:"paused,omitempty"`
}

type VersionInfoMsg struct {
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
}

type AuthInfoMsg struct {
	Callers         []string `json:"callers"`
	CopytradeTarget string   `json:"copytrade_target"`
}

type AssignmentUpdateMsg struct {
	SourceType      *string `json:"source_type"`
	CallerName      *string `json:"caller_name"`
	CopytradeTarget *string `json:"copytrade_target"`
}

func parseType(data []byte) string {
	var m struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		slog.Warn("relay: failed to parse message type", "error", err)
	}
	return m.Type
}
