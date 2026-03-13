<p align="center">
  <img src="assets/logo.png" width="120" alt="DegenBox" />
</p>

<h1 align="center">DegenBox</h1>

<p align="center">
  <strong>Self-custodial trading executor for Hyperliquid</strong>
</p>

<p align="center">
  <a href="https://go.dev/dl/"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go 1.24+" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT" /></a>
  <a href="https://github.com/inflationcom/degenbox-hyperliquid-client/releases/latest"><img src="https://img.shields.io/github/v/release/inflationcom/degenbox-hyperliquid-client?label=release&color=5CDB95" alt="Latest Release" /></a>
</p>

<p align="center">
  <img src="assets/screenshot.png" width="700" alt="DegenBox TUI" />
</p>

---

Connects to a signal relay server over WebSocket, validates incoming instructions against configurable risk limits, and executes trades on [Hyperliquid](https://hyperliquid.xyz) on your behalf.

Your private key **never leaves your machine**. The server sends signed instructions — this client verifies and executes them locally.

## Features

- **Signal relay** — Receives signed trading instructions over WebSocket with Ed25519 signature verification
- **Risk validation** — Configurable limits on leverage, order size, price deviation, and rate limiting
- **Encrypted keystore** — AES-256-GCM + Argon2id encryption for private key storage
- **Agent wallet support** — Operate with delegated wallet permissions
- **Terminal UI** — Live account state, positions, trade history, settings, and log viewer

## Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- A [Hyperliquid](https://app.hyperliquid.xyz) account with funds
- A registration token (`rt_`) from the [dashboard](https://scheme24.com)

## Install

### Download (recommended)

Grab the latest binary for your platform from [GitHub Releases](https://github.com/inflationcom/degenbox-hyperliquid-client/releases/latest).

```bash
# Linux (x86_64)
curl -L -o bot https://github.com/inflationcom/degenbox-hyperliquid-client/releases/latest/download/bot-linux-amd64
chmod +x bot
./bot
```

### Build from source

Requires [Go 1.24+](https://go.dev/dl/).

```bash
git clone https://github.com/inflationcom/degenbox-hyperliquid-client.git
cd degenbox-hyperliquid-client
make build
./bin/bot
```

The bot walks you through setup on first run — just paste your private key and registration token. Use `--testnet` for testnet.

For a full step-by-step guide (including VPS setup, key encryption, and tmux), see the [Setup Guide](https://scheme24.com/docs).

## Usage

```
bot <command> [options]

Commands:
  run           Connect to relay and start trading (runs setup if needed)
  setup         Run the setup wizard manually
  config        View current configuration
  encrypt-key   Encrypt your private key with a passphrase
  version       Show version info

Flags:
  --testnet     Use testnet instead of mainnet
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
