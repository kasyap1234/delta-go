package strategy

import (
	"log"
	"sort"
	"sync"

	"github.com/kasyap/delta-go/go/pkg/delta"
)

// AssetSignal represents a trading signal for a specific asset
type AssetSignal struct {
	Symbol        string
	Signal        Signal
	Regime        delta.MarketRegime
	HMMConfidence float64 // Confidence from HMM model
	TotalScore    float64 // Combined score for ranking
}

// SignalAggregator evaluates multiple assets and selects the best opportunity
type SignalAggregator struct {
	strategyMgr *Manager

	// Minimum thresholds
	MinConfidence    float64
	MinHMMConfidence float64
	MinTotalScore    float64 // Minimum combined score to trade
}

// NewSignalAggregator creates a new signal aggregator
func NewSignalAggregator(mgr *Manager) *SignalAggregator {
	return &SignalAggregator{
		strategyMgr:      mgr,
		MinConfidence:    0.5, // Increased from 0.4 for higher selectivity
		MinHMMConfidence: 0.7, // Increased from 0.3 (Fix #5: Major drawdown reduction)
		MinTotalScore:    0.6, // Increased from 0.5 for more robust entries
	}
}

// AssetData contains the data needed to evaluate an asset
type AssetData struct {
	Symbol  string
	Candles []delta.Candle
	Regime  delta.MarketRegime
	HMMConf float64
}

// EvaluateAssets evaluates all assets and returns ranked signals
func (sa *SignalAggregator) EvaluateAssets(assets []AssetData) []AssetSignal {
	var wg sync.WaitGroup
	results := make(chan AssetSignal, len(assets))

	// Limit concurrent goroutines to avoid excessive fan-out
	maxConcurrent := 8
	sem := make(chan struct{}, maxConcurrent)

	for _, asset := range assets {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore
		go func(a AssetData) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			signal := sa.strategyMgr.GetSignal(a.Candles, a.Regime)

			if signal.Action == ActionNone {
				return
			}

			totalScore := (signal.Confidence * 0.6) + (a.HMMConf * 0.4)
			totalScore *= sa.getRegimeMultiplier(a.Regime, signal.Side)

			results <- AssetSignal{
				Symbol:        a.Symbol,
				Signal:        signal,
				Regime:        a.Regime,
				HMMConfidence: a.HMMConf,
				TotalScore:    totalScore,
			}
		}(asset)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var signals []AssetSignal
	for sig := range results {
		// Filter by minimum thresholds
		if sig.Signal.Confidence < sa.MinConfidence {
			log.Printf("  %s: strategy confidence %.2f below threshold %.2f", sig.Symbol, sig.Signal.Confidence, sa.MinConfidence)
			continue
		}
		if sig.HMMConfidence < sa.MinHMMConfidence {
			log.Printf("  %s: HMM confidence %.2f below threshold %.2f", sig.Symbol, sig.HMMConfidence, sa.MinHMMConfidence)
			continue
		}
		if sig.TotalScore < sa.MinTotalScore {
			log.Printf("  %s: total score %.2f below threshold %.2f", sig.Symbol, sig.TotalScore, sa.MinTotalScore)
			continue
		}
		signals = append(signals, sig)
	}

	// Sort by total score (highest first)
	sort.Slice(signals, func(i, j int) bool {
		return signals[i].TotalScore > signals[j].TotalScore
	})

	return signals
}

// SelectBestSignal returns the single best signal across all assets
func (sa *SignalAggregator) SelectBestSignal(assets []AssetData) *AssetSignal {
	signals := sa.EvaluateAssets(assets)

	if len(signals) == 0 {
		return nil
	}

	best := signals[0]
	log.Printf("Best signal: %s %s (score: %.3f, regime: %s, confidence: %.2f)",
		best.Symbol, best.Signal.Side, best.TotalScore, best.Regime, best.Signal.Confidence)

	// Log other opportunities
	if len(signals) > 1 {
		log.Printf("Other opportunities:")
		for _, s := range signals[1:] {
			log.Printf("  - %s %s (score: %.3f)", s.Symbol, s.Signal.Side, s.TotalScore)
		}
	}

	return &best
}

// getRegimeMultiplier applies additional scoring based on regime alignment
func (sa *SignalAggregator) getRegimeMultiplier(regime delta.MarketRegime, side string) float64 {
	// Boost signals that align with regime
	switch regime {
	case delta.RegimeBull:
		if side == "buy" {
			return 1.2 // Boost longs in bull
		}
		return 0.8 // Penalize shorts in bull

	case delta.RegimeBear:
		if side == "sell" {
			return 1.2 // Boost shorts in bear
		}
		return 0.8 // Penalize longs in bear

	case delta.RegimeHighVol:
		return 0.9 // Slightly penalize all in high vol (risky)

	default:
		return 1.0
	}
}

// SetThresholds updates the minimum thresholds
func (sa *SignalAggregator) SetThresholds(minConf, minHMMConf float64) {
	sa.MinConfidence = minConf
	sa.MinHMMConfidence = minHMMConf
}
