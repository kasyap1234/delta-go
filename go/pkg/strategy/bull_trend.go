package strategy

import (
	"github.com/kasyap/delta-go/go/pkg/delta"
)

// BullTrendStrategy implements trend following for bull markets
// Entry: Pullback to 20EMA with RSI 40-55, 4H trend alignment, volume confirmation
type BullTrendStrategy struct {
	indicators *TechnicalIndicators

	// Parameters
	FastEMA       int
	SlowEMA       int
	RSILow        float64
	RSIHigh       float64
	ATRMultiplier float64
	VolumeRatio   float64
}

// NewBullTrendStrategy creates a new bull market strategy
func NewBullTrendStrategy() *BullTrendStrategy {
	return &BullTrendStrategy{
		indicators:    NewIndicators(),
		FastEMA:       20,
		SlowEMA:       50,
		RSILow:        30,  // Deeper pullback (was 35)
		RSIHigh:       45,  // Avoid late entries (was 50)
		ATRMultiplier: 3.0, // Wider stops (was 2.5)
		VolumeRatio:   1.2,
	}
}

func (s *BullTrendStrategy) Name() string {
	return "bull_trend_following"
}

func (s *BullTrendStrategy) UpdateParams(params map[string]interface{}) {
	if v, ok := params["fast_ema"].(int); ok {
		s.FastEMA = v
	}
	if v, ok := params["slow_ema"].(int); ok {
		s.SlowEMA = v
	}
}

func (s *BullTrendStrategy) Analyze(candles []delta.Candle, regime delta.MarketRegime) Signal {
	if len(candles) < s.SlowEMA+10 {
		return Signal{Action: ActionNone, Reason: "insufficient data"}
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
	volumeOK := currentVolume >= avgVolume*s.VolumeRatio

	// Check for bullish reversal candle
	prevClose := closes[n-2]
	currClose := closes[n-1]
	currOpen := candles[n-1].Open
	bullishCandle := currClose > currOpen && currClose > prevClose

	// Check for consecutive bullish candles (require 2+)
	prevPrevClose := closes[n-3]
	prevOpen := candles[n-2].Open
	prevBullish := prevClose > prevOpen && prevClose > prevPrevClose
	consecutiveBullish := bullishCandle && prevBullish

	// Entry conditions for Bull Trend Following:
	// 1. Price pulled back near 20EMA (within 0.5% of EMA)
	// 2. RSI between 40-55 (not overbought, showing pullback)
	// 3. Price above 50EMA (overall trend is up)
	// 4. Bullish reversal candle with decent volume

	nearEMA20 := abs(currentPrice-currentEMA20)/currentEMA20 < 0.005 ||
		(currentPrice > currentEMA20 && prevClose < ema20[n-2]) // Price just crossed above

	trendUp := currentPrice > currentEMA50 && currentEMA20 > currentEMA50
	rsiInRange := currentRSI >= s.RSILow && currentRSI <= s.RSIHigh

	// Check for bearish divergence (exit signal)
	bearishDivergence := s.checkBearishDivergence(closes, rsi, 10)

	if bearishDivergence {
		return Signal{
			Action:     ActionClose,
			Side:       "sell",
			Confidence: 0.7,
			Reason:     "bearish divergence detected",
		}
	}

	// Entry signal - require consecutive bullish candles
	if trendUp && nearEMA20 && rsiInRange && consecutiveBullish && volumeOK {
		stopLoss := currentPrice - (s.ATRMultiplier * currentATR)
		// Also consider below pullback low
		recentLow := minSlice(lows[n-5:]...)
		if recentLow > stopLoss {
			stopLoss = recentLow - (0.001 * currentPrice) // Just below recent low
		}

		takeProfit := currentPrice + (4.0 * currentATR) // 4x ATR for better R:R

		return Signal{
			Action:     ActionBuy,
			Side:       "buy",
			Confidence: s.calculateConfidence(trendUp, rsiInRange, volumeOK, bullishCandle),
			Price:      currentPrice,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "bull pullback to 20EMA with RSI confirming",
		}
	}

	return Signal{Action: ActionNone, Reason: "no valid bull entry signal"}
}

// checkBearishDivergence checks for price making higher high while RSI makes lower high
func (s *BullTrendStrategy) checkBearishDivergence(closes, rsi []float64, lookback int) bool {
	n := len(closes)
	if n < lookback*2 {
		return false
	}

	// Find recent price highs
	priceHigh1 := maxSlice(closes[n-lookback:]...)
	priceHigh2 := maxSlice(closes[n-lookback*2 : n-lookback]...)

	// Find corresponding RSI values
	rsiHigh1 := maxSlice(rsi[n-lookback:]...)
	rsiHigh2 := maxSlice(rsi[n-lookback*2 : n-lookback]...)

	// Bearish divergence: price higher high, RSI lower high
	return priceHigh1 > priceHigh2 && rsiHigh1 < rsiHigh2
}

func (s *BullTrendStrategy) calculateConfidence(trendUp, rsiOK, volumeOK, bullishCandle bool) float64 {
	confidence := 0.5
	if trendUp {
		confidence += 0.15
	}
	if rsiOK {
		confidence += 0.15
	}
	if volumeOK {
		confidence += 0.1
	}
	if bullishCandle {
		confidence += 0.1
	}
	return confidence
}
