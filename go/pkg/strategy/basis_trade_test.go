package strategy

import (
	"testing"

	"github.com/kasyap/delta-go/go/pkg/features"
)

func TestBasisTradeMonitor_AnalyzeBasis(t *testing.T) {
	cfg := DefaultBasisTradeConfig()
	cfg.EntryThresholdAnnualized = 0.10 // 10%
	cfg.ExitThresholdAnnualized = 0.02  // 2%
	cfg.Enabled = true

	monitor := NewBasisTradeMonitor(cfg)
	symbol := "BTCUSD"

	// 1. Funding below threshold
	f := features.MarketFeatures{
		Symbol:          symbol,
		BasisAnnualized: 0.05, // 5%
	}
	sig := monitor.AnalyzeBasis(f)
	if sig.Action != "none" {
		t.Errorf("Expected action none, got %s", sig.Action)
	}

	// 2. Funding above entry threshold (Bullish funding -> Short Perp)
	f.BasisAnnualized = 0.15 // 15%
	sig = monitor.AnalyzeBasis(f)
	if sig.Action != "enter" || sig.PerpSide != "sell" {
		t.Errorf("Expected action enter/sell, got %s/%s", sig.Action, sig.PerpSide)
	}

	// 3. Negative funding (Bearish funding -> Long Perp)
	f.BasisAnnualized = -0.15 // -15%
	sig = monitor.AnalyzeBasis(f)
	if sig.Action != "enter" || sig.PerpSide != "buy" {
		t.Errorf("Expected action enter/buy, got %s/%s", sig.Action, sig.PerpSide)
	}

	// 4. Exit condition
	monitor.RecordEntry(&BasisPosition{
		ID:         "test_pos",
		PerpSymbol: symbol,
		PerpSide:   "buy",
	})
	f.BasisAnnualized = -0.01 // Converged
	sig = monitor.AnalyzeBasis(f)
	if sig.Action != "close" || sig.PerpSide != "sell" {
		t.Errorf("Expected action close/sell, got %s/%s", sig.Action, sig.PerpSide)
	}
}
