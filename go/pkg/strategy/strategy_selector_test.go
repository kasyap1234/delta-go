package strategy

import (
	"testing"

	"github.com/kasyap/delta-go/go/pkg/features"
)

func TestStrategySelector_SelectBest(t *testing.T) {
	scalper := NewFeeAwareScalper(DefaultScalperConfig(), nil)
	fundingArb := NewFundingArbitrageStrategy(DefaultFundingArbitrageConfig())
	gridTrader := NewGridTradingStrategy(DefaultGridConfig(), "")
	selector := NewStrategySelector(scalper, fundingArb, gridTrader)

	tests := []struct {
		name     string
		features features.MarketFeatures
		wantName string
	}{
		{
			name: "Funding Arbitrage Priority",
			features: features.MarketFeatures{
				Symbol:          "BTCUSD",
				BasisAnnualized: 0.20, // > 15%
			},
			wantName: "funding_arbitrage",
		},
		{
			name: "Grid Trading Priority (Low Vol)",
			features: features.MarketFeatures{
				Symbol:          "BTCUSD",
				BasisAnnualized: 0.02,
				HistoricalVol:   0.20, // 20% < 30% threshold
			},
			wantName: "grid_trading",
		},
		{
			name: "Default to Scalper",
			features: features.MarketFeatures{
				Symbol:          "BTCUSD",
				BasisAnnualized: 0.02,
				HistoricalVol:   0.60, // 60% > 50% threshold
			},
			wantName: "fee_aware_scalper",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, _ := selector.SelectBest(tt.features, nil)
			if gotName != tt.wantName {
				t.Errorf("SelectBest() gotName = %v, want %v", gotName, tt.wantName)
			}
		})
	}
}
