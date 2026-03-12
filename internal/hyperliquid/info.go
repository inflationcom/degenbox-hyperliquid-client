package hyperliquid

import "context"

type ClearinghouseState struct {
	MarginSummary      MarginSummary   `json:"marginSummary"`
	CrossMarginSummary MarginSummary   `json:"crossMarginSummary"`
	AssetPositions     []AssetPosition `json:"assetPositions"`
	Withdrawable       string          `json:"withdrawable"`
}

type MarginSummary struct {
	AccountValue    string `json:"accountValue"`
	TotalNtlPos     string `json:"totalNtlPos"`
	TotalRawUsd     string `json:"totalRawUsd"`
	TotalMarginUsed string `json:"totalMarginUsed"`
}

type AssetPosition struct {
	Position PositionData `json:"position"`
	Type     string       `json:"type"`
}

type PositionData struct {
	Coin          string       `json:"coin"`
	EntryPx       string       `json:"entryPx"`
	Leverage      LeverageInfo `json:"leverage"`
	LiquidationPx string       `json:"liquidationPx"`
	PositionValue string       `json:"positionValue"`
	ReturnOnEquity string      `json:"returnOnEquity"`
	Szi           string       `json:"szi"`
	UnrealizedPnl string       `json:"unrealizedPnl"`
	MarginUsed    string       `json:"marginUsed"`
}

type LeverageInfo struct {
	Type  string `json:"type"`
	Value int    `json:"value"`
}

type OpenOrder struct {
	Coin      string `json:"coin"`
	LimitPx   string `json:"limitPx"`
	Oid       int64  `json:"oid"`
	Side      string `json:"side"`
	Sz        string `json:"sz"`
	Timestamp int64  `json:"timestamp"`
}

func (c *Client) GetClearinghouseState(ctx context.Context) (*ClearinghouseState, error) {
	req := map[string]string{"type": "clearinghouseState", "user": c.mainAddress}
	var resp ClearinghouseState
	if err := c.postInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetOpenOrders(ctx context.Context) ([]OpenOrder, error) {
	req := map[string]string{"type": "openOrders", "user": c.mainAddress}
	var resp []OpenOrder
	if err := c.postInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
