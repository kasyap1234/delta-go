package main

import (
	"strings"
	"testing"
)

// Mock for capturing console output (conceptually)
// Since ConsoleLog writes to stdout, we can't easily capture it in a simple unit test without
// redirecting os.Stdout, which is invasive. 
// Instead, we will test the formatting logic if exposed, or rely on integration tests.
// Here we verify that the LogHeartbeat function exists and generates a formatted string.

func TestLogHeartbeatFormatting(t *testing.T) {
	// This test simulates the logic inside LogHeartbeat
	stats := map[string]interface{}{
		"pnl_abs": 100.0,
		"pnl_pct": 1.5,
		"open_positions": 2,
		"last_equity": 10100.0,
	}
	
	msg := formatHeartbeat(stats)
	
	if !strings.Contains(msg, "PnL: +100.00") {
		t.Errorf("Expected PnL in message, got: %s", msg)
	}
	if !strings.Contains(msg, "+1.50%") {
		t.Errorf("Expected PnL percent in message, got: %s", msg)
	}
	if !strings.Contains(msg, "Pos: 2") {
		t.Errorf("Expected Open Positions in message, got: %s", msg)
	}
}

// Helper to test private logic if we were in the same package, 
// but since this is main_test, we might need to export it or put this test in main package.
// We will put it in main package for this test.
