package strategy

import (
	"math"
	"sync"

	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/features"
)

// Signal represents a trading signal
type Signal struct {
	Action     SignalAction
	Side       string  // "buy" or "sell"
	Confidence float64 // 0-1
	Price      float64
	StopLoss   float64
	TakeProfit float64
	Reason     string
	IsHedged   bool
}

// Strategy interface for backtest compatibility
type Strategy interface {
	Name() string
	Analyze(f features.MarketFeatures, candles []delta.Candle) Signal
	UpdateParams(params map[string]interface{})
}

// Manager manages multiple strategies for backtest compatibility
type Manager struct {
	mu               sync.RWMutex
	strategies       map[string]Strategy
	regimeStrategies map[delta.MarketRegime]string
}

// NewManager creates a new strategy manager
func NewManager() *Manager {
	return &Manager{
		strategies:       make(map[string]Strategy),
		regimeStrategies: make(map[delta.MarketRegime]string),
	}
}

// RegisterStrategy registers a strategy with the manager
func (m *Manager) RegisterStrategy(s Strategy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.strategies[s.Name()] = s
}

// SetRegimeStrategy sets which strategy to use for a given regime
func (m *Manager) SetRegimeStrategy(regime delta.MarketRegime, strategyName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.regimeStrategies[regime] = strategyName
}

// GetSignal gets a trading signal for the given regime (thread-safe)
func (m *Manager) GetSignal(f features.MarketFeatures, candles []delta.Candle) Signal {
	m.mu.RLock()
	// Attempt to get strategy for current regime
	// MarketFeatures likely contains regime info, but for legacy Manager support
	// we might need to rely on what's passed or default.
	// Let's assume f.HMMRegime is the key if available, otherwise default.
	strategyName, ok := m.regimeStrategies[f.HMMRegime]
	if !ok {
		// Fallback: just pick the first one
		for name := range m.strategies {
			strategyName = name
			break
		}
	}
	strategy, exists := m.strategies[strategyName]
	m.mu.RUnlock()

	if !exists {
		return Signal{Action: ActionNone, Reason: "no strategy available"}
	}

	return strategy.Analyze(f, candles)
}

// SignalAction represents what action to take
type SignalAction string

const (
	ActionNone       SignalAction = "none"
	ActionBuy        SignalAction = "buy"
	ActionSell       SignalAction = "sell"
	ActionClose      SignalAction = "close"
	ActionReduceSize SignalAction = "reduce"
)

// TechnicalIndicators provides common technical analysis functions
type TechnicalIndicators struct{}

// NewIndicators creates a new indicators helper
func NewIndicators() *TechnicalIndicators {
	return &TechnicalIndicators{}
}

// EMA calculates Exponential Moving Average
func (ti *TechnicalIndicators) EMA(closes []float64, period int) []float64 {
	if len(closes) < period {
		return make([]float64, len(closes))
	}

	ema := make([]float64, len(closes))
	multiplier := 2.0 / float64(period+1)

	// Start with SMA for first value
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += closes[i]
	}
	ema[period-1] = sum / float64(period)

	// Calculate EMA for rest
	for i := period; i < len(closes); i++ {
		ema[i] = (closes[i]-ema[i-1])*multiplier + ema[i-1]
	}

	return ema
}

// SMA calculates Simple Moving Average
func (ti *TechnicalIndicators) SMA(closes []float64, period int) []float64 {
	if len(closes) < period {
		return make([]float64, len(closes))
	}

	sma := make([]float64, len(closes))

	for i := period - 1; i < len(closes); i++ {
		sum := 0.0
		for j := 0; j < period; j++ {
			sum += closes[i-j]
		}
		sma[i] = sum / float64(period)
	}

	return sma
}

// RSI calculates Relative Strength Index
func (ti *TechnicalIndicators) RSI(closes []float64, period int) []float64 {
	if len(closes) < period+1 {
		return make([]float64, len(closes))
	}

	rsi := make([]float64, len(closes))
	gains := make([]float64, len(closes))
	losses := make([]float64, len(closes))

	// Calculate gains and losses
	for i := 1; i < len(closes); i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			gains[i] = diff
		} else {
			losses[i] = -diff
		}
	}

	// Calculate initial averages
	avgGain := 0.0
	avgLoss := 0.0
	for i := 1; i <= period; i++ {
		avgGain += gains[i]
		avgLoss += losses[i]
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	// Calculate RSI
	for i := period; i < len(closes); i++ {
		if i > period {
			avgGain = (avgGain*float64(period-1) + gains[i]) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + losses[i]) / float64(period)
		}

		if avgLoss == 0 {
			rsi[i] = 100
		} else {
			rs := avgGain / avgLoss
			rsi[i] = 100 - (100 / (1 + rs))
		}
	}

	return rsi
}

// BollingerBands calculates Bollinger Bands
func (ti *TechnicalIndicators) BollingerBands(closes []float64, period int, stdDev float64) (upper, middle, lower []float64) {
	n := len(closes)
	upper = make([]float64, n)
	middle = make([]float64, n)
	lower = make([]float64, n)

	if n < period {
		return
	}

	for i := period - 1; i < n; i++ {
		// Calculate SMA
		sum := 0.0
		for j := 0; j < period; j++ {
			sum += closes[i-j]
		}
		sma := sum / float64(period)

		// Calculate standard deviation
		sqSum := 0.0
		for j := 0; j < period; j++ {
			diff := closes[i-j] - sma
			sqSum += diff * diff
		}
		std := math.Sqrt(sqSum / float64(period))

		middle[i] = sma
		upper[i] = sma + (std * stdDev)
		lower[i] = sma - (std * stdDev)
	}

	return
}

// ATR calculates Average True Range
func (ti *TechnicalIndicators) ATR(highs, lows, closes []float64, period int) []float64 {
	if len(closes) < 2 {
		return make([]float64, len(closes))
	}

	n := len(closes)
	atr := make([]float64, n)
	tr := make([]float64, n)

	// Calculate True Range
	tr[0] = highs[0] - lows[0]
	for i := 1; i < n; i++ {
		hl := highs[i] - lows[i]
		hc := abs(highs[i] - closes[i-1])
		lc := abs(lows[i] - closes[i-1])
		tr[i] = max(hl, max(hc, lc))
	}

	// Calculate ATR
	if n >= period {
		sum := 0.0
		for i := 0; i < period; i++ {
			sum += tr[i]
		}
		atr[period-1] = sum / float64(period)

		for i := period; i < n; i++ {
			atr[i] = (atr[i-1]*float64(period-1) + tr[i]) / float64(period)
		}
	}

	return atr
}

// RSILast calculates only the final RSI value (optimized, no slice allocation)
func (ti *TechnicalIndicators) RSILast(closes []float64, period int) float64 {
	n := len(closes)
	if n < period+1 {
		return 50
	}

	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			avgGain += diff
		} else {
			avgLoss -= diff
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	for i := period + 1; i < n; i++ {
		diff := closes[i] - closes[i-1]
		g, l := 0.0, 0.0
		if diff > 0 {
			g = diff
		} else {
			l = -diff
		}
		avgGain = (avgGain*float64(period-1) + g) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + l) / float64(period)
	}

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

// EMALast calculates only the final EMA value (optimized)
func (ti *TechnicalIndicators) EMALast(closes []float64, period int) float64 {
	n := len(closes)
	if n < period {
		return 0
	}

	multiplier := 2.0 / float64(period+1)

	sum := 0.0
	for i := 0; i < period; i++ {
		sum += closes[i]
	}
	ema := sum / float64(period)

	for i := period; i < n; i++ {
		ema = (closes[i]-ema)*multiplier + ema
	}
	return ema
}

// ATRLast calculates only the final ATR value (optimized)
func (ti *TechnicalIndicators) ATRLast(highs, lows, closes []float64, period int) float64 {
	n := len(closes)
	if n < period || n < 2 {
		return 0
	}

	tr := make([]float64, n)
	tr[0] = highs[0] - lows[0]
	for i := 1; i < n; i++ {
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		tr[i] = math.Max(hl, math.Max(hc, lc))
	}

	sum := 0.0
	for i := 0; i < period; i++ {
		sum += tr[i]
	}
	atr := sum / float64(period)

	for i := period; i < n; i++ {
		atr = (atr*float64(period-1) + tr[i]) / float64(period)
	}
	return atr
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func oppositeSide(side string) string {
	if side == "buy" {
		return "sell"
	}
	return "buy"
}

func max(a, b float64) float64 {
	return math.Max(a, b)
}
