package backtest

import (
	"testing"

	"github.com/kasyap/delta-go/go/pkg/delta"
)

func TestFixedSlippage(t *testing.T) {
	slippage := NewFixedSlippage(2.0) // 2 bps

	candle := delta.Candle{
		High:  50100,
		Low:   49900,
		Close: 50000,
	}

	result := slippage.Calculate("buy", 1, candle, 0)

	// Expected: mid price (50000) * 2/10000 = 10
	expected := 10.0
	if abs(result-expected) > 0.01 {
		t.Errorf("Expected slippage %.2f, got %.2f", expected, result)
	}
}

func TestVolatilitySlippage(t *testing.T) {
	slippage := NewVolatilitySlippage(1.5, 0.5)

	// Low volatility candle
	lowVolCandle := delta.Candle{
		High:  50100,
		Low:   50000,
		Close: 50050,
	}
	lowSlip := slippage.Calculate("buy", 1, lowVolCandle, 0)

	// High volatility candle
	highVolCandle := delta.Candle{
		High:  51000,
		Low:   49000,
		Close: 50000,
	}
	highSlip := slippage.Calculate("buy", 1, highVolCandle, 0)

	if highSlip <= lowSlip {
		t.Errorf("High vol slippage (%.2f) should be greater than low vol (%.2f)", highSlip, lowSlip)
	}
}

func TestApplySlippage(t *testing.T) {
	price := 50000.0
	slippage := 10.0

	buyPrice := ApplySlippage(price, slippage, "buy")
	if buyPrice != 50010 {
		t.Errorf("Buy price should be 50010, got %.2f", buyPrice)
	}

	sellPrice := ApplySlippage(price, slippage, "sell")
	if sellPrice != 49990 {
		t.Errorf("Sell price should be 49990, got %.2f", sellPrice)
	}
}

func TestCalculateFee(t *testing.T) {
	price := 50000.0
	size := 10
	contractValue := 1.0
	feeBps := 5.0 // 0.05%

	fee := CalculateFee(price, size, contractValue, feeBps)

	// Expected: 50000 * 10 * 1 * 5/10000 = 250
	expected := 250.0
	if abs(fee-expected) > 0.01 {
		t.Errorf("Expected fee %.2f, got %.2f", expected, fee)
	}
}

func TestPositionUnrealizedPnL(t *testing.T) {
	pos := &Position{
		Side:       "buy",
		Size:       10,
		EntryPrice: 50000,
	}

	// Price goes up 1%
	pnl := pos.UnrealizedPnL(50500, 1.0)

	// Expected: (50500-50000)/50000 * 10 * 1 = 0.1
	expected := 0.1
	if abs(pnl-expected) > 0.01 {
		t.Errorf("Expected PnL %.4f, got %.4f", expected, pnl)
	}

	// Short position
	shortPos := &Position{
		Side:       "sell",
		Size:       10,
		EntryPrice: 50000,
	}

	// Price goes down - short profits
	shortPnl := shortPos.UnrealizedPnL(49500, 1.0)
	if shortPnl <= 0 {
		t.Errorf("Short position should profit when price drops, got %.4f", shortPnl)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
