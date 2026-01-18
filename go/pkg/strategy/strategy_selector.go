package strategy

import (
	"math"

	"github.com/kasyap/delta-go/go/pkg/features"
)

type StrategySelector struct {
	scalper    *FeeAwareScalper
	fundingArb *FundingArbitrageStrategy
	gridTrader *GridTradingStrategy
}

func NewStrategySelector(scalper *FeeAwareScalper, fundingArb *FundingArbitrageStrategy, gridTrader *GridTradingStrategy) *StrategySelector {
	return &StrategySelector{
		scalper:    scalper,
		fundingArb: fundingArb,
		gridTrader: gridTrader,
	}
}

// SelectBest chooses the best strategy based on objective market data
// Priority order:
// 1. Funding Arbitrage (if |basis| > 15% annualized)
// 2. Grid Trading (if volatility is low < 30% and spread is tight)
// 3. Fee-Aware Scalper (default fallback)
func (s *StrategySelector) SelectBest(f features.MarketFeatures, candles []interface{}) (string, Signal) {
	// 1. High Funding Check (Priority 1)
	if math.Abs(f.BasisAnnualized) > 0.15 {
		sig := s.fundingArb.Analyze(f, candles)
		if sig.Action != ActionNone {
			return "funding_arbitrage", sig
		}
	}

	// 2. Ranging Market Check (Priority 2)
	// Check if grid trader is active or should be activated
	if s.gridTrader.IsEnabled() {
		sig := s.gridTrader.Analyze(f, candles)
		if s.gridTrader.isActive {
			return "grid_trading", sig
		}
	}

	// 3. Default: Fee-Aware Scalper
	sig := s.scalper.AnalyzeWithFeatures(f, nil) // candles not used for basic Scalper flow
	return "fee_aware_scalper", sig
}

func (s *StrategySelector) GetScalper() *FeeAwareScalper {
	return s.scalper
}

func (s *StrategySelector) GetFundingArb() *FundingArbitrageStrategy {
	return s.fundingArb
}

func (s *StrategySelector) GetGridTrader() *GridTradingStrategy {
	return s.gridTrader
}
