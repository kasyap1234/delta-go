package strategy

import (
	"testing"
	"time"

	"github.com/kasyap/delta-go/go/pkg/features"
)

func TestFeeAwareScalper_FeeWindow(t *testing.T) {
	scalper := NewFeeAwareScalper(DefaultScalperConfig(), nil)
	symbolBTC := "BTCUSD"
	symbolETH := "ETHUSD"

	// BTC Window should be 30m
	if scalper.GetFeeWindow(symbolBTC) != 30*time.Minute {
		t.Errorf("Expected 30m for BTC, got %v", scalper.GetFeeWindow(symbolBTC))
	}

	// Other Window should be 15m
	if scalper.GetFeeWindow(symbolETH) != 15*time.Minute {
		t.Errorf("Expected 15m for ETH, got %v", scalper.GetFeeWindow(symbolETH))
	}
}

func TestFeeAwareScalper_EntryExit(t *testing.T) {
	scalper := NewFeeAwareScalper(DefaultScalperConfig(), nil)
	symbol := "BTCUSD"

	scalper.RecordEntry(symbol)
	if _, ok := scalper.entryTimes[symbol]; !ok {
		t.Error("Expected entry time to be recorded")
	}

	scalper.RecordExit(symbol)
	if _, ok := scalper.entryTimes[symbol]; ok {
		t.Error("Expected entry time to be removed")
	}
}

func TestFeeAwareScalper_ShouldCloseForFees(t *testing.T) {
	scalper := NewFeeAwareScalper(DefaultScalperConfig(), nil)
	symbol := "BTCUSD"

	// No entry recorded
	if scalper.ShouldCloseForFees(symbol) != false {
		t.Error("Should return false when no entry recorded")
	}

	// Entry recorded just now
	scalper.RecordEntry(symbol)
	if scalper.ShouldCloseForFees(symbol) != true {
		t.Error("Should return true when inside fee window")
	}

	// Entry recorded 31m ago (BTC window is 30m)
	scalper.entryTimes[symbol] = time.Now().Add(-31 * time.Minute)
	if scalper.ShouldCloseForFees(symbol) != false {
		t.Error("Should return false when outside fee window")
	}
}

func TestFeeAwareScalper_AnalyzeWithFeatures(t *testing.T) {
	engine := features.NewEngine()
	cfg := DefaultScalperConfig()
	cfg.PersistenceSnapshots = 3
	cfg.ImbalanceThreshold = 0.5
	cfg.ConfirmationPricePct = 1.0 // 1%
	scalper := NewFeeAwareScalper(cfg, engine)

	f := features.MarketFeatures{
		Symbol:    "BTCUSD",
		SpreadBps: 5.0,
		BestBid:   50000,
		BestAsk:   50050,
	}

	// 1. Scalper disabled
	scalper.cfg.Enabled = false
	sig := scalper.AnalyzeWithFeatures(f, nil)
	if sig.Action != ActionNone || sig.Reason != "scalper disabled" {
		t.Errorf("Expected disabled reason, got %v", sig.Reason)
	}
	scalper.cfg.Enabled = true

	// 2. Spread too tight
	f.SpreadBps = 0.5
	sig = scalper.AnalyzeWithFeatures(f, nil)
	if sig.Action != ActionNone || sig.Reason != "spread too tight" {
		t.Errorf("Expected spread too tight reason, got %v", sig.Reason)
	}
	f.SpreadBps = 5.0

	// 3. Insufficient OBI history
	sig = scalper.AnalyzeWithFeatures(f, nil)
	if sig.Action != ActionNone || sig.Reason != "insufficient OBI history" {
		t.Errorf("Expected insufficient OBI, got %v", sig.Reason)
	}

	// 4. Bullish Setup
	// Add 3 BULLISH snapshots
	engine.AddOBISnapshot(features.OBISnapshot{Imbalance: 0.8, MidPrice: 50000})
	engine.AddOBISnapshot(features.OBISnapshot{Imbalance: 0.8, MidPrice: 50200})
	engine.AddOBISnapshot(features.OBISnapshot{Imbalance: 0.8, MidPrice: 50600}) // >1% increase from 50000

	sig = scalper.AnalyzeWithFeatures(f, nil)
	if sig.Action != ActionBuy {
		t.Errorf("Expected ActionBuy, got %v (Reason: %s)", sig.Action, sig.Reason)
	}

	// 5. Bearish Setup
	engine = features.NewEngine()
	scalper.engine = engine
	engine.AddOBISnapshot(features.OBISnapshot{Imbalance: -0.8, MidPrice: 50000})
	engine.AddOBISnapshot(features.OBISnapshot{Imbalance: -0.8, MidPrice: 49800})
	engine.AddOBISnapshot(features.OBISnapshot{Imbalance: -0.8, MidPrice: 49400}) // >1% decrease from 50000

	sig = scalper.AnalyzeWithFeatures(f, nil)
	if sig.Action != ActionSell {
		t.Errorf("Expected ActionSell, got %v (Reason: %s)", sig.Action, sig.Reason)
	}
}
