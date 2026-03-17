//go:debug x509usefallbackroots=1

package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"golang.org/x/term"
	_ "golang.org/x/crypto/x509roots/fallback"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	// On Windows, if double-clicked (no terminal), pause before exit so the user can read output
	if runtime.GOOS == "windows" && !term.IsTerminal(int(os.Stdout.Fd())) {
		defer func() {
			fmt.Println("\nPress Enter to exit...")
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		}()
	}

	if len(os.Args) < 2 {
		cmdRun([]string{})
		return
	}

	cmd := strings.ToLower(os.Args[1])

	if strings.HasPrefix(cmd, "-") {
		cmdRun(os.Args[1:])
		return
	}

	switch cmd {
	case "run":
		cmdRun(os.Args[2:])
	case "setup":
		cmdSetup(os.Args[2:])
	case "config":
		cmdConfig(os.Args[2:])
	case "encrypt-key":
		cmdEncryptKey(os.Args[2:])
	case "version":
		cmdVersion()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`Hyperliquid Trading Bot %s

Usage: bot <command> [options]

Commands:
  setup       Set up your bot (interactive)
  run         Connect to relay server and execute trades
  config      View current configuration
  encrypt-key Encrypt your private key with a passphrase
  version     Show version info

Run 'bot <command> --help' for details on a specific command.
`, version)
}

func cmdVersion() {
	fmt.Printf("bot %s (built %s)\n", version, buildTime)
}
