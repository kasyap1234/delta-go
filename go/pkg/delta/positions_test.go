package delta

import (
	"testing"
)

func TestPositionParsing(t *testing.T) {
	// Verify Position struct fields
	p := Position{
		ProductSymbol: "BTCUSD",
		Size:          10,
		EntryPrice:    "50000.0",
		UnrealizedPnL: "100.0",
	}

	if p.ProductSymbol != "BTCUSD" || p.Size != 10 {
		t.Error("Position fields not set correctly")
	}
}
