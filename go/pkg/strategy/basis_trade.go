package strategy

import (
	"fmt"
	"time"

	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/features"
)

type BasisTradeConfig struct {
	EntryThresholdAnnualized float64 // Annualized funding % to enter (e.g., 0.15 = 15%)
	ExitThresholdAnnualized  float64 // Annualized funding % to exit (e.g., 0.05 = 5%)
	MaxHoldingDays           float64 // Max days to hold (unused for funding carry, but kept for compat)
	MinDaysToExpiry          float64 // Min time to expiry (unused, kept for compat)
	MaxLeverage              int
	PositionSizePct          float64
	AbortIfLegNotFilled      time.Duration
	Enabled                  bool
	IsCarryStrategy          bool // If true, treat as funding carry (single leg); if false, dated futures basis (two leg)
}

func DefaultBasisTradeConfig() BasisTradeConfig {
	return BasisTradeConfig{
		EntryThresholdAnnualized: 0.10, // 10% annualized funding for carry entry (conservative for small drawdown)
		ExitThresholdAnnualized:  0.02, // 2% annualized for exit
		MaxHoldingDays:           30,
		MinDaysToExpiry:          7,
		MaxLeverage:              3,
		PositionSizePct:          5.0,
		AbortIfLegNotFilled:      5 * time.Second,
		Enabled:                  true,
		IsCarryStrategy:          true, // Default to funding carry on Delta India
	}
}

type BasisPosition struct {
	ID             string
	PerpSymbol     string
	PerpSide       string
	PerpSize       int
	EntryBasis     float64
	EntryTime      time.Time
	PerpEntryPrice float64
}

type BasisTradeMonitor struct {
	cfg       BasisTradeConfig
	positions map[string]*BasisPosition
}

func NewBasisTradeMonitor(cfg BasisTradeConfig) *BasisTradeMonitor {
	return &BasisTradeMonitor{
		cfg:       cfg,
		positions: make(map[string]*BasisPosition),
	}
}

func (b *BasisTradeMonitor) Name() string {
	return "basis_trade_monitor"
}

func (b *BasisTradeMonitor) Analyze(candles []delta.Candle, regime delta.MarketRegime) Signal {
	return Signal{Action: ActionNone, Reason: "use AnalyzeWithFeatures"}
}

func (b *BasisTradeMonitor) UpdateParams(params map[string]interface{}) {
	if v, ok := params["entry_threshold"].(float64); ok {
		b.cfg.EntryThresholdAnnualized = v
	}
	if v, ok := params["exit_threshold"].(float64); ok {
		b.cfg.ExitThresholdAnnualized = v
	}
	if v, ok := params["enabled"].(bool); ok {
		b.cfg.Enabled = v
	}
}

type BasisSignal struct {
	Action        string
	PerpSymbol    string
	FuturesSymbol string
	PerpSide      string
	FuturesSide   string
	BasisPct      float64
	Annualized    float64
	Reason        string
	PositionID    string
}

func (b *BasisTradeMonitor) AnalyzeBasis(
	perpFeatures features.MarketFeatures,
) BasisSignal {
	if !b.cfg.Enabled {
		return BasisSignal{Action: "none", Reason: "basis trade disabled"}
	}

	return b.analyzeFundingCarry(perpFeatures)
}

func (b *BasisTradeMonitor) analyzeFundingCarry(perpFeatures features.MarketFeatures) BasisSignal {
	// perpFeatures.BasisAnnualized holds annualized funding rate from features engine
	fundingAnnualized := perpFeatures.BasisAnnualized

	// Check for existing open position
	for _, pos := range b.positions {
		if pos.PerpSymbol == perpFeatures.Symbol {
			// Position exists - check exit condition
			if fundingAnnualized < b.cfg.ExitThresholdAnnualized {
				return BasisSignal{
					Action:        "close",
					PerpSymbol:    pos.PerpSymbol,
					FuturesSymbol: "",
					PerpSide:      oppositeSide(pos.PerpSide),
					FuturesSide:   "",
					BasisPct:      fundingAnnualized,
					Annualized:    fundingAnnualized,
					Reason:        "funding converged below exit threshold",
					PositionID:    pos.ID,
				}
			}

			holdingDays := time.Since(pos.EntryTime).Hours() / 24
			if holdingDays > b.cfg.MaxHoldingDays {
				return BasisSignal{
					Action:        "close",
					PerpSymbol:    pos.PerpSymbol,
					FuturesSymbol: "",
					PerpSide:      oppositeSide(pos.PerpSide),
					FuturesSide:   "",
					BasisPct:      fundingAnnualized,
					Annualized:    fundingAnnualized,
					Reason:        "max holding period reached",
					PositionID:    pos.ID,
				}
			}

			return BasisSignal{Action: "hold", Reason: "holding existing carry position"}
		}
	}

	// No existing position - check entry condition
	if abs(fundingAnnualized) > b.cfg.EntryThresholdAnnualized {
		// Positive funding: shorts earn, so short the perp
		// Negative funding: longs earn, so go long the perp
		perpSide := "sell" // Default: short if funding positive
		if fundingAnnualized < 0 {
			perpSide = "buy" // Long if funding negative
		}

		return BasisSignal{
			Action:        "enter",
			PerpSymbol:    perpFeatures.Symbol,
			FuturesSymbol: "",
			PerpSide:      perpSide,
			FuturesSide:   "",
			BasisPct:      fundingAnnualized,
			Annualized:    fundingAnnualized,
			Reason:        fmt.Sprintf("funding %.2f%% annualized exceeds entry threshold %.2f%%", fundingAnnualized*100, b.cfg.EntryThresholdAnnualized*100),
		}
	}

	return BasisSignal{Action: "none", Reason: "funding below entry threshold"}
}

// analyzeDatedFuturesBasis was removed as Delta India doesn't support dated futures actively for this bot.

func (b *BasisTradeMonitor) RecordEntry(pos *BasisPosition) {
	b.positions[pos.ID] = pos
}

func (b *BasisTradeMonitor) RecordExit(positionID string) {
	delete(b.positions, positionID)
}

func (b *BasisTradeMonitor) GetOpenPositions() []*BasisPosition {
	result := make([]*BasisPosition, 0, len(b.positions))
	for _, p := range b.positions {
		result = append(result, p)
	}
	return result
}

func (b *BasisTradeMonitor) IsEnabled() bool {
	return b.cfg.Enabled
}
