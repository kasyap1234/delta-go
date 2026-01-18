package strategy

import (
	"time"

	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/features"
)

type ScalperConfig struct {
	ImbalanceThreshold   float64
	PersistenceSnapshots int
	MinSpreadBps         float64
	MaxSpreadBps         float64
	TargetProfitBps      float64
	MaxLossBps           float64
	ScalpWindowBTC       time.Duration
	ScalpWindowOther     time.Duration
	ConfirmationPricePct float64
	Enabled              bool
}

func DefaultScalperConfig() ScalperConfig {
	return ScalperConfig{
		ImbalanceThreshold:   0.5,
		PersistenceSnapshots: 2,
		MinSpreadBps:         1.0,
		MaxSpreadBps:         10.0,
		TargetProfitBps:      20.0,
		MaxLossBps:           15.0,
		ScalpWindowBTC:       30 * time.Minute,
		ScalpWindowOther:     15 * time.Minute,
		ConfirmationPricePct: 0.02,
		Enabled:              true,
	}
}

type FeeAwareScalper struct {
	cfg        ScalperConfig
	ti         *TechnicalIndicators
	engine     *features.Engine
	entryTimes map[string]time.Time
}

func NewFeeAwareScalper(cfg ScalperConfig, engine *features.Engine) *FeeAwareScalper {
	return &FeeAwareScalper{
		cfg:        cfg,
		ti:         NewIndicators(),
		engine:     engine,
		entryTimes: make(map[string]time.Time),
	}
}

func (s *FeeAwareScalper) Name() string {
	return "fee_aware_scalper"
}

func (s *FeeAwareScalper) UpdateParams(params map[string]interface{}) {
	if v, ok := params["imbalance_threshold"].(float64); ok {
		s.cfg.ImbalanceThreshold = v
	}
	if v, ok := params["persistence_snapshots"].(int); ok {
		s.cfg.PersistenceSnapshots = v
	}
	if v, ok := params["enabled"].(bool); ok {
		s.cfg.Enabled = v
	}
}

func (s *FeeAwareScalper) Analyze(f features.MarketFeatures, candles []delta.Candle) Signal {
	if !s.cfg.Enabled {
		return Signal{Action: ActionNone, Reason: "scalper disabled"}
	}

	if f.HistoricalVol < 0.10 {
		return Signal{Action: ActionNone, Reason: "volatility too low for scalping"}
	}

	if f.SpreadBps < s.cfg.MinSpreadBps {
		return Signal{Action: ActionNone, Reason: "spread too tight"}
	}
	if f.SpreadBps > s.cfg.MaxSpreadBps {
		return Signal{Action: ActionNone, Reason: "spread too wide"}
	}

	snapshots := s.engine.GetOBISnapshots()
	if len(snapshots) < s.cfg.PersistenceSnapshots {
		return Signal{Action: ActionNone, Reason: "insufficient OBI history"}
	}

	persistent, direction := s.checkPersistence(snapshots)
	if !persistent {
		return Signal{Action: ActionNone, Reason: "imbalance not persistent"}
	}

	confirmed := s.checkPriceConfirmation(snapshots, direction)
	if !confirmed {
		return Signal{Action: ActionNone, Reason: "no price confirmation"}
	}

	mid := (f.BestBid + f.BestAsk) / 2
	signal := Signal{
		Confidence: 0.7,
		Price:      mid,
	}

	slippageBps := f.SpreadBps / 2
	effectiveTarget := s.cfg.TargetProfitBps - slippageBps

	if direction == "bullish" {
		signal.Action = ActionBuy
		signal.Side = "buy"
		signal.StopLoss = mid * (1 - s.cfg.MaxLossBps/10000)
		signal.TakeProfit = mid * (1 + effectiveTarget/10000)
		signal.Reason = "persistent bullish OBI with price confirmation"
	} else {
		signal.Action = ActionSell
		signal.Side = "sell"
		signal.StopLoss = mid * (1 + s.cfg.MaxLossBps/10000)
		signal.TakeProfit = mid * (1 - effectiveTarget/10000)
		signal.Reason = "persistent bearish OBI with price confirmation"
	}

	return signal
}

func (s *FeeAwareScalper) checkPersistence(snapshots []features.OBISnapshot) (bool, string) {
	required := s.cfg.PersistenceSnapshots
	if len(snapshots) < required {
		return false, ""
	}

	bullishCount := 0
	bearishCount := 0

	for i := len(snapshots) - required; i < len(snapshots); i++ {
		if snapshots[i].Imbalance > s.cfg.ImbalanceThreshold {
			bullishCount++
		} else if snapshots[i].Imbalance < -s.cfg.ImbalanceThreshold {
			bearishCount++
		}
	}

	if bullishCount >= required {
		return true, "bullish"
	}
	if bearishCount >= required {
		return true, "bearish"
	}
	return false, ""
}

func (s *FeeAwareScalper) checkPriceConfirmation(snapshots []features.OBISnapshot, direction string) bool {
	if len(snapshots) < s.cfg.PersistenceSnapshots {
		return false
	}

	startIdx := len(snapshots) - s.cfg.PersistenceSnapshots
	startPrice := snapshots[startIdx].MidPrice
	endPrice := snapshots[len(snapshots)-1].MidPrice

	if startPrice == 0 {
		return false
	}

	priceChange := (endPrice - startPrice) / startPrice

	if direction == "bullish" {
		return priceChange > s.cfg.ConfirmationPricePct/100
	}
	return priceChange < -s.cfg.ConfirmationPricePct/100
}

func (s *FeeAwareScalper) GetFeeWindow(symbol string) time.Duration {
	if symbol == "BTCUSD" || symbol == "BTCINR" {
		return s.cfg.ScalpWindowBTC
	}
	return s.cfg.ScalpWindowOther
}

func (s *FeeAwareScalper) RecordEntry(symbol string) {
	s.entryTimes[symbol] = time.Now()
}

func (s *FeeAwareScalper) RecordExit(symbol string) {
	delete(s.entryTimes, symbol)
}

func (s *FeeAwareScalper) ShouldCloseForFees(symbol string) bool {
	entryTime, ok := s.entryTimes[symbol]
	if !ok {
		return false
	}
	window := s.GetFeeWindow(symbol)
	return time.Since(entryTime) < window
}

func (s *FeeAwareScalper) IsEnabled() bool {
	return s.cfg.Enabled
}
