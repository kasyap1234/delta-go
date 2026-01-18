package backtest

import (
	"math"

	"github.com/kasyap/delta-go/go/pkg/delta"
)

// SlippageModel defines how slippage is calculated
type SlippageModel interface {
	// Calculate returns slippage in price terms (always positive)
	Calculate(side string, size float64, candle delta.Candle, volatility float64) float64
}

// ---------------------- Fixed Slippage ----------------------

// FixedSlippage applies a constant slippage in basis points
type FixedSlippage struct {
	Bps float64 // Basis points (1 bps = 0.01%)
}

// NewFixedSlippage creates a fixed slippage model
func NewFixedSlippage(bps float64) *FixedSlippage {
	return &FixedSlippage{Bps: bps}
}

func (s *FixedSlippage) Calculate(side string, size float64, candle delta.Candle, volatility float64) float64 {
	mid := (candle.High + candle.Low) / 2
	return mid * (s.Bps / 10000)
}

// ---------------------- Volatility Slippage ----------------------

// VolatilitySlippage adjusts slippage based on market volatility
// Higher volatility = larger slippage (more realistic for volatile markets)
type VolatilitySlippage struct {
	BaseBps   float64 // Base slippage in bps (e.g., 1.5)
	VolFactor float64 // Multiplier for volatility component (e.g., 0.5)
}

// NewVolatilitySlippage creates a volatility-adjusted slippage model
func NewVolatilitySlippage(baseBps, volFactor float64) *VolatilitySlippage {
	return &VolatilitySlippage{
		BaseBps:   baseBps,
		VolFactor: volFactor,
	}
}

func (s *VolatilitySlippage) Calculate(side string, size float64, candle delta.Candle, volatility float64) float64 {
	mid := (candle.High + candle.Low) / 2

	// Intrabar volatility as proxy (high-low range as % of mid)
	intrabarVol := (candle.High - candle.Low) / mid * 100 // As percentage

	// Combine base slippage with volatility component
	// Cap volatility contribution at 10x base
	volContribution := math.Min(intrabarVol*s.VolFactor, s.BaseBps*10)
	totalBps := s.BaseBps + volContribution

	return mid * (totalBps / 10000)
}

// ---------------------- Volume Impact Slippage ----------------------

// VolumeImpactSlippage models price impact from order size
// Larger orders relative to volume = more slippage
type VolumeImpactSlippage struct {
	BaseBps     float64 // Minimum slippage
	ImpactCoeff float64 // sqrt(size/volume) coefficient
}

// NewVolumeImpactSlippage creates a volume-impact slippage model
func NewVolumeImpactSlippage(baseBps, impactCoeff float64) *VolumeImpactSlippage {
	return &VolumeImpactSlippage{
		BaseBps:     baseBps,
		ImpactCoeff: impactCoeff,
	}
}

func (s *VolumeImpactSlippage) Calculate(side string, size float64, candle delta.Candle, volatility float64) float64 {
	mid := (candle.High + candle.Low) / 2

	// Base slippage
	baseSlip := mid * (s.BaseBps / 10000)

	// Volume impact using square-root model (industry standard)
	// Reference: Almgren & Chriss market impact model
	if candle.Volume > 0 {
		participation := size / candle.Volume
		impact := s.ImpactCoeff * math.Sqrt(participation) * mid
		return baseSlip + impact
	}

	return baseSlip
}

// ---------------------- Composite Slippage ----------------------

// CompositeSlippage combines multiple slippage models
type CompositeSlippage struct {
	Models []SlippageModel
}

// NewCompositeSlippage creates a combined slippage model
func NewCompositeSlippage(models ...SlippageModel) *CompositeSlippage {
	return &CompositeSlippage{Models: models}
}

func (s *CompositeSlippage) Calculate(side string, size float64, candle delta.Candle, volatility float64) float64 {
	total := 0.0
	for _, model := range s.Models {
		total += model.Calculate(side, size, candle, volatility)
	}
	return total
}

// ---------------------- Helper Functions ----------------------

// ApplySlippage adjusts price for slippage based on order side
func ApplySlippage(price, slippage float64, side string) float64 {
	if side == "buy" {
		return price + slippage // Buys fill higher
	}
	return price - slippage // Sells fill lower
}

// CalculateFee computes trading fee
// Note: size is the NOTIONAL VALUE in dollars, not contract count
func CalculateFee(price float64, size float64, contractValue, feeBps float64) float64 {
	// Size is already notional value in dollars
	return size * (feeBps / 10000)
}
