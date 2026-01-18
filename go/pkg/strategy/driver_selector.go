package strategy

import (
	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/features"
)

type DriverSelector struct {
	scalper       *FeeAwareScalper
	fundingArb    *FundingArbitrageStrategy
	gridTrader    *GridTradingStrategy
	selector      *StrategySelector
	featureEngine *features.Engine
}

type DriverSelectorConfig struct {
	ScalperConfig ScalperConfig
	FundingConfig FundingArbitrageConfig
	GridConfig    GridConfig
}

func DefaultDriverSelectorConfig() DriverSelectorConfig {
	return DriverSelectorConfig{
		ScalperConfig: DefaultScalperConfig(),
		FundingConfig: DefaultFundingArbitrageConfig(),
		GridConfig:    DefaultGridConfig(),
	}
}

func NewDriverSelector(cfg DriverSelectorConfig) *DriverSelector {
	engine := features.NewEngine()
	scalper := NewFeeAwareScalper(cfg.ScalperConfig, engine)
	fundingArb := NewFundingArbitrageStrategy(cfg.FundingConfig)
	gridTrader := NewGridTradingStrategy(cfg.GridConfig, "")

	return &DriverSelector{
		scalper:       scalper,
		fundingArb:    fundingArb,
		gridTrader:    gridTrader,
		selector:      NewStrategySelector(scalper, fundingArb, gridTrader),
		featureEngine: engine,
	}
}

func (d *DriverSelector) GetFeatureEngine() *features.Engine {
	return d.featureEngine
}

func (d *DriverSelector) SelectStrategy(f features.MarketFeatures, candles []delta.Candle) (SelectedStrategy, Signal) {
	name, signal := d.selector.SelectBest(f, candles)

	return SelectedStrategy{
		Name:           name,
		Driver:         f.DominantDriver,
		DriverStrength: f.DriverStrength,
	}, signal
}

func (d *DriverSelector) GetScalper() *FeeAwareScalper {
	return d.scalper
}

func (d *DriverSelector) GetFundingArb() *FundingArbitrageStrategy {
	return d.fundingArb
}

func (d *DriverSelector) GetGridTrader() *GridTradingStrategy {
	return d.gridTrader
}

// SelectedStrategy represents the chosen strategy for a symbol
type SelectedStrategy struct {
	Name           string
	Driver         features.DriverType
	DriverStrength float64
}
