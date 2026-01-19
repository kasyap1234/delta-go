// Package backtest provides a realistic backtesting framework for trading strategies.
// It simulates order execution with fees, slippage, and funding rate costs.
package backtest

import (
	"time"

	"github.com/kasyap/delta-go/go/pkg/delta"
)

// Config defines backtesting parameters
type Config struct {
	// Time range
	StartTime time.Time
	EndTime   time.Time

	// Assets
	Symbols    []string
	Resolution string // "5m", "15m", "1h"

	// Capital
	InitialCapital float64
	Leverage       int

	// Realistic costs (in basis points, 1 bps = 0.01%)
	MakerFeeBps   float64 // Delta: 2 bps (0.02%)
	TakerFeeBps   float64 // Delta: 5 bps (0.05%)
	SlippageModel SlippageModel

	// Latency simulation
	LatencyMs int // Typical: 50-100ms

	// Funding simulation
	SimulateFunding bool

	// Data caching
	DataCacheDir string

	// Product metadata for contract value conversions
	Products map[string]*delta.Product
}

// DefaultConfig returns sensible defaults calibrated to Delta Exchange India
func DefaultConfig() Config {
	symbols := []string{"BTCUSD", "ETHUSD", "SOLUSD"}
	products := make(map[string]*delta.Product)
	for _, sym := range symbols {
		products[sym] = delta.MockProduct(sym)
	}

	return Config{
		Symbols:         symbols,
		Resolution:      "5m",
		InitialCapital:  200.0,
		Leverage:        10,
		MakerFeeBps:     2.0, // 0.02%
		TakerFeeBps:     5.0, // 0.05%
		SlippageModel:   NewVolatilitySlippage(1.5, 0.5),
		LatencyMs:       50,
		SimulateFunding: true,
		DataCacheDir:    ".backtest_cache",
		Products:        products,
	}
}

// Position represents an open position during backtesting
type Position struct {
	Symbol     string
	Side       string // "buy" or "sell"
	Size       float64
	EntryPrice float64
	EntryTime  time.Time
	StopLoss   float64
	TakeProfit float64

	// Margin tracking
	InitialMargin float64

	// Accumulated costs
	EntryFee    float64
	EntrySlip   float64
	FundingPaid float64
}

// UnrealizedPnL calculates unrealized P&L at given price
// Size is now contract count, contractValue is from Product.ContractValue
func (p *Position) UnrealizedPnL(currentPrice float64, contractValue float64) float64 {
	multiplier := 1.0
	if p.Side == "sell" {
		multiplier = -1.0
	}
	// P&L = contracts * contractValue * (currentPrice - entryPrice) * direction
	priceDiff := currentPrice - p.EntryPrice
	return p.Size * contractValue * priceDiff * multiplier
}

// Trade represents a completed trade with all costs
type Trade struct {
	ID     string
	Symbol string
	Side   string
	Size   float64

	// Entry
	EntryPrice float64
	EntryTime  time.Time
	EntryFee   float64
	EntrySlip  float64

	// Exit
	ExitPrice float64
	ExitTime  time.Time
	ExitFee   float64
	ExitSlip  float64

	// Slippage costs in dollars (not price units)
	EntrySlipCost float64
	ExitSlipCost  float64

	// Funding (for perpetuals)
	FundingPaid float64

	// P&L
	GrossPnL float64
	NetPnL   float64 // After all costs: fees, slippage costs, funding

	// Exit reason
	Reason string // "stop_loss", "take_profit", "signal", "timeout"
}

// FundingRate represents a funding payment event
type FundingRate struct {
	Timestamp time.Time
	Symbol    string
	Rate      float64 // 8-hourly rate (e.g., 0.0001 = 0.01%)
}

// EquityPoint tracks equity over time
type EquityPoint struct {
	Timestamp time.Time
	Equity    float64
	Drawdown  float64 // As percentage (0.1 = 10%)
}

// CandleWithFunding combines candle data with funding info
type CandleWithFunding struct {
	delta.Candle
	FundingRate float64 // Only populated at funding times (every 8h)
}

// Order represents a simulated order
type Order struct {
	ID           string
	Symbol       string
	Side         string
	Size         float64
	OrderType    string // "market", "limit"
	LimitPrice   float64
	StopLoss     float64
	TakeProfit   float64
	SubmittedAt  time.Time
	FilledAt     time.Time
	FilledPrice  float64
	Fee          float64
	Slippage     float64
	Status       string // "pending", "filled", "cancelled", "rejected"
	RejectReason string
}
