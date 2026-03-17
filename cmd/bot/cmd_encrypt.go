package main

import (
	"flag"
	"fmt"
	"strings"
	"syscall"

	"github.com/inflationcom/degenbox-hyperliquid-client/internal/keystore"
	"golang.org/x/term"
)

const defaultKeystorePath = ".keystore.json"

func cmdEncryptKey(args []string) {
	fs := flag.NewFlagSet("encrypt-key", flag.ExitOnError)
	keystorePath := fs.String("keystore", defaultKeystorePath, "Output path for encrypted keystore")
	inputKey := fs.String("key", "", "Private key hex (if empty, reads from .env)")
	fs.Parse(args)

	privateKey := *inputKey
	if privateKey == "" {
		privateKey = privateKeyFromEnv()
	}
	if privateKey == "" {
		die("No private key found. Provide --key flag, set HL_PRIVATE_KEY, or create .env file.")
	}

	privateKey = strings.TrimPrefix(privateKey, "0x")

	if keystore.Exists(*keystorePath) {
		fmt.Printf("Keystore already exists at %s\n", *keystorePath)
		fmt.Print("Overwrite? (y/N): ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(answer) != "y" {
			fmt.Println("Aborted.")
			return
		}
	}

	fmt.Print("Enter passphrase: ")
	pass1, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		die("Failed to read passphrase: %v", err)
	}
	if len(pass1) < 8 {
		die("Passphrase must be at least 8 characters.")
	}

	fmt.Print("Confirm passphrase: ")
	pass2, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		die("Failed to read passphrase: %v", err)
	}

	if string(pass1) != string(pass2) {
		die("Passphrases do not match.")
	}

	fmt.Println("Encrypting key (this takes a few seconds)...")
	if err := keystore.Encrypt(privateKey, pass1, *keystorePath); err != nil {
		die("Encryption failed: %v", err)
	}

	address, err := keystore.ReadAddress(*keystorePath)
	if err != nil {
		die("Failed to read back keystore: %v", err)
	}

	fmt.Printf("\nKeystore saved to %s\n", *keystorePath)
	fmt.Printf("Wallet address: %s\n", address)
	fmt.Println("\nYou can now delete your .env file (or remove HL_PRIVATE_KEY from it).")
	fmt.Println("On next bot start, you'll be prompted for your passphrase.")

	for i := range pass1 {
		pass1[i] = 0
	}
	for i := range pass2 {
		pass2[i] = 0
	}
}
