package risk

import (
	"testing"
	"time"

	"github.com/kasyap/delta-go/go/config"
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
