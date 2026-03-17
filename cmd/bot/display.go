package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

const logo = `
  ██████╗ ███████╗ ██████╗ ███████╗███╗   ██╗██████╗  ██████╗ ██╗  ██╗
  ██╔══██╗██╔════╝██╔════╝ ██╔════╝████╗  ██║██╔══██╗██╔═══██╗╚██╗██╔╝
  ██║  ██║█████╗  ██║  ███╗█████╗  ██╔██╗ ██║██████╔╝██║   ██║ ╚███╔╝
  ██║  ██║██╔══╝  ██║   ██║██╔══╝  ██║╚██╗██║██╔══██╗██║   ██║ ██╔██╗
  ██████╔╝███████╗╚██████╔╝███████╗██║ ╚████║██████╔╝╚██████╔╝██╔╝ ██╗
  ╚═════╝ ╚══════╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝╚═════╝  ╚═════╝ ╚═╝  ╚═╝`

func shortenAddr(addr string) string {
	if len(addr) < 10 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}

func formatUSD(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return "$" + s
	}
	if f < 0 {
		return fmt.Sprintf("-$%s", addCommas(fmt.Sprintf("%.2f", -f)))
	}
	return fmt.Sprintf("$%s", addCommas(fmt.Sprintf("%.2f", f)))
}

func formatUSDf(f float64) string {
	if f < 0 {
		return fmt.Sprintf("-$%s", addCommas(fmt.Sprintf("%.2f", -f)))
	}
	return fmt.Sprintf("$%s", addCommas(fmt.Sprintf("%.2f", f)))
}

func addCommas(s string) string {
	parts := strings.Split(s, ".")
	intPart := parts[0]

	if len(intPart) <= 3 {
		return s
	}

	var result []byte
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}

	if len(parts) == 2 {
		return string(result) + "." + parts[1]
	}
	return string(result)
}

func calcMarginPct(marginUsed, accountValue string) string {
	m, err1 := strconv.ParseFloat(marginUsed, 64)
	a, err2 := strconv.ParseFloat(accountValue, 64)
	if err1 != nil || err2 != nil || a == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", (m/a)*100)
}

func countPositions(positions []hyperliquid.AssetPosition) int {
	count := 0
	for _, p := range positions {
		f, err := strconv.ParseFloat(p.Position.Szi, 64)
		if err == nil && f != 0 {
			count++
		}
	}
	return count
}
