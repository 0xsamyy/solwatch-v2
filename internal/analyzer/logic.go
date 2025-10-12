// internal/analyzer/logic.go
package analyzer

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	usdcMint        = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	wsolMint        = "So11111111111111111111111111111111111111112"
	filterThreshold = 0.0001
	lamportsPerSol  = 1_000_000_000
)

// isPriceTracked checks if a mint is SOL/USDC and returns its CoinGecko ID.
func isPriceTracked(mint string) (string, bool) {
	switch mint {
	case wsolMint:
		return "solana", true
	case usdcMint:
		return "usd-coin", true
	default:
		return "", false
	}
}

// shouldFilter ignores tiny dust-only SOL moves when no other tokens move.
func shouldFilter(tx *HeliusTransaction, trackedAddr string) bool {
	if tx.TransactionError != nil && string(*tx.TransactionError) != "null" {
		return false
	}

	// Native SOL change (includes fees)
	var nativeChange int64
	for _, ad := range tx.AccountData {
		if ad.Account == trackedAddr {
			nativeChange = ad.NativeBalanceChange
			break
		}
	}
	solValueChange := math.Abs(float64(nativeChange) / lamportsPerSol)

	// Did any non-WSOL tokens move for the user?
	hasOtherTokens := false
	for _, tt := range tx.TokenTransfers {
		if tt.Mint == wsolMint {
			continue
		}
		if tt.FromUserAccount == trackedAddr || tt.ToUserAccount == trackedAddr {
			hasOtherTokens = true
			break
		}
	}

	return !hasOtherTokens && solValueChange < filterThreshold
}

// calculateNetBalanceChanges nets balances for the tracked address.
//
// Core SOL rule:
//   - Trust ONLY accountData.nativeBalanceChange for SOL exposure.
//   - DO NOT add negative WSOL deltas (those are usually spends of newly wrapped SOL).
//   - Add positive WSOL delta to SOL ONLY if there was a WSOL inflow from a different user.
//
// Everything else (non-WSOL SPL) is summed normally across the tx.
//
// We ignore nativeTransfers entirely (wrap/unwrap/rent noise).
func calculateNetBalanceChanges(
	tx *HeliusTransaction,
	trackedAddr string,
	metadataCache map[string]TokenMetadata,
	oracle *PriceOracle,
) (sent []string, received []string) {

	// 1) Per-mint SPL deltas for the tracked user
	tokenDeltas := make(map[string]float64)

	// Track whether we saw any WSOL inflow from someone else (not a self wrap)
	wsolInflowFromOther := false

	for _, tt := range tx.TokenTransfers {
		if tt.FromUserAccount == trackedAddr {
			tokenDeltas[tt.Mint] -= tt.TokenAmount
		}
		if tt.ToUserAccount == trackedAddr {
			tokenDeltas[tt.Mint] += tt.TokenAmount
			// Detect true WSOL inflow (from another user)
			if tt.Mint == wsolMint && tt.FromUserAccount != trackedAddr {
				wsolInflowFromOther = true
			}
		}
	}

	// 2) Native SOL net (includes fees) for the tracked user
	var nativeChangeLamports int64
	for _, ad := range tx.AccountData {
		if ad.Account == trackedAddr {
			nativeChangeLamports = ad.NativeBalanceChange
			break
		}
	}
	nativeSol := float64(nativeChangeLamports) / lamportsPerSol

	// 3) WSOL handling (see rule above)
	wsolDelta := tokenDeltas[wsolMint]
	delete(tokenDeltas, wsolMint)

	// Start with native SOL only
	totalSolChange := nativeSol

	// If user truly received WSOL from someone else, add only the positive net to SOL.
	// Never subtract negative WSOL deltas (avoids double count when wrapping + spending).
	if wsolInflowFromOther && wsolDelta > 1e-12 {
		totalSolChange += wsolDelta
	}

	// 4) Emit SOL (with USD)
	if math.Abs(totalSolChange) > 1e-12 {
		amount := math.Abs(totalSolChange)
		formatted := fmt.Sprintf("%s SOL", formatHumanReadable(amount))
		if price, ok := oracle.GetPriceUSD(context.Background(), "solana"); ok {
			usd := amount * price
			formatted += fmt.Sprintf(" ($%.2f)", usd)
		}
		if totalSolChange > 0 {
			received = append(received, formatted)
		} else {
			sent = append(sent, formatted)
		}
	}

	// 5) Emit remaining SPL tokens
	for mint, delta := range tokenDeltas {
		if math.Abs(delta) < 1e-18 {
			continue
		}
		amount := math.Abs(delta)

		meta, ok := metadataCache[mint]
		if !ok {
			meta = TokenMetadata{Symbol: fmt.Sprintf("Mint(%s...)", mint[:4]), Decimals: 6}
		}

		formatted := fmt.Sprintf("%s %s", formatHumanReadable(amount), meta.Symbol)

		if coinID, tracked := isPriceTracked(mint); tracked {
			if price, ok := oracle.GetPriceUSD(context.Background(), coinID); ok {
				usd := amount * price
				formatted += fmt.Sprintf(" ($%.2f)", usd)
			}
		}

		if delta > 0 {
			received = append(received, formatted)
		} else {
			sent = append(sent, formatted)
		}
	}

	return sent, received
}

// parseAmount is a new helper from analyzer.go, consolidated here for reuse.
func parseAmount(amountStr string, decimals int) float64 {
	val, _ := strconv.ParseFloat(amountStr, 64)
	return val / math.Pow10(decimals)
}

// formatHumanReadable formats numbers according to the specific rules:
// - Adds thousand separators to the integer part.
// - For numbers >= 1000, shows 0 decimal places.
// - For numbers >= 1, shows 2 decimal places.
// - For numbers < 1, shows 3 significant figures (e.g., 0.123 or 0.000123).
func formatHumanReadable(f float64) string {
	// Rule for numbers >= 1
	if f >= 1 {
		prec := 2
		if f >= 1000 {
			prec = 0
		}

		// Format with precision and split into integer and fractional parts
		s := strconv.FormatFloat(f, 'f', prec, 64)
		parts := strings.Split(s, ".")
		integerPart := parts[0]
		fractionalPart := ""
		if len(parts) > 1 {
			fractionalPart = "." + parts[1]
		}

		// Add thousand separators to the integer part
		n := len(integerPart)
		if n <= 3 {
			return s
		}
		// Calculate the number of commas
		commas := (n - 1) / 3
		// Create a new string with space for commas
		b := make([]byte, n+commas)
		// Fill from right to left
		for i, j, k := n-1, len(b)-1, 0; ; i, j = i-1, j-1 {
			b[j] = integerPart[i]
			if i == 0 {
				return string(b) + fractionalPart
			}
			k++
			if k%3 == 0 {
				j--
				b[j] = ','
			}
		}
	}

	// Rule for numbers < 1 (3 significant figures)
	s := strconv.FormatFloat(f, 'g', 3, 64)
	return s
}
