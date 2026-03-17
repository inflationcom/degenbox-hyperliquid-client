package main

import (
	"fmt"
	"strings"
	"time"
)

func renderTrades(store *TradeStore, width int) string {
	records := store.Recent(50)
	if len(records) == 0 {
		return styleHeaderDim.Render("  No trades yet")
	}

	var sb strings.Builder
	header := fmt.Sprintf("  %-10s %-10s %-18s %s", "Time", "Market", "Action", "Result")
	sb.WriteString(styleHeaderDim.Render(header))
	sb.WriteString("\n")
	sb.WriteString(styleHeaderDim.Render("  " + strings.Repeat("─", max(0, min(width-4, 70)))))
	sb.WriteString("\n")

	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		ts := r.Time.Format(time.TimeOnly)

		result := styleGreen.Render("OK")
		if !r.Success {
			errMsg := r.Error
			if len(errMsg) > 30 {
				errMsg = errMsg[:30] + "..."
			}
			result = styleRed.Render("FAIL: " + errMsg)
		}

		line := fmt.Sprintf("  %-10s %-10s %-18s %s", ts, r.Market, r.Action, result)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}
