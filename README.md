# solwatch v2

solwatch v2 is a self-hosted Telegram bot for monitoring Solana wallet activity. It listens for user-signed transactions over WebSocket, enriches them with Helius data, resolves token metadata on-chain, and sends concise summaries to Telegram.

## Features
- Transaction enrichment via Helius (swaps, creates, transfers)
- On-chain token metadata resolution with caching
- Dust and spam filtering
- USD value hints for SOL and USDC via CoinGecko
- Persistent wallet storage with automatic resubscribe
- `/test` command for replaying a transaction signature

## Requirements
- Go 1.25
- Telegram bot token and admin chat ID
- Helius API key (WebSocket and REST)
- Solana RPC endpoint for metadata lookups

## Quick start
1. Clone and build

```bash
git clone https://github.com/0xsamyy/solwatch-v2.git
cd solwatch-v2
go build ./cmd/solwatch
```

2. Configure

```bash
cp .env.example .env
```

Edit `.env` with your values.

```dotenv
TELEGRAM_BOT_TOKEN=YOUR_TELEGRAM_BOT_TOKEN
TELEGRAM_ADMIN_CHAT_ID=YOUR_NUMERIC_TELEGRAM_CHAT_ID
HELIUS_WSS=wss://mainnet.helius-rpc.com/?api-key=YOUR_API_KEY
HELIUS_API_URL=https://api.helius.xyz/v0/transactions/?api-key=YOUR_API_KEY
SOLANA_RPC_URL=https://api.mainnet-beta.solana.com
DB_PATH=solwatch.db
COMMITMENT=processed
```

3. Run

```bash
./solwatch
```

Or run directly from source:

```bash
go run ./cmd/solwatch
```

## Configuration

| Variable | Description |
| --- | --- |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token |
| `TELEGRAM_ADMIN_CHAT_ID` | Chat ID that receives notifications |
| `HELIUS_WSS` | Helius WebSocket URL with API key |
| `HELIUS_API_URL` | Helius REST URL with API key |
| `SOLANA_RPC_URL` | Solana RPC for on-chain metadata lookups |
| `DB_PATH` | Path to the BoltDB file |
| `COMMITMENT` | Solana commitment level (e.g. `processed`) |

## Example notification

```
Activity on 5hAg...G84zM

SWAP via PUMP_AMM
User swapped 9.40 SOL for 3,772,284 Sora

Transaction Breakdown:
  Sent: 9.40 SOL ($1,650.12)
  Received: 3,772,284 Sora

https://solscan.io/tx/2JsXQv...k9Rk
```

## How it works
1. Subscribe to `logsSubscribe` and detect user-signed transactions for tracked wallets.
2. Fetch transaction details from the Helius API.
3. Resolve token metadata on-chain and cache it.
4. Build and send a formatted summary to Telegram.

## Commands

| Command | Description |
| --- | --- |
| `/help` | Show available commands |
| `/track <address>` | Start tracking a wallet |
| `/untrack <address>` | Stop tracking a wallet |
| `/trackmany <addr1> <addr2> ...` | Track multiple wallets |
| `/untrackmany <addr1> <addr2> ...` | Untrack multiple wallets |
| `/tracked` | List tracked wallets |
| `/health` | Show service statistics |
| `/kill` | Gracefully shut down the bot |
| `/test <signature> <address>` | Run analysis on a past signature |

## Maintainer
- GitHub: https://github.com/0xsamyy
- Telegram: https://t.me/ox_fbac
