package strategy

import (
	"time"

	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/features"
)

type FundingArbitrageConfig struct {
	EntryThresholdAnnualized float64 // 15% annualized
	ExitThresholdAnnualized  float64 // 5% annualized
	MaxHoldingHours          float64 // 24h timeout
	MaxPositionPct           float64 // 33% of portfolio
	Enabled                  bool
}

func DefaultFundingArbitrageConfig() FundingArbitrageConfig {
	return FundingArbitrageConfig{
		EntryThresholdAnnualized: 0.15,
		ExitThresholdAnnualized:  0.05,
		MaxHoldingHours:          24,
		MaxPositionPct:           33.0,
		Enabled:                  true,
	}
}

type FundingPosition struct {
	Symbol    string
	Side      string
	EntryTime time.Time
	EntryRate float64
}

type FundingArbitrageStrategy struct {
	cfg       FundingArbitrageConfig
	positions map[string]*FundingPosition
}

func NewFundingArbitrageStrategy(cfg FundingArbitrageConfig) *FundingArbitrageStrategy {
	return &FundingArbitrageStrategy{
		cfg:       cfg,
		positions: make(map[string]*FundingPosition),
	}
}

func (s *FundingArbitrageStrategy) Name() string {
	return "funding_arbitrage"
}

func (s *FundingArbitrageStrategy) Analyze(f features.MarketFeatures, candles []delta.Candle) Signal {
	if !s.cfg.Enabled {
		return Signal{Action: ActionNone, Reason: "funding arb disabled"}
	}

	fundingAnn := f.BasisAnnualized

	// Check existing position
	if pos, exists := s.positions[f.Symbol]; exists {
		if abs(fundingAnn) < s.cfg.ExitThresholdAnnualized {
			return Signal{
				Action:     ActionClose,
				Side:       oppositeSide(pos.Side),
				Confidence: 0.8,
				Reason:     "funding dropped below exit threshold",
			}
		}
		if time.Since(pos.EntryTime).Hours() > s.cfg.MaxHoldingHours {
			return Signal{
				Action:     ActionClose,
				Side:       oppositeSide(pos.Side),
				Confidence: 0.7,
				Reason:     "max holding time exceeded",
			}
		}
		return Signal{Action: ActionNone, Reason: "holding funding position"}
	}

	// Entry conditions
	if abs(fundingAnn) > s.cfg.EntryThresholdAnnualized {
		side := "sell" // Positive funding -> short to earn
		action := ActionSell
		if fundingAnn < 0 {
			side = "buy" // Negative funding -> long to earn
			action = ActionBuy
		}
		return Signal{
			Action:     action,
			Side:       side,
			Confidence: 0.65,
			Reason:     "high funding rate opportunity",
		}
	}

	return Signal{Action: ActionNone, Reason: "funding below threshold"}
}

func (s *FundingArbitrageStrategy) UpdateParams(params map[string]interface{}) {
	if v, ok := params["entry_threshold"].(float64); ok {
		s.cfg.EntryThresholdAnnualized = v
	}
	if v, ok := params["exit_threshold"].(float64); ok {
		s.cfg.ExitThresholdAnnualized = v
	}
	if v, ok := params["enabled"].(bool); ok {
		s.cfg.Enabled = v
	}
}

func (s *FundingArbitrageStrategy) RecordEntry(symbol, side string, rate float64) {
	s.positions[symbol] = &FundingPosition{
		Symbol:    symbol,
		Side:      side,
		EntryTime: time.Now(),
		EntryRate: rate,
	}
}

func (s *FundingArbitrageStrategy) RecordExit(symbol string) {
	delete(s.positions, symbol)
}

func (s *FundingArbitrageStrategy) IsEnabled() bool {
	return s.cfg.Enabled
}

func (s *FundingArbitrageStrategy) AnalyzeWithLegs(f features.MarketFeatures, candles []delta.Candle) Signal {
	return s.Analyze(f, candles)
}
