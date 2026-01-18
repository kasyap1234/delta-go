package strategy

import (
	"fmt"
	"math"

	"github.com/kasyap/delta-go/go/pkg/delta"
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

var debugCounter int

// SelectBest chooses the best strategy based on objective market data
// Priority order:
// 1. Funding Arbitrage (if |basis| > 15% annualized)
// 2. Grid Trading (if volatility is low < 30% and spread is tight)
// 3. Fee-Aware Scalper (default fallback)
func (s *StrategySelector) SelectBest(f features.MarketFeatures, candles []delta.Candle) (string, Signal) {
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
		// Log vol for debugging
		if f.HistoricalVol > 0.0 && debugCounter < 5 {
			fmt.Printf("DEBUG: Vol=%.2f%% Basis=%.2f%%\n", f.HistoricalVol*100, f.BasisAnnualized*100)
			debugCounter++
		}
		sig := s.gridTrader.Analyze(f, candles)
		if s.gridTrader.IsActive {
			return "grid_trading", sig
		}
	}

	// 3. Default: Fee-Aware Scalper
	sig := s.scalper.Analyze(f, candles)
	return "fee_aware_scalper", sig
}

// Analyze implements the Strategy interface
func (s *StrategySelector) Analyze(f features.MarketFeatures, candles []delta.Candle) Signal {
	_, signal := s.SelectBest(f, candles)
	return signal
}

func (s *StrategySelector) Name() string {
	return "strategy_selector"
}

func (s *StrategySelector) UpdateParams(params map[string]interface{}) {
	// Delegate parameter updates to sub-strategies if keys match
	// For now, empty implementation is sufficient for basic interface compliance
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
