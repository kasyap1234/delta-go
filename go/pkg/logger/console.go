package logger

import (
	"fmt"
	"time"
)

// ANSI Color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
)

// FormatConsoleLog formats a message with color based on level
func FormatConsoleLog(level string, msg string) string {
	timestamp := time.Now().Format("15:04:05")
	var color string
	switch level {
	case "INFO":
		color = ColorGreen
	case "WARN":
		color = ColorYellow
	case "ERROR":
		color = ColorRed
	case "DEBUG":
		color = ColorCyan
	default:
		color = ColorReset
	}
	
	return fmt.Sprintf("%s%s [%s] %s%s", color, timestamp, level, msg, ColorReset)
}

// ConsoleLog prints a formatted log to stdout immediately (bypass structured logger for CLI/UI)
func ConsoleLog(level, msg string) {
	fmt.Println(FormatConsoleLog(level, msg))
}
