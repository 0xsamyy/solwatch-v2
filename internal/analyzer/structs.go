package analyzer

import "encoding/json"

type HeliusTransaction struct {
	Signature        string            `json:"signature"`
	Timestamp        int64             `json:"timestamp"`
	Fee              int64             `json:"fee"`
	FeePayer         string            `json:"feePayer"`
	Type             string            `json:"type"`
	Source           string            `json:"source"`
	Description      string            `json:"description"`
	TokenTransfers   []TokenTransfer   `json:"tokenTransfers"`
	NativeTransfers  []NativeTransfer  `json:"nativeTransfers"`
	AccountData      []AccountData     `json:"accountData"`
	TransactionError *json.RawMessage  `json:"transactionError"`
	Events           TransactionEvents `json:"events"`
}
type TokenTransfer struct {
	FromTokenAccount string  `json:"fromTokenAccount"`
	ToTokenAccount   string  `json:"toTokenAccount"`
	FromUserAccount  string  `json:"fromUserAccount"`
	ToUserAccount    string  `json:"toUserAccount"`
	Mint             string  `json:"mint"`
	TokenAmount      float64 `json:"tokenAmount"`
	TokenStandard    string  `json:"tokenStandard,omitempty"`
}
type NativeTransfer struct {
	FromUserAccount string `json:"fromUserAccount"`
	ToUserAccount   string `json:"toUserAccount"`
	Amount          int64  `json:"amount"` // lamports
}
type AccountData struct {
	Account             string               `json:"account"`
	NativeBalanceChange int64                `json:"nativeBalanceChange"`
	TokenBalanceChanges []TokenBalanceChange `json:"tokenBalanceChanges"`
}
type TokenBalanceChange struct {
	UserAccount    string         `json:"userAccount"`
	TokenAccount   string         `json:"tokenAccount"`
	RawTokenAmount RawTokenAmount `json:"rawTokenAmount"`
	Mint           string         `json:"mint"`
}
type TransactionEvents struct {
	Swap *SwapEvent `json:"swap"`
}
type SwapEvent struct {
	TokenInputs  []TokenSwapAmount `json:"tokenInputs"`
	TokenOutputs []TokenSwapAmount `json:"tokenOutputs"`
}
type TokenSwapAmount struct {
	UserAccount    string         `json:"userAccount"`
	RawTokenAmount RawTokenAmount `json:"rawTokenAmount"`
	Mint           string         `json:"mint"`
}
type RawTokenAmount struct {
	TokenAmount string `json:"tokenAmount"`
	Decimals    int    `json:"decimals"`
}
type TokenMetadata struct {
	Symbol   string
	Decimals int
}
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// GetAccountInfoResponse is for jsonParsed requests.
type GetAccountInfoResponse struct {
	Result struct {
		Value struct {
			Owner string `json:"owner"`
			Data  struct {
				Parsed struct {
					Info struct {
						Decimals int `json:"decimals"`
					} `json:"info"`
				} `json:"parsed"`
			} `json:"data"`
		} `json:"value"`
	} `json:"result"`
}

// GetAccountInfoResponse_Base64 is for base64 requests.
type GetAccountInfoResponse_Base64 struct {
	Result struct {
		Value struct {
			Data []string `json:"data"` // e.g., ["base64_string", "base64"]
		} `json:"value"`
	} `json:"result"`
}

type GetProgramAccountsResponse struct {
	Result []struct {
		Pubkey string `json:"pubkey"`
	} `json:"result"`
}
