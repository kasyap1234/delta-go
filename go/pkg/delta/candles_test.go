package delta

import (
	"testing"
)

func TestCandleHistory(t *testing.T) {
	// Simple test for Candle struct and slicing
	candles := []Candle{
		{Time: 1000, Open: 50000, Close: 50100},
		{Time: 2000, Open: 50100, Close: 49900},
	}

	if len(candles) != 2 {
		t.Error("Candle slice length mismatch")
	}

	if candles[1].Time != 2000 {
		t.Error("Candle time mismatch")
	}
}
