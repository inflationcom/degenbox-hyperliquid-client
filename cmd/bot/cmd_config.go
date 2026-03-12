package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/config"
)

func cmdConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	configPath := fs.String("config", "config.json", "Path to config file")
	fs.Parse(args)

	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No config found. Run './bot setup' first.\n")
		os.Exit(1)
	}

	printConfig(cfg)
}

func printConfig(cfg *config.Config) {
	netColor := colorGreen
	if cfg.Network == "testnet" {
		netColor = colorYellow
	}

	name := cfg.ClientName
	if name == "" {
		name = "(not set)"
	}

	relayURL := cfg.Relay.ServerURL
	if relayURL == "" {
		relayURL = "(not configured)"
	}

	fmt.Println()
	fmt.Printf("  %sCurrent Configuration%s\n", colorBold, colorReset)
	fmt.Printf("  %s\n", strings.Repeat("─", 40))
	fmt.Println()
	fmt.Printf("  Network          %s%s%s\n", netColor, strings.ToUpper(cfg.Network), colorReset)
	fmt.Printf("  Client name      %s\n", name)
	fmt.Printf("  Agent mode       %v\n", cfg.IsAgentMode)
	if cfg.WalletAddr != "" {
		fmt.Printf("  Wallet address   %s\n", cfg.WalletAddr)
	}
	fmt.Println()
	fmt.Printf("  %sRelay%s\n", colorBold, colorReset)
	fmt.Printf("  Server           %s\n", relayURL)
	if cfg.Relay.ClientID > 0 {
		fmt.Printf("  Client ID        %d\n", cfg.Relay.ClientID)
	}
	fmt.Println()
	fmt.Printf("  %sRisk Limits%s\n", colorBold, colorReset)
	fmt.Printf("  Max leverage     %d\n", cfg.RiskLimits.MaxLeverage)
	fmt.Printf("  Max order USD    $%.0f\n", cfg.RiskLimits.MaxOrderSizeUSD)
	fmt.Printf("  Price deviation  %.0f%%\n", cfg.RiskLimits.MaxPriceDevPct)
	fmt.Printf("  Orders/step      %d\n", cfg.RiskLimits.MaxOrdersPerStep)
	fmt.Printf("  Steps/instr      %d\n", cfg.RiskLimits.MaxStepsPerInstr)
	fmt.Printf("  Rate limit       %d/min\n", cfg.RiskLimits.MaxPerMinute)
	fmt.Println()
	fmt.Printf("  %sLog%s\n", colorBold, colorReset)
	fmt.Printf("  Level            %s\n", cfg.LogLevel)
	fmt.Printf("  Format           %s\n", cfg.LogFormat)
	fmt.Println()
	fmt.Println("  To change settings, edit config.json or re-run './bot setup'.")
	fmt.Println()
}
