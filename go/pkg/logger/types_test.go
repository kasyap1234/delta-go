package logger_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kasyap/delta-go/go/pkg/logger"
)

func TestTradeEventSchema(t *testing.T) {
	// This test verifies that TradeEvent struct is defined with the expected JSON tags
	event := logger.TradeEvent{
		Symbol:    "BTCUSD",
		Side:      "BUY",
		Price:     50000.0,
		Quantity:  1.0,
		Timestamp: time.Now(),
		OrderID:   "12345",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal TradeEvent: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal TradeEvent: %v", err)
	}

	// Verify keys exist (checking JSON tags)
	expectedKeys := []string{"symbol", "side", "price", "quantity", "timestamp", "order_id"}
	for _, key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("TradeEvent JSON missing key: %s", key)
		}
	}
}

func TestSystemHealthEventSchema(t *testing.T) {
	// This test verifies that SystemHealthEvent struct is defined with the expected JSON tags
	event := logger.SystemHealthEvent{
		Component:   "RiskManager",
		Status:      "OK",
		Latency:     15 * time.Millisecond,
		MemoryUsage: 1024,
		Timestamp:   time.Now(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal SystemHealthEvent: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal SystemHealthEvent: %v", err)
	}

	// Verify keys exist
	expectedKeys := []string{"component", "status", "latency_ms", "memory_bytes", "timestamp"}
	for _, key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("SystemHealthEvent JSON missing key: %s", key)
		}
	}
}

func TestLogConstants(t *testing.T) {
	// Verify that we have some standardized log keys
	expectedKeys := []string{
		logger.KeyTraceID,
		logger.KeyComponent,
		logger.KeyEnvironment,
	}

	if len(expectedKeys) == 0 {
		t.Fatal("Expected log constants to be defined")
	}
}
