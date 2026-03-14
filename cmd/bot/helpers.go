package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"syscall"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/keystore"
	"golang.org/x/term"
)

func loadPrivateKeyFromKeystore() string {
	path := defaultKeystorePath
	if v := os.Getenv("HL_KEYSTORE_PATH"); v != "" {
		path = v
	}

	if !keystore.Exists(path) {
		return ""
	}

	address, err := keystore.ReadAddress(path)
	if err != nil {
		slog.Error("failed to read keystore", "path", path, "error", err)
		return ""
	}

	slog.Info("found encrypted keystore", "path", path, "wallet", address)
	fmt.Print("Enter passphrase: ")
	passphrase, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		die("Failed to read passphrase: %v", err)
	}

	key, _, err := keystore.Decrypt(path, passphrase)
	for i := range passphrase {
		passphrase[i] = 0
	}
	if err != nil {
		die("Keystore decryption failed: %v", err)
	}

	slog.Info("keystore decrypted", "wallet", address)
	return key
}

func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()

	if runtime.GOOS != "windows" {
		if info, err := f.Stat(); err == nil {
			if info.Mode().Perm()&0077 != 0 {
				slog.Warn(".env has loose permissions, should be 0600",
					"current", fmt.Sprintf("%04o", info.Mode().Perm()))
			}
		}
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = strings.Trim(val, "\"'")
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func privateKeyFromEnv() string {
	return os.Getenv("HL_PRIVATE_KEY")
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[38;2;94;234;212m"
)

func color(s, code string) string {
	return code + s + colorReset
}

func botCmd() string {
	if runtime.GOOS == "windows" {
		return ".\\bot.exe"
	}
	return "./bot"
}
