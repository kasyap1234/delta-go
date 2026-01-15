package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the trading bot
type Config struct {
	// Delta Exchange API
	APIKey       string
	APISecret    string
	BaseURL      string
	WebSocketURL string
	IsTestnet    bool

	// Trading
	Symbol         string   // Primary/single symbol (backward compatible)
	Symbols        []string // Multi-asset: list of symbols to scan
	Leverage       int
	MaxPositionPct float64 // Max % of wallet to use per position
	MultiAssetMode bool    // Enable multi-asset signal selection

	// Cloud Run HMM
	HMMEndpoint string

	// Risk Management
	MaxDrawdownPct  float64
	StopLossPct     float64
	TakeProfitPct   float64
	RiskPerTradePct float64

	// Intervals
	CandleInterval    string        // "1m", "5m", "15m", etc.
	RegimeCheckPeriod time.Duration // How often to check market regime
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	cfg := &Config{
		APIKey:         getEnv("DELTA_API_KEY", ""),
		APISecret:      getEnv("DELTA_API_SECRET", ""),
		IsTestnet:      getEnvBool("DELTA_TESTNET", true),
		Symbol:         getEnv("DELTA_SYMBOL", "BTCUSD"),
		Symbols:        parseSymbols(getEnv("DELTA_SYMBOLS", "BTCUSD,ETHUSD,SOLUSD")),
		Leverage:       getEnvInt("DELTA_LEVERAGE", 10),
		MaxPositionPct: getEnvFloat("DELTA_MAX_POSITION_PCT", 10.0),
		MultiAssetMode: getEnvBool("MULTI_ASSET_MODE", false),
		HMMEndpoint:    getEnv("HMM_ENDPOINT", "http://localhost:8080"),

		// Risk defaults
		MaxDrawdownPct:  getEnvFloat("MAX_DRAWDOWN_PCT", 10.0),
		StopLossPct:     getEnvFloat("STOP_LOSS_PCT", 2.0),
		TakeProfitPct:   getEnvFloat("TAKE_PROFIT_PCT", 4.0),
		RiskPerTradePct: getEnvFloat("RISK_PER_TRADE_PCT", 1.0),

		// Intervals
		CandleInterval:    getEnv("CANDLE_INTERVAL", "5m"),
		RegimeCheckPeriod: time.Duration(getEnvInt("REGIME_CHECK_SECONDS", 300)) * time.Second,
	}

	// Set URLs based on testnet flag
	if cfg.IsTestnet {
		cfg.BaseURL = "https://api-ind.testnet.deltaex.org/v2"
		cfg.WebSocketURL = "wss://socket-ind.testnet.deltaex.org"
	} else {
		cfg.BaseURL = "https://api.india.delta.exchange/v2"
		cfg.WebSocketURL = "wss://socket.india.delta.exchange"
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

// parseSymbols splits comma-separated symbols into a slice
func parseSymbols(s string) []string {
	symbols := []string{}
	for _, sym := range strings.Split(s, ",") {
		sym = strings.TrimSpace(sym)
		if sym != "" {
			symbols = append(symbols, sym)
		}
	}
	return symbols
}
