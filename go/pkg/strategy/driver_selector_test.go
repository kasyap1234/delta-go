package strategy

import (
	"testing"
)

func TestDriverSelector_Initialization(t *testing.T) {
	cfg := DriverSelectorConfig{
		ScalperConfig: DefaultScalperConfig(),
		FundingConfig: DefaultFundingArbitrageConfig(),
		GridConfig:    DefaultGridConfig(),
	}

	ds := NewDriverSelector(cfg)

	if ds.GetScalper() == nil {
		t.Error("Scalper not initialized")
	}
	if ds.GetFundingArb() == nil {
		t.Error("FundingArb not initialized")
	}
	if ds.GetGridTrader() == nil {
		t.Error("GridTrader not initialized")
	}
	if ds.GetFeatureEngine() == nil {
		t.Error("FeatureEngine not initialized")
	}
}

func TestDriverSelector_SelectStrategy(t *testing.T) {
	cfg := DriverSelectorConfig{
		ScalperConfig: DefaultScalperConfig(),
		FundingConfig: DefaultFundingArbitrageConfig(),
		GridConfig:    DefaultGridConfig(),
	}
	ds := NewDriverSelector(cfg)
	_ = ds // Verified ds can be created
}
