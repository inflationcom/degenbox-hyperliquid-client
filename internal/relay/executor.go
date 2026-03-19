package relay

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

func (c *Client) executeInstruction(instr *ExecutionInstruction) {
	slog.Info("executing instruction",
		"instruction_id", instr.InstructionID,
		"signal_id", instr.SignalID,
		"market", instr.Market,
		"steps", len(instr.Steps),
	)

	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	var results []StepResult

	for _, step := range instr.Steps {
		result := c.executeStep(ctx, instr.Market, step)
		results = append(results, result)

		if c.recorder != nil {
			c.recorder.RecordTrade(TradeEvent{
				Time:     time.Now(),
				Market:   instr.Market,
				Action:   step.Action,
				Success:  result.Success,
				Error:    result.Error,
				SignalID: instr.SignalID,
			})
		}

		if !result.Success {
			slog.Error("step failed",
				"market", instr.Market,
				"action", step.Action,
				"error", result.Error,
			)
			break
		}

		slog.Info("step ok",
			"market", instr.Market,
			"action", step.Action,
		)
	}

	c.sendJSON(instructionResultMsg{
		relayMsg:      relayMsg{Type: "instruction_result", Timestamp: time.Now().UnixMilli()},
		InstructionID: instr.InstructionID,
		Results:       results,
	})
}

func (c *Client) executeStep(ctx context.Context, market string, step ExecutionStep) StepResult {
	switch step.Action {
	case "place_order":
		return c.executePlaceOrder(ctx, market, step)
	case "update_leverage":
		return c.executeUpdateLeverage(ctx, step)
	case "cancel_by_cloid":
		return c.executeCancelByCloid(ctx, step)
	case "modify_order":
		return c.executeModifyOrder(ctx, step)
	default:
		return StepResult{
			Action:  step.Action,
			Success: false,
			Error:   "unknown step action: " + step.Action,
		}
	}
}

func (c *Client) executePlaceOrder(ctx context.Context, market string, step ExecutionStep) StepResult {
	grouping := hyperliquid.Grouping(step.Grouping)
	if grouping == "" {
		grouping = hyperliquid.GroupingNA
	}

	slog.Info("placing orders", "market", market, "count", len(step.Orders))

	resp, err := c.hlClient.PlaceOrder(ctx, step.Orders, grouping)
	if err != nil {
		return StepResult{Action: step.Action, Success: false, Error: err.Error()}
	}

	// Check individual order statuses — a batch can return "ok" at HTTP level
	// but have individual order errors (e.g. entry succeeds, SL fails)
	if resp != nil {
		for i, status := range resp.Statuses {
			if status.Error != "" {
				slog.Error("order failed within batch",
					"market", market,
					"order_index", i,
					"error", status.Error,
				)
				return StepResult{
					Action:   step.Action,
					Success:  false,
					Error:    fmt.Sprintf("order %d/%d failed: %s", i+1, len(resp.Statuses), status.Error),
					Response: resp,
				}
			}
		}
	}

	return StepResult{Action: step.Action, Success: true, Response: resp}
}

func (c *Client) executeUpdateLeverage(ctx context.Context, step ExecutionStep) StepResult {
	err := c.hlClient.UpdateLeverage(ctx, step.Asset, step.Leverage, step.IsCross)
	if err != nil {
		return StepResult{Action: step.Action, Success: false, Error: err.Error()}
	}
	return StepResult{Action: step.Action, Success: true}
}

func (c *Client) executeCancelByCloid(ctx context.Context, step ExecutionStep) StepResult {
	err := c.hlClient.CancelOrderByCloid(ctx, step.Cancels)
	if err != nil {
		return StepResult{Action: step.Action, Success: false, Error: err.Error()}
	}
	return StepResult{Action: step.Action, Success: true}
}

func (c *Client) executeModifyOrder(ctx context.Context, step ExecutionStep) StepResult {
	resp, err := c.hlClient.ModifyOrder(ctx, step.Modifications)
	if err != nil {
		return StepResult{Action: step.Action, Success: false, Error: err.Error()}
	}
	return StepResult{Action: step.Action, Success: true, Response: resp}
}
