package strategy

import (
	"testing"
	"time"

	"github.com/kasyap/delta-go/go/pkg/features"
)

func TestFundingArbitrage_Analyze(t *testing.T) {
	cfg := DefaultFundingArbitrageConfig()
	cfg.EntryThresholdAnnualized = 0.15 // 15%
	cfg.ExitThresholdAnnualized = 0.05  // 5%
	cfg.MaxHoldingHours = 24
	cfg.Enabled = true

	s := NewFundingArbitrageStrategy(cfg)
	symbol := "BTCUSD"

	// 1. Funding below threshold - No Signal
	f := features.MarketFeatures{
		Symbol:          symbol,
		BasisAnnualized: 0.10, // 10%
	}
	sig := s.Analyze(f, nil)
	if sig.Action != ActionNone {
		t.Errorf("Expected ActionNone for 10%% funding, got %v", sig.Action)
	}

	// 2. High Positive Funding - Short Signal
	f.BasisAnnualized = 0.20 // 20%
	sig = s.Analyze(f, nil)
	if sig.Action != ActionSell || sig.Side != "sell" {
		t.Errorf("Expected ActionSell/sell for 20%% funding, got %v/%v", sig.Action, sig.Side)
	}

	// 3. High Negative Funding - Long Signal
	f.BasisAnnualized = -0.20 // -20%
	sig = s.Analyze(f, nil)
	if sig.Action != ActionBuy || sig.Side != "buy" {
		t.Errorf("Expected ActionBuy/buy for -20%% funding, got %v/%v", sig.Action, sig.Side)
	}

	// 4. Position Exit on Converged Funding
	s.RecordEntry(symbol, "buy", -0.20)
	f.BasisAnnualized = -0.04 // -4% (converged below 5% threshold)
	sig = s.Analyze(f, nil)
	if sig.Action != ActionClose || sig.Side != "sell" {
		t.Errorf("Expected ActionClose/sell (closing long), got %v/%v", sig.Action, sig.Side)
	}

	// 5. Position Exit on Timeout
	s.RecordEntry(symbol, "sell", 0.20)
	s.positions[symbol].EntryTime = time.Now().Add(-25 * time.Hour)
	f.BasisAnnualized = 0.10 // Still above 5% exit, but timeout applies
	sig = s.Analyze(f, nil)
	if sig.Action != ActionClose || sig.Side != "buy" {
		t.Errorf("Expected ActionClose/buy (closing short) due to timeout, got %v/%v", sig.Action, sig.Side)
	}
}
