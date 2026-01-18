package logger

import (
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds the configuration for the logger
type Config struct {
	FilePath   string
	Level      string // DEBUG, INFO, WARN, ERROR
	MaxSize    int    // megabytes
	MaxBackups int
	MaxAge     int // days
}

// New creates a new structured logger
func New(cfg Config) (*slog.Logger, error) {
	// Default to stdout if no file path provided (or handle as error, but stdout is safe fallback/default)
	// Spec says "Implement structured (JSON) logging to files".
	// The test provides a file path.

	var w *lumberjack.Logger
	if cfg.FilePath != "" {
		w = &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    cfg.MaxSize, // megabytes
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge, // days
		}
		// Set defaults if zero
		if w.MaxSize == 0 {
			w.MaxSize = 100
		}
		if w.MaxBackups == 0 {
			w.MaxBackups = 3
		}
		if w.MaxAge == 0 {
			w.MaxAge = 28
		}
	}

	var level slog.Level
	switch cfg.Level {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	// If a file writer is configured, use it. Otherwise stdout.
	var handler slog.Handler
	if w != nil {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	
	// Set as default logger? Maybe not, keep it explicit.
	// slog.SetDefault(logger) 

	return logger, nil
}
