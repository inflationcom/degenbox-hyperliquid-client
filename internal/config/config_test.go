package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "testnet", cfg.Network)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "pretty", cfg.LogFormat)
}

func TestRiskLimitsApplyDefaults(t *testing.T) {
	rl := RiskLimits{}
	rl.ApplyDefaults()

	assert.Equal(t, 5.0, rl.MaxPriceDevPct)
	assert.Equal(t, 10, rl.MaxOrdersPerStep)
	assert.Equal(t, 8, rl.MaxStepsPerInstr)
	assert.Equal(t, 30, rl.MaxPerMinute)
}

func TestRiskLimitsValidate(t *testing.T) {
	t.Run("missing leverage", func(t *testing.T) {
		rl := RiskLimits{MaxOrderSizeUSD: 1000}
		err := rl.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_leverage")
	})

	t.Run("missing order size", func(t *testing.T) {
		rl := RiskLimits{MaxLeverage: 10}
		err := rl.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_order_size_usd")
	})

	t.Run("valid", func(t *testing.T) {
		rl := RiskLimits{MaxLeverage: 10, MaxOrderSizeUSD: 1000}
		assert.NoError(t, rl.Validate())
	})
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	content := `{"network": "mainnet", "log_level": "debug", "risk_limits": {"max_leverage": 20, "max_order_size_usd": 5000}}`
	err := os.WriteFile(path, []byte(content), 0600)
	require.NoError(t, err)

	cfg, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "mainnet", cfg.Network)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 20, cfg.RiskLimits.MaxLeverage)
	assert.Equal(t, 5000.0, cfg.RiskLimits.MaxOrderSizeUSD)
}

func TestLoadFromEnv(t *testing.T) {
	cfg := DefaultConfig()

	t.Setenv("HL_NETWORK", "mainnet")
	LoadFromEnv(cfg)
	assert.Equal(t, "mainnet", cfg.Network)
}

func TestSaveToFileRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.PrivateKey = "secret-key"
	cfg.Relay.APIKey = "secret-api-key"

	err := cfg.SaveToFile(path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret-key")
	assert.NotContains(t, string(data), "secret-api-key")
}

func TestPortfolioRiskLimitsInConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	content := `{"risk_limits": {"max_leverage": 10, "max_order_size_usd": 1000, "max_total_exposure_usd": 50000, "max_positions_per_market": 2}}`
	err := os.WriteFile(path, []byte(content), 0600)
	require.NoError(t, err)

	cfg, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, 50000.0, cfg.RiskLimits.MaxTotalExposureUSD)
	assert.Equal(t, 2, cfg.RiskLimits.MaxPositionsPerMarket)
}
