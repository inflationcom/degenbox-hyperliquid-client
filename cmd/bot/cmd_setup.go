package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/config"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/relay"
)

const defaultServerURL = "https://scheme24.com"

func cmdSetup(args []string) {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	configPath := fs.String("config", "config.json", "Config file output path")
	fs.Parse(args)

	w := &setupWizard{
		reader: bufio.NewReader(os.Stdin),
	}
	w.run(*configPath)
}

type setupWizard struct {
	reader *bufio.Reader
}

func (w *setupWizard) run(configPath string) {
	fmt.Println()
	fmt.Println(color("  Hyperliquid Bot Setup", colorBold))
	fmt.Println("  " + strings.Repeat("-", 22))
	fmt.Println()

	if existing, err := config.LoadFromFile(configPath); err == nil {
		w.runReconfigure(configPath, existing)
		return
	}

	w.runFresh(configPath)
}

func (w *setupWizard) runReconfigure(configPath string, cfg *config.Config) {
	fmt.Println("  Existing configuration found.")
	fmt.Println()
	fmt.Println("  [1] Reconfigure everything from scratch")
	fmt.Println("  [2] Update risk limits only")
	fmt.Println("  [3] Update relay connection only")
	fmt.Println("  [4] Cancel")
	fmt.Println()
	choice := w.promptChoice("  Choice", 1, 4, 4)

	switch choice {
	case 1:
		w.runFresh(configPath)
	case 2:
		w.updateRiskLimits(configPath, cfg)
	case 3:
		w.updateRelay(configPath, cfg)
	case 4:
		fmt.Println("  No changes made.")
	}
}

func (w *setupWizard) updateRiskLimits(configPath string, cfg *config.Config) {
	fmt.Println()
	fmt.Println(color("  Update Risk Limits", colorBold))
	fmt.Println()
	fmt.Printf("  Current: max leverage=%d, max order=$%.0f, price dev=%.0f%%\n",
		cfg.RiskLimits.MaxLeverage, cfg.RiskLimits.MaxOrderSizeUSD, cfg.RiskLimits.MaxPriceDevPct)
	fmt.Println()

	cfg.RiskLimits.MaxLeverage = w.promptInt("  Max leverage per trade", cfg.RiskLimits.MaxLeverage, true)
	cfg.RiskLimits.MaxOrderSizeUSD = w.promptFloat("  Max order size in USD", cfg.RiskLimits.MaxOrderSizeUSD, true)
	fmt.Println()
	cfg.RiskLimits.MaxPriceDevPct = w.promptFloat("  Max price deviation from oracle %", cfg.RiskLimits.MaxPriceDevPct, false)
	cfg.RiskLimits.ApplyDefaults()

	if err := cfg.SaveToFile(configPath); err != nil {
		die("failed to save config: %v", err)
	}

	fmt.Println()
	fmt.Println(color("  Risk limits updated!", colorGreen))
	fmt.Printf("  Config saved to: %s\n\n", configPath)
}

func (w *setupWizard) updateRelay(configPath string, cfg *config.Config) {
	fmt.Println()
	fmt.Println(color("  Update Relay Connection", colorBold))
	fmt.Println()

	serverURL := w.promptString("  Server URL", defaultServerURL, false)
	fmt.Println()
	fmt.Println("  Generate a registration token from the dashboard")
	fmt.Println("  (Wallets page > Add Wallet > Generate)")
	fmt.Println()
	var token string
	for {
		token = w.promptString("  Registration token (rt_...)", "", true)
		if strings.HasPrefix(token, "rt_") {
			break
		}
		fmt.Println("  " + color("Token must start with rt_ — copy it from the dashboard.", colorRed))
		fmt.Println()
	}

	walletAddr := cfg.WalletAddr
	if walletAddr == "" {
		walletAddr = w.promptString("  Wallet address (0x...)", "", true)
	}

	fmt.Printf("\n  Registering with %s... ", serverURL)
	hostname, _ := os.Hostname()
	regName := fmt.Sprintf("bot-%s", hostname)

	result, regErr := relay.Register(serverURL, token, regName, walletAddr, cfg.Network)
	if regErr != nil {
		fmt.Println(color("FAILED", colorRed))
		fmt.Printf("  Error: %v\n\n", regErr)
		return
	}
	fmt.Println(color("OK", colorGreen))
	fmt.Printf("  Client ID: %d\n", result.ClientID)

	relayURL := httpToWS(serverURL)

	cfg.Relay.ServerURL = relayURL
	cfg.Relay.APIKey = result.APIKey
	cfg.Relay.ClientID = result.ClientID
	if result.ServerPublicKey != "" {
		cfg.Relay.ServerPublicKey = result.ServerPublicKey
	}

	if err := cfg.SaveToFile(configPath); err != nil {
		die("failed to save config: %v", err)
	}

	envLines := []string{
		fmt.Sprintf("HL_RELAY_URL=%s", relayURL),
		fmt.Sprintf("HL_RELAY_API_KEY=%s", result.APIKey),
		fmt.Sprintf("HL_RELAY_CLIENT_ID=%d", result.ClientID),
	}

	existingEnv := readEnvFile(".env")
	for k := range existingEnv {
		if strings.HasPrefix(k, "HL_RELAY_") {
			delete(existingEnv, k)
		}
	}
	var lines []string
	for k, v := range existingEnv {
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}
	lines = append(lines, envLines...)
	envContent := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(".env", []byte(envContent), 0600); err != nil {
		fmt.Printf("  Warning: could not update .env: %v\n", err)
	}

	fmt.Println()
	fmt.Println(color("  Relay connection updated!", colorGreen))
	fmt.Printf("  Config saved to: %s\n\n", configPath)
}

func httpToWS(serverURL string) string {
	u := strings.TrimRight(serverURL, "/")
	u = strings.Replace(strings.Replace(u, "https://", "wss://", 1), "http://", "ws://", 1)
	return u + "/ws"
}

func readEnvFile(path string) map[string]string {
	result := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func (w *setupWizard) runFresh(configPath string) {
	fmt.Println("  Step 1/7: Network")
	fmt.Println()
	fmt.Println("  [1] Testnet (recommended to start)")
	fmt.Println("  [2] Mainnet")
	fmt.Println()
	networkChoice := w.promptChoice("  Choice", 1, 2, 1)
	network := "testnet"
	if networkChoice == 2 {
		network = "mainnet"
	}
	fmt.Println()

	fmt.Println("  Step 2/7: Private Key")
	fmt.Println()
	fmt.Println("  Enter your wallet private key (hex, starts with 0x).")
	fmt.Println("  This will be saved to .env, NOT to config.json.")
	fmt.Println()
	net := hyperliquid.Testnet
	if network == "mainnet" {
		net = hyperliquid.Mainnet
	}

	var privateKey string
	var signer *hyperliquid.Signer
	for {
		privateKey = w.promptString("  Key", "", true)
		var err error
		signer, err = hyperliquid.NewSigner(privateKey, net)
		if err == nil {
			break
		}
		fmt.Println("  " + color("Invalid key. Make sure it's a 64-character hex string (with or without 0x prefix).", colorRed))
		fmt.Println()
	}
	walletAddress := signer.Address()
	fmt.Printf("\n  Wallet address: %s\n", walletAddress)
	fmt.Println()

	fmt.Println("  Step 3/7: Wallet Mode")
	fmt.Println()
	fmt.Println("  [1] Direct wallet (your own key)")
	fmt.Println("  [2] Agent wallet (delegated from another wallet)")
	fmt.Println()
	modeChoice := w.promptChoice("  Choice", 1, 2, 1)
	isAgent := modeChoice == 2
	walletAddr := ""
	if isAgent {
		fmt.Println()
		walletAddr = w.promptString("  Main wallet address (0x...)", "", true)
		walletAddress = walletAddr
	}
	fmt.Println()

	fmt.Println("  Step 4/7: Signal Server")
	fmt.Println()
	fmt.Println("  Connect to signal server for automated trading signals?")
	fmt.Println("  [1] Yes (recommended)")
	fmt.Println("  [2] No (standalone mode)")
	fmt.Println()
	connectChoice := w.promptChoice("  Choice", 1, 2, 1)

	var relayURL, relayAPIKey, serverPublicKey string
	var relayClientID int

	if connectChoice == 1 {
		fmt.Println()
		serverURL := w.promptString("  Server URL", defaultServerURL, false)
		fmt.Println()
		fmt.Println("  Generate a registration token from the dashboard")
		fmt.Println("  (Wallets page > Add Wallet > Generate)")
		fmt.Println()
		var token string
		for {
			token = w.promptString("  Registration token (rt_...)", "", true)
			if strings.HasPrefix(token, "rt_") {
				break
			}
			fmt.Println("  " + color("Token must start with rt_ — copy it from the dashboard.", colorRed))
			fmt.Println()
		}

		fmt.Printf("\n  Registering with %s... ", serverURL)
		hostname, _ := os.Hostname()
		regName := fmt.Sprintf("bot-%s", hostname)

		result, regErr := relay.Register(serverURL, token, regName, walletAddress, network)
		if regErr != nil {
			fmt.Println(color("FAILED", colorRed))
			fmt.Printf("  Error: %v\n", regErr)
			fmt.Println("  You can configure relay manually in .env later.")
			fmt.Println()
		} else {
			fmt.Println(color("OK", colorGreen))
			fmt.Printf("  Client ID: %d\n", result.ClientID)
			fmt.Printf("  Subscribed to %d callers\n", result.Subscriptions)

			relayURL = httpToWS(serverURL)
			relayAPIKey = result.APIKey
			relayClientID = result.ClientID

			if result.ServerPublicKey != "" {
				serverPublicKey = result.ServerPublicKey
				fmt.Printf("  Server public key: %s...%s\n", serverPublicKey[:8], serverPublicKey[len(serverPublicKey)-8:])
				fmt.Println("  Ed25519 instruction signing: " + color("enabled", colorGreen))
			}

			fmt.Println()
		}
	}
	fmt.Println()

	fmt.Println("  Step 5/7: Client Name")
	fmt.Println()
	fmt.Println("  Give this instance a name (shown in the CLI banner).")
	fmt.Println()
	hostname, _ := os.Hostname()
	defaultName := fmt.Sprintf("bot-%s", hostname)
	clientName := w.promptString("  Name", defaultName, false)
	fmt.Println()

	fmt.Println("  Step 6/7: Risk Limits")
	fmt.Println()
	fmt.Println("  Set safety limits to protect your account.")
	fmt.Println("  The bot will refuse instructions that exceed these limits.")
	fmt.Println()
	maxLeverage := w.promptInt("  Max leverage per trade", 0, true)
	maxOrderSize := w.promptFloat("  Max order size in USD", 0, true)
	fmt.Println()
	fmt.Println("  Advanced (press Enter for defaults):")
	maxPriceDev := w.promptFloat("  Max price deviation from oracle %", 5.0, false)
	fmt.Println()

	fmt.Println("  Step 7/7: Testing Connection")
	fmt.Println()

	var clientSigner *hyperliquid.Signer
	if isAgent {
		var agentErr error
		clientSigner, agentErr = hyperliquid.NewAgentSigner(privateKey, walletAddr, net)
		if agentErr != nil {
			die("Could not set up agent wallet. Check that your main wallet address is correct: %v", agentErr)
		}
	} else {
		clientSigner = signer
	}

	client, err := hyperliquid.NewClient(hyperliquid.ClientConfig{
		Network:     net,
		MainAddress: clientSigner.SourceAddress(),
		Signer:      clientSigner,
	})
	if err != nil {
		die("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("  Connecting to Hyperliquid %s... ", network)
	if err := client.RefreshAssets(ctx); err != nil {
		fmt.Println(color("FAILED", colorRed))
		die("connection failed: %v", err)
	}
	fmt.Println(color("OK", colorGreen))
	fmt.Println()

	cfg := config.DefaultConfig()
	cfg.Network = network
	cfg.IsAgentMode = isAgent
	cfg.WalletAddr = walletAddr
	cfg.ClientName = clientName

	if relayURL != "" {
		cfg.Relay.ServerURL = relayURL
		cfg.Relay.APIKey = relayAPIKey
		cfg.Relay.ClientID = relayClientID
		cfg.Relay.ServerPublicKey = serverPublicKey
	}

	cfg.RiskLimits = config.RiskLimits{
		MaxLeverage:     maxLeverage,
		MaxOrderSizeUSD: maxOrderSize,
		MaxPriceDevPct:  maxPriceDev,
	}
	cfg.RiskLimits.ApplyDefaults()

	if err := cfg.SaveToFile(configPath); err != nil {
		die("failed to save config: %v", err)
	}

	envLines := []string{
		fmt.Sprintf("HL_PRIVATE_KEY=%s", privateKey),
	}
	if relayURL != "" {
		envLines = append(envLines,
			fmt.Sprintf("HL_RELAY_URL=%s", relayURL),
			fmt.Sprintf("HL_RELAY_API_KEY=%s", relayAPIKey),
			fmt.Sprintf("HL_RELAY_CLIENT_ID=%d", relayClientID),
		)
	}
	envContent := strings.Join(envLines, "\n") + "\n"

	if err := os.WriteFile(".env", []byte(envContent), 0600); err != nil {
		die("failed to save .env: %v", err)
	}

	fmt.Println(color("  Setup complete!", colorGreen))
	fmt.Println()
	fmt.Printf("  Config saved to:    %s\n", configPath)
	fmt.Printf("  Private key in:     .env\n")
	fmt.Println()
	fmt.Println("  To encrypt your private key:")
	fmt.Printf("    ./bot encrypt-key\n")
	fmt.Println()
	fmt.Println("  To start the bot:")
	fmt.Printf("    ./bot\n")
	fmt.Println()
}

func (w *setupWizard) promptChoice(prompt string, min, max, defaultVal int) int {
	for {
		fmt.Printf("%s [%d]: ", prompt, defaultVal)
		line, _ := w.reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return defaultVal
		}
		val, err := strconv.Atoi(line)
		if err != nil || val < min || val > max {
			fmt.Printf("  Please enter %d-%d\n", min, max)
			continue
		}
		return val
	}
}

func (w *setupWizard) promptInt(prompt string, defaultVal int, required bool) int {
	for {
		if defaultVal > 0 {
			fmt.Printf("%s [%d]: ", prompt, defaultVal)
		} else {
			fmt.Printf("%s: ", prompt)
		}
		line, _ := w.reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			if defaultVal > 0 {
				return defaultVal
			}
			if required {
				fmt.Println("  This field is required.")
				continue
			}
			return 0
		}
		val, err := strconv.Atoi(line)
		if err != nil || val <= 0 {
			fmt.Println("  Please enter a positive number.")
			continue
		}
		return val
	}
}

func (w *setupWizard) promptFloat(prompt string, defaultVal float64, required bool) float64 {
	for {
		if defaultVal > 0 {
			fmt.Printf("%s [%.0f]: ", prompt, defaultVal)
		} else {
			fmt.Printf("%s: ", prompt)
		}
		line, _ := w.reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			if defaultVal > 0 {
				return defaultVal
			}
			if required {
				fmt.Println("  This field is required.")
				continue
			}
			return 0
		}
		val, err := strconv.ParseFloat(line, 64)
		if err != nil || val <= 0 {
			fmt.Println("  Please enter a positive number.")
			continue
		}
		return val
	}
}

func (w *setupWizard) promptString(prompt string, defaultVal string, required bool) string {
	for {
		if defaultVal != "" {
			fmt.Printf("%s [%s]: ", prompt, defaultVal)
		} else {
			fmt.Printf("%s: ", prompt)
		}
		line, _ := w.reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			if defaultVal != "" {
				return defaultVal
			}
			if required {
				fmt.Println("  This field is required.")
				continue
			}
			return ""
		}
		return line
	}
}
