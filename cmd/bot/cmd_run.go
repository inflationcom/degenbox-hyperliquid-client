package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/config"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/notify"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/relay"
	"golang.org/x/term"
)

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configFile := fs.String("config", "", "Path to config file (JSON)")
	testnet := fs.Bool("testnet", false, "Use testnet (overrides config)")
	fs.Parse(args)

	cfg := config.DefaultConfig()

	if *configFile != "" {
		var err error
		cfg, err = config.LoadFromFile(*configFile)
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}
	} else {
		if _, err := os.Stat("config.json"); err == nil {
			loaded, err := config.LoadFromFile("config.json")
			if err == nil {
				cfg = loaded
			}
		}
	}

	loadDotEnv()
	config.LoadFromEnv(cfg)

	if cfg.PrivateKey == "" {
		cfg.PrivateKey = loadPrivateKeyFromKeystore()
	}
	if cfg.PrivateKey == "" {
		cfg.PrivateKey = privateKeyFromEnv()
	}

	// Open log file early so pre-TUI logs are captured
	logFile, err := os.OpenFile("degenbox.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open degenbox.log: %v\n", err)
	}

	// Set up initial logger (file + stderr for pre-TUI phase)
	logLevel := parseLogLevel(cfg.LogLevel)
	tuiHandler := NewTUILogHandler(logLevel)

	var handlers []slog.Handler
	handlers = append(handlers, tuiHandler)
	if logFile != nil {
		handlers = append(handlers, slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: logLevel}))
	}
	multiHandler := NewMultiHandler(handlers...)
	slog.SetDefault(slog.New(multiHandler))

	if *testnet {
		cfg.Network = "testnet"
	}

	if err := cfg.Validate(); err != nil {
		if cfg.PrivateKey == "" || cfg.RiskLimits.MaxLeverage <= 0 {
			fmt.Println()
			fmt.Println("  No configuration found. Starting setup...")
			fmt.Println()
			w := &setupWizard{
				reader:  bufio.NewReader(os.Stdin),
				testnet: *testnet,
			}
			w.runFresh("config.json")

			cfg, _ = config.LoadFromFile("config.json")
			if cfg == nil {
				cfg = config.DefaultConfig()
			}
			// Clear stale env vars so loadDotEnv picks up the fresh .env
			os.Unsetenv("HL_PRIVATE_KEY")
			os.Unsetenv("HL_WALLET_ADDR")
			os.Unsetenv("HL_AGENT_MODE")
			os.Unsetenv("HL_RELAY_URL")
			os.Unsetenv("HL_RELAY_API_KEY")
			os.Unsetenv("HL_RELAY_CLIENT_ID")
			loadDotEnv()
			config.LoadFromEnv(cfg)
			if cfg.PrivateKey == "" {
				cfg.PrivateKey = loadPrivateKeyFromKeystore()
			}
			if cfg.PrivateKey == "" {
				cfg.PrivateKey = privateKeyFromEnv()
			}
			if *testnet {
				cfg.Network = "testnet"
			}
			if err := cfg.Validate(); err != nil {
				slog.Error("configuration still invalid after setup", "error", err)
				os.Exit(1)
			}
		} else {
			slog.Error("invalid configuration", "error", err)
			fmt.Fprintf(os.Stderr, "\nRun '%s setup' to configure your bot.\n", botCmd())
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configPath := "config.json"
	if *configFile != "" {
		configPath = *configFile
	}

	if err := run(ctx, cancel, cfg, configPath, tuiHandler, multiHandler, logFile); err != nil {
		slog.Error("bot error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cancel context.CancelFunc, cfg *config.Config, configPath string, tuiHandler *TUILogHandler, multiHandler *MultiHandler, logFile *os.File) error {
	if logFile != nil {
		defer logFile.Close()
	}

	instanceName := config.ResolveClientName(cfg)
	slog.Info("starting bot", "name", instanceName, "network", cfg.Network)

	var signer *hyperliquid.Signer
	var err error

	if cfg.IsAgentMode {
		slog.Info("wallet mode", "mode", "agent", "source", cfg.WalletAddr)
		signer, err = hyperliquid.NewAgentSigner(cfg.PrivateKey, cfg.WalletAddr, cfg.GetNetwork())
	} else {
		slog.Info("wallet mode", "mode", "direct")
		signer, err = hyperliquid.NewSigner(cfg.PrivateKey, cfg.GetNetwork())
	}
	if err != nil {
		return fmt.Errorf("invalid private key — check your .env file or run '%s setup': %w", botCmd(), err)
	}
	cfg.PrivateKey = ""
	os.Unsetenv("HL_PRIVATE_KEY")
	os.Unsetenv("HL_RELAY_API_KEY")
	slog.Info("wallet ready", "address", signer.Address())

	client, err := hyperliquid.NewClient(hyperliquid.ClientConfig{
		Network:     cfg.GetNetwork(),
		MainAddress: signer.SourceAddress(),
		Signer:      signer,
	})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	if err := client.RefreshAssets(ctx); err != nil {
		return fmt.Errorf("could not reach Hyperliquid API — check your internet connection: %w", err)
	}
	slog.Info("API connection ok")

	state, err := client.GetClearinghouseState(ctx)
	if err != nil {
		slog.Warn("could not fetch account state", "error", err)
	}
	spotUSDC := fetchSpotUSDC(ctx, client)

	if cfg.Relay.ServerURL == "" {
		return fmt.Errorf("relay server URL not configured — run '%s setup' first", botCmd())
	}

	cfg.RiskLimits.ApplyDefaults()

	tradeStore := NewTradeStore(200)

	auditLog, err := relay.NewAuditLog("trades.jsonl")
	if err != nil {
		slog.Warn("could not open audit log", "error", err)
	}
	recorders := []relay.TradeRecorder{tradeStore}
	if auditLog != nil {
		recorders = append(recorders, auditLog)
	}
	if cfg.DiscordWebhookURL != "" {
		dw := notify.NewDiscordWebhook(cfg.DiscordWebhookURL)
		recorders = append(recorders, &discordRecorderAdapter{dw})
		slog.Info("discord webhook notifications enabled")
	}
	var recorder relay.TradeRecorder
	if len(recorders) == 1 {
		recorder = recorders[0]
	} else {
		recorder = relay.NewMultiRecorder(recorders...)
	}

	validator := relay.NewRiskValidator(cfg.RiskLimits, client)
	relayClient, err := relay.NewClient(relay.Config{
		ServerURL:       cfg.Relay.ServerURL,
		APIKey:          cfg.Relay.APIKey,
		ClientID:        cfg.Relay.ClientID,
		ServerPublicKey:      cfg.Relay.ServerPublicKey,
		Version:              version,
		MaxConsecutiveLosses: cfg.CircuitBreaker.MaxConsecutiveLosses,
		CooldownMinutes:      cfg.CircuitBreaker.CooldownMinutes,
	}, client, validator, recorder)
	if err != nil {
		return fmt.Errorf("relay setup failed: %w", err)
	}
	cfg.Relay.APIKey = ""
	relayClient.Start(ctx)
	multiHandler.Add(relayClient.NewLogHandler(slog.LevelInfo))
	slog.Info("relay client started", "url", cfg.Relay.ServerURL)
	slog.Info("ready, listening for instructions")

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if !isTTY {
		return runPlain(ctx, cancel, client, relayClient, state)
	}

	settings := &SettingsSnapshot{
		Network:        cfg.Network,
		WalletAddr:     signer.SourceAddress(),
		IsAgentMode:    cfg.IsAgentMode,
		ClientName:     instanceName,
		RelayURL:       cfg.Relay.ServerURL,
		ClientID:       cfg.Relay.ClientID,
		LogLevel:       cfg.LogLevel,
		MaxLeverage:    cfg.RiskLimits.MaxLeverage,
		MaxOrderUSD:    cfg.RiskLimits.MaxOrderSizeUSD,
		MaxPriceDevPct: cfg.RiskLimits.MaxPriceDevPct,
		MaxPerMinute:   cfg.RiskLimits.MaxPerMinute,
		SigningEnabled: cfg.Relay.ServerPublicKey != "",
		Version:        version,
	}

	m := newTUIModel(tuiConfig{
		instanceName:   instanceName,
		walletAddr:     signer.SourceAddress(),
		network:        cfg.Network,
		state:          state,
		spotUSDC:       spotUSDC,
		connected:      relayClient.Connected(),
		tradeStore:     tradeStore,
		settings:       settings,
		logHandler:     tuiHandler,
		quitFunc:       cancel,
		configPath:     configPath,
		relayClient:    relayClient,
		relayServerURL: cfg.Relay.ServerURL,
		isAgentMode:    cfg.IsAgentMode,
		mainWalletAddr: cfg.WalletAddr,
		validator:      validator,
		tickerAssets:   cfg.TickerAssets,
		hlClient:       client,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	tuiHandler.SetProgram(p)

	relayClient.OnConfigUpdate(func(msg relay.ConfigUpdateMsg) {
		if msg.Name != "" {
			p.Send(nameUpdateMsg(msg.Name))
			settings.ClientName = msg.Name
		}
		if msg.Paused != nil {
			p.Send(pauseUpdateMsg(*msg.Paused))
		}
	})

	relayClient.OnVersionInfo(func(msg relay.VersionInfoMsg) {
		if msg.UpdateAvailable {
			p.Send(versionInfoMsg(msg))
		}
	})

	relayClient.OnAuthInfo(func(msg relay.AuthInfoMsg) {
		p.Send(authInfoMsg(msg))
	})

	relayClient.OnAssignmentUpdate(func(msg relay.AssignmentUpdateMsg) {
		p.Send(assignmentUpdateMsg(msg))
	})

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s, err := client.GetClearinghouseState(ctx)
				if err == nil {
					su := fetchSpotUSDC(ctx, client)
					p.Send(accountUpdateMsg{state: s, spotUSDC: su, connected: relayClient.Connected()})
					writeHeartbeat(relayClient.Connected(), s, su)
				}
				// Fetch ticker prices (allMids + l2Book for xyz)
				p.Send(tickerUpdateMsg(fetchTickerPrices(ctx, client, m.tickerEnabled)))
			}
		}
	}()

	// Initial ticker fetch
	go func() {
		p.Send(tickerUpdateMsg(fetchTickerPrices(ctx, client, m.tickerEnabled)))
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
			p.Quit()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	relayClient.Stop()
	if auditLog != nil {
		auditLog.Close()
	}
	os.Remove(heartbeatPath())

	if fm, ok := finalModel.(tuiModel); ok && fm.updateCompleted {
		if logFile != nil {
			logFile.Close()
		}
		fmt.Println("\nRestarting with new version...")
		if err := reExecSelf(); err != nil {
			if err.Error() == "windows_restart_required" {
				fmt.Println("Update complete! Please restart the bot.")
				return nil
			}
			return fmt.Errorf("restart failed: %w", err)
		}
		return nil // unreachable on Unix (syscall.Exec replaces process)
	}

	fmt.Println("\nShutdown complete. Logs saved to degenbox.log")
	return nil
}

func runPlain(ctx context.Context, cancel context.CancelFunc, client *hyperliquid.Client, relayClient *relay.Client, state *hyperliquid.ClearinghouseState) error {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s, err := client.GetClearinghouseState(ctx)
				if err == nil {
					su := fetchSpotUSDC(ctx, client)
					writeHeartbeat(relayClient.Connected(), s, su)
				}
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("shutting down")
	cancel()
	relayClient.Stop()
	os.Remove(heartbeatPath())
	slog.Info("shutdown complete")
	return nil
}

func fetchSpotUSDC(ctx context.Context, client *hyperliquid.Client) float64 {
	spot, err := client.GetSpotClearinghouseState(ctx)
	if err != nil || spot == nil {
		return 0
	}
	for _, b := range spot.Balances {
		if b.Coin == "USDC" {
			f, _ := strconv.ParseFloat(b.Total, 64)
			return f
		}
	}
	return 0
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// discordRecorderAdapter wraps notify.DiscordWebhook as a relay.TradeRecorder.
type discordRecorderAdapter struct {
	dw *notify.DiscordWebhook
}

func (a *discordRecorderAdapter) RecordTrade(e relay.TradeEvent) {
	a.dw.RecordTrade(notify.TradeEvent{
		Time:     e.Time,
		Market:   e.Market,
		Action:   e.Action,
		Success:  e.Success,
		Error:    e.Error,
		SignalID: e.SignalID,
	})
}
