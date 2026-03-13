# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in DegenBox, please report it responsibly.

**Email:** security@scheme24.com

Please include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact

We will acknowledge your report within 48 hours and aim to release a fix within 7 days for critical issues.

## Scope

This policy covers the DegenBox client (`degenbox-hyperliquid-client`). For server-side or dashboard vulnerabilities, please use the same email.

## Security Design

- **Private keys never leave your machine.** The relay server sends signed instructions — the client verifies and executes locally.
- **Ed25519 signature verification** on all trading instructions prevents tampering.
- **Encrypted keystore** (AES-256-GCM + Argon2id) protects keys at rest.
- **Risk validation** enforces configurable limits on leverage, order size, and rate before any trade executes.
