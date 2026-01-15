package delta

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// OrderbookEntry represents a single price level in the orderbook
type OrderbookEntry struct {
	Depth string `json:"depth"`
	Price string `json:"price"`
	Size  int    `json:"size"`
}

// Orderbook represents the L2 orderbook for a symbol
type Orderbook struct {
	Buy           []OrderbookEntry `json:"buy"`
	Sell          []OrderbookEntry `json:"sell"`
	Symbol        string           `json:"symbol"`
	LastUpdatedAt int64            `json:"last_updated_at"`
}

// BestBidAsk holds the best bid and ask prices
type BestBidAsk struct {
	BestBid     float64
	BestAsk     float64
	BestBidSize int
	BestAskSize int
	Spread      float64
	SpreadPct   float64
}

// GetOrderbook fetches the L2 orderbook for a symbol
func (c *Client) GetOrderbook(symbol string) (*Orderbook, error) {
	resp, err := c.Get("/l2orderbook/"+symbol, nil)
	if err != nil {
		return nil, err
	}

	var orderbook Orderbook
	if err := json.Unmarshal(resp.Result, &orderbook); err != nil {
		return nil, fmt.Errorf("failed to parse orderbook: %v", err)
	}

	return &orderbook, nil
}

// GetBestBidAsk fetches the best bid and ask prices for a symbol
func (c *Client) GetBestBidAsk(symbol string) (*BestBidAsk, error) {
	orderbook, err := c.GetOrderbook(symbol)
	if err != nil {
		return nil, err
	}

	if len(orderbook.Buy) == 0 || len(orderbook.Sell) == 0 {
		return nil, fmt.Errorf("orderbook is empty for %s", symbol)
	}

	bestBid, err := strconv.ParseFloat(orderbook.Buy[0].Price, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse best bid: %v", err)
	}

	bestAsk, err := strconv.ParseFloat(orderbook.Sell[0].Price, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse best ask: %v", err)
	}

	spread := bestAsk - bestBid
	spreadPct := (spread / bestBid) * 100

	return &BestBidAsk{
		BestBid:     bestBid,
		BestAsk:     bestAsk,
		BestBidSize: orderbook.Buy[0].Size,
		BestAskSize: orderbook.Sell[0].Size,
		Spread:      spread,
		SpreadPct:   spreadPct,
	}, nil
}
