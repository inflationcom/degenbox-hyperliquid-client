package config

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

type Config struct {
	Network     string `json:"network"`
	PrivateKey  string `json:"private_key"`
	WalletAddr  string `json:"wallet_addr"`
	IsAgentMode bool   `json:"is_agent_mode"`

	Relay      RelayConfig `json:"relay"`
	LogLevel   string      `json:"log_level"`
	LogFormat  string      `json:"log_format"`
	ClientName string      `json:"client_name"`
	RiskLimits RiskLimits  `json:"risk_limits"`
}

type RiskLimits struct {
	MaxLeverage      int     `json:"max_leverage"`
	MaxOrderSizeUSD  float64 `json:"max_order_size_usd"`
	MaxPriceDevPct   float64 `json:"max_price_dev_pct"`
	MaxOrdersPerStep int     `json:"max_orders_per_step"`
	MaxStepsPerInstr int     `json:"max_steps_per_instr"`
	MaxPerMinute     int     `json:"max_per_minute"`
}

type RelayConfig struct {
	ServerURL       string `json:"relay_url"`
	APIKey          string `json:"relay_api_key"`
	ClientID        int    `json:"relay_client_id"`
	ServerPublicKey string `json:"server_public_key"`
}

func DefaultConfig() *Config {
	return &Config{
		Network:   "testnet",
		LogLevel:  "info",
		LogFormat: "pretty",
	}
}

func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

func LoadFromEnv(cfg *Config) {
	if cfg == nil {
		return
	}

	if v := os.Getenv("HL_NETWORK"); v != "" {
		cfg.Network = v
	}
	if v := os.Getenv("HL_PRIVATE_KEY"); v != "" {
		cfg.PrivateKey = v
	}
	if v := os.Getenv("HL_WALLET_ADDR"); v != "" {
		cfg.WalletAddr = v
	}
	if v := os.Getenv("HL_AGENT_MODE"); v != "" {
		cfg.IsAgentMode = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("HL_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("HL_LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
	}
	if v := os.Getenv("HL_CLIENT_NAME"); v != "" {
		cfg.ClientName = v
	}

	if v := os.Getenv("HL_RELAY_URL"); v != "" {
		cfg.Relay.ServerURL = v
	}
	if v := os.Getenv("HL_RELAY_API_KEY"); v != "" {
		cfg.Relay.APIKey = v
	}
	if v := os.Getenv("HL_RELAY_CLIENT_ID"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Relay.ClientID = i
		}
	}
	if v := os.Getenv("HL_SERVER_PUBLIC_KEY"); v != "" {
		cfg.Relay.ServerPublicKey = v
	}

	if v := os.Getenv("HL_MAX_LEVERAGE"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.RiskLimits.MaxLeverage = i
		}
	}
	if v := os.Getenv("HL_MAX_ORDER_SIZE_USD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RiskLimits.MaxOrderSizeUSD = f
		}
	}
}

func (c *Config) Validate() error {
	if c.PrivateKey == "" {
		return fmt.Errorf("private_key is required")
	}

	if c.Network != "mainnet" && c.Network != "testnet" {
		return fmt.Errorf("network must be 'mainnet' or 'testnet', got '%s'", c.Network)
	}

	if c.IsAgentMode && c.WalletAddr == "" {
		return fmt.Errorf("wallet_addr is required in agent mode")
	}
	if c.WalletAddr != "" && !isValidHexAddress(c.WalletAddr) {
		return fmt.Errorf("wallet_addr must be a valid 0x-prefixed Ethereum address")
	}

	if err := c.RiskLimits.Validate(); err != nil {
		return err
	}

	return nil
}

func (r *RiskLimits) Validate() error {
	if r.MaxLeverage <= 0 {
		return fmt.Errorf("risk_limits.max_leverage must be set (run 'bot setup')")
	}
	if r.MaxOrderSizeUSD <= 0 {
		return fmt.Errorf("risk_limits.max_order_size_usd must be set (run 'bot setup')")
	}
	return nil
}

func (r *RiskLimits) ApplyDefaults() {
	if r.MaxPriceDevPct == 0 {
		r.MaxPriceDevPct = 5.0
	}
	if r.MaxOrdersPerStep == 0 {
		r.MaxOrdersPerStep = 10
	}
	if r.MaxStepsPerInstr == 0 {
		r.MaxStepsPerInstr = 8
	}
	if r.MaxPerMinute == 0 {
		r.MaxPerMinute = 30
	}
}

func (c *Config) GetNetwork() hyperliquid.Network {
	if c.Network == "mainnet" {
		return hyperliquid.Mainnet
	}
	return hyperliquid.Testnet
}

func (c *Config) SaveToFile(path string) error {
	safeCopy := *c
	safeCopy.PrivateKey = ""
	safeCopy.Relay.APIKey = ""

	data, err := json.MarshalIndent(safeCopy, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

// ResolveClientName returns the instance name: config/env override, or the working directory name.
func ResolveClientName(cfg *Config) string {
	if cfg.ClientName != "" {
		return cfg.ClientName
	}
	if cwd, err := os.Getwd(); err == nil {
		name := filepath.Base(cwd)
		if name != "." && name != "/" {
			return name
		}
	}
	return "DegenBox"
}

func isValidHexAddress(s string) bool {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return false
	}
	b, err := hex.DecodeString(s[2:])
	return err == nil && len(b) == 20
}
