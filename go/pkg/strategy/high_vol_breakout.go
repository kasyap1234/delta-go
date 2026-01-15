package strategy

import (
	"github.com/kasyap/delta-go/go/pkg/delta"
)

// HighVolBreakoutStrategy implements momentum breakout for high volatility regimes
// Entry: Wait for strong candle close outside recent range with volume > 150% of average
// Avoid: Do not chase moves already 2-3% beyond breakout point
type HighVolBreakoutStrategy struct {
	indicators *TechnicalIndicators

	// Parameters
	RangeLookback   int
	VolumeThreshold float64 // 1.5 = 150%
	MaxChasePercent float64 // Don't chase beyond this % from breakout
	ATRMultiplier   float64
}

// NewHighVolBreakoutStrategy creates a new high volatility breakout strategy
func NewHighVolBreakoutStrategy() *HighVolBreakoutStrategy {
	return &HighVolBreakoutStrategy{
		indicators:      NewIndicators(),
		RangeLookback:   20,
		VolumeThreshold: 1.5,
		MaxChasePercent: 0.02, // 2%
		ATRMultiplier:   2.0,
	}
}

func (s *HighVolBreakoutStrategy) Name() string {
	return "high_vol_breakout"
}

func (s *HighVolBreakoutStrategy) UpdateParams(params map[string]interface{}) {
	if v, ok := params["volume_threshold"].(float64); ok {
		s.VolumeThreshold = v
	}
	if v, ok := params["max_chase"].(float64); ok {
		s.MaxChasePercent = v
	}
}

func (s *HighVolBreakoutStrategy) Analyze(candles []delta.Candle, regime delta.MarketRegime) Signal {
	if len(candles) < s.RangeLookback+10 {
		return Signal{Action: ActionNone, Reason: "insufficient data"}
	}

	// Extract price arrays
	closes := extractCloses(candles)
	highs := extractHighs(candles)
	lows := extractLows(candles)
	volumes := extractVolumes(candles)

	n := len(closes)
	currentPrice := closes[n-1]
	currentHigh := highs[n-1]
	currentLow := lows[n-1]

	// Calculate indicators
	atr := s.indicators.ATR(highs, lows, closes, 14)
	currentATR := atr[n-1]

	// Find recent range (before current candle)
	rangeHighs := highs[n-s.RangeLookback-1 : n-1]
	rangeLows := lows[n-s.RangeLookback-1 : n-1]

	rangeHigh := maxSlice(rangeHighs...)
	rangeLow := minSlice(rangeLows...)
	rangeMiddle := (rangeHigh + rangeLow) / 2

	// Volume check
	avgVolume := average(volumes[n-20-1 : n-1])
	currentVolume := volumes[n-1]
	volumeConfirm := currentVolume >= avgVolume*s.VolumeThreshold

	// Check for breakout
	breakoutUp := closes[n-1] > rangeHigh && currentHigh > rangeHigh
	breakoutDown := closes[n-1] < rangeLow && currentLow < rangeLow

	// Calculate chase distance
	var chaseDistance float64
	if breakoutUp {
		chaseDistance = (currentPrice - rangeHigh) / rangeHigh
	} else if breakoutDown {
		chaseDistance = (rangeLow - currentPrice) / rangeLow
	}

	// Don't chase moves too far extended
	if chaseDistance > s.MaxChasePercent {
		return Signal{
			Action: ActionNone,
			Reason: "price too extended from breakout point - false breakout risk",
		}
	}

	// Check for strong candle (large body)
	candleBody := abs(closes[n-1] - candles[n-1].Open)
	candleRange := currentHigh - currentLow
	strongCandle := candleRange > 0 && candleBody/candleRange > 0.6 // Body is 60%+ of range

	// Bullish breakout
	if breakoutUp && volumeConfirm && strongCandle {
		// Stop at middle of broken range (re-entry = false breakout)
		stopLoss := rangeMiddle
		takeProfit := currentPrice + (currentATR * 3)

		return Signal{
			Action:     ActionBuy,
			Side:       "buy",
			Confidence: s.calculateConfidence(volumeConfirm, strongCandle, chaseDistance),
			Price:      currentPrice,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "high vol breakout UP with volume confirmation",
		}
	}

	// Bearish breakout
	if breakoutDown && volumeConfirm && strongCandle {
		stopLoss := rangeMiddle
		takeProfit := currentPrice - (currentATR * 3)

		return Signal{
			Action:     ActionSell,
			Side:       "sell",
			Confidence: s.calculateConfidence(volumeConfirm, strongCandle, chaseDistance),
			Price:      currentPrice,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "high vol breakout DOWN with volume confirmation",
		}
	}

	return Signal{Action: ActionNone, Reason: "no valid breakout signal"}
}

func (s *HighVolBreakoutStrategy) calculateConfidence(volumeOK, strongCandle bool, chaseDistance float64) float64 {
	confidence := 0.5

	if volumeOK {
		confidence += 0.2
	}
	if strongCandle {
		confidence += 0.15
	}

	// Lower confidence if chasing
	if chaseDistance > 0.01 {
		confidence -= 0.1
	}

	return confidence
}
