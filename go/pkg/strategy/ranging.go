package strategy

import (
	"sort"

	"github.com/kasyap/delta-go/go/pkg/delta"
)

// RangingStrategy implements mean reversion within defined support/resistance levels
// Entry: At S/R with RSI confirmation (<30 at support, >70 at resistance)
// Exit: At opposite S/R level or middle of range
type RangingStrategy struct {
	indicators *TechnicalIndicators

	// Parameters
	LookbackPeriod   int // Period to find S/R
	RSIOversold      float64
	RSIOverbought    float64
	RangeConfirmBars int // Bars to confirm range
}

// NewRangingStrategy creates a new ranging market strategy
func NewRangingStrategy() *RangingStrategy {
	return &RangingStrategy{
		indicators:       NewIndicators(),
		LookbackPeriod:   50,
		RSIOversold:      25, // Tightened from 30 for stronger bounce signals
		RSIOverbought:    75, // Tightened from 70 for stronger fade signals
		RangeConfirmBars: 10,
	}
}

func (s *RangingStrategy) Name() string {
	return "ranging_mean_reversion"
}

func (s *RangingStrategy) UpdateParams(params map[string]interface{}) {
	if v, ok := params["lookback"].(int); ok {
		s.LookbackPeriod = v
	}
}

func (s *RangingStrategy) Analyze(candles []delta.Candle, regime delta.MarketRegime) Signal {
	if len(candles) < s.LookbackPeriod+10 {
		return Signal{Action: ActionNone, Reason: "insufficient data"}
	}

	// Extract price arrays
	closes := extractCloses(candles)
	highs := extractHighs(candles)
	lows := extractLows(candles)

	n := len(closes)
	currentPrice := closes[n-1]

	// Calculate indicators
	rsi := s.indicators.RSI(closes, 14)
	currentRSI := rsi[n-1]

	// Find support and resistance levels
	support, resistance := s.findSupportResistance(highs, lows, closes)
	rangeSize := resistance - support

	// Validate range exists and is meaningful (minimum 2% range)
	if rangeSize <= 0 || rangeSize/currentPrice < 0.02 {
		return Signal{Action: ActionNone, Reason: "range too small (< 2%)"}
	}

	// Check if price has been ranging (staying within S/R)
	isRanging := s.confirmRange(closes, support, resistance)
	if !isRanging {
		return Signal{Action: ActionNone, Reason: "range not confirmed"}
	}

	// Calculate distances
	distToSupport := (currentPrice - support) / rangeSize
	distToResistance := (resistance - currentPrice) / rangeSize

	// Calculate Bollinger Bands for additional confirmation
	upper, _, lower := s.indicators.BollingerBands(closes, 20, 2.0)
	currentUpper := upper[n-1]
	currentLower := lower[n-1]

	// Entry at support (buy) - require price below lower BB
	if distToSupport < 0.10 && currentRSI < s.RSIOversold && currentPrice <= currentLower {
		stopLoss := support - (rangeSize * 0.1)   // Stop beyond range
		takeProfit := support + (rangeSize * 0.7) // 70% of range target

		return Signal{
			Action:     ActionBuy,
			Side:       "buy",
			Confidence: s.calculateConfidence(distToSupport, currentRSI, true),
			Price:      currentPrice,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "ranging: buy at support with RSI oversold + below lower BB",
		}
	}

	// Entry at resistance (sell) - require price above upper BB
	if distToResistance < 0.10 && currentRSI > s.RSIOverbought && currentPrice >= currentUpper {
		stopLoss := resistance + (rangeSize * 0.1)   // Stop beyond range
		takeProfit := resistance - (rangeSize * 0.7) // 70% of range target

		return Signal{
			Action:     ActionSell,
			Side:       "sell",
			Confidence: s.calculateConfidence(distToResistance, currentRSI, false),
			Price:      currentPrice,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     "ranging: sell at resistance with RSI overbought + above upper BB",
		}
	}

	return Signal{Action: ActionNone, Reason: "price not at range extremes with BB confirmation"}
}

// findSupportResistance finds key S/R levels from recent price action
func (s *RangingStrategy) findSupportResistance(highs, lows, closes []float64) (support, resistance float64) {
	n := len(closes)
	lookback := s.LookbackPeriod
	if n < lookback {
		lookback = n
	}

	// Simple approach: find swing highs and lows
	recentHighs := highs[n-lookback:]
	recentLows := lows[n-lookback:]

	// Find clusters of highs (resistance) and lows (support)
	resistance = maxSlice(recentHighs...)
	support = minSlice(recentLows...)

	// Refine by looking at where price reversed multiple times
	// Count touches near support and resistance
	tolerance := (resistance - support) * 0.05

	supportTouches := 0
	resistanceTouches := 0

	for i := range recentLows {
		if abs(recentLows[i]-support) < tolerance {
			supportTouches++
		}
		if abs(recentHighs[i]-resistance) < tolerance {
			resistanceTouches++
		}
	}

	// Require at least 2 touches to confirm levels
	if supportTouches < 2 || resistanceTouches < 2 {
		// Use percentile-based levels if not enough touches
		sortedCloses := make([]float64, len(closes[n-lookback:]))
		copy(sortedCloses, closes[n-lookback:])

		// Simple percentile
		support = percentile(sortedCloses, 10)
		resistance = percentile(sortedCloses, 90)
	}

	return support, resistance
}

// confirmRange checks if price has been staying within the range
func (s *RangingStrategy) confirmRange(closes []float64, support, resistance float64) bool {
	n := len(closes)
	confirmBars := s.RangeConfirmBars
	if n < confirmBars {
		confirmBars = n
	}

	inRangeCount := 0
	for i := n - confirmBars; i < n; i++ {
		if closes[i] >= support*0.99 && closes[i] <= resistance*1.01 {
			inRangeCount++
		}
	}

	// At least 80% of bars should be in range
	return float64(inRangeCount)/float64(confirmBars) >= 0.8
}

func (s *RangingStrategy) calculateConfidence(distToEdge, rsi float64, isBuy bool) float64 {
	confidence := 0.5

	// Closer to edge = higher confidence
	if distToEdge < 0.1 {
		confidence += 0.2
	} else if distToEdge < 0.15 {
		confidence += 0.1
	}

	// Stronger RSI = higher confidence
	if isBuy && rsi < 25 {
		confidence += 0.15
	} else if !isBuy && rsi > 75 {
		confidence += 0.15
	}

	return confidence
}

// percentile calculates approximate percentile using stdlib sort
func percentile(data []float64, p float64) float64 {
	n := len(data)
	if n == 0 {
		return 0
	}

	sorted := make([]float64, n)
	copy(sorted, data)
	sort.Float64s(sorted)

	idx := int(float64(n) * p / 100)
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
