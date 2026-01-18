package backtest

import (
	"testing"
	"time"
)

func TestMetricsCalculator_TotalReturn(t *testing.T) {
	config := DefaultConfig()
	config.InitialCapital = 1000

	mc := NewMetricsCalculator(config)

	equityCurve := []EquityPoint{
		{Timestamp: time.Now().Add(-24 * time.Hour), Equity: 1000},
		{Timestamp: time.Now(), Equity: 1100},
	}

	metrics := mc.Calculate(nil, equityCurve)

	// Expected 10% return
	expected := 0.10
	if absMetrics(metrics.TotalReturn-expected) > 0.001 {
		t.Errorf("Expected return %.4f, got %.4f", expected, metrics.TotalReturn)
	}
}

func TestMetricsCalculator_MaxDrawdown(t *testing.T) {
	config := DefaultConfig()
	config.InitialCapital = 1000

	mc := NewMetricsCalculator(config)

	// Equity goes 1000 -> 1200 -> 1000 -> 1100 (max DD occurs at second 1000)
	now := time.Now()
	equityCurve := []EquityPoint{
		{Timestamp: now.Add(-72 * time.Hour), Equity: 1000},
		{Timestamp: now.Add(-48 * time.Hour), Equity: 1200}, // Peak
		{Timestamp: now.Add(-24 * time.Hour), Equity: 1000}, // 16.67% drawdown
		{Timestamp: now, Equity: 1100},
	}

	metrics := mc.Calculate(nil, equityCurve)

	// Max DD should be (1200-1000)/1200 = 16.67%
	expectedDD := 200.0 / 1200.0
	if absMetrics(metrics.MaxDrawdown-expectedDD) > 0.01 {
		t.Errorf("Expected max drawdown %.4f, got %.4f", expectedDD, metrics.MaxDrawdown)
	}
}

func TestMetricsCalculator_WinRate(t *testing.T) {
	config := DefaultConfig()
	mc := NewMetricsCalculator(config)

	trades := []Trade{
		{NetPnL: 100}, // Win
		{NetPnL: 50},  // Win
		{NetPnL: -30}, // Loss
		{NetPnL: 80},  // Win
		{NetPnL: -40}, // Loss
	}

	equityCurve := []EquityPoint{
		{Timestamp: time.Now(), Equity: 1160},
	}

	metrics := mc.Calculate(trades, equityCurve)

	// 3 wins out of 5 = 60%
	if absMetrics(metrics.WinRate-0.6) > 0.001 {
		t.Errorf("Expected win rate 0.6, got %.4f", metrics.WinRate)
	}
}

func TestMetricsCalculator_ProfitFactor(t *testing.T) {
	config := DefaultConfig()
	mc := NewMetricsCalculator(config)

	trades := []Trade{
		{NetPnL: 100}, // Win
		{NetPnL: 50},  // Win
		{NetPnL: -30}, // Loss
		{NetPnL: -20}, // Loss
	}

	equityCurve := []EquityPoint{
		{Timestamp: time.Now(), Equity: 1100},
	}

	metrics := mc.Calculate(trades, equityCurve)

	// Profit factor: 150 / 50 = 3.0
	if absMetrics(metrics.ProfitFactor-3.0) > 0.001 {
		t.Errorf("Expected profit factor 3.0, got %.4f", metrics.ProfitFactor)
	}
}

func TestMetricsCalculator_CostBreakdown(t *testing.T) {
	config := DefaultConfig()
	mc := NewMetricsCalculator(config)

	trades := []Trade{
		{EntryFee: 1.0, ExitFee: 1.0, EntrySlip: 0.5, ExitSlip: 0.5, FundingPaid: 0.2},
		{EntryFee: 1.5, ExitFee: 1.5, EntrySlip: 0.8, ExitSlip: 0.7, FundingPaid: 0.3},
	}

	equityCurve := []EquityPoint{
		{Timestamp: time.Now(), Equity: 1000},
	}

	metrics := mc.Calculate(trades, equityCurve)

	expectedFees := 5.0
	expectedSlip := 2.5
	expectedFunding := 0.5

	if absMetrics(metrics.TotalFees-expectedFees) > 0.001 {
		t.Errorf("Expected fees %.2f, got %.2f", expectedFees, metrics.TotalFees)
	}
	if absMetrics(metrics.TotalSlippage-expectedSlip) > 0.001 {
		t.Errorf("Expected slippage %.2f, got %.2f", expectedSlip, metrics.TotalSlippage)
	}
	if absMetrics(metrics.TotalFunding-expectedFunding) > 0.001 {
		t.Errorf("Expected funding %.2f, got %.2f", expectedFunding, metrics.TotalFunding)
	}
}

func absMetrics(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
