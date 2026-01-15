package delta

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// GetProducts returns list of all products
func (c *Client) GetProducts() ([]Product, error) {
	resp, err := c.Get("/products", nil)
	if err != nil {
		return nil, err
	}

	var products []Product
	if err := json.Unmarshal(resp.Result, &products); err != nil {
		return nil, fmt.Errorf("failed to parse products: %v", err)
	}

	return products, nil
}

// GetProductBySymbol returns a product by its symbol
func (c *Client) GetProductBySymbol(symbol string) (*Product, error) {
	resp, err := c.Get("/products/"+symbol, nil)
	if err != nil {
		return nil, err
	}

	var product Product
	if err := json.Unmarshal(resp.Result, &product); err != nil {
		return nil, fmt.Errorf("failed to parse product: %v", err)
	}

	return &product, nil
}

// GetTicker returns ticker for a symbol
func (c *Client) GetTicker(symbol string) (*Ticker, error) {
	resp, err := c.Get("/tickers/"+symbol, nil)
	if err != nil {
		return nil, err
	}

	var ticker Ticker
	if err := json.Unmarshal(resp.Result, &ticker); err != nil {
		return nil, fmt.Errorf("failed to parse ticker: %v", err)
	}

	return &ticker, nil
}

// PlaceOrder places a new order
func (c *Client) PlaceOrder(req *OrderRequest) (*Order, error) {
	resp, err := c.Post("/orders", req)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(resp.Result, &order); err != nil {
		return nil, fmt.Errorf("failed to parse order: %v", err)
	}

	return &order, nil
}

// CancelOrder cancels an order by ID
func (c *Client) CancelOrder(orderID int64, productID int) error {
	query := url.Values{}
	query.Set("id", fmt.Sprintf("%d", orderID))
	query.Set("product_id", fmt.Sprintf("%d", productID))

	_, err := c.Delete("/orders", query)
	return err
}

// CancelAllOrders cancels all open orders
func (c *Client) CancelAllOrders(productID int) error {
	query := url.Values{}
	if productID > 0 {
		query.Set("product_id", fmt.Sprintf("%d", productID))
	}

	_, err := c.Delete("/orders/all", query)
	return err
}

// GetActiveOrders returns all active orders
func (c *Client) GetActiveOrders(productID int) ([]Order, error) {
	query := url.Values{}
	query.Set("state", "open")
	if productID > 0 {
		query.Set("product_id", fmt.Sprintf("%d", productID))
	}

	resp, err := c.Get("/orders", query)
	if err != nil {
		return nil, err
	}

	var orders []Order
	if err := json.Unmarshal(resp.Result, &orders); err != nil {
		return nil, fmt.Errorf("failed to parse orders: %v", err)
	}

	return orders, nil
}

// GetOrderByID returns an order by ID
func (c *Client) GetOrderByID(orderID int64) (*Order, error) {
	resp, err := c.Get(fmt.Sprintf("/orders/%d", orderID), nil)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(resp.Result, &order); err != nil {
		return nil, fmt.Errorf("failed to parse order: %v", err)
	}

	return &order, nil
}

// SetLeverage sets leverage for a product
func (c *Client) SetLeverage(productID int, leverage int) error {
	body := map[string]interface{}{
		"product_id": productID,
		"leverage":   leverage,
	}

	_, err := c.Post("/orders/leverage", body)
	return err
}

// PlaceLimitOrder places a limit order at the specified price
func (c *Client) PlaceLimitOrder(req *OrderRequest) (*Order, error) {
	// Ensure order type is limit
	req.OrderType = "limit_order"
	if req.TimeInForce == "" {
		req.TimeInForce = "gtc" // Good-til-cancelled
	}

	return c.PlaceOrder(req)
}

// PlaceAggressiveLimitOrder places a limit order at best bid/ask with optional offset
// For buys: places at best ask (or slightly below to be maker)
// For sells: places at best bid (or slightly above to be maker)
// offsetPct is the percentage offset into the spread (e.g., 0.01 = 0.01%)
func (c *Client) PlaceAggressiveLimitOrder(req *OrderRequest, symbol string, offsetPct float64) (*Order, error) {
	// Get current best bid/ask
	bba, err := c.GetBestBidAsk(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get orderbook: %v", err)
	}

	var limitPrice float64
	if req.Side == "buy" {
		// For aggressive buy: place at best ask (or just below to still be maker)
		// Using best bid + small offset to be near top of book but still maker
		limitPrice = bba.BestBid * (1 + offsetPct/100)
		// Cap at best ask to avoid crossing spread
		if limitPrice > bba.BestAsk {
			limitPrice = bba.BestAsk
		}
	} else {
		// For aggressive sell: place at best bid (or just above to still be maker)
		limitPrice = bba.BestAsk * (1 - offsetPct/100)
		// Floor at best bid to avoid crossing spread
		if limitPrice < bba.BestBid {
			limitPrice = bba.BestBid
		}
	}

	req.LimitPrice = fmt.Sprintf("%.2f", limitPrice)
	req.OrderType = "limit_order"
	req.TimeInForce = "gtc"

	return c.PlaceOrder(req)
}

// WaitForOrderFill polls order status until filled or timeout
// Returns the order if filled, nil if timed out, or error
func (c *Client) WaitForOrderFill(orderID int64, timeoutSeconds int) (*Order, error) {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		order, err := c.GetOrderByID(orderID)
		if err != nil {
			return nil, err
		}

		// Check if order is filled
		if order.State == "filled" || order.UnfilledSize == 0 {
			return order, nil
		}

		// Check if order is cancelled or rejected
		if order.State == "cancelled" || order.State == "rejected" {
			return nil, fmt.Errorf("order %d was %s", orderID, order.State)
		}

		time.Sleep(pollInterval)
	}

	// Timeout - return nil to indicate not filled
	return nil, nil
}

// PlaceLimitOrderWithFallback places a limit order and falls back to market if not filled
// timeoutSeconds: how long to wait for limit order to fill before converting to market
func (c *Client) PlaceLimitOrderWithFallback(req *OrderRequest, symbol string, timeoutSeconds int) (*Order, error) {
	// First, try aggressive limit order
	limitOrder, err := c.PlaceAggressiveLimitOrder(req, symbol, 0.01) // 0.01% offset
	if err != nil {
		// If limit order fails, try market order immediately
		req.OrderType = "market_order"
		req.LimitPrice = ""
		return c.PlaceOrder(req)
	}

	// Wait for fill
	filledOrder, err := c.WaitForOrderFill(limitOrder.ID, timeoutSeconds)
	if err != nil {
		// Cancel the limit order and try market
		c.CancelOrder(limitOrder.ID, req.ProductID)
		req.OrderType = "market_order"
		req.LimitPrice = ""
		return c.PlaceOrder(req)
	}

	if filledOrder != nil {
		return filledOrder, nil
	}

	// Timed out - cancel limit order and place market order
	c.CancelOrder(limitOrder.ID, req.ProductID)

	// Check how much was filled
	order, _ := c.GetOrderByID(limitOrder.ID)
	if order != nil && order.UnfilledSize == 0 {
		// Fully filled during cancel
		return order, nil
	}

	// Place market order for remaining size
	if order != nil && order.UnfilledSize > 0 {
		req.Size = order.UnfilledSize
	}
	req.OrderType = "market_order"
	req.LimitPrice = ""
	return c.PlaceOrder(req)
}
