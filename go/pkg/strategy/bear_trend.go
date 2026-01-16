package strategy

import (
	"github.com/kasyap/delta-go/go/pkg/delta"
)

// BearTrendStrategy implements trend following shorts or capital preservation for bear markets
// Entry (Short): Rally to 20EMA resistance with RSI 45-60, 4H trend bearish
// Alternative: No new longs, reduce existing positions
type BearTrendStrategy struct {
	indicators *TechnicalIndicators

	// Parameters
	FastEMA       int
	SlowEMA       int
	RSILow        float64
	RSIHigh       float64
	ATRMultiplier float64
	SafeMode      bool // If true, only preserve capital, no shorts
}

// NewBearTrendStrategy creates a new bear market strategy
func NewBearTrendStrategy() *BearTrendStrategy {
	return &BearTrendStrategy{
		indicators:    NewIndicators(),
		FastEMA:       20,
		SlowEMA:       50,
		RSILow:        55,  // More overbought (was 50)
		RSIHigh:       70,  // More extended rallies (was 65)
		ATRMultiplier: 3.0, // Wider stops (was 2.5)
		SafeMode:      false,
	}
}

func (s *BearTrendStrategy) Name() string {
	return "bear_trend_following"
}

func (s *BearTrendStrategy) UpdateParams(params map[string]interface{}) {
	if v, ok := params["safe_mode"].(bool); ok {
		s.SafeMode = v
	}
}

func (s *BearTrendStrategy) Analyze(candles []delta.Candle, regime delta.MarketRegime) Signal {
	if len(candles) < s.SlowEMA+10 {
		return Signal{Action: ActionNone, Reason: "insufficient data"}
	}

	// In safe mode, just signal to reduce positions
	if s.SafeMode {
		return Signal{
			Action:     ActionReduceSize,
			Confidence: 0.8,
			Reason:     "bear regime - capital preservation mode",
		}
	}

	// Extract price arrays
	closes := extractCloses(candles)
	highs := extractHighs(candles)
	lows := extractLows(candles)
	volumes := extractVolumes(candles)

	n := len(closes)
	currentPrice := closes[n-1]

	// Calculate indicators
	ema20 := s.indicators.EMA(closes, s.FastEMA)
	ema50 := s.indicators.EMA(closes, s.SlowEMA)
	rsi := s.indicators.RSI(closes, 14)
	atr := s.indicators.ATR(highs, lows, closes, 14)

	currentEMA20 := ema20[n-1]
	currentEMA50 := ema50[n-1]
	currentRSI := rsi[n-1]
	currentATR := atr[n-1]

	// Volume analysis
	avgVolume := average(volumes[n-20:])
	currentVolume := volumes[n-1]
	volumeOK := currentVolume >= avgVolume

	// Check for bearish reversal candle
	currClose := closes[n-1]
	currOpen := candles[n-1].Open
	bearishCandle := currClose < currOpen

	// Check for consecutive bearish candles (require 2+)
	prevClose := closes[n-2]
	prevPrevClose := closes[n-3]
	prevOpen := candles[n-2].Open
	prevBearish := prevClose < prevOpen && prevClose < prevPrevClose
	consecutiveBearish := bearishCandle && prevBearish

	// Entry conditions for Bear Short:
	// 1. Price rallied near 20EMA resistance (within 0.5%)
	// 2. RSI between 45-60 (showing relief rally, not oversold)
	// 3. Price below 50EMA (overall trend is down)
	// 4. Bearish rejection candle

	nearEMA20 := abs(currentPrice-currentEMA20)/currentEMA20 < 0.005 ||
		(currentPrice < currentEMA20 && closes[n-2] > ema20[n-2]) // Just rejected from above

	trendDown := currentPrice < currentEMA50 && currentEMA20 < currentEMA50
	rsiInRange := currentRSI >= s.RSILow && currentRSI <= s.RSIHigh

	// Check for bullish divergence (exit signal for shorts)
	bullishDivergence := s.checkBullishDivergence(closes, rsi, 10)

	if bullishDivergence {
		return Signal{
			Action:     ActionClose,
			Side:       "buy",
			Confidence: 0.7,
			Reason:     "bullish divergence detected - cover shorts",
		}
	}

	// Short entry signal - require consecutive bearish candles
	if trendDown && nearEMA20 && rsiInRange && consecutiveBearish && volumeOK {
		// Find recent rally high for stop
		recentHigh := maxSlice(highs[n-5:]...)
		stopLoss := recentHigh + (0.001 * currentPrice) // Just above rally high

		// Also check ATR-based stop
		atrStop := currentPrice + (s.ATRMultiplier * currentATR)
		if atrStop < stopLoss {
			stopLoss = atrStop
		}

		takeProfit := currentPrice - (4.0 * currentATR) // 4x ATR for better R:R

		return Signal{
			Action:     ActionSell,
			Side:       "sell",
			Confidence: s.calculateConfidence(trendDown, rsiInRange, volumeOK, bearishCandle),
			Price:      currentPrice,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "bear rally to 20EMA resistance with rejection",
		}
	}

	return Signal{Action: ActionNone, Reason: "no valid bear entry signal"}
}

// checkBullishDivergence checks for price making lower low while RSI makes higher low
func (s *BearTrendStrategy) checkBullishDivergence(closes, rsi []float64, lookback int) bool {
	n := len(closes)
	if n < lookback*2 {
		return false
	}

	// Find recent price lows
	priceLow1 := minSlice(closes[n-lookback:]...)
	priceLow2 := minSlice(closes[n-lookback*2 : n-lookback]...)

	// Find corresponding RSI values
	rsiLow1 := minSlice(rsi[n-lookback:]...)
	rsiLow2 := minSlice(rsi[n-lookback*2 : n-lookback]...)

	// Bullish divergence: price lower low, RSI higher low
	return priceLow1 < priceLow2 && rsiLow1 > rsiLow2
}

func (s *BearTrendStrategy) calculateConfidence(trendDown, rsiOK, volumeOK, bearishCandle bool) float64 {
	confidence := 0.5
	if trendDown {
		confidence += 0.15
	}
	if rsiOK {
		confidence += 0.15
	}
	if volumeOK {
		confidence += 0.1
	}
	if bearishCandle {
		confidence += 0.1
	}
	return confidence
}
