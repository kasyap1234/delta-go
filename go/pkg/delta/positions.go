package delta

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// GetPositions returns all margined positions
func (c *Client) GetPositions() ([]Position, error) {
	resp, err := c.Get("/positions/margined", nil)
	if err != nil {
		return nil, err
	}

	var positions []Position
	if err := json.Unmarshal(resp.Result, &positions); err != nil {
		return nil, fmt.Errorf("failed to parse positions: %v", err)
	}

	return positions, nil
}

// GetPosition returns position for a specific product
func (c *Client) GetPosition(productID int) (*Position, error) {
	query := url.Values{}
	query.Set("product_id", fmt.Sprintf("%d", productID))

	resp, err := c.Get("/positions", query)
	if err != nil {
		return nil, err
	}

	var position Position
	if err := json.Unmarshal(resp.Result, &position); err != nil {
		return nil, fmt.Errorf("failed to parse position: %v", err)
	}

	return &position, nil
}

// ClosePosition closes a position by placing a reduce-only limit order
// Falls back to market order if limit doesn't fill within timeout
// positionSide should be "buy" for long positions (size > 0) or "sell" for short positions (size < 0)
func (c *Client) ClosePosition(symbol string, productID int, size int, positionSide string) error {
	// Determine close side (opposite of position side)
	closeSide := "sell"
	if positionSide == "sell" {
		closeSide = "buy"
	}

	// NOTE: Delta API expects only one of product_id or product_symbol, not both
	req := &OrderRequest{
		ProductID:  productID,
		Size:       size,
		Side:       closeSide,
		ReduceOnly: true,
	}

	// Use limit order with 3-second timeout for faster exit, then fallback to market
	_, err := c.PlaceLimitOrderWithFallback(req, symbol, 3)
	return err
}

// CloseAllPositions closes all open positions
func (c *Client) CloseAllPositions() error {
	body := map[string]interface{}{
		"close_all_portfolio": true,
		"close_all_isolated":  true,
	}

	_, err := c.Post("/positions/close_all", body)
	return err
}

// AddPositionMargin adds margin to a position
func (c *Client) AddPositionMargin(productID int, marginAmount string) error {
	body := map[string]interface{}{
		"product_id":   productID,
		"delta_margin": marginAmount,
	}

	_, err := c.Post("/positions/change_margin", body)
	return err
}
