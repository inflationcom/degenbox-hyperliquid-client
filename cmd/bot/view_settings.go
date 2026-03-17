package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type SettingsSnapshot struct {
	Network        string
	WalletAddr     string
	IsAgentMode    bool
	ClientName     string
	RelayURL       string
	ClientID       int
	LogLevel       string
	MaxLeverage    int
	MaxOrderUSD    float64
	MaxPriceDevPct float64
	MaxPerMinute   int
	SigningEnabled bool
	Version        string
}

func renderSettings(s *SettingsSnapshot, connected bool) string {
	if s == nil {
		return styleHeaderDim.Render("  No settings available")
	}

	section := lipgloss.NewStyle().Bold(true).Foreground(colorBrand)

	pad := func(s string, w int) string {
		if len(s) >= w {
			return s
		}
		return s + strings.Repeat(" ", w-len(s))
	}

	row := func(label, val string) string {
		return "  " + styleHeaderDim.Render(pad(label, 18)) + styleHeaderValue.Render(val) + "\n"
	}

	var sb strings.Builder

	sb.WriteString("  " + section.Render("General") + "\n\n")

	netLabel := strings.ToUpper(s.Network)
	netStyled := styleTestnet.Render(netLabel)
	if s.Network == "mainnet" {
		netStyled = styleMainnet.Render(netLabel)
	}
	sb.WriteString("  " + styleHeaderDim.Render(pad("Network", 18)) + netStyled + "\n")
	sb.WriteString(row("Wallet", shortenAddr(s.WalletAddr)))
	if s.IsAgentMode {
		sb.WriteString(row("Wallet mode", "Agent"))
	}
	sb.WriteString(row("Log level", s.LogLevel))

	sb.WriteString("\n  " + section.Render("Relay") + "\n\n")

	if s.RelayURL != "" {
		sb.WriteString(row("Server", s.RelayURL))
	} else {
		sb.WriteString("  " + styleHeaderDim.Render(pad("Server", 18)) + styleLogWarn.Render("not configured") + "\n")
	}
	sb.WriteString(row("Client ID", fmt.Sprintf("%d", s.ClientID)))

	if connected {
		sb.WriteString("  " + styleHeaderDim.Render(pad("Status", 18)) + styleConnected.Render("● Connected") + "\n")
	} else {
		sb.WriteString("  " + styleHeaderDim.Render(pad("Status", 18)) + styleDisconnected.Render("○ Disconnected") + "\n")
	}

	signingVal := styleGreen.Render("Enabled")
	if !s.SigningEnabled {
		signingVal = styleLogWarn.Render("Disabled")
	}
	sb.WriteString("  " + styleHeaderDim.Render(pad("Signing", 18)) + signingVal + "\n")

	sb.WriteString("\n  " + section.Render("Risk Limits") + "\n\n")

	sb.WriteString(row("Max leverage", fmt.Sprintf("%d×", s.MaxLeverage)))
	sb.WriteString(row("Max order", fmt.Sprintf("$%.0f", s.MaxOrderUSD)))
	sb.WriteString(row("Price dev", fmt.Sprintf("%.0f%%", s.MaxPriceDevPct)))
	sb.WriteString(row("Rate limit", fmt.Sprintf("%d/min", s.MaxPerMinute)))

	if s.Version != "" {
		sb.WriteString("\n")
		sb.WriteString("  " + styleHeaderDim.Render(pad("Version", 18)) + styleHeaderDim.Render(s.Version) + "\n")
	}

	return sb.String()
}
