package delta

import (
	"testing"
)

func TestRoundToTickSize(t *testing.T) {
	tests := []struct {
		name     string
		price    float64
		tickSize string
		want     string
	}{
		{"Round nearest", 50000.123, "0.1", "50000.1"},
		{"Round nearest 2", 50000.123, "0.5", "50000.0"},
		{"Exact match", 50000.5, "0.50", "50000.50"},
		{"High precision tick", 1.23456, "0.01", "1.23"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RoundToTickSize(tt.price, tt.tickSize)
			if err != nil {
				t.Errorf("RoundToTickSize() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("RoundToTickSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderRequest_Validation(t *testing.T) {
	// Simple test for OrderRequest fields
	req := &OrderRequest{
		ProductID:  1,
		Size:       10,
		Side:       "buy",
		OrderType:  "limit_order",
		LimitPrice: "50000.0",
	}

	if req.ProductID != 1 || req.Size != 10 || req.Side != "buy" {
		t.Error("OrderRequest fields not set correctly")
	}
}
