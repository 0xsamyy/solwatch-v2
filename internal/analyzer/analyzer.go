package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var solanaAddressRegex = regexp.MustCompile(`[1-9A-HJ-NP-Za-km-z]{32,44}`)

type Analyzer struct {
	HeliusTxURL   string
	SolanaRPCURL  string // The mainnet-beta RPC for on-chain lookups
	httpClient    *http.Client
	metadataCache *sync.Map
	priceOracle   *PriceOracle
}

func New(heliusTxURL, solanaRPCURL string) *Analyzer {
	cache := &sync.Map{}
	cache.Store(wsolMint, TokenMetadata{Symbol: "SOL", Decimals: 9})
	cache.Store(usdcMint, TokenMetadata{Symbol: "USDC", Decimals: 6})

	return &Analyzer{
		HeliusTxURL:   heliusTxURL,
		SolanaRPCURL:  solanaRPCURL,                            // Store the public RPC URL
		httpClient:    &http.Client{Timeout: 20 * time.Second}, // Increased timeout for RPC calls
		metadataCache: cache,
		priceOracle:   NewPriceOracle(),
	}
}

func (a *Analyzer) AnalyzeSignature(ctx context.Context, signature, trackedAddr string) (string, error) {
	tx, err := fetchHeliusTransaction(ctx, signature, a.HeliusTxURL, a.httpClient)
	if err != nil {
		return "", fmt.Errorf("failed to fetch tx %s: %w", signature, err)
	}

	if shouldFilter(tx, trackedAddr) {
		return "", nil
	}

	a.ensureMetadataIsCached(ctx, tx)

	var sent, received []string
	var interpretation string
	metadataMap := a.getMetadataMap()

	switch tx.Type {
	case "CREATE":
		sent, received = calculateNetBalanceChanges(tx, trackedAddr, metadataMap, a.priceOracle)
		tokenName := "new token"
		if len(received) > 0 {
			tokenName = received[0]
		}
		interpretation = fmt.Sprintf("🧱 CREATE & BUY via %s: Bought %s", tx.Source, tokenName)
	case "SWAP":
		sent, received = a.parseSwapEvent(tx, trackedAddr, metadataMap)
		interpretation = fmt.Sprintf("🔁 SWAP via %s", tx.Source)
	default:
		sent, received = calculateNetBalanceChanges(tx, trackedAddr, metadataMap, a.priceOracle)
		if len(sent) > 0 && len(received) > 0 {
			interpretation = fmt.Sprintf("↔️ INTERACTION via %s", tx.Source)
		} else if len(sent) > 0 {
			interpretation = fmt.Sprintf("⬆️ SEND via %s", tx.Source)
		} else if len(received) > 0 {
			interpretation = fmt.Sprintf("⬇️ RECEIVE via %s", tx.Source)
		} else {
			interpretation = fmt.Sprintf("⚙️ %s via %s", strings.ToTitle(strings.ToLower(tx.Type)), tx.Source)
		}
	}
	return a.buildSummary(tx, interpretation, sent, received), nil
}

func (a *Analyzer) ensureMetadataIsCached(ctx context.Context, tx *HeliusTransaction) {
	mints := make(map[string]bool)
	for _, transfer := range tx.TokenTransfers {
		if transfer.Mint != "" {
			mints[transfer.Mint] = true
		}
	}
	if tx.Events.Swap != nil {
		for _, item := range tx.Events.Swap.TokenInputs {
			mints[item.Mint] = true
		}
		for _, item := range tx.Events.Swap.TokenOutputs {
			mints[item.Mint] = true
		}
	}

	for mint := range mints {
		if _, found := a.metadataCache.Load(mint); !found {
			meta, err := fetchOnChainMetadata(ctx, mint, a.SolanaRPCURL, a.httpClient)
			if err != nil {
				log.Printf("[analyzer] failed to fetch on-chain metadata for %s: %v. Using fallback.", mint, err)
				a.metadataCache.Store(mint, TokenMetadata{Symbol: fmt.Sprintf("Mint(%s)", shortenAddress(mint)), Decimals: 6})
				continue
			}
			log.Printf("[analyzer] fetched and cached on-chain metadata for %s (%s)", mint, meta.Symbol)
			a.metadataCache.Store(mint, *meta)
		}
	}
}

func (a *Analyzer) buildSummary(tx *HeliusTransaction, interpretation string, sent, received []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>%s</b>\n", interpretation))
	if tx.Description != "" {
		cleanedDesc := solanaAddressRegex.ReplaceAllStringFunc(tx.Description, func(addr string) string {
			if len(addr) > 8 {
				return fmt.Sprintf("%s...%s", addr[:4], addr[len(addr)-4:])
			}
			return addr
		})
		b.WriteString(fmt.Sprintf("ℹ️ <i>%s</i>\n", cleanedDesc))
	}
	b.WriteString("\n")
	if len(sent) > 0 {
		b.WriteString(fmt.Sprintf("💰 <b>Sent:</b> %s\n", strings.Join(sent, ", ")))
	}
	if len(received) > 0 {
		b.WriteString(fmt.Sprintf("💸 <b>Received:</b> %s\n", strings.Join(received, ", ")))
	}
	b.WriteString(fmt.Sprintf("\n<a href=\"https://solscan.io/tx/%s\">%s...%s</a>", tx.Signature, tx.Signature[:6], tx.Signature[len(tx.Signature)-6:]))
	return b.String()
}
func (a *Analyzer) parseSwapEvent(tx *HeliusTransaction, trackedAddr string, metadataMap map[string]TokenMetadata) (sent, received []string) {
	if tx.Events.Swap == nil {
		return calculateNetBalanceChanges(tx, trackedAddr, metadataMap, a.priceOracle)
	}
	addFormattedItem := func(list *[]string, item TokenSwapAmount) {
		amount := parseAmount(item.RawTokenAmount.TokenAmount, item.RawTokenAmount.Decimals)
		meta, ok := metadataMap[item.Mint]
		if !ok { // Should be rare now
			meta = TokenMetadata{Symbol: fmt.Sprintf("Mint(%s)", shortenAddress(item.Mint)), Decimals: item.RawTokenAmount.Decimals}
		}
		formattedStr := fmt.Sprintf("%s %s", formatHumanReadable(amount), meta.Symbol)
		if coinID, isTracked := isPriceTracked(item.Mint); isTracked {
			if price, ok := a.priceOracle.GetPriceUSD(context.Background(), coinID); ok {
				usdValue := amount * price
				formattedStr += fmt.Sprintf(" ($%.2f)", usdValue)
			}
		}
		*list = append(*list, formattedStr)
	}
	for _, item := range tx.Events.Swap.TokenInputs {
		if item.UserAccount == trackedAddr {
			addFormattedItem(&sent, item)
		}
	}
	for _, item := range tx.Events.Swap.TokenOutputs {
		if item.UserAccount == trackedAddr {
			addFormattedItem(&received, item)
		}
	}
	return sent, received
}
func shortenAddress(addr string) string {
	if len(addr) <= 8 {
		return fmt.Sprintf("<code>%s</code>", addr)
	}
	shortened := addr[:4] + "..." + addr[len(addr)-4:]
	return fmt.Sprintf("<code>%s</code>", shortened)
}
func (a *Analyzer) getMetadataMap() map[string]TokenMetadata {
	m := make(map[string]TokenMetadata)
	a.metadataCache.Range(func(key, value any) bool {
		m[key.(string)] = value.(TokenMetadata)
		return true
	})
	return m
}

type PriceOracle struct {
	httpClient *http.Client
	cache      *sync.Map
}
type cachedPrice struct {
	Price       float64
	LastFetched time.Time
}

func NewPriceOracle() *PriceOracle {
	return &PriceOracle{httpClient: &http.Client{Timeout: 5 * time.Second}, cache: &sync.Map{}}
}
func (o *PriceOracle) GetPriceUSD(ctx context.Context, coinID string) (float64, bool) {
	if val, found := o.cache.Load(coinID); found {
		if time.Since(val.(cachedPrice).LastFetched) < 60*time.Second {
			return val.(cachedPrice).Price, true
		}
	}
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd", coinID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	var result map[string]map[string]float64
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if priceData, ok := result[coinID]; ok {
		if price, ok := priceData["usd"]; ok {
			o.cache.Store(coinID, cachedPrice{Price: price, LastFetched: time.Now()})
			return price, true
		}
	}
	return 0, false
}
