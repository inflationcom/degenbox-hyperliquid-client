# DegenBox

[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A lightweight trading executor for [Hyperliquid](https://hyperliquid.xyz). Connects to a signal relay server over WebSocket, validates incoming instructions against configurable risk limits, and executes trades on your behalf.

Your private key **never leaves your machine**. The server sends signed instructions — this client verifies and executes them locally.

## Features

- **Signal relay** — Receives signed trading instructions over WebSocket with Ed25519 signature verification
- **Risk validation** — Configurable limits on leverage, order size, price deviation, and rate limiting
- **Encrypted keystore** — AES-256-GCM + Argon2id encryption for private key storage
- **Agent wallet support** — Operate with delegated wallet permissions
- **Terminal UI** — Live account state, positions, trade history, and log viewer

## Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- A [Hyperliquid](https://app.hyperliquid.xyz) account with funds
- A registration token (`rt_`) from the [dashboard](https://scheme24.com)

## Quick Start

```bash
git clone https://github.com/inflationcom/degenbox-hyperliquid-client.git
cd degenbox-hyperliquid-client
make build
./bin/bot setup
./bin/bot
```

The interactive setup wizard walks you through network selection, private key input, relay server registration, and risk limit configuration.

For a full step-by-step guide (including VPS setup, key encryption, and tmux), see the [Setup Guide](https://scheme24.com/docs).

## Usage

```
bot <command> [options]

Commands:
  setup         Set up your bot (interactive)
  run           Connect to relay server and execute trades
  config        View current configuration
  encrypt-key   Encrypt your private key with a passphrase
  version       Show version info
```

## Configuration

Configuration is loaded in order of precedence:

1. Command-line flags
2. Environment variables (`HL_*`)
3. `config.json`
4. `.env` file

See [config.example.json](config.example.json) for all available options.

### Environment Variables

| Variable | Description |
|---|---|
| `HL_PRIVATE_KEY` | Wallet private key (hex) |
| `HL_NETWORK` | `mainnet` or `testnet` |
| `HL_WALLET_ADDR` | Main wallet address (agent mode) |
| `HL_AGENT_MODE` | Enable agent wallet mode |
| `HL_RELAY_URL` | Signal relay WebSocket URL |
| `HL_RELAY_API_KEY` | Relay authentication key |
| `HL_RELAY_CLIENT_ID` | Relay client identifier |
| `HL_SERVER_PUBLIC_KEY` | Ed25519 public key for instruction verification |
| `HL_MAX_LEVERAGE` | Maximum allowed leverage |
| `HL_MAX_ORDER_SIZE_USD` | Maximum order notional value |

### Encrypted Keystore

Instead of storing your private key in `.env`, you can encrypt it:

```bash
./bin/bot encrypt-key
```

On startup, the bot will prompt for your passphrase. The keystore uses Argon2id key derivation with AES-256-GCM authenticated encryption.

## Architecture

```
cmd/
  bot/            CLI entrypoint, TUI, setup wizard
  healthcheck/    Docker healthcheck binary
internal/
  config/         Configuration loading and validation
  hyperliquid/    Hyperliquid API client, EIP-712 signing
  keystore/       Encrypted private key storage
  relay/          WebSocket relay client, instruction execution, risk validation
```

## Development

```bash
make deps       # Download dependencies
make build      # Build binaries
make test       # Run tests
make lint       # Run linter
make fmt        # Format code
```

## License

[MIT](LICENSE)
