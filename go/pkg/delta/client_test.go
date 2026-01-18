package delta

import (
	"testing"

	"github.com/kasyap/delta-go/go/config"
)

func TestNewClient_ExtractsPathPrefix(t *testing.T) {
	cfg := &config.Config{
		BaseURL:           "https://api.india.delta.exchange/v2",
		APIKey:            "k",
		APISecret:         "s",
		APIRateLimitRPS:   8,
		IsTestnet:         false,
		WebSocketURL:      "wss://socket.india.delta.exchange",
		DailyLossLimitPct: -5,
	}

	c := NewClient(cfg)
	if c.apiPathPrefix != "/v2" {
		t.Fatalf("apiPathPrefix mismatch: got=%q want=%q", c.apiPathPrefix, "/v2")
	}
}
