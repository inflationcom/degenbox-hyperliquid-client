package hyperliquid

import (
	"encoding/json"
	"fmt"
)

const (
	MainnetInfoURL     = "https://api.hyperliquid.xyz/info"
	MainnetExchangeURL = "https://api.hyperliquid.xyz/exchange"

	TestnetInfoURL     = "https://api.hyperliquid-testnet.xyz/info"
	TestnetExchangeURL = "https://api.hyperliquid-testnet.xyz/exchange"

	MainnetChainID = 42161  // Arbitrum One
	TestnetChainID = 421614 // Arbitrum Sepolia
)

type MetaAndAssetCtxsRequest struct {
	Type string `json:"type"`
}

type Meta struct {
	Universe []AssetInfo `json:"universe"`
}

type AssetInfo struct {
	Name        string `json:"name"`
	SzDecimals  int    `json:"szDecimals"`
	MaxLeverage int    `json:"maxLeverage"`
}

type AssetCtx struct {
	Funding      string `json:"funding"`
	OpenInterest string `json:"openInterest"`
	OraclePx     string `json:"oraclePx"`
	DayNtlVlm   string `json:"dayNtlVlm"`
	Premium      string `json:"premium"`
}

type ExchangeRequest struct {
	Action       any        `json:"action"`
	Nonce        int64      `json:"nonce"`
	Signature    *Signature `json:"signature"`
	VaultAddress string     `json:"vaultAddress,omitempty"`
	ExpiresAfter int64      `json:"expiresAfter,omitempty"`
}

type Signature struct {
	R string `json:"r"`
	S string `json:"s"`
	V int    `json:"v"`
}

type OrderAction struct {
	Type     string      `json:"type"`
	Orders   []OrderWire `json:"orders"`
	Grouping Grouping    `json:"grouping"`
	Builder  *BuilderFee `json:"builder,omitempty"`
}

type BuilderFee struct {
	B string `json:"b"`
	F int    `json:"f"`
}

type OrderWire struct {
	A int       `json:"a"`
	B bool      `json:"b"`
	P string    `json:"p"`
	S string    `json:"s"`
	R bool      `json:"r"`
	T OrderType `json:"t"`
	C string    `json:"c,omitempty"`
}

type OrderType struct {
	Limit   *LimitSpec   `json:"limit,omitempty"`
	Trigger *TriggerWire `json:"trigger,omitempty"`
}

type LimitSpec struct {
	Tif string `json:"tif"`
}

type TriggerWire struct {
	IsMarket  bool   `json:"isMarket"`
	TriggerPx string `json:"triggerPx"`
	TpSl      string `json:"tpsl"`
}

type Grouping string

const (
	GroupingNA           Grouping = "na"
	GroupingNormalTpsl   Grouping = "normalTpsl"
	GroupingPositionTpsl Grouping = "positionTpsl"
)

type CancelAction struct {
	Type    string       `json:"type"`
	Cancels []CancelSpec `json:"cancels"`
}

type CancelSpec struct {
	A int   `json:"a"`
	O int64 `json:"o"`
}

type CancelByCloidAction struct {
	Type    string            `json:"type"`
	Cancels []CancelCloidSpec `json:"cancels"`
}

type CancelCloidSpec struct {
	Asset int    `json:"asset"`
	Cloid string `json:"cloid"`
}

type ModifyAction struct {
	Type     string       `json:"type"`
	Modifies []ModifySpec `json:"modifies"`
}

type ModifySpec struct {
	Oid   int64     `json:"oid"`
	Order OrderWire `json:"order"`
}

type UpdateLeverageAction struct {
	Type     string `json:"type"`
	Asset    int    `json:"asset"`
	IsCross  bool   `json:"isCross"`
	Leverage int    `json:"leverage"`
}

type ExchangeResponse struct {
	Status   string                `json:"status"`
	Response *ExchangeResponseData `json:"-"`
	RawError string                `json:"-"`
}

type exchangeResponseRaw struct {
	Status   string          `json:"status"`
	Response json.RawMessage `json:"response,omitempty"`
}

func (e *ExchangeResponse) UnmarshalJSON(data []byte) error {
	var raw exchangeResponseRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Status = raw.Status

	if len(raw.Response) == 0 {
		return nil
	}

	var s string
	if err := json.Unmarshal(raw.Response, &s); err == nil {
		e.RawError = s
		return nil
	}

	var rd ExchangeResponseData
	if err := json.Unmarshal(raw.Response, &rd); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w (body: %s)", err, string(data))
	}
	e.Response = &rd
	return nil
}

func (e *ExchangeResponse) ErrorMessage() string {
	if e.RawError != "" {
		return e.RawError
	}
	if e.Response != nil {
		return e.Response.Message
	}
	return e.Status
}

type ExchangeResponseData struct {
	Type    string         `json:"type"`
	Data    *OrderResponse `json:"data,omitempty"`
	Message string         `json:"message,omitempty"`
}

type OrderResponse struct {
	Statuses []OrderStatus `json:"statuses"`
}

type OrderStatus struct {
	Resting *RestingStatus `json:"resting,omitempty"`
	Filled  *FilledStatus  `json:"filled,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type RestingStatus struct {
	Oid int64 `json:"oid"`
}

type FilledStatus struct {
	TotalSz  string `json:"totalSz"`
	AvgPx    string `json:"avgPx"`
	Oid      int64  `json:"oid"`
}

type APIError struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("hyperliquid API error: %s", e.Message)
}

type OrderError struct {
	Index   int
	Message string
}

func (e *OrderError) Error() string {
	return fmt.Sprintf("order %d error: %s", e.Index, e.Message)
}

type Network string

const (
	Mainnet Network = "mainnet"
	Testnet Network = "testnet"
)

func (n Network) GetInfoURL() string {
	if n == Testnet {
		return TestnetInfoURL
	}
	return MainnetInfoURL
}

func (n Network) GetExchangeURL() string {
	if n == Testnet {
		return TestnetExchangeURL
	}
	return MainnetExchangeURL
}

func (n Network) GetChainID() int64 {
	if n == Testnet {
		return TestnetChainID
	}
	return MainnetChainID
}
