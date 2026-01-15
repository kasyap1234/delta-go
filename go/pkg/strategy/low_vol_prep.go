package strategy

import (
	"github.com/kasyap/delta-go/go/pkg/delta"
)

// LowVolPrepStrategy implements preparation and scalping for low volatility regimes
// Main Action: Place limit orders at range extremes to prepare for breakout
// Advanced: Very short-term scalping with tiny position sizes
type LowVolPrepStrategy struct {
	indicators *TechnicalIndicators

	// Parameters
	RangeLookback int
	ScalpingMode  bool    // If true, generate short-term scalp signals
	ScalpATRMult  float64 // ATR multiplier for scalp targets
	PrepOrderDist float64 // Distance from S/R for prep orders (% of range)
}

// NewLowVolPrepStrategy creates a new low volatility preparation strategy
func NewLowVolPrepStrategy() *LowVolPrepStrategy {
	return &LowVolPrepStrategy{
		indicators:    NewIndicators(),
		RangeLookback: 30,
		ScalpingMode:  false,
		ScalpATRMult:  0.5,
		PrepOrderDist: 0.02, // 2% into the range
	}
}

func (s *LowVolPrepStrategy) Name() string {
	return "low_vol_preparation"
}

func (s *LowVolPrepStrategy) UpdateParams(params map[string]interface{}) {
	if v, ok := params["scalping_mode"].(bool); ok {
		s.ScalpingMode = v
	}
}

func (s *LowVolPrepStrategy) Analyze(candles []delta.Candle, regime delta.MarketRegime) Signal {
	if len(candles) < s.RangeLookback+10 {
		return Signal{Action: ActionNone, Reason: "insufficient data"}
	}

	// Extract price arrays
	closes := extractCloses(candles)
	highs := extractHighs(candles)
	lows := extractLows(candles)

	n := len(closes)
	currentPrice := closes[n-1]

	// Calculate indicators
	atr := s.indicators.ATR(highs, lows, closes, 14)
	currentATR := atr[n-1]
	rsi := s.indicators.RSI(closes, 14)
	currentRSI := rsi[n-1]

	// Find range
	rangeHigh := maxSlice(highs[n-s.RangeLookback:]...)
	rangeLow := minSlice(lows[n-s.RangeLookback:]...)
	rangeSize := rangeHigh - rangeLow

	if rangeSize <= 0 {
		return Signal{Action: ActionNone, Reason: "invalid range"}
	}

	// In scalping mode, look for very short-term opportunities
	if s.ScalpingMode {
		return s.scalpingAnalysis(candles, currentPrice, currentATR, currentRSI)
	}

	// Normal prep mode: Generate limit order suggestions
	// These are prep orders to catch a breakout or fade at extremes

	// Calculate prep order levels
	buyPrepLevel := rangeLow + (rangeSize * s.PrepOrderDist)
	sellPrepLevel := rangeHigh - (rangeSize * s.PrepOrderDist)

	// If price near lower range, suggest buy prep
	distToLow := (currentPrice - rangeLow) / rangeSize
	distToHigh := (rangeHigh - currentPrice) / rangeSize

	if distToLow < 0.2 {
		// Near support, prepare buy order
		stopLoss := rangeLow - (rangeSize * 0.1)
		takeProfit := currentPrice + (rangeSize * 0.3)

		return Signal{
			Action:     ActionBuy,
			Side:       "buy",
			Confidence: 0.4, // Low confidence, preparatory
			Price:      buyPrepLevel,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "low vol: prep buy limit at support",
		}
	}

	if distToHigh < 0.2 {
		// Near resistance, prepare sell order
		stopLoss := rangeHigh + (rangeSize * 0.1)
		takeProfit := currentPrice - (rangeSize * 0.3)

		return Signal{
			Action:     ActionSell,
			Side:       "sell",
			Confidence: 0.4,
			Price:      sellPrepLevel,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "low vol: prep sell limit at resistance",
		}
	}

	// In the middle - just monitor
	return Signal{
		Action: ActionNone,
		Reason: "low vol: monitoring for opportunity at range extremes",
	}
}

// scalpingAnalysis generates very short-term scalp signals
func (s *LowVolPrepStrategy) scalpingAnalysis(candles []delta.Candle, currentPrice, atr, rsi float64) Signal {
	// Look at last few candles for micro patterns
	n := len(candles)
	if n < 5 {
		return Signal{Action: ActionNone, Reason: "insufficient data for scalp"}
	}

	// Simple scalp logic: RSI extremes on very short timeframe
	// with tight ATR-based stops

	// Oversold bounce
	if rsi < 25 {
		stopLoss := currentPrice - (atr * s.ScalpATRMult)
		takeProfit := currentPrice + (atr * s.ScalpATRMult)

		return Signal{
			Action:     ActionBuy,
			Side:       "buy",
			Confidence: 0.35, // Very low, it's a scalp
			Price:      currentPrice,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "low vol scalp: RSI oversold bounce",
		}
	}

	// Overbought fade
	if rsi > 75 {
		stopLoss := currentPrice + (atr * s.ScalpATRMult)
		takeProfit := currentPrice - (atr * s.ScalpATRMult)

		return Signal{
			Action:     ActionSell,
			Side:       "sell",
			Confidence: 0.35,
			Price:      currentPrice,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "low vol scalp: RSI overbought fade",
		}
	}

	return Signal{Action: ActionNone, Reason: "no scalp setup"}
}
