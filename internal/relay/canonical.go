package relay

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// instructionCanonical produces deterministic JSON for signature verification.
// Must match the server's instruction-canonical.ts.
func instructionCanonical(instr *ExecutionInstruction) ([]byte, error) {
	m := make(map[string]any)

	m["instruction_id"] = instr.InstructionID
	m["market"] = instr.Market

	if instr.SignalID != "" {
		m["signal_id"] = instr.SignalID
	}
	if instr.IntentID != "" {
		m["intent_id"] = instr.IntentID
	}

	steps := make([]map[string]any, len(instr.Steps))
	for i, step := range instr.Steps {
		s := make(map[string]any)
		s["action"] = step.Action

		if len(step.Orders) > 0 {
			g, err := toGeneric(step.Orders)
			if err != nil {
				return nil, fmt.Errorf("canonical: orders: %w", err)
			}
			s["orders"] = g
		}
		if step.Grouping != "" {
			s["grouping"] = step.Grouping
		}
		if len(step.Cancels) > 0 {
			g, err := toGeneric(step.Cancels)
			if err != nil {
				return nil, fmt.Errorf("canonical: cancels: %w", err)
			}
			s["cancels"] = g
		}
		if step.Action == "update_leverage" {
			s["asset"] = step.Asset
			s["leverage"] = step.Leverage
			// Only include is_cross when true to match server canonical form.
			if step.IsCross {
				s["is_cross"] = step.IsCross
			}
		}
		if len(step.Modifications) > 0 {
			g, err := toGeneric(step.Modifications)
			if err != nil {
				return nil, fmt.Errorf("canonical: modifications: %w", err)
			}
			s["modifications"] = g
		}

		steps[i] = s
	}
	m["steps"] = steps

	return []byte(sortedStringify(m)), nil
}

func toGeneric(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal failed: %w", err)
	}
	var result any
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("unmarshal failed: %w", err)
	}
	return result, nil
}

func sortedStringify(v any) string {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		parts := make([]string, len(keys))
		for i, k := range keys {
			keyJSON, _ := json.Marshal(k)
			parts[i] = fmt.Sprintf("%s:%s", keyJSON, sortedStringify(val[k]))
		}
		return "{" + strings.Join(parts, ",") + "}"

	case []any:
		items := make([]string, len(val))
		for i, item := range val {
			items[i] = sortedStringify(item)
		}
		return "[" + strings.Join(items, ",") + "]"

	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
