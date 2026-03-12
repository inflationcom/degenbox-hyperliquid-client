package relay

import (
	"fmt"
	"math"
	"strconv"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/config"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

type RiskValidator struct {
	limits   config.RiskLimits
	hlClient *hyperliquid.Client
}

func NewRiskValidator(limits config.RiskLimits, hlClient *hyperliquid.Client) *RiskValidator {
	return &RiskValidator{limits: limits, hlClient: hlClient}
}

func (v *RiskValidator) Validate(instr *ExecutionInstruction) error {
	if len(instr.Steps) == 0 {
		return fmt.Errorf("empty instruction (no steps)")
	}
	if len(instr.Steps) > v.limits.MaxStepsPerInstr {
		return fmt.Errorf("too many steps: %d (max %d)", len(instr.Steps), v.limits.MaxStepsPerInstr)
	}

	for i, step := range instr.Steps {
		if err := v.validateStep(instr.Market, step); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, step.Action, err)
		}
	}
	return nil
}

func (v *RiskValidator) validateStep(market string, step ExecutionStep) error {
	switch step.Action {
	case "update_leverage":
		return v.validateLeverage(market, step)
	case "place_order":
		return v.validatePlaceOrder(step)
	case "cancel_by_cloid":
		if len(step.Cancels) == 0 {
			return fmt.Errorf("empty cancels list")
		}
		return nil
	case "modify_order":
		for j, mod := range step.Modifications {
			if err := v.validateOrder(mod.Order); err != nil {
				return fmt.Errorf("modification %d: %w", j, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown action: %s", step.Action)
	}
}

func (v *RiskValidator) validateLeverage(market string, step ExecutionStep) error {
	if step.Leverage <= 0 {
		return fmt.Errorf("invalid leverage: %d", step.Leverage)
	}
	if step.Leverage > v.limits.MaxLeverage {
		return fmt.Errorf("leverage %d exceeds limit %d", step.Leverage, v.limits.MaxLeverage)
	}
	if market != "" {
		if maxLev, err := v.hlClient.GetMaxLeverage(market); err == nil {
			if step.Leverage > maxLev {
				return fmt.Errorf("leverage %d exceeds exchange max %d for %s", step.Leverage, maxLev, market)
			}
		}
	}
	return nil
}

func (v *RiskValidator) validatePlaceOrder(step ExecutionStep) error {
	if len(step.Orders) == 0 {
		return fmt.Errorf("empty orders list")
	}
	if len(step.Orders) > v.limits.MaxOrdersPerStep {
		return fmt.Errorf("too many orders: %d (max %d)", len(step.Orders), v.limits.MaxOrdersPerStep)
	}
	for j, order := range step.Orders {
		if err := v.validateOrder(order); err != nil {
			return fmt.Errorf("order %d: %w", j, err)
		}
	}
	return nil
}

func (v *RiskValidator) validateOrder(order hyperliquid.OrderWire) error {
	market, err := v.hlClient.GetAssetName(order.A)
	if err != nil {
		return fmt.Errorf("unknown asset ID %d", order.A)
	}

	price, err := strconv.ParseFloat(order.P, 64)
	if err != nil || price <= 0 {
		return fmt.Errorf("invalid price: %s", order.P)
	}

	size, err := strconv.ParseFloat(order.S, 64)
	if err != nil || size <= 0 {
		return fmt.Errorf("invalid size: %s", order.S)
	}

	notional := size * price
	if notional > v.limits.MaxOrderSizeUSD {
		return fmt.Errorf("order notional $%.0f exceeds limit $%.0f", notional, v.limits.MaxOrderSizeUSD)
	}

	if order.T.Trigger == nil {
		oraclePxStr, oracleErr := v.hlClient.GetOraclePrice(market)
		if oracleErr == nil {
			oraclePx, parseErr := strconv.ParseFloat(oraclePxStr, 64)
			if parseErr == nil && oraclePx > 0 {
				deviation := math.Abs(price-oraclePx) / oraclePx * 100
				if deviation > v.limits.MaxPriceDevPct {
					hint := ""
					if v.hlClient.IsTestnet() {
						hint = " — TESTNET SLIPPAGE: mid price diverges from oracle due to thin liquidity"
					}
					return fmt.Errorf("price %s deviates %.1f%% from oracle %s (max %.1f%%)%s",
						order.P, deviation, oraclePxStr, v.limits.MaxPriceDevPct, hint)
				}
			}
		}
	}

	return nil
}
