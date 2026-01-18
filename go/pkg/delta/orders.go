package delta

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
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

// CancelOrder cancels an order by ID using Delta v2 API (JSON body, not query params)
func (c *Client) CancelOrder(orderID int64, productID int) error {
	body := map[string]interface{}{
		"id":         orderID,
		"product_id": productID,
	}

	_, err := c.DeleteWithBody("/orders", body)
	return err
}

// CancelAllOrders cancels all open orders using Delta v2 API (JSON body)
func (c *Client) CancelAllOrders(productID int) error {
	body := map[string]interface{}{}
	if productID > 0 {
		body["product_id"] = productID
	}

	_, err := c.DeleteWithBody("/orders/all", body)
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

// SetLeverage sets leverage for a product using Delta v2 API
// Correct endpoint: POST /v2/products/{product_id}/orders/leverage
func (c *Client) SetLeverage(productID int, leverage int) error {
	body := map[string]interface{}{
		"leverage": fmt.Sprintf("%d", leverage), // Delta expects string
	}

	_, err := c.Post(fmt.Sprintf("/products/%d/orders/leverage", productID), body)
	return err
}

// RoundToTickSize rounds a price to the nearest valid tick size
func RoundToTickSize(price float64, tickSize string) (string, error) {
	return RoundToTickSizeWithDirection(price, tickSize, "nearest")
}

// RoundToTickSizeWithDirection rounds price to tick size with directional control
// direction: "up" (for sells), "down" (for buys), "nearest" (default)
func RoundToTickSizeWithDirection(price float64, tickSize string, direction string) (string, error) {
	tick, err := strconv.ParseFloat(tickSize, 64)
	if err != nil || tick <= 0 {
		return fmt.Sprintf("%.2f", price), nil
	}

	var rounded float64
	switch direction {
	case "down":
		rounded = math.Floor(price/tick) * tick
	case "up":
		rounded = math.Ceil(price/tick) * tick
	default:
		rounded = math.Round(price/tick) * tick
	}

	precision := 0
	if tick < 1 {
		tickStr := strconv.FormatFloat(tick, 'f', -1, 64)
		if idx := len(tickStr) - 1; idx > 0 {
			for i := len(tickStr) - 1; i >= 0; i-- {
				if tickStr[i] == '.' {
					precision = len(tickStr) - 1 - i
					break
				}
			}
		}
	}

	return strconv.FormatFloat(rounded, 'f', precision, 64), nil
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
// For buys: places at best ask to maximize fill probability
// For sells: places at best bid to maximize fill probability
// offsetPct is the percentage offset from best price (e.g., 0.01 = 0.01%)
func (c *Client) PlaceAggressiveLimitOrder(req *OrderRequest, symbol string, offsetPct float64) (*Order, error) {
	bba, err := c.GetBestBidAsk(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get orderbook: %w", err)
	}

	product, err := c.GetProductBySymbol(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get product: %w", err)
	}

	var limitPrice float64
	var roundDirection string
	if req.Side == "buy" {
		// For aggressive buy: place at best ask (cross spread to take liquidity)
		// With small offset below to potentially get maker rebate
		limitPrice = bba.BestAsk * (1 - offsetPct/100)
		// Floor at best bid (don't go below top of book)
		if limitPrice < bba.BestBid {
			limitPrice = bba.BestBid
		}
		roundDirection = "down" // Round down for buys to avoid overpaying
	} else {
		// For aggressive sell: place at best bid (cross spread to take liquidity)
		// With small offset above to potentially get maker rebate
		limitPrice = bba.BestBid * (1 + offsetPct/100)
		// Cap at best ask (don't go above top of book)
		if limitPrice > bba.BestAsk {
			limitPrice = bba.BestAsk
		}
		roundDirection = "up" // Round up for sells to avoid underselling
	}

	req.LimitPrice, _ = RoundToTickSizeWithDirection(limitPrice, product.TickSize, roundDirection)
	req.OrderType = "limit_order"
	req.TimeInForce = "gtc"

	return c.PlaceOrder(req)
}

// WaitForOrderFill polls order status until filled or timeout
// Returns the order if filled, nil if timed out, or error
func (c *Client) WaitForOrderFill(orderID int64, timeoutSeconds int) (*Order, error) {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	pollInterval := 500 * time.Millisecond
	maxRetries := 3
	consecutiveErrors := 0

	var lastOrder *Order
	for time.Now().Before(deadline) {
		order, err := c.GetOrderByID(orderID)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= maxRetries {
				return nil, fmt.Errorf("failed to get order status after %d retries: %w", maxRetries, err)
			}
			time.Sleep(pollInterval)
			continue
		}
		consecutiveErrors = 0
		lastOrder = order

		// Check if order is filled - require explicit state check
		if order.State == "filled" {
			return order, nil
		}

		// Check terminal failure states - do NOT fallback to market on rejection
		if order.State == "cancelled" {
			return nil, fmt.Errorf("order %d was cancelled", orderID)
		}
		if order.State == "rejected" {
			return nil, &OrderRejectedError{OrderID: orderID, Reason: "order rejected by exchange"}
		}

		// Unknown terminal states
		if order.State != "open" && order.State != "pending" && order.State != "partially_filled" {
			return nil, fmt.Errorf("order %d in unexpected state: %s", orderID, order.State)
		}

		time.Sleep(pollInterval)
	}

	// Final check after deadline to catch fills at the last moment
	order, err := c.GetOrderByID(orderID)
	if err == nil && order.State == "filled" {
		return order, nil
	}
	if err == nil {
		lastOrder = order
	}

	// Return the last known order state (nil indicates timeout with no fill)
	if lastOrder != nil && lastOrder.State == "filled" {
		return lastOrder, nil
	}
	return nil, nil
}

// OrderRejectedError indicates an order was rejected by the exchange
type OrderRejectedError struct {
	OrderID int64
	Reason  string
}

func (e *OrderRejectedError) Error() string {
	return fmt.Sprintf("order %d rejected: %s", e.OrderID, e.Reason)
}

// PlaceLimitOrderWithFallback places a limit order and falls back to market if not filled
// timeoutSeconds: how long to wait for limit order to fill before converting to market
func (c *Client) PlaceLimitOrderWithFallback(req *OrderRequest, symbol string, timeoutSeconds int) (*Order, error) {
	// Store original bracket fields - we only attach them to the first order
	hasBracket := req.BracketStopLossPrice != "" || req.BracketTakeProfitPrice != ""
	originalSL := req.BracketStopLossPrice
	originalTP := req.BracketTakeProfitPrice
	originalSLLimit := req.BracketStopLossLimitPrice
	originalTPLimit := req.BracketTakeProfitLimitPrice
	originalSize := req.Size

	// First, try aggressive limit order
	limitOrder, err := c.PlaceAggressiveLimitOrder(req, symbol, 0.01)
	if err != nil {
		// If limit order fails, try market order immediately (keep bracket fields)
		marketReq := &OrderRequest{
			ProductID:                   req.ProductID,
			Size:                        originalSize,
			Side:                        req.Side,
			OrderType:                   "market_order",
			BracketStopLossPrice:        originalSL,
			BracketStopLossLimitPrice:   originalSLLimit,
			BracketTakeProfitPrice:      originalTP,
			BracketTakeProfitLimitPrice: originalTPLimit,
		}
		return c.PlaceOrder(marketReq)
	}

	// Wait for fill
	filledOrder, err := c.WaitForOrderFill(limitOrder.ID, timeoutSeconds)

	// Handle rejection errors - do NOT fallback to market
	var rejectedErr *OrderRejectedError
	if errors.As(err, &rejectedErr) {
		return nil, err
	}

	if err != nil {
		// Other error during polling - try to cancel and verify state
		finalOrder, safeToReplace := c.waitForCancelConfirmation(limitOrder.ID, req.ProductID)
		if finalOrder != nil && finalOrder.State == "filled" {
			return finalOrder, nil
		}
		if !safeToReplace {
			return nil, fmt.Errorf("cannot safely replace order: %w", err)
		}

		// Safe to place market for full size (nothing filled)
		marketReq := &OrderRequest{
			ProductID:                   req.ProductID,
			Size:                        originalSize,
			Side:                        req.Side,
			OrderType:                   "market_order",
			BracketStopLossPrice:        originalSL,
			BracketStopLossLimitPrice:   originalSLLimit,
			BracketTakeProfitPrice:      originalTP,
			BracketTakeProfitLimitPrice: originalTPLimit,
		}
		return c.PlaceOrder(marketReq)
	}

	if filledOrder != nil {
		return filledOrder, nil
	}

	// Timed out - cancel and verify state before placing market
	finalOrder, safeToReplace := c.waitForCancelConfirmation(limitOrder.ID, req.ProductID)
	if finalOrder != nil && finalOrder.State == "filled" {
		return finalOrder, nil
	}
	if !safeToReplace {
		return nil, fmt.Errorf("cannot safely replace order after timeout")
	}

	// Determine how much was filled and calculate remaining size
	filledQty := 0
	remainingSize := originalSize
	if finalOrder != nil {
		filledQty = finalOrder.Size - finalOrder.UnfilledSize
		remainingSize = finalOrder.UnfilledSize
	}

	if remainingSize <= 0 {
		return finalOrder, nil
	}

	// Place market order for remaining size
	// IMPORTANT: If any quantity was already filled, do NOT attach bracket fields
	// to avoid duplicate SL/TP orders
	marketReq := &OrderRequest{
		ProductID: req.ProductID,
		Size:      remainingSize,
		Side:      req.Side,
		OrderType: "market_order",
	}

	// Only attach bracket if nothing was filled (no partial fill scenario)
	if filledQty == 0 && hasBracket {
		marketReq.BracketStopLossPrice = originalSL
		marketReq.BracketStopLossLimitPrice = originalSLLimit
		marketReq.BracketTakeProfitPrice = originalTP
		marketReq.BracketTakeProfitLimitPrice = originalTPLimit
	}

	return c.PlaceOrder(marketReq)
}

// waitForCancelConfirmation cancels an order and waits for confirmation
// Returns the final order state and whether it's safe to place a replacement
func (c *Client) waitForCancelConfirmation(orderID int64, productID int) (*Order, bool) {
	_ = c.CancelOrder(orderID, productID)

	// Poll for cancel confirmation (max 2 seconds)
	deadline := time.Now().Add(2 * time.Second)
	pollInterval := 200 * time.Millisecond

	for time.Now().Before(deadline) {
		order, err := c.GetOrderByID(orderID)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		// Terminal states where we know the final outcome
		if order.State == "filled" {
			return order, false // Already filled, don't replace
		}
		if order.State == "cancelled" {
			return order, true // Cancelled, safe to replace
		}

		time.Sleep(pollInterval)
	}

	// Final check
	order, err := c.GetOrderByID(orderID)
	if err != nil {
		return nil, false // Can't verify, not safe
	}

	if order.State == "filled" {
		return order, false
	}
	if order.State == "cancelled" {
		return order, true
	}

	// Order still active after cancel attempt - not safe to replace
	return order, false
}
