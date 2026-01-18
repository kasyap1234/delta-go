package logger_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kasyap/delta-go/go/pkg/logger"
)

func TestFileLogging(t *testing.T) {
	// Create a temporary directory for logs
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "bot.log")

	cfg := logger.Config{
		FilePath: logFile,
		Level:    "INFO",
	}

	// Initialize Logger
	logInstance, err := logger.New(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Write a log entry
	testMessage := "Test log message"
	logInstance.Info(testMessage, "component", "test_runner")

	// Allow some time for async writes if any (slog is usually sync but file IO might lag slightly?)
	// Not needed for sync file writing.

	// Read file content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Fatal("Log file is empty")
	}

	// Parse JSON
	var entry map[string]interface{}
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("Log file content is not valid JSON: %s", string(content))
	}

	// Verify content
	if msg, ok := entry["msg"]; !ok || msg != testMessage {
		t.Errorf("Expected message %q, got %q", testMessage, msg)
	}
	if level, ok := entry["level"]; !ok || level != "INFO" {
		t.Errorf("Expected level INFO, got %v", level)
	}
	if comp, ok := entry["component"]; !ok || comp != "test_runner" {
		t.Errorf("Expected component 'test_runner', got %v", comp)
	}
}

func TestLoggerLevels(t *testing.T) {
	levels := []string{"DEBUG", "WARN", "ERROR", "INVALID_DEFAULT"}
	for _, lvl := range levels {
		cfg := logger.Config{
			Level: lvl,
		}
		_, err := logger.New(cfg)
		if err != nil {
			t.Errorf("Failed to init logger with level %s: %v", lvl, err)
		}
	}
}
