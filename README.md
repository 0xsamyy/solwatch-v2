# 🔍 solwatch V2: The Intelligent Solana Wallet Tracker

`solwatch V2` is a complete overhaul of the original wallet monitor. It's a robust, self-hosted Telegram bot that provides **deep, human-readable analysis** of wallet activity on the Solana blockchain. Instead of just telling you *that* something happened, V2 tells you **exactly what happened.**

Built in Go for high performance and reliability, V2 uses the Solana WebSocket API to detect activity and a powerful analysis pipeline to interpret it, delivering rich, real-time insights directly to your Telegram chat.

-----

## ✨ V2 Features: Beyond Simple Notifications

V2 inherits all the robustness of V1 and adds a powerful new analysis engine:

  - ✅ **Deep Transaction Analysis:** Instantly understand swaps, token creations (pump.fun), and other DeFi interactions.
  - ✅ **Intelligent Filtering:** Automatically ignores insignificant dust transactions and spam, keeping your feed clean.
  - ✅ **Real-Time Price Oracle:** Converts SOL and USDC amounts to their current USD value, powered by the CoinGecko API.
  - ✅ **On-Chain Metadata:** Automatically fetches token symbols and decimals directly from the Solana blockchain, ensuring accuracy for new and obscure tokens.
  - ✅ **Precise Detection:** Uses `logsSubscribe` to capture only user-signed transactions, providing a high-signal feed of a wallet's actions.
  - ✅ **Debug Command:** A powerful `/test` command allows you to re-run the analysis on any past transaction signature.
  - ✅ **Persistence & Reliability:** All V1 features like BoltDB storage, automatic reconnects, and graceful shutdowns are retained and improved.

-----

## 📸 Example Notification

V2 replaces the simple "Activity Detected" alert with a rich, detailed summary.

**A Swap on a Pump.fun AMM:**

```
🚨 Activity on 5hAg...G84zM

➡️ SWAP via PUMP_AMM
ℹ️ User swapped 9.40 SOL for 3,772,284 Sora

💰 Transaction Breakdown:
  📤 Sent: 9.40 SOL ($1,650.12)
  📥 Received: 3,772,284 Sora

<a href="https://solscan.io/tx/2JsXQv...">2JsXQv...k9Rk</a>
```

-----

## 🚀 Quick Start

### 1\. Clone & Build

```bash
git clone https://github.com/0xsamyy/solwatch-v2.git
cd solwatch-v2
go build ./cmd/solwatch
```

### 2\. Configure

Copy the example `.env` file. This file will hold your private API keys and settings.

```bash
cp .env.example .env
```

Now, open the `.env` file and fill in your values.

```dotenv
# Telegram Bot Credentials
TELEGRAM_BOT_TOKEN=YOUR_TELEGRAM_BOT_TOKEN
TELEGRAM_ADMIN_CHAT_ID=YOUR_NUMERIC_TELEGRAM_CHAT_ID

# Helius WebSocket & API
# Ensure the API key is the same for both.
HELIUS_WSS=wss://mainnet.helius-rpc.com/?api-key=YOUR_API_KEY
HELIUS_API_URL=https://api.helius.xyz/v0/transactions/?api-key=YOUR_API_KEY

# Public Solana RPC for on-chain token metadata lookups
SOLANA_RPC_URL=https://api.mainnet-beta.solana.com

# --- Service Configuration ---
DB_PATH=solwatch.db
COMMITMENT=processed
```

### 3\. Run

Execute the compiled binary or run directly from the source code.

```bash
# Run the compiled binary
./solwatch

# Or run directly
go run ./cmd/solwatch
```

The bot will start, connect to Telegram, and begin listening for commands. To stop it, press `Ctrl + C`.

-----

## 🔬 How It Works: The V2 Analysis Pipeline

`solwatch V2` employs a sophisticated, multi-stage pipeline to turn a raw blockchain event into a human-readable summary.

1.  **Detection (WebSocket):** The bot establishes a persistent WebSocket connection to the Helius RPC using `logsSubscribe`. It listens for any logs that `mention` a tracked wallet address as a signer, providing an instant signal that the user has initiated a transaction.

2.  **Signature Extraction:** From the WebSocket message, the bot extracts the unique transaction **signature**. A de-duplication cache ensures that multiple log events from a single complex transaction only trigger one analysis.

3.  **Enrichment (Helius API):** The signature is sent to the Helius Transaction API. Helius provides a powerful, pre-parsed JSON object detailing the transaction type (`SWAP`, `CREATE`, etc.), a high-level description, and a breakdown of all token and SOL movements.

4.  **Metadata Fetching (On-Chain):** For any token mint address the bot hasn't seen before, it queries the public Solana RPC (`mainnet-beta`). It performs the same on-chain lookups as the Python CLI to find the Metaplex Metadata PDA and parse the raw Borsh-encoded account data to extract the token's official **symbol** and **decimals**. This information is then cached indefinitely to minimize future RPC calls.

5.  **Analysis & Formatting:** The enriched data is processed by the Go analyzer.

      * It first checks if the transaction is insignificant "dust" and should be ignored.
      * It uses a prioritized system: it trusts Helius's high-level `type` and `events` data first. If that's unavailable, it falls back to calculating the net balance changes.
      * It calls the CoinGecko API to get real-time USD prices for SOL and USDC.
      * Finally, it assembles the clean, formatted HTML message with human-readable numbers and shortened addresses.

6.  **Notification (Telegram):** The final formatted summary is sent to your Telegram chat.

This entire pipeline executes in seconds, providing near real-time intelligence on wallet activity.

-----

## 🛠 Commands

| Command | Description |
| :--- | :--- |
| `/help` | Shows the list of available commands. |
| `/track <address>` | Starts tracking a new wallet. |
| `/untrack <address>` | Stops tracking a wallet. |
| `/trackmany <addr1> <addr2> ...` | Tracks multiple wallets in a single command. |
| `/untrackmany <addr1> <addr2> ...` | Untracks multiple wallets. |
| `/tracked` | Displays a list of all currently tracked wallets. |
| `/health` | Shows service statistics, including active and dropped connections. |
| `/kill` | Remotely and gracefully shuts down the bot service. |
| `/test <signature> <address>` | **[Debug]** Manually runs the analysis pipeline on any past transaction signature for a specified wallet. This is extremely useful for testing and debugging. |

-----

## ⚙️ Tech Details

  * **Language**: Go 1.25
  * **Database**: BoltDB (for persistent wallet storage)
  * **APIs & Networking**:
      * Gorilla WebSocket for `logsSubscribe`.
      * Helius API for transaction enrichment.
      * Public Solana RPC for on-chain metadata resolution.
      * CoinGecko API for USD price data.
  * **Telegram API**: `github.com/go-telegram/bot`
  * **Concurrency**: Each tracked wallet runs in its own lightweight goroutine for high performance.

-----

## 🔒 Robustness

  * **Automatic Reconnects:** Uses exponential backoff with jitter to gracefully handle network errors or RPC downtime.
  * **Heartbeat Pings:** Actively keeps WebSocket connections alive.
  * **Persistent Storage:** Tracked wallets are saved to disk and automatically re-subscribed when the bot restarts.
  * **Graceful Shutdown:** The `/kill` command or a `SIGTERM` signal allows the bot to shut down cleanly.

-----

## 🧑‍💻 Author

[![GitHub](https://img.shields.io/badge/GitHub-0xsamyy-black?logo=github)](https://github.com/0xsamyy)
[![Telegram](https://img.shields.io/badge/Telegram-@ox__fbac-2CA5E0?logo=telegram&logoColor=white)](https://t.me/ox_fbac)

Vibecoded all the way so don't give me too much credit. Thx GPT & Gemini <3