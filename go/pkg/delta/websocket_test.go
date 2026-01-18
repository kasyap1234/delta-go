package delta

import (
	"testing"

	"github.com/kasyap/delta-go/go/config"
)

func TestWebSocketSubscribe_Deduplicates(t *testing.T) {
	ws := NewWebSocketClient(&config.Config{WebSocketURL: "wss://example"})

	if err := ws.SubscribeTicker("BTCUSD"); err != nil {
		t.Fatalf("subscribe ticker: %v", err)
	}
	if err := ws.SubscribeTicker("BTCUSD"); err != nil {
		t.Fatalf("subscribe ticker twice: %v", err)
	}

	if got := len(ws.subscriptions); got != 1 {
		t.Fatalf("expected 1 subscription, got %d", got)
	}
	if ws.subscriptions[0].name != "v2/ticker" {
		t.Fatalf("expected channel %q, got %q", "v2/ticker", ws.subscriptions[0].name)
	}
	if len(ws.subscriptions[0].symbols) != 1 || ws.subscriptions[0].symbols[0] != "BTCUSD" {
		t.Fatalf("unexpected symbols: %#v", ws.subscriptions[0].symbols)
	}
}
