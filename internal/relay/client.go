package relay

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

type Config struct {
	ServerURL       string
	APIKey          string
	ClientID        int
	ServerPublicKey string
	Version         string
}

type Client struct {
	cfg      Config
	hlClient *hyperliquid.Client
	conn     *websocket.Conn
	connMu   sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	connected          atomic.Bool
	reconnectDelay     time.Duration
	maxReconnect       time.Duration
	consecutiveFailures int

	validator      *RiskValidator
	dedup          *InstructionDedup
	rateLimit      *rateLimiter
	serverPubKey   ed25519.PublicKey
	recorder       TradeRecorder
	onConfigUpdate func(ConfigUpdateMsg)
	onVersionInfo  func(VersionInfoMsg)
	execQueue      chan *ExecutionInstruction
}

func NewClient(cfg Config, hlClient *hyperliquid.Client, validator *RiskValidator, recorder TradeRecorder) (*Client, error) {
	c := &Client{
		cfg:            cfg,
		hlClient:       hlClient,
		validator:      validator,
		recorder:       recorder,
		dedup:          NewInstructionDedup(1000, 5*time.Minute),
		rateLimit:      newRateLimiter(validator.limits.MaxPerMinute),
		reconnectDelay: time.Second,
		maxReconnect:   30 * time.Second,
		execQueue:      make(chan *ExecutionInstruction, 16),
	}

	if cfg.ServerPublicKey != "" {
		pubBytes, err := hex.DecodeString(cfg.ServerPublicKey)
		if err != nil || len(pubBytes) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid server_public_key: must be 64-char hex (32 bytes)")
		}
		c.serverPubKey = ed25519.PublicKey(pubBytes)
		slog.Info("Ed25519 instruction verification enabled")
	} else {
		slog.Warn("no server_public_key configured, signatures unverified")
	}

	return c, nil
}

func (c *Client) Start(ctx context.Context) {
	c.ctx, c.cancel = context.WithCancel(ctx)

	c.wg.Add(2)
	go func() {
		defer c.wg.Done()
		c.connectLoop()
	}()
	go func() {
		defer c.wg.Done()
		c.execWorker()
	}()
}

// execWorker processes instructions sequentially to prevent concurrent
// execution races (e.g. two entries for the same market at once).
func (c *Client) execWorker() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case instr := <-c.execQueue:
			c.executeInstruction(instr)
		}
	}
}

func (c *Client) Connected() bool {
	return c.connected.Load()
}

func (c *Client) OnConfigUpdate(fn func(ConfigUpdateMsg)) {
	c.onConfigUpdate = fn
}

func (c *Client) OnVersionInfo(fn func(VersionInfoMsg)) {
	c.onVersionInfo = fn
}

func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		c.conn.Close()
	}
	c.connMu.Unlock()
	c.wg.Wait()
}

func (c *Client) connectLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if err := c.connect(); err != nil {
			c.consecutiveFailures++
			level := slog.LevelWarn
			if c.consecutiveFailures >= 100 {
				level = slog.LevelError
			}
			slog.Log(c.ctx, level, "relay connection failed",
				"error", err,
				"retry_in", c.reconnectDelay,
				"consecutive_failures", c.consecutiveFailures,
			)
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(c.reconnectDelay):
			}
			c.reconnectDelay *= 2
			if c.reconnectDelay > c.maxReconnect {
				c.reconnectDelay = c.maxReconnect
			}
			continue
		}

		if c.consecutiveFailures > 0 {
			slog.Info("relay reconnected after failures", "consecutive_failures", c.consecutiveFailures)
		}
		c.consecutiveFailures = 0
		c.reconnectDelay = time.Second

		refreshDone := make(chan struct{})
		go c.oracleRefreshLoop(refreshDone)

		c.readLoop()

		c.connected.Store(false)
		close(refreshDone)
		slog.Info("relay disconnected, reconnecting")
	}
}

func (c *Client) connect() error {
	if !strings.HasPrefix(c.cfg.ServerURL, "wss://") {
		return fmt.Errorf("relay: refusing insecure connection, URL must use wss://")
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(c.ctx, c.cfg.ServerURL, nil)
	if err != nil {
		return err
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	auth := authMsg{
		relayMsg: relayMsg{Type: "auth", Timestamp: time.Now().UnixMilli()},
		Role:     "client",
		APIKey:   c.cfg.APIKey,
		ClientID: c.cfg.ClientID,
		Version:  c.cfg.Version,
	}

	if err := c.sendJSON(auth); err != nil {
		conn.Close()
		return err
	}

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return err
	}
	_ = conn.SetReadDeadline(time.Time{})

	var result authResultMsg
	if err := json.Unmarshal(data, &result); err != nil {
		conn.Close()
		return err
	}

	if !result.Success {
		conn.Close()
		return &authError{msg: result.Error}
	}

	c.connected.Store(true)
	slog.Info("relay authenticated", "session_id", result.SessionID)
	return nil
}

func (c *Client) readLoop() {
	conn := c.conn

	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	done := make(chan struct{})
	go c.heartbeatLoop(done)

	defer func() {
		close(done)
		conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				slog.Warn("relay read error", "error", err)
			}
			return
		}

		msgType := parseType(data)
		switch msgType {
		case "instruction_relay":
			c.handleInstruction(data)

		case "ping":
			_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			_ = c.sendJSON(relayMsg{Type: "pong", Timestamp: time.Now().UnixMilli()})

		case "config_update":
			var msg struct {
				ConfigUpdateMsg
			}
			if err := json.Unmarshal(data, &msg); err == nil && c.onConfigUpdate != nil {
				c.onConfigUpdate(msg.ConfigUpdateMsg)
			}

		case "version_info":
			var msg struct {
				VersionInfoMsg
			}
			if err := json.Unmarshal(data, &msg); err == nil && c.onVersionInfo != nil {
				c.onVersionInfo(msg.VersionInfoMsg)
			}

		case "client_status", "auth_result":

		default:
			slog.Debug("unknown relay message", "type", msgType)
		}
	}
}

func (c *Client) handleInstruction(data []byte) {
	var msg instructionRelayMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("failed to parse instruction_relay", "error", err)
		return
	}
	instr := &msg.ExecutionInstruction

	if !c.dedup.Check(instr.InstructionID) {
		slog.Warn("duplicate instruction rejected", "id", instr.InstructionID)
		return
	}

	if msg.Timestamp <= 0 {
		slog.Warn("instruction rejected: missing timestamp", "id", instr.InstructionID)
		return
	}
	age := time.Now().UnixMilli() - msg.Timestamp
	if age > 60_000 || age < -10_000 {
		slog.Warn("stale instruction rejected", "id", instr.InstructionID, "age_ms", age)
		return
	}

	if c.serverPubKey != nil {
		if instr.Signature == "" {
			slog.Error("instruction REJECTED: missing signature", "id", instr.InstructionID)
			c.sendJSON(instructionResultMsg{
				relayMsg:      relayMsg{Type: "instruction_result", Timestamp: time.Now().UnixMilli()},
				InstructionID: instr.InstructionID,
				Results:       []StepResult{{Action: "verify_signature", Success: false, Error: "missing signature"}},
			})
			return
		}

		sigBytes, err := hex.DecodeString(instr.Signature)
		if err != nil {
			slog.Error("instruction REJECTED: invalid signature hex", "id", instr.InstructionID)
			return
		}

		canonical, err := instructionCanonical(instr)
		if err != nil {
			slog.Error("instruction REJECTED: canonical encoding failed", "id", instr.InstructionID, "error", err)
			c.sendJSON(instructionResultMsg{
				relayMsg:      relayMsg{Type: "instruction_result", Timestamp: time.Now().UnixMilli()},
				InstructionID: instr.InstructionID,
				Results:       []StepResult{{Action: "verify_signature", Success: false, Error: "canonical encoding failed"}},
			})
			return
		}
		if !ed25519.Verify(c.serverPubKey, canonical, sigBytes) {
			slog.Error("instruction REJECTED: signature verification failed", "id", instr.InstructionID)
			c.sendJSON(instructionResultMsg{
				relayMsg:      relayMsg{Type: "instruction_result", Timestamp: time.Now().UnixMilli()},
				InstructionID: instr.InstructionID,
				Results:       []StepResult{{Action: "verify_signature", Success: false, Error: "invalid signature"}},
			})
			return
		}

		slog.Debug("instruction signature verified", "id", instr.InstructionID)
	}

	if !c.rateLimit.Allow() {
		slog.Warn("rate limit exceeded", "id", instr.InstructionID)
		return
	}

	if err := c.validator.Validate(instr); err != nil {
		slog.Error("instruction REJECTED by risk validator",
			"id", instr.InstructionID, "reason", err)
		c.sendJSON(instructionResultMsg{
			relayMsg:      relayMsg{Type: "instruction_result", Timestamp: time.Now().UnixMilli()},
			InstructionID: instr.InstructionID,
			Results:       []StepResult{{Action: "validate", Success: false, Error: err.Error()}},
		})
		return
	}

	select {
	case c.execQueue <- instr:
	default:
		slog.Error("execution queue full, instruction dropped", "id", instr.InstructionID)
		c.sendJSON(instructionResultMsg{
			relayMsg:      relayMsg{Type: "instruction_result", Timestamp: time.Now().UnixMilli()},
			InstructionID: instr.InstructionID,
			Results:       []StepResult{{Action: "queue", Success: false, Error: "execution queue full"}},
		})
	}
}

func (c *Client) oracleRefreshLoop(done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.hlClient.RefreshAssets(c.ctx); err != nil {
				slog.Warn("oracle price refresh failed", "error", err)
			} else {
				slog.Debug("oracle prices refreshed")
			}
		}
	}
}

func (c *Client) heartbeatLoop(done <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.sendJSON(relayMsg{Type: "ping", Timestamp: time.Now().UnixMilli()})
		}
	}
}

func (c *Client) sendJSON(v any) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("relay: not connected")
	}
	return c.conn.WriteJSON(v)
}

func (c *Client) SendLog(msg clientLogMsg) {
	if !c.connected.Load() {
		return
	}
	_ = c.sendJSON(msg)
}

func (c *Client) NewLogHandler(level slog.Leveler) *WSLogHandler {
	return NewWSLogHandler(level, c)
}

type authError struct {
	msg string
}

func (e *authError) Error() string {
	return "relay auth failed: " + e.msg
}

type rateLimiter struct {
	mu     sync.Mutex
	count  int
	limit  int
	window time.Time
}

func newRateLimiter(perMinute int) *rateLimiter {
	if perMinute <= 0 {
		perMinute = 30
	}
	return &rateLimiter{
		limit:  perMinute,
		window: time.Now(),
	}
}

func (r *rateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.Sub(r.window) >= time.Minute {
		r.count = 0
		r.window = now
	}

	r.count++
	return r.count <= r.limit
}
