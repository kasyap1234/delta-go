package logger_test

import (
	"testing"

	"github.com/kasyap/delta-go/go/pkg/logger"
)

func TestConsoleColor(t *testing.T) {
	// Verify color codes are defined
	if logger.ColorGreen == "" {
		t.Error("Expected ColorGreen to be defined")
	}
	if logger.ColorRed == "" {
		t.Error("Expected ColorRed to be defined")
	}
	if logger.ColorReset == "" {
		t.Error("Expected ColorReset to be defined")
	}
}

func TestConsoleLogFormatting(t *testing.T) {
	// Test helper function for formatting
	msg := "Test Message"
	formatted := logger.FormatConsoleLog("INFO", msg)
	// We expect some coloring or formatting
	if len(formatted) <= len(msg) {
		t.Error("Formatted message should contain color codes or structure")
	}
}
