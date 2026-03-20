package relay

import (
	"context"
	"fmt"
	"testing"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/config"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockResolver implements AssetResolver for testing.
type mockResolver struct {
	assets     map[int]string // asset ID → name
	prices     map[string]string // market → oracle price
	leverages  map[string]int // market → max leverage
	state      *hyperliquid.ClearinghouseState
	stateErr   error
	testnet    bool
}

func (m *mockResolver) GetAssetName(id int) (string, error) {
	if name, ok := m.assets[id]; ok {
		return name, nil
	}
	return "", fmt.Errorf("unknown asset %d", id)
}

func (m *mockResolver) GetOraclePrice(market string) (string, error) {
	if px, ok := m.prices[market]; ok {
		return px, nil
	}
	return "", fmt.Errorf("no price for %s", market)
}

func (m *mockResolver) GetMaxLeverage(market string) (int, error) {
	if lev, ok := m.leverages[market]; ok {
		return lev, nil
	}
	return 0, fmt.Errorf("no leverage for %s", market)
}

func (m *mockResolver) GetClearinghouseState(_ context.Context) (*hyperliquid.ClearinghouseState, error) {
	if m.stateErr != nil {
		return nil, m.stateErr
	}
	return m.state, nil
}

func (m *mockResolver) IsTestnet() bool { return m.testnet }

func defaultMock() *mockResolver {
	return &mockResolver{
		assets:    map[int]string{0: "BTC", 1: "ETH"},
		prices:    map[string]string{"BTC": "60000", "ETH": "3000"},
		leverages: map[string]int{"BTC": 50, "ETH": 20},
	}
}

func defaultLimits() config.RiskLimits {
	l := config.RiskLimits{
		MaxLeverage:     20,
		MaxOrderSizeUSD: 10000,
	}
	l.ApplyDefaults()
	return l
}

func validOrder(asset int, price, size string) hyperliquid.OrderWire {
	return hyperliquid.OrderWire{
		A: asset, B: true, P: price, S: size, R: false,
		T: hyperliquid.OrderType{Limit: &hyperliquid.LimitSpec{Tif: "Ioc"}},
	}
}

func TestValidate_EmptySteps(t *testing.T) {
	v := NewRiskValidator(defaultLimits(), defaultMock())
	err := v.Validate(&ExecutionInstruction{InstructionID: "x", Market: "BTC"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty instruction")
}

func TestValidate_TooManySteps(t *testing.T) {
	limits := defaultLimits()
	limits.MaxStepsPerInstr = 2
	v := NewRiskValidator(limits, defaultMock())

	instr := &ExecutionInstruction{
		InstructionID: "x", Market: "BTC",
		Steps: []ExecutionStep{
			{Action: "update_leverage", Leverage: 5},
			{Action: "update_leverage", Leverage: 5},
			{Action: "update_leverage", Leverage: 5},
		},
	}
	err := v.Validate(instr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many steps")
}

func TestValidate_MaxLeverage(t *testing.T) {
	v := NewRiskValidator(defaultLimits(), defaultMock())

	t.Run("exceeds config limit", func(t *testing.T) {
		instr := &ExecutionInstruction{
			InstructionID: "x", Market: "BTC",
			Steps: []ExecutionStep{{Action: "update_leverage", Leverage: 50}},
		}
		err := v.Validate(instr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})

	t.Run("clamped to exchange max", func(t *testing.T) {
		mock := defaultMock()
		mock.leverages["ETH"] = 10
		limits := defaultLimits()
		limits.MaxLeverage = 20
		v := NewRiskValidator(limits, mock)

		instr := &ExecutionInstruction{
			InstructionID: "x", Market: "ETH",
			Steps: []ExecutionStep{{Action: "update_leverage", Asset: 1, Leverage: 15}},
		}
		err := v.Validate(instr)
		require.NoError(t, err)
		assert.Equal(t, 10, instr.Steps[0].Leverage) // clamped
	})

	t.Run("invalid leverage", func(t *testing.T) {
		instr := &ExecutionInstruction{
			InstructionID: "x", Market: "BTC",
			Steps: []ExecutionStep{{Action: "update_leverage", Leverage: 0}},
		}
		err := v.Validate(instr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid leverage")
	})
}

func TestValidate_MaxOrderSize(t *testing.T) {
	v := NewRiskValidator(defaultLimits(), defaultMock())

	t.Run("under limit", func(t *testing.T) {
		instr := &ExecutionInstruction{
			InstructionID: "x", Market: "BTC",
			Steps: []ExecutionStep{{
				Action: "place_order", Grouping: "na",
				Orders: []hyperliquid.OrderWire{validOrder(0, "60000", "0.1")}, // $6000
			}},
		}
		assert.NoError(t, v.Validate(instr))
	})

	t.Run("over limit", func(t *testing.T) {
		instr := &ExecutionInstruction{
			InstructionID: "x", Market: "BTC",
			Steps: []ExecutionStep{{
				Action: "place_order", Grouping: "na",
				Orders: []hyperliquid.OrderWire{validOrder(0, "60000", "1")}, // $60000
			}},
		}
		err := v.Validate(instr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})
}

func TestValidate_TriggerOrderNotional(t *testing.T) {
	v := NewRiskValidator(defaultLimits(), defaultMock())

	// Trigger order: order.P is slippage price (near 0), trigger price is the real one
	trigger := hyperliquid.OrderWire{
		A: 0, B: true, P: "1", S: "0.5", R: false,
		T: hyperliquid.OrderType{Trigger: &hyperliquid.TriggerWire{
			IsMarket: true, TriggerPx: "60000", TpSl: "tp",
		}},
	}
	instr := &ExecutionInstruction{
		InstructionID: "x", Market: "BTC",
		Steps: []ExecutionStep{{
			Action: "place_order", Grouping: "na",
			Orders: []hyperliquid.OrderWire{trigger}, // notional = 0.5 * 60000 = $30000
		}},
	}
	err := v.Validate(instr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds limit")
}

func TestValidate_PriceDeviation(t *testing.T) {
	limits := defaultLimits()
	limits.MaxPriceDevPct = 5.0
	v := NewRiskValidator(limits, defaultMock())

	t.Run("within deviation", func(t *testing.T) {
		instr := &ExecutionInstruction{
			InstructionID: "x", Market: "BTC",
			Steps: []ExecutionStep{{
				Action: "place_order", Grouping: "na",
				Orders: []hyperliquid.OrderWire{validOrder(0, "61000", "0.1")}, // ~1.7% dev
			}},
		}
		assert.NoError(t, v.Validate(instr))
	})

	t.Run("exceeds deviation", func(t *testing.T) {
		instr := &ExecutionInstruction{
			InstructionID: "x", Market: "BTC",
			Steps: []ExecutionStep{{
				Action: "place_order", Grouping: "na",
				Orders: []hyperliquid.OrderWire{validOrder(0, "70000", "0.1")}, // ~16.7% dev
			}},
		}
		err := v.Validate(instr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "deviates")
	})
}

func TestValidate_UnknownAsset(t *testing.T) {
	v := NewRiskValidator(defaultLimits(), defaultMock())

	instr := &ExecutionInstruction{
		InstructionID: "x", Market: "BTC",
		Steps: []ExecutionStep{{
			Action: "place_order", Grouping: "na",
			Orders: []hyperliquid.OrderWire{validOrder(99, "100", "1")},
		}},
	}
	err := v.Validate(instr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown asset")
}

func TestValidate_UnknownAction(t *testing.T) {
	v := NewRiskValidator(defaultLimits(), defaultMock())

	instr := &ExecutionInstruction{
		InstructionID: "x", Market: "BTC",
		Steps: []ExecutionStep{{Action: "explode"}},
	}
	err := v.Validate(instr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown action")
}

func TestValidate_TooManyOrders(t *testing.T) {
	limits := defaultLimits()
	limits.MaxOrdersPerStep = 2
	v := NewRiskValidator(limits, defaultMock())

	instr := &ExecutionInstruction{
		InstructionID: "x", Market: "BTC",
		Steps: []ExecutionStep{{
			Action: "place_order", Grouping: "na",
			Orders: []hyperliquid.OrderWire{
				validOrder(0, "60000", "0.01"),
				validOrder(0, "60000", "0.01"),
				validOrder(0, "60000", "0.01"),
			},
		}},
	}
	err := v.Validate(instr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many orders")
}

func TestValidatePortfolio_ExposureLimit(t *testing.T) {
	mock := defaultMock()
	mock.state = &hyperliquid.ClearinghouseState{
		MarginSummary: hyperliquid.MarginSummary{TotalNtlPos: "40000"},
		AssetPositions: []hyperliquid.AssetPosition{},
	}

	limits := defaultLimits()
	limits.MaxTotalExposureUSD = 50000

	v := NewRiskValidator(limits, mock)

	t.Run("under limit", func(t *testing.T) {
		assert.NoError(t, v.ValidatePortfolio(context.Background(), "BTC", 5000))
	})

	t.Run("over limit", func(t *testing.T) {
		err := v.ValidatePortfolio(context.Background(), "BTC", 15000)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})
}

func TestValidatePortfolio_PositionsPerMarket(t *testing.T) {
	mock := defaultMock()
	mock.state = &hyperliquid.ClearinghouseState{
		MarginSummary: hyperliquid.MarginSummary{TotalNtlPos: "10000"},
		AssetPositions: []hyperliquid.AssetPosition{
			{Position: hyperliquid.PositionData{Coin: "BTC", Szi: "0.5"}},
		},
	}

	limits := defaultLimits()
	limits.MaxPositionsPerMarket = 1

	v := NewRiskValidator(limits, mock)

	t.Run("blocked", func(t *testing.T) {
		err := v.ValidatePortfolio(context.Background(), "BTC", 5000)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already have")
	})

	t.Run("different market ok", func(t *testing.T) {
		assert.NoError(t, v.ValidatePortfolio(context.Background(), "ETH", 5000))
	})
}

func TestValidatePortfolio_FailOpen(t *testing.T) {
	mock := defaultMock()
	mock.stateErr = fmt.Errorf("API down")

	limits := defaultLimits()
	limits.MaxTotalExposureUSD = 50000

	v := NewRiskValidator(limits, mock)
	assert.NoError(t, v.ValidatePortfolio(context.Background(), "BTC", 100000))
}

func TestValidatePortfolio_NoLimitsConfigured(t *testing.T) {
	v := NewRiskValidator(defaultLimits(), defaultMock())
	assert.NoError(t, v.ValidatePortfolio(context.Background(), "BTC", 999999))
}
