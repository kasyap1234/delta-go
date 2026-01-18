package risk

import (
	"testing"
	"time"

	"github.com/kasyap/delta-go/go/config"
	"github.com/kasyap/delta-go/go/pkg/delta"
)

func TestCanTrade_ResetsCircuitBreakerAfter24Hours(t *testing.T) {
	rm := NewRiskManager(&config.Config{
		MaxDrawdownPct:    10,
		DailyLossLimitPct: -5,
		RiskPerTradePct:   1,
		Leverage:          1,
		MaxPositionPct:    10,
		StopLossPct:       2,
		TakeProfitPct:     4,
		CandleInterval:    "5m",
		RegimeCheckPeriod: 5 * time.Minute,
		APIRateLimitRPS:   8,
		BaseURL:           "https://api.india.delta.exchange/v2",
		WebSocketURL:      "wss://socket.india.delta.exchange",
	})

	rm.mu.Lock()
	rm.currentBalance = 100
	rm.peakBalance = 100
	rm.isCircuitBroken = true
	rm.circuitBrokenAt = time.Now().Add(-25 * time.Hour)
	rm.mu.Unlock()

	can, _ := rm.CanTrade()
	if !can {
		t.Fatalf("expected trading to resume")
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()
	if rm.isCircuitBroken {
		t.Fatalf("expected circuit breaker to reset")
	}
	if rm.peakBalance != rm.currentBalance {
		t.Fatalf("expected peak balance to reset to current balance")
	}
}

func TestNewRiskManager_UsesConfiguredDailyLossLimit(t *testing.T) {
	rm := NewRiskManager(&config.Config{DailyLossLimitPct: -2.5})
	if rm.dailyLossLimit != -2.5 {
		t.Fatalf("dailyLossLimit mismatch: got=%v want=%v", rm.dailyLossLimit, -2.5)
	}
}

func TestCalculatePositionSize_DoesNotScaleWithLeverage(t *testing.T) {
	rm := NewRiskManager(&config.Config{
		RiskPerTradePct: 1,
		StopLossPct:     2,
		Leverage:        20,
		MaxPositionPct:  100,
	})

	size := rm.CalculatePositionSize(
		1000,
		100,
		98,
		delta.RegimeRanging,
		&delta.Product{ContractValue: "1"},
	)

	if size != 5 {
		t.Fatalf("size mismatch: got=%d want=%d", size, 5)
	}
}

func TestCalculatePositionSize_UsesContractValue(t *testing.T) {
	rm := NewRiskManager(&config.Config{
		RiskPerTradePct: 1,
		StopLossPct:     2,
		Leverage:        10,
		MaxPositionPct:  100,
	})

	size := rm.CalculatePositionSize(
		1000,
		100,
		98,
		delta.RegimeRanging,
		&delta.Product{ContractValue: "0.1"},
	)

	if size != 50 {
		t.Fatalf("size mismatch: got=%d want=%d", size, 50)
	}
}

func TestCalculatePositionSize_ReturnsZeroWhenRiskTooSmall(t *testing.T) {
	rm := NewRiskManager(&config.Config{
		RiskPerTradePct: 1,
		StopLossPct:     2,
		Leverage:        10,
		MaxPositionPct:  100,
	})

	size := rm.CalculatePositionSize(
		1000,
		100,
		0,
		delta.RegimeRanging,
		&delta.Product{ContractValue: "1"},
	)

	if size != 0 {
		t.Fatalf("size mismatch: got=%d want=%d", size, 0)
	}
}
func TestRiskManager_DailyLossLimit(t *testing.T) {
	rm := NewRiskManager(&config.Config{
		DailyLossLimitPct: -5.0,
	})

	rm.UpdateBalance(100)
	rm.UpdateBalance(94) // -6% loss

	can, reason := rm.CanTrade()
	if can {
		t.Errorf("Expected CanTrade to be false after 6%% loss, got true. Reason: %s", reason)
	}
}

func TestRiskManager_DrawdownCircuitBreaker(t *testing.T) {
	rm := NewRiskManager(&config.Config{
		MaxDrawdownPct: 10.0,
	})

	rm.UpdateBalance(100)
	rm.UpdateBalance(89) // -11% drawdown from peak

	can, reason := rm.CanTrade()
	if can {
		t.Errorf("Expected CanTrade to be false after 11%% drawdown, got true. Reason: %s", reason)
	}

	if !rm.isCircuitBroken {
		t.Error("Expected isCircuitBroken to be true")
	}
}
