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
	// Note: size is NOTIONAL VALUE in dollars, not contract count
	// For a position worth $10,000 at 5 bps (0.05%) fee
	notionalValue := 10000.0
	feeBps := 5.0 // 0.05%

	fee := CalculateFee(50000.0, notionalValue, 1.0, feeBps)

	// Expected: 10000 * 5/10000 = $5
	expected := 5.0
	if abs(fee-expected) > 0.01 {
		t.Errorf("Expected fee %.2f, got %.2f", expected, fee)
	}
}

func TestPositionUnrealizedPnL(t *testing.T) {
	// Test with contract-based semantics:
	// Size = contracts (e.g., 10 contracts)
	// ContractValue = 0.001 (for BTCUSD, 1 contract = 0.001 BTC)
	// P&L = contracts * contractValue * (currentPrice - entryPrice) * direction
	pos := &Position{
		Side:       "buy",
		Size:       10.0, // 10 contracts
		EntryPrice: 50000,
	}

	// Price goes up by 500 (1%)
	// P&L = 10 * 0.001 * (50500 - 50000) * 1 = 10 * 0.001 * 500 = 5.0
	pnl := pos.UnrealizedPnL(50500, 0.001)

	expected := 5.0
	if abs(pnl-expected) > 0.01 {
		t.Errorf("Expected PnL %.4f, got %.4f", expected, pnl)
	}

	// Short position
	shortPos := &Position{
		Side:       "sell",
		Size:       10.0,
		EntryPrice: 50000,
	}

	// Price goes down by 500 - short profits
	// P&L = 10 * 0.001 * (49500 - 50000) * -1 = 10 * 0.001 * 500 = 5.0
	shortPnl := shortPos.UnrealizedPnL(49500, 0.001)
	expected = 5.0
	if abs(shortPnl-expected) > 0.01 {
		t.Errorf("Short position should profit when price drops, expected %.4f got %.4f", expected, shortPnl)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
