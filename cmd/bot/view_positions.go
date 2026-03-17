package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

func renderPositions(positions []hyperliquid.AssetPosition, width int) string {
	type row struct {
		coin  string
		size  string
		entry string
		value string
		pnl   string
		roe   string
		lev   string
		long  bool
		green bool
	}

	var rows []row
	for _, ap := range positions {
		p := ap.Position
		szi, err := strconv.ParseFloat(p.Szi, 64)
		if err != nil || szi == 0 {
			continue
		}

		entryPx, _ := strconv.ParseFloat(p.EntryPx, 64)
		posValue := math.Abs(szi * entryPx)
		unrealizedPnl, _ := strconv.ParseFloat(p.UnrealizedPnl, 64)
		leverage := float64(p.Leverage.Value)

		var roe float64
		if posValue > 0 && leverage > 0 {
			roe = (unrealizedPnl / posValue) * leverage * 100
		}

		rows = append(rows, row{
			coin:  ap.Position.Coin,
			size:  fmt.Sprintf("%.4f", szi),
			entry: formatUSD(p.EntryPx),
			value: fmt.Sprintf("$%s", addCommas(fmt.Sprintf("%.2f", posValue))),
			pnl:   formatPnl(unrealizedPnl),
			roe:   fmt.Sprintf("%+.1f%%", roe),
			lev:   fmt.Sprintf("%.0fx", leverage),
			long:  szi > 0,
			green: unrealizedPnl >= 0,
		})
	}

	if len(rows) == 0 {
		return styleHeaderDim.Render("  No open positions")
	}

	var sb strings.Builder
	header := fmt.Sprintf("  %-8s %12s %12s %12s %12s %8s %6s", "Coin", "Size", "Entry", "Value", "uPnL", "ROE", "Lev")
	sb.WriteString(styleHeaderDim.Render(header))
	sb.WriteString("\n")
	sb.WriteString(styleHeaderDim.Render("  " + strings.Repeat("─", max(0, min(width-4, 76)))))
	sb.WriteString("\n")

	for _, r := range rows {
		sideStyle := styleGreen
		if !r.long {
			sideStyle = styleRed
		}
		pnlStyle := styleGreen
		if !r.green {
			pnlStyle = styleRed
		}

		line := fmt.Sprintf("  %s %12s %12s %12s %s %s %6s",
			sideStyle.Render(fmt.Sprintf("%-8s", r.coin)),
			r.size,
			r.entry,
			r.value,
			pnlStyle.Render(fmt.Sprintf("%12s", r.pnl)),
			pnlStyle.Render(fmt.Sprintf("%8s", r.roe)),
			r.lev,
		)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatPnl(v float64) string {
	if v >= 0 {
		return fmt.Sprintf("+$%s", addCommas(fmt.Sprintf("%.2f", v)))
	}
	return fmt.Sprintf("-$%s", addCommas(fmt.Sprintf("%.2f", -v)))
}
