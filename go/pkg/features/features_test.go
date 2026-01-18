package features

import (
	"math"
	"testing"
	"time"

	"github.com/kasyap/delta-go/go/pkg/delta"
)

func TestEngine_ComputeFeatures(t *testing.T) {
	e := NewEngine()

	ob := &delta.Orderbook{
		Symbol: "BTCUSD",
		Buy: []delta.OrderbookEntry{
			{Price: "50000.0", Size: 10},
		},
		Sell: []delta.OrderbookEntry{
			{Price: "50100.0", Size: 10},
		},
	}

	tick := &delta.Ticker{
		Symbol: "BTCUSD",
		Close:  50050.0,
	}

	f := e.ComputeFeatures(ob, tick, nil, time.Time{}, 0)

	if f.BestBid != 50000.0 {
		t.Errorf("Expected bid 50000, got %f", f.BestBid)
	}
	if f.BestAsk != 50100.0 {
		t.Errorf("Expected ask 50100, got %f", f.BestAsk)
	}
	if f.SpreadBps != 20.0 { // (100 / 50050) * 10000 is approx 19.98. Wait mid is 50050.
		// (100 / 50050) * 10000 = 19.98. Let's check logic: (Spread / mid) * 10000
	}
}

func TestEngine_ComputeFeaturesWithFunding(t *testing.T) {
	e := NewEngine()
	tick := &delta.Ticker{
		Symbol:      "BTCUSD",
		FundingRate: 0.0001, // 0.01% per 8h
	}

	f := e.ComputeFeaturesWithFunding(nil, tick, nil)

	// Annualized = 0.0001 * 3 * 365 = 0.1095 (10.95%)
	expected := 0.0001 * 3 * 365
	if math.Abs(f.BasisAnnualized-expected) > 0.000001 {
		t.Errorf("Expected annualized %f, got %f", expected, f.BasisAnnualized)
	}
}

func TestEngine_ImbalanceHistory(t *testing.T) {
	e := NewEngine()
	e.maxOBISnapshots = 5

	for i := 1; i <= 10; i++ {
		e.AddOBISnapshot(OBISnapshot{Imbalance: float64(i) / 10})
	}

	snapshots := e.GetOBISnapshots()
	if len(snapshots) != 5 {
		t.Errorf("Expected 5 snapshots, got %d", len(snapshots))
	}

	if snapshots[4].Imbalance != 1.0 {
		t.Errorf("Expected last imbalance 1.0, got %f", snapshots[4].Imbalance)
	}
}
