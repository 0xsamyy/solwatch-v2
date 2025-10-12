// internal/analyzer/fetch.go
package analyzer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	splTokenProgramID         = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	metaplexMetadataProgramID = "metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s"
)

// fetchHeliusTransaction is unchanged.
func fetchHeliusTransaction(ctx context.Context, signature, heliusURL string, client *http.Client) (*HeliusTransaction, error) {
	payload := map[string][]string{"transactions": {signature}}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", heliusURL, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("helius api returned non-200 status: %d %s", resp.StatusCode, string(bodyBytes))
	}
	var transactions []HeliusTransaction
	if err := json.NewDecoder(resp.Body).Decode(&transactions); err != nil || len(transactions) == 0 {
		return nil, fmt.Errorf("failed to decode or empty helius response for signature %s", signature)
	}
	return &transactions[0], nil
}

// rpcCall is unchanged.
func rpcCall(ctx context.Context, rpcURL string, client *http.Client, method string, params []interface{}, result interface{}) error {
	payload := RPCRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", rpcURL, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rpc call to %s failed with status %d", rpcURL, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// fetchOnChainMetadata now uses the correct structs.
func fetchOnChainMetadata(ctx context.Context, mint, rpcURL string, client *http.Client) (*TokenMetadata, error) {
	// 1. Get account info to find owner and decimals.
	var accInfo GetAccountInfoResponse
	params := []interface{}{mint, map[string]string{"encoding": "jsonParsed"}}
	if err := rpcCall(ctx, rpcURL, client, "getAccountInfo", params, &accInfo); err != nil {
		return nil, fmt.Errorf("getAccountInfo for mint failed: %w", err)
	}

	owner := accInfo.Result.Value.Owner
	decimals := accInfo.Result.Value.Data.Parsed.Info.Decimals

	if owner != splTokenProgramID {
		return nil, fmt.Errorf("unsupported token program: %s", owner)
	}

	// 2. Find the Metaplex PDA.
	var progAccounts GetProgramAccountsResponse
	params = []interface{}{
		metaplexMetadataProgramID,
		map[string]interface{}{
			"encoding": "base64",
			"filters": []map[string]interface{}{
				{"memcmp": map[string]interface{}{"offset": 33, "bytes": mint}},
			},
		},
	}
	if err := rpcCall(ctx, rpcURL, client, "getProgramAccounts", params, &progAccounts); err != nil {
		return nil, fmt.Errorf("getProgramAccounts for pda failed: %w", err)
	}

	if len(progAccounts.Result) == 0 {
		return nil, errors.New("metaplex pda not found")
	}
	pdaAddress := progAccounts.Result[0].Pubkey

	// 3. Get the raw data of the PDA using the CORRECT struct.
	// --- FIX IS HERE ---
	var pdaInfo GetAccountInfoResponse_Base64
	params = []interface{}{pdaAddress, map[string]string{"encoding": "base64"}}
	if err := rpcCall(ctx, rpcURL, client, "getAccountInfo", params, &pdaInfo); err != nil {
		return nil, fmt.Errorf("getAccountInfo for pda (base64) failed: %w", err)
	}

	if len(pdaInfo.Result.Value.Data) < 1 {
		return nil, errors.New("pda has no data")
	}
	rawData, err := base64.StdEncoding.DecodeString(pdaInfo.Result.Value.Data[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode pda data: %w", err)
	}

	// 4. Parse the Borsh data to get the symbol.
	const headerOffset = 65
	if len(rawData) < headerOffset {
		return nil, errors.New("metadata account data is too short")
	}

	nameLen := binary.LittleEndian.Uint32(rawData[headerOffset : headerOffset+4])
	symbolOffset := headerOffset + 4 + int(nameLen)
	if symbolOffset+4 > len(rawData) {
		return nil, errors.New("failed to parse name: length exceeds buffer")
	}

	symbolLen := binary.LittleEndian.Uint32(rawData[symbolOffset : symbolOffset+4])
	symbolEnd := symbolOffset + 4 + int(symbolLen)
	if symbolEnd > len(rawData) {
		return nil, errors.New("failed to parse symbol: length exceeds buffer")
	}

	symbolBytes := rawData[symbolOffset+4 : symbolEnd]
	symbol := string(bytes.TrimRight(symbolBytes, "\x00")) // Trim trailing null bytes

	return &TokenMetadata{Symbol: symbol, Decimals: decimals}, nil
}
