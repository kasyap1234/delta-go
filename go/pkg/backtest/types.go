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
}

// DefaultConfig returns sensible defaults calibrated to Delta Exchange India
func DefaultConfig() Config {
	return Config{
		Symbols:         []string{"BTCUSD", "ETHUSD", "SOLUSD"},
		Resolution:      "5m",
		InitialCapital:  200.0,
		Leverage:        10,
		MakerFeeBps:     2.0, // 0.02%
		TakerFeeBps:     5.0, // 0.05%
		SlippageModel:   NewVolatilitySlippage(1.5, 0.5),
		LatencyMs:       50,
		SimulateFunding: true,
		DataCacheDir:    ".backtest_cache",
	}
}

// Position represents an open position during backtesting
type Position struct {
	Symbol     string
	Side       string // "buy" or "sell"
	Size       int
	EntryPrice float64
	EntryTime  time.Time
	StopLoss   float64
	TakeProfit float64

	// Accumulated costs
	EntryFee    float64
	EntrySlip   float64
	FundingPaid float64
}

// UnrealizedPnL calculates unrealized P&L at given price
func (p *Position) UnrealizedPnL(currentPrice float64, contractValue float64) float64 {
	multiplier := 1.0
	if p.Side == "sell" {
		multiplier = -1.0
	}
	priceDiff := (currentPrice - p.EntryPrice) * multiplier
	return (priceDiff / p.EntryPrice) * float64(p.Size) * contractValue
}

// Trade represents a completed trade with all costs
type Trade struct {
	ID     string
	Symbol string
	Side   string
	Size   int

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

	// Funding (for perpetuals)
	FundingPaid float64

	// P&L
	GrossPnL float64
	NetPnL   float64

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
	Size         int
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
