package strategy

import (
	"fmt"
	"math"

	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/features"
)

type GridConfig struct {
	GridLevels           int     // 10 levels
	GridRangePct         float64 // 3% each side
	PositionSizePerLevel int     // Contracts per level
	MaxVolatilityPct     float64 // Exit if vol > 50%
	MinVolatilityPct     float64 // Enter if vol < 30%
	Enabled              bool
}

func DefaultGridConfig() GridConfig {
	return GridConfig{
		GridLevels:           10,
		GridRangePct:         3.0,
		PositionSizePerLevel: 1,
		MaxVolatilityPct:     200.0,
		MinVolatilityPct:     150.0,
		Enabled:              true,
	}
}

type GridLevel struct {
	Price    float64
	Side     string
	OrderID  int64
	IsActive bool
}

type GridTradingStrategy struct {
	cfg         GridConfig
	levels      []GridLevel
	IsActive    bool
	symbol      string
	centerPrice float64
}

func NewGridTradingStrategy(cfg GridConfig, symbol string) *GridTradingStrategy {
	return &GridTradingStrategy{
		cfg:    cfg,
		symbol: symbol,
	}
}

func (g *GridTradingStrategy) Name() string {
	return "grid_trading"
}

func (g *GridTradingStrategy) Analyze(f features.MarketFeatures, candles []delta.Candle) Signal {
	if !g.cfg.Enabled {
		return Signal{Action: ActionNone, Reason: "grid disabled"}
	}

	volPct := f.HistoricalVol * 100
	midPrice := (f.BestBid + f.BestAsk) / 2

	// Activation logic
	if !g.IsActive {
		if volPct < g.cfg.MinVolatilityPct && volPct > 5 {
			g.IsActive = true
			g.centerPrice = midPrice
			g.levels = g.CalculateLevels(midPrice)
			return Signal{Action: ActionNone, Reason: "grid activated, placing levels"}
		}
		return Signal{Action: ActionNone, Reason: "conditions not met for grid"}
	}

	// Deactivation logic (High Volatility)
	if volPct > g.cfg.MaxVolatilityPct {
		g.IsActive = false
		return Signal{Action: ActionClose, Reason: "grid deactivated: high volatility"}
	}

	// Recenter Logic (Trend Following)
	// If price drifts near the edge of the grid, reset to follow the trend
	driftPct := math.Abs(midPrice-g.centerPrice) / g.centerPrice * 100
	if driftPct > g.cfg.GridRangePct*0.8 {
		g.IsActive = false
		return Signal{Action: ActionClose, Reason: "grid recentering"}
	}

	// Backtest Logic: Mean Reversion at Grid Boundaries
	// Since backtester doesn't handle multiple limit orders, we simulate
	// buying at the bottom of the grid and selling at the top.
	if len(g.levels) > 0 {
		lowerBound := g.levels[0].Price
		upperBound := g.levels[len(g.levels)-1].Price

		// Ensure sorted
		if lowerBound > upperBound {
			lowerBound, upperBound = upperBound, lowerBound
		}

		if midPrice < lowerBound {
			return Signal{
				Action:     ActionBuy,
				Side:       "buy",
				Price:      midPrice,
				Reason:     "price below grid lower bound",
				Confidence: 0.8,
			}
		}
		if midPrice > upperBound {
			return Signal{
				Action:     ActionSell,
				Side:       "sell",
				Price:      midPrice,
				Reason:     "price above grid upper bound",
				Confidence: 0.8,
			}
		}
	}

	return Signal{Action: ActionNone, Reason: "grid monitoring"}
}

func (g *GridTradingStrategy) CalculateLevels(midPrice float64) []GridLevel {
	levels := make([]GridLevel, g.cfg.GridLevels)
	rangeAmt := midPrice * (g.cfg.GridRangePct / 100)
	step := (rangeAmt * 2) / float64(g.cfg.GridLevels-1)

	startPrice := midPrice - rangeAmt
	for i := 0; i < g.cfg.GridLevels; i++ {
		price := startPrice + (float64(i) * step)
		side := "buy"
		if price > midPrice {
			side = "sell"
		}
		levels[i] = GridLevel{Price: price, Side: side, IsActive: true}
	}
	return levels
}

func (g *GridTradingStrategy) UpdateParams(params map[string]interface{}) {
	if v, ok := params["grid_levels"].(int); ok {
		g.cfg.GridLevels = v
	}
	if v, ok := params["grid_range"].(float64); ok {
		g.cfg.GridRangePct = v
	}
	if v, ok := params["enabled"].(bool); ok {
		g.cfg.Enabled = v
	}
}

func (g *GridTradingStrategy) IsEnabled() bool {
	return g.cfg.Enabled
}

func (g *GridTradingStrategy) GetLevels() []GridLevel {
	return g.levels
}

func (g *GridTradingStrategy) OnFill(orderID int64) Signal {
	for i, level := range g.levels {
		if level.OrderID == orderID {
			// Level filled, place counter order
			g.levels[i].IsActive = false

			// Logic to place counter order at adjacent level
			// This would be handled by a higher-level controller usually
			return Signal{
				Action: ActionNone, // Placeholder
				Reason: fmt.Sprintf("level at %f filled", level.Price),
			}
		}
	}
	return Signal{Action: ActionNone}
}
