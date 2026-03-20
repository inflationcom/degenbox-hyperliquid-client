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
	testnet := fs.Bool("testnet", false, "Use testnet instead of mainnet")
	fs.Parse(args)

	w := &setupWizard{
		reader:  bufio.NewReader(os.Stdin),
		testnet: *testnet,
	}
	w.run(*configPath)
}

type setupWizard struct {
	reader  *bufio.Reader
	testnet bool
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

	// Derive the key address for registration (unique per API wallet)
	// Try: config → .env → keystore → prompt
	pk := cfg.PrivateKey
	if pk == "" {
		loadDotEnv()
		config.LoadFromEnv(cfg)
		pk = cfg.PrivateKey
	}
	if pk == "" {
		pk = loadPrivateKeyFromKeystore()
	}
	if pk == "" {
		pk = privateKeyFromEnv()
	}
	var keyAddr string
	if pk != "" {
		net := hyperliquid.Mainnet
		if cfg.Network == "testnet" {
			net = hyperliquid.Testnet
		}
		if s, err := hyperliquid.NewSigner(pk, net); err == nil {
			keyAddr = s.Address()
		}
	}
	if keyAddr == "" {
		keyAddr = w.promptString("  Wallet address (0x...)", "", true)
	}

	// main_wallet_address is only needed for agent mode
	mainWalletAddr := ""
	if cfg.IsAgentMode && cfg.WalletAddr != "" {
		mainWalletAddr = cfg.WalletAddr
	}

	fmt.Printf("\n  Registering with %s... ", serverURL)
	hostname, _ := os.Hostname()
	regName := fmt.Sprintf("bot-%s", hostname)

	result, regErr := relay.Register(serverURL, token, regName, keyAddr, mainWalletAddr, cfg.Network)
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
	// Only convert HTTPS → WSS; refuse to silently downgrade HTTP → WS
	if strings.HasPrefix(u, "https://") {
		u = strings.Replace(u, "https://", "wss://", 1)
	} else if strings.HasPrefix(u, "http://") {
		fmt.Fprintf(os.Stderr, "Warning: server URL uses HTTP (insecure). Connection will be rejected.\n")
		u = strings.Replace(u, "http://", "ws://", 1)
	}
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
	network := "mainnet"
	if w.testnet {
		network = "testnet"
	}
	net := hyperliquid.Mainnet
	if w.testnet {
		net = hyperliquid.Testnet
	}

	// Step 1: Private Key
	fmt.Println("  Step 1/4: Private Key")
	fmt.Println()
	fmt.Println("  Enter your wallet private key (hex, starts with 0x).")
	fmt.Println("  This can be your main wallet key or an API wallet key.")
	fmt.Println("  This will be saved to .env, NOT to config.json.")
	fmt.Println()

	var privateKey string
	var signer *hyperliquid.Signer
	var isAgentMode bool
	var mainWalletAddr string

	for {
		privateKey = w.promptString("  Key", "", true)
		var err error
		signer, err = hyperliquid.NewSigner(privateKey, net)
		if err == nil {
			break
		}
		fmt.Println("  " + color("Invalid key. Must be a 64-character hex string (with or without 0x prefix).", colorRed))
		fmt.Println()
	}
	keyAddress := signer.Address()
	fmt.Println()
	fmt.Printf("  Key address: %s\n", keyAddress)
	fmt.Println()

	fmt.Println("  Is this an " + color("API wallet", colorBold) + " (created on Hyperliquid)?")
	fmt.Println("  If yes, it trades on behalf of your main wallet (shared balance).")
	fmt.Println("  If this is your main wallet key, just press Enter.")
	fmt.Println()
	isAgent := w.promptYesNo("  API wallet?", false)

	if isAgent {
		fmt.Println()
		fmt.Println("  Enter the " + color("main wallet address", colorBold) + " this API wallet is authorized for.")
		fmt.Println("  (The wallet that holds your funds on Hyperliquid)")
		fmt.Println()
		for {
			mainWalletAddr = w.promptString("  Main wallet (0x...)", "", true)
			if len(mainWalletAddr) == 42 && strings.HasPrefix(mainWalletAddr, "0x") {
				break
			}
			fmt.Println("  " + color("Must be a 0x-prefixed Ethereum address (42 characters).", colorRed))
			fmt.Println()
		}
		isAgentMode = true

		var err error
		signer, err = hyperliquid.NewAgentSigner(privateKey, mainWalletAddr, net)
		if err != nil {
			die("failed to create agent signer: %v", err)
		}
	}

	walletAddress := signer.SourceAddress()
	fmt.Println()
	if isAgentMode {
		fmt.Printf("  API wallet:   %s\n", keyAddress)
		fmt.Printf("  Main wallet:  %s\n", mainWalletAddr)
	} else {
		fmt.Printf("  Wallet:  %s\n", walletAddress)
	}
	fmt.Printf("  Network: %s\n", strings.ToUpper(network))
	fmt.Println()

	// Step 2: Registration Token
	fmt.Println("  Step 2/4: Signal Server")
	fmt.Println()
	fmt.Println("  Paste your registration token from the dashboard.")
	fmt.Println("  (Wallets page > Add Wallet > Generate)")
	fmt.Println()

	var relayURL, relayAPIKey, serverPublicKey string
	var relayClientID int

	var token string
	for {
		token = w.promptString("  Token (rt_...)", "", true)
		if strings.HasPrefix(token, "rt_") {
			break
		}
		fmt.Println("  " + color("Token must start with rt_ — copy it from the dashboard.", colorRed))
		fmt.Println()
	}

	hostname, _ := os.Hostname()
	clientName := fmt.Sprintf("bot-%s", hostname)

	fmt.Printf("\n  Registering... ")
	result, regErr := relay.Register(defaultServerURL, token, clientName, keyAddress, mainWalletAddr, network)
	if regErr != nil {
		fmt.Println(color("FAILED", colorRed))
		fmt.Printf("  Error: %v\n", regErr)
		fmt.Println("  You can configure relay manually in .env later.")
		fmt.Println()
	} else {
		fmt.Println(color("OK", colorGreen))
		fmt.Printf("  Client ID: %d\n", result.ClientID)

		// Use the name the server stored (dashboard name takes priority)
		if result.Name != "" {
			clientName = result.Name
		}

		relayURL = httpToWS(defaultServerURL)
		relayAPIKey = result.APIKey
		relayClientID = result.ClientID

		if result.ServerPublicKey != "" {
			serverPublicKey = result.ServerPublicKey
		}
		fmt.Println()
	}

	// Step 3: Risk Limits
	fmt.Println("  Step 3/4: Risk Limits")
	fmt.Println()
	fmt.Println("  Safety limits protect your account from rogue signals.")
	fmt.Println("  Press Enter to accept defaults.")
	fmt.Println()
	maxLeverage := w.promptInt("  Max leverage per trade", 20, true)
	maxOrderSize := w.promptFloat("  Max order size in USD", 500, true)
	fmt.Println()

	// Step 4: Connection Test
	fmt.Printf("  Connecting to Hyperliquid %s... ", network)

	client, err := hyperliquid.NewClient(hyperliquid.ClientConfig{
		Network:     net,
		MainAddress: signer.SourceAddress(),
		Signer:      signer,
	})
	if err != nil {
		die("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.RefreshAssets(ctx); err != nil {
		fmt.Println(color("FAILED", colorRed))
		die("connection failed: %v", err)
	}
	fmt.Println(color("OK", colorGreen))
	fmt.Println()

	// Save config
	cfg := config.DefaultConfig()
	cfg.Network = network
	cfg.ClientName = clientName
	cfg.IsAgentMode = isAgentMode
	cfg.WalletAddr = mainWalletAddr

	if relayURL != "" {
		cfg.Relay.ServerURL = relayURL
		cfg.Relay.APIKey = relayAPIKey
		cfg.Relay.ClientID = relayClientID
		cfg.Relay.ServerPublicKey = serverPublicKey
	}

	cfg.RiskLimits = config.RiskLimits{
		MaxLeverage:     maxLeverage,
		MaxOrderSizeUSD: maxOrderSize,
		MaxPriceDevPct:  5.0,
	}
	cfg.RiskLimits.ApplyDefaults()

	if err := cfg.SaveToFile(configPath); err != nil {
		die("failed to save config: %v", err)
	}

	envLines := []string{
		fmt.Sprintf("HL_PRIVATE_KEY=%s", privateKey),
	}
	if isAgentMode {
		envLines = append(envLines,
			fmt.Sprintf("HL_WALLET_ADDR=%s", mainWalletAddr),
			"HL_AGENT_MODE=true",
		)
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
	fmt.Printf("  Config saved to:  %s\n", configPath)
	fmt.Printf("  Private key in:   .env\n")
	fmt.Println()
	fmt.Println("  To encrypt your private key:")
	fmt.Printf("    %s encrypt-key\n", botCmd())
	fmt.Println()
	fmt.Println("  To start the bot:")
	fmt.Printf("    %s run\n", botCmd())
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

func (w *setupWizard) promptYesNo(prompt string, defaultVal bool) bool {
	defStr := "y/N"
	if defaultVal {
		defStr = "Y/n"
	}
	for {
		fmt.Printf("%s [%s]: ", prompt, defStr)
		line, _ := w.reader.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			return defaultVal
		}
		if line == "y" || line == "yes" {
			return true
		}
		if line == "n" || line == "no" {
			return false
		}
		fmt.Println("  Please enter y or n.")
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
