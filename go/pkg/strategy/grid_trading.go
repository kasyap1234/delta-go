package strategy

import (
	"fmt"

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
		MaxVolatilityPct:     50.0,
		MinVolatilityPct:     30.0,
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
	cfg      GridConfig
	levels   []GridLevel
	isActive bool
	symbol   string
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

func (g *GridTradingStrategy) Analyze(f features.MarketFeatures, candles []interface{}) Signal {
	if !g.cfg.Enabled {
		return Signal{Action: ActionNone, Reason: "grid disabled"}
	}

	volPct := f.HistoricalVol * 100
	midPrice := (f.BestBid + f.BestAsk) / 2

	// Activation logic
	if !g.isActive {
		if volPct < g.cfg.MinVolatilityPct && volPct > 5 {
			g.isActive = true
			g.levels = g.CalculateLevels(midPrice)
			return Signal{Action: ActionNone, Reason: "grid activated, placing levels"}
		}
		return Signal{Action: ActionNone, Reason: "conditions not met for grid"}
	}

	// Deactivation logic
	if volPct > g.cfg.MaxVolatilityPct {
		g.isActive = false
		return Signal{Action: ActionClose, Reason: "grid deactivated: high volatility"}
	}

	return Signal{Action: ActionNone, Reason: "grid monitoring fill events"}
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
