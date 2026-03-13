package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/config"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
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
	logFile, err := os.OpenFile("degenbox.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
			fmt.Fprintf(os.Stderr, "\nRun './bot setup' to configure your bot.\n")
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx, cancel, cfg, tuiHandler, multiHandler, logFile); err != nil {
		slog.Error("bot error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cancel context.CancelFunc, cfg *config.Config, tuiHandler *TUILogHandler, multiHandler *MultiHandler, logFile *os.File) error {
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
		return fmt.Errorf("invalid private key — check your .env file or run './bot setup': %w", err)
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

	if cfg.Relay.ServerURL == "" {
		return fmt.Errorf("relay server URL not configured — run './bot setup' first")
	}

	cfg.RiskLimits.ApplyDefaults()

	tradeStore := NewTradeStore(200)

	validator := relay.NewRiskValidator(cfg.RiskLimits, client)
	relayClient, err := relay.NewClient(relay.Config{
		ServerURL:       cfg.Relay.ServerURL,
		APIKey:          cfg.Relay.APIKey,
		ClientID:        cfg.Relay.ClientID,
		ServerPublicKey: cfg.Relay.ServerPublicKey,
		Version:         version,
	}, client, validator, tradeStore)
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

	m := newTUIModel(
		instanceName,
		signer.SourceAddress(),
		cfg.Network,
		state,
		relayClient.Connected(),
		tradeStore,
		settings,
		tuiHandler,
		cancel,
	)

	p := tea.NewProgram(m, tea.WithAltScreen())
	tuiHandler.SetProgram(p)

	relayClient.OnConfigUpdate(func(msg relay.ConfigUpdateMsg) {
		if msg.Name != "" {
			p.Send(nameUpdateMsg(msg.Name))
			settings.ClientName = msg.Name
		}
	})

	relayClient.OnVersionInfo(func(msg relay.VersionInfoMsg) {
		if msg.UpdateAvailable {
			p.Send(versionInfoMsg(msg))
		}
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
					p.Send(accountUpdateMsg{state: s, connected: relayClient.Connected()})
					writeHeartbeat(relayClient.Connected(), s)
				}
			}
		}
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

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	relayClient.Stop()
	os.Remove(heartbeatPath())

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
					writeHeartbeat(relayClient.Connected(), s)
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
