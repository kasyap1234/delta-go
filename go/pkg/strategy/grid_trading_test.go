package strategy

import (
	"testing"

	"github.com/kasyap/delta-go/go/pkg/features"
)

func TestGridTrading_CalculateLevels(t *testing.T) {
	cfg := DefaultGridConfig()
	cfg.GridLevels = 10
	cfg.GridRangePct = 3.0 // ±3%
	g := NewGridTradingStrategy(cfg, "BTCUSD")

	midPrice := 50000.0
	levels := g.CalculateLevels(midPrice)

	if len(levels) != 10 {
		t.Errorf("Expected 10 levels, got %d", len(levels))
	}

	// Range should be 48500 to 51500
	min := 50000.0 * (1 - 0.03)
	max := 50000.0 * (1 + 0.03)

	if levels[0].Price != min {
		t.Errorf("Expected min level %f, got %f", min, levels[0].Price)
	}
	if levels[9].Price != max {
		t.Errorf("Expected max level %f, got %f", max, levels[9].Price)
	}
}

func TestGridTrading_Analyze_Activation(t *testing.T) {
	cfg := DefaultGridConfig()
	cfg.MaxVolatilityPct = 50.0
	cfg.MinVolatilityPct = 30.0
	cfg.Enabled = true
	g := NewGridTradingStrategy(cfg, "BTCUSD")

	// 1. Volatility too high (60%)
	f := features.MarketFeatures{
		HistoricalVol: 0.60,
		BestBid:       50000,
		BestAsk:       50050,
	}
	sig := g.Analyze(f, nil)
	if g.IsActive {
		t.Error("Grid should not be active when vol > 30%")
	}
	if sig.Action != ActionNone {
		t.Errorf("Expected ActionNone, got %v", sig.Action)
	}

	// 2. Volatility in range (20%)
	f.HistoricalVol = 0.20
	g.Analyze(f, nil)
	if !g.IsActive {
		t.Error("Grid should be active when vol < 30%")
	}

	// 3. Volatility spikes (70%) -> Deactivation
	f.HistoricalVol = 0.70
	sig = g.Analyze(f, nil)
	if g.IsActive {
		t.Error("Grid should deactivate when vol > 50%")
	}
	if sig.Action != ActionClose {
		t.Errorf("Expected ActionClose on deactivation, got %v", sig.Action)
	}
}
