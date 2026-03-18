package hyperliquid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type Client struct {
	httpClient *http.Client
	network    Network

	lastNonce int64

	mainAddress  string

	signer       SignerInterface
	signExternal func(action any, nonce int64) (*Signature, error)

	assetsMu   sync.RWMutex
	assets     map[string]int       // market name -> asset ID
	assetNames map[int]string       // asset ID -> market name
	assetsInfo map[string]*AssetInfo // market name -> info
	assetsCtx  map[string]*AssetCtx // market name -> context
}

type SignerInterface interface {
	SignAction(action any, nonce int64, vaultAddress string) (*Signature, error)
}

type ClientConfig struct {
	Network      Network
	MainAddress  string

	Signer       SignerInterface
	SignExternal func(action any, nonce int64) (*Signature, error)

	HTTPClient *http.Client
	Timeout    time.Duration
}

func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.MainAddress == "" {
		return nil, fmt.Errorf("main address is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	return &Client{
		httpClient:   httpClient,
		network:      cfg.Network,
		mainAddress:  cfg.MainAddress,
		signer:       cfg.Signer,
		signExternal: cfg.SignExternal,
		assets:       make(map[string]int),
		assetNames:   make(map[int]string),
		assetsInfo:   make(map[string]*AssetInfo),
		assetsCtx:    make(map[string]*AssetCtx),
	}, nil
}

func (c *Client) IsTestnet() bool {
	return c.network == Testnet
}

func (c *Client) PlaceOrder(ctx context.Context, orders []OrderWire, grouping Grouping) (*OrderResponse, error) {
	action := OrderAction{
		Type:     "order",
		Orders:   orders,
		Grouping: grouping,
	}

	resp, err := c.postExchange(ctx, action)
	if err != nil {
		return nil, err
	}

	if resp.Status != "ok" {
		return nil, &APIError{
			Status:  resp.Status,
			Message: resp.ErrorMessage(),
		}
	}

	if resp.Response != nil {
		return resp.Response.Data, nil
	}
	return nil, nil
}

func (c *Client) CancelOrder(ctx context.Context, cancels []CancelSpec) error {
	action := CancelAction{
		Type:    "cancel",
		Cancels: cancels,
	}

	resp, err := c.postExchange(ctx, action)
	if err != nil {
		return err
	}

	if resp.Status != "ok" {
		return &APIError{
			Status:  resp.Status,
			Message: resp.ErrorMessage(),
		}
	}

	return nil
}

func (c *Client) CancelOrderByCloid(ctx context.Context, cancels []CancelCloidSpec) error {
	action := CancelByCloidAction{
		Type:    "cancelByCloid",
		Cancels: cancels,
	}

	resp, err := c.postExchange(ctx, action)
	if err != nil {
		return err
	}

	if resp.Status != "ok" {
		return &APIError{
			Status:  resp.Status,
			Message: resp.ErrorMessage(),
		}
	}

	return nil
}

func (c *Client) ModifyOrder(ctx context.Context, modifies []ModifySpec) (*OrderResponse, error) {
	action := ModifyAction{
		Type:     "batchModify",
		Modifies: modifies,
	}

	resp, err := c.postExchange(ctx, action)
	if err != nil {
		return nil, err
	}

	if resp.Status != "ok" {
		return nil, &APIError{
			Status:  resp.Status,
			Message: resp.ErrorMessage(),
		}
	}

	if resp.Response != nil {
		return resp.Response.Data, nil
	}
	return nil, nil
}

func (c *Client) UpdateLeverage(ctx context.Context, asset int, leverage int, isCross bool) error {
	action := UpdateLeverageAction{
		Type:     "updateLeverage",
		Asset:    asset,
		IsCross:  isCross,
		Leverage: leverage,
	}

	resp, err := c.postExchange(ctx, action)
	if err != nil {
		return err
	}

	if resp.Status != "ok" {
		return &APIError{
			Status:  resp.Status,
			Message: resp.ErrorMessage(),
		}
	}

	return nil
}

func (c *Client) RefreshAssets(ctx context.Context) error {
	req := MetaAndAssetCtxsRequest{Type: "metaAndAssetCtxs"}

	var resp []json.RawMessage
	if err := c.postInfo(ctx, req, &resp); err != nil {
		return fmt.Errorf("failed to get meta: %w", err)
	}

	if len(resp) < 2 {
		return fmt.Errorf("unexpected response format")
	}

	var meta Meta
	if err := json.Unmarshal(resp[0], &meta); err != nil {
		return fmt.Errorf("failed to unmarshal meta: %w", err)
	}

	var assetCtxs []AssetCtx
	if err := json.Unmarshal(resp[1], &assetCtxs); err != nil {
		return fmt.Errorf("failed to unmarshal asset contexts: %w", err)
	}

	c.assetsMu.Lock()
	defer c.assetsMu.Unlock()

	c.assets = make(map[string]int)
	c.assetNames = make(map[int]string)
	c.assetsInfo = make(map[string]*AssetInfo)
	c.assetsCtx = make(map[string]*AssetCtx)

	for i, asset := range meta.Universe {
		c.assets[asset.Name] = i
		c.assetNames[i] = asset.Name
		c.assetsInfo[asset.Name] = &meta.Universe[i]
		if i < len(assetCtxs) {
			c.assetsCtx[asset.Name] = &assetCtxs[i]
		}
	}

	// Load builder dex assets (xyz, etc.) with correct ID offset
	c.refreshBuilderDexAssets(ctx)

	return nil
}

func (c *Client) refreshBuilderDexAssets(ctx context.Context) {
	var dexes []json.RawMessage
	if err := c.postInfo(ctx, map[string]string{"type": "perpDexs"}, &dexes); err != nil {
		return // perpDexs not available (e.g. testnet)
	}

	for dexIndex := 1; dexIndex < len(dexes); dexIndex++ {
		if string(dexes[dexIndex]) == "null" {
			continue
		}
		var dex PerpDex
		if err := json.Unmarshal(dexes[dexIndex], &dex); err != nil || dex.Name == "" {
			continue
		}

		req := MetaAndAssetCtxsRequest{Type: "metaAndAssetCtxs", Dex: dex.Name}
		var dexResp []json.RawMessage
		if err := c.postInfo(ctx, req, &dexResp); err != nil || len(dexResp) < 2 {
			continue
		}

		var dexMeta Meta
		if err := json.Unmarshal(dexResp[0], &dexMeta); err != nil {
			continue
		}
		var dexCtxs []AssetCtx
		json.Unmarshal(dexResp[1], &dexCtxs) // best effort

		offset := 100000 + dexIndex*10000
		for i, asset := range dexMeta.Universe {
			assetID := offset + i
			c.assets[asset.Name] = assetID
			c.assetNames[assetID] = asset.Name
			c.assetsInfo[asset.Name] = &dexMeta.Universe[i]
			if i < len(dexCtxs) {
				c.assetsCtx[asset.Name] = &dexCtxs[i]
			}
		}
	}
}

func (c *Client) GetAssetID(market string) (int, error) {
	c.assetsMu.RLock()
	defer c.assetsMu.RUnlock()

	id, ok := c.assets[market]
	if !ok {
		return 0, fmt.Errorf("unknown market: %s", market)
	}
	return id, nil
}

func (c *Client) GetAssetName(id int) (string, error) {
	c.assetsMu.RLock()
	defer c.assetsMu.RUnlock()

	name, ok := c.assetNames[id]
	if !ok {
		return "", fmt.Errorf("unknown asset ID: %d", id)
	}
	return name, nil
}

func (c *Client) GetAssetInfo(market string) (*AssetInfo, error) {
	c.assetsMu.RLock()
	defer c.assetsMu.RUnlock()

	info, ok := c.assetsInfo[market]
	if !ok {
		return nil, fmt.Errorf("unknown market: %s", market)
	}
	return info, nil
}

func (c *Client) GetOraclePrice(market string) (string, error) {
	c.assetsMu.RLock()
	defer c.assetsMu.RUnlock()

	ctx, ok := c.assetsCtx[market]
	if !ok || ctx == nil {
		return "", fmt.Errorf("no oracle price for %s", market)
	}
	return ctx.OraclePx, nil
}

func (c *Client) GetAllMids(ctx context.Context) (map[string]string, error) {
	req := map[string]string{"type": "allMids"}
	var resp map[string]string
	if err := c.postInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetL2Mid fetches the mid price for a single asset from its order book.
// Useful for xyz (prediction) markets that aren't included in allMids.
func (c *Client) GetL2Mid(ctx context.Context, coin string) (float64, error) {
	req := map[string]any{"type": "l2Book", "coin": coin}
	var resp struct {
		Levels [2][]struct {
			Px string `json:"px"`
		} `json:"levels"`
	}
	if err := c.postInfo(ctx, req, &resp); err != nil {
		return 0, err
	}
	if len(resp.Levels[0]) == 0 || len(resp.Levels[1]) == 0 {
		return 0, fmt.Errorf("no liquidity for %s", coin)
	}
	ask, err1 := strconv.ParseFloat(resp.Levels[0][0].Px, 64)
	bid, err2 := strconv.ParseFloat(resp.Levels[1][0].Px, 64)
	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("invalid price data for %s", coin)
	}
	return (ask + bid) / 2, nil
}

func (c *Client) GetMaxLeverage(market string) (int, error) {
	c.assetsMu.RLock()
	defer c.assetsMu.RUnlock()

	info, ok := c.assetsInfo[market]
	if !ok {
		return 0, fmt.Errorf("unknown market: %s", market)
	}
	return info.MaxLeverage, nil
}

func (c *Client) postInfo(ctx context.Context, req any, resp any) error {
	return c.post(ctx, c.network.GetInfoURL(), req, resp)
}

func (c *Client) postExchange(ctx context.Context, action any) (*ExchangeResponse, error) {
	var nonce int64
	for {
		now := time.Now().UnixMilli()
		old := atomic.LoadInt64(&c.lastNonce)
		next := now
		if next <= old {
			next = old + 1
		}
		if atomic.CompareAndSwapInt64(&c.lastNonce, old, next) {
			nonce = next
			break
		}
	}

	sig, err := c.sign(action, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to sign action: %w", err)
	}

	exchangeReq := ExchangeRequest{
		Action:    action,
		Nonce:     nonce,
		Signature: sig,
	}

	var resp ExchangeResponse
	if err := c.post(ctx, c.network.GetExchangeURL(), exchangeReq, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) sign(action any, nonce int64) (*Signature, error) {
	if c.signer != nil {
		return c.signer.SignAction(action, nonce, "")
	}
	if c.signExternal != nil {
		return c.signExternal(action, nonce)
	}
	return nil, fmt.Errorf("no signer configured")
}

func (c *Client) post(ctx context.Context, url string, reqBody any, respBody any) error {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	const maxResponseSize = 10 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if respBody != nil {
		if err := json.Unmarshal(body, respBody); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w (body: %s)", err, string(body))
		}
	}

	return nil
}
