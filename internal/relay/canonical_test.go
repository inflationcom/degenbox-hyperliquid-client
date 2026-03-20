package relay

import (
	"testing"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonical_Deterministic(t *testing.T) {
	instr := &ExecutionInstruction{
		InstructionID: "test-123",
		Market:        "BTC",
		SignalID:      "sig-1",
		Steps: []ExecutionStep{
			{Action: "update_leverage", Asset: 0, Leverage: 10},
		},
	}

	out1, err := instructionCanonical(instr)
	require.NoError(t, err)
	out2, err := instructionCanonical(instr)
	require.NoError(t, err)
	assert.Equal(t, string(out1), string(out2))
}

func TestCanonical_SortedKeys(t *testing.T) {
	instr := &ExecutionInstruction{
		InstructionID: "test-123",
		Market:        "ETH",
		SignalID:      "sig-1",
		Steps: []ExecutionStep{
			{Action: "place_order", Orders: []hyperliquid.OrderWire{
				{A: 1, B: true, P: "100", S: "0.1", R: false, T: hyperliquid.OrderType{Limit: &hyperliquid.LimitSpec{Tif: "Ioc"}}},
			}, Grouping: "na"},
		},
	}

	out, err := instructionCanonical(instr)
	require.NoError(t, err)
	s := string(out)

	// Keys should be alphabetically sorted
	assert.Contains(t, s, `"instruction_id"`)
	assert.Contains(t, s, `"market"`)
	assert.Contains(t, s, `"steps"`)
}

func TestCanonical_OptionalFieldsOmitted(t *testing.T) {
	instr := &ExecutionInstruction{
		InstructionID: "test-123",
		Market:        "SOL",
		Steps: []ExecutionStep{
			{Action: "update_leverage", Asset: 0, Leverage: 5},
		},
	}

	out, err := instructionCanonical(instr)
	require.NoError(t, err)
	s := string(out)

	// signal_id and intent_id should be omitted when empty
	assert.NotContains(t, s, `"signal_id"`)
	assert.NotContains(t, s, `"intent_id"`)
	// is_cross should be omitted when false
	assert.NotContains(t, s, `"is_cross"`)
}

func TestCanonical_WithIntentID(t *testing.T) {
	instr := &ExecutionInstruction{
		InstructionID: "test-123",
		Market:        "SOL",
		IntentID:      "intent-456",
		Steps: []ExecutionStep{
			{Action: "update_leverage", Asset: 0, Leverage: 5},
		},
	}

	out, err := instructionCanonical(instr)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"intent_id":"intent-456"`)
}
