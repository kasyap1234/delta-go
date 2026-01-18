package features

import (
	"math"
	"sync"
	"time"

	"github.com/kasyap/delta-go/go/pkg/delta"
)

type DriverType string

const (
	DriverNone           DriverType = "none"
	DriverHighIV         DriverType = "high_iv"
	DriverHighBasis      DriverType = "high_basis"
	DriverOrderImbalance DriverType = "order_imbalance"
)

type MarketFeatures struct {
	Symbol    string
	Timestamp time.Time

	SpotPrice  float64
	MarkPrice  float64
	IndexPrice float64

	BestBid     float64
	BestAsk     float64
	Spread      float64
	SpreadBps   float64
	BidDepth    float64
	AskDepth    float64
	Imbalance   float64
	ImbalanceMA float64

	HistoricalVol float64
	ImpliedVol    float64
	IVPremium     float64

	BasisAbs        float64
	BasisPct        float64
	BasisAnnualized float64
	FuturesExpiry   time.Time
	DaysToExpiry    float64

	DominantDriver DriverType
	DriverStrength float64

	HMMRegime     delta.MarketRegime
	HMMConfidence float64
}

type OBISnapshot struct {
	Timestamp time.Time
	Imbalance float64
	MidPrice  float64
}

type Engine struct {
	mu               sync.RWMutex
	obi              []OBISnapshot
	maxOBISnapshots  int
	imbalancePeriod  int
	imbalanceHistory []float64
}

func NewEngine() *Engine {
	return &Engine{
		maxOBISnapshots: 60,
		imbalancePeriod: 10,
	}
}

func (e *Engine) ComputeFeaturesWithFunding(
	orderbook *delta.Orderbook,
	ticker *delta.Ticker,
	candles []delta.Candle,
) MarketFeatures {
	f := e.ComputeFeatures(orderbook, ticker, candles, time.Time{}, 0)

	// If ticker has funding rate, convert to annualized basis
	if ticker != nil && ticker.FundingRate != 0 {
		// ticker.FundingRate is 8-hourly rate
		// Annualized = funding_rate * 3 times per day * 365 days
		annualizedFunding := ticker.FundingRate * 3 * 365

		f.BasisAnnualized = annualizedFunding
		f.BasisPct = ticker.FundingRate
		f.BasisAbs = ticker.FundingRate

		f.DominantDriver, f.DriverStrength = e.detectDominantDriver(f)
	}

	return f
}

func (e *Engine) ComputeFeaturesWithFundingRate(
	orderbook *delta.Orderbook,
	ticker *delta.Ticker,
	candles []delta.Candle,
	fundingRate float64, // 8-hourly funding rate (e.g., 0.001 = 0.1%)
) MarketFeatures {
	f := e.ComputeFeatures(orderbook, ticker, candles, time.Time{}, 0)

	// Convert 8-hourly funding rate to annualized
	annualizedFunding := fundingRate * 3 * 365

	f.BasisAnnualized = annualizedFunding
	f.BasisPct = fundingRate
	f.BasisAbs = fundingRate

	f.DominantDriver, f.DriverStrength = e.detectDominantDriver(f)
	return f
}

func (e *Engine) ComputeFeatures(
	orderbook *delta.Orderbook,
	ticker *delta.Ticker,
	candles []delta.Candle,
	futuresExpiry time.Time,
	perpMid float64,
) MarketFeatures {
	f := MarketFeatures{
		Timestamp: time.Now(),
	}

	if ticker != nil {
		f.Symbol = ticker.Symbol
		f.SpotPrice = ticker.Close
		f.MarkPrice = ticker.MarkPrice
	}

	if orderbook != nil && len(orderbook.Buy) > 0 && len(orderbook.Sell) > 0 {
		f.BestBid = parseFloat(orderbook.Buy[0].Price)
		f.BestAsk = parseFloat(orderbook.Sell[0].Price)
		f.Spread = f.BestAsk - f.BestBid
		mid := (f.BestBid + f.BestAsk) / 2
		if mid > 0 {
			f.SpreadBps = (f.Spread / mid) * 10000
		}

		bidDepth, askDepth := e.computeDepth(orderbook, 10)
		f.BidDepth = bidDepth
		f.AskDepth = askDepth
		if bidDepth+askDepth > 0 {
			f.Imbalance = (bidDepth - askDepth) / (bidDepth + askDepth)
		}

		e.mu.Lock()
		e.obi = append(e.obi, OBISnapshot{
			Timestamp: time.Now(),
			Imbalance: f.Imbalance,
			MidPrice:  mid,
		})
		if len(e.obi) > e.maxOBISnapshots {
			e.obi = e.obi[len(e.obi)-e.maxOBISnapshots:]
		}
		f.ImbalanceMA = e.computeImbalanceMA()
		e.mu.Unlock()
	}

	if len(candles) >= 20 {
		f.HistoricalVol = e.computeHistoricalVol(candles, 20)
	}
	return f
}

func (e *Engine) AddOBISnapshot(s OBISnapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.obi = append(e.obi, s)
	if len(e.obi) > e.maxOBISnapshots {
		e.obi = e.obi[len(e.obi)-e.maxOBISnapshots:]
	}
}

func (e *Engine) computeDepth(ob *delta.Orderbook, levels int) (bidDepth, askDepth float64) {
	for i := 0; i < levels && i < len(ob.Buy); i++ {
		bidDepth += float64(ob.Buy[i].Size) * parseFloat(ob.Buy[i].Price)
	}
	for i := 0; i < levels && i < len(ob.Sell); i++ {
		askDepth += float64(ob.Sell[i].Size) * parseFloat(ob.Sell[i].Price)
	}
	return
}

func (e *Engine) computeImbalanceMA() float64 {
	if len(e.obi) == 0 {
		return 0
	}
	period := e.imbalancePeriod
	if period > len(e.obi) {
		period = len(e.obi)
	}
	sum := 0.0
	for i := len(e.obi) - period; i < len(e.obi); i++ {
		sum += e.obi[i].Imbalance
	}
	return sum / float64(period)
}

func (e *Engine) computeHistoricalVol(candles []delta.Candle, period int) float64 {
	if len(candles) < period+1 {
		return 0
	}

	returns := make([]float64, period)
	for i := 0; i < period; i++ {
		idx := len(candles) - period + i
		if candles[idx-1].Close > 0 {
			returns[i] = math.Log(candles[idx].Close / candles[idx-1].Close)
		}
	}

	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(period)

	variance := 0.0
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(period)

	dailyVol := math.Sqrt(variance) * math.Sqrt(float64(periodsPerDay(candles)))
	return dailyVol * math.Sqrt(365)
}

func periodsPerDay(candles []delta.Candle) int {
	if len(candles) < 2 {
		return 288
	}
	interval := candles[1].Time - candles[0].Time
	if interval <= 0 {
		return 288
	}
	return int(86400 / interval)
}

func (e *Engine) detectDominantDriver(f MarketFeatures) (DriverType, float64) {
	const (
		basisThreshold     = 0.15
		ivPremiumThreshold = 0.10
		imbalanceThreshold = 0.6
		persistenceReq     = 5
	)

	if f.BasisAnnualized > basisThreshold {
		strength := math.Min(f.BasisAnnualized/basisThreshold, 2.0) - 1.0
		return DriverHighBasis, strength
	}

	if f.IVPremium > ivPremiumThreshold {
		strength := math.Min(f.IVPremium/ivPremiumThreshold, 2.0) - 1.0
		return DriverHighIV, strength
	}

	e.mu.RLock()
	persistent := e.isImbalancePersistent(imbalanceThreshold, persistenceReq)
	e.mu.RUnlock()

	if persistent {
		strength := math.Abs(f.ImbalanceMA) / imbalanceThreshold
		return DriverOrderImbalance, math.Min(strength, 1.0)
	}

	return DriverNone, 0
}

func (e *Engine) isImbalancePersistent(threshold float64, required int) bool {
	if len(e.obi) < required {
		return false
	}
	positiveCount := 0
	negativeCount := 0
	for i := len(e.obi) - required; i < len(e.obi); i++ {
		if e.obi[i].Imbalance > threshold {
			positiveCount++
		} else if e.obi[i].Imbalance < -threshold {
			negativeCount++
		}
	}
	return positiveCount >= required || negativeCount >= required
}

func (e *Engine) GetImbalanceDirection() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.obi) == 0 {
		return "neutral"
	}
	avg := e.computeImbalanceMA()
	if avg > 0.3 {
		return "bullish"
	}
	if avg < -0.3 {
		return "bearish"
	}
	return "neutral"
}

func (e *Engine) GetOBISnapshots() []OBISnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]OBISnapshot, len(e.obi))
	copy(result, e.obi)
	return result
}

func parseFloat(s string) float64 {
	var f float64
	parseFloatInto(s, &f)
	return f
}

func parseFloatInto(s string, out *float64) bool {
	if s == "" {
		return false
	}
	n := 0.0
	decimal := false
	decimalPlaces := 1.0
	negative := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' && i == 0 {
			negative = true
			continue
		}
		if c == '.' {
			decimal = true
			continue
		}
		if c >= '0' && c <= '9' {
			digit := float64(c - '0')
			if decimal {
				decimalPlaces *= 10
				n += digit / decimalPlaces
			} else {
				n = n*10 + digit
			}
		}
	}
	if negative {
		n = -n
	}
	*out = n
	return true
}
