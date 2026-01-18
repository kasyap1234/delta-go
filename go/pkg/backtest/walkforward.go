package backtest

import (
	"fmt"
	"time"
)

// WalkForwardConfig defines walk-forward analysis parameters
type WalkForwardConfig struct {
	TrainingPeriod time.Duration // e.g., 6 months
	TestingPeriod  time.Duration // e.g., 1 month
	Anchored       bool          // If true, training window expands from start
}

// DefaultWalkForwardConfig returns sensible defaults
func DefaultWalkForwardConfig() WalkForwardConfig {
	return WalkForwardConfig{
		TrainingPeriod: 180 * 24 * time.Hour, // 6 months
		TestingPeriod:  30 * 24 * time.Hour,  // 1 month
		Anchored:       false,                // Rolling window
	}
}

// WindowResult contains results for a single walk-forward window
type WindowResult struct {
	TrainStart  time.Time
	TrainEnd    time.Time
	TestStart   time.Time
	TestEnd     time.Time
	TestMetrics Metrics
}

// WalkForwardResult contains combined walk-forward analysis results
type WalkForwardResult struct {
	Windows   []WindowResult
	Combined  Metrics // Combined OOS metrics
	Stability float64 // Consistency score (0-1)
	Summary   string
}

// WalkForwardAnalyzer performs walk-forward optimization testing
type WalkForwardAnalyzer struct {
	baseConfig    Config
	wfConfig      WalkForwardConfig
	engineFactory func(Config) *Engine
}

// NewWalkForwardAnalyzer creates a walk-forward analyzer
func NewWalkForwardAnalyzer(baseConfig Config, wfConfig WalkForwardConfig, factory func(Config) *Engine) *WalkForwardAnalyzer {
	return &WalkForwardAnalyzer{
		baseConfig:    baseConfig,
		wfConfig:      wfConfig,
		engineFactory: factory,
	}
}

// Run performs walk-forward analysis
func (wf *WalkForwardAnalyzer) Run() (*WalkForwardResult, error) {
	fmt.Println("=== Walk-Forward Analysis ===")
	fmt.Printf("Training Period: %d days\n", int(wf.wfConfig.TrainingPeriod.Hours()/24))
	fmt.Printf("Testing Period: %d days\n", int(wf.wfConfig.TestingPeriod.Hours()/24))
	fmt.Printf("Mode: %s\n", wf.modeString())
	fmt.Println()

	windows := wf.generateWindows()
	if len(windows) == 0 {
		return nil, fmt.Errorf("insufficient data for walk-forward analysis")
	}

	fmt.Printf("Generated %d windows\n\n", len(windows))

	result := &WalkForwardResult{
		Windows: make([]WindowResult, 0, len(windows)),
	}

	// Process each window
	var allTrades []Trade
	var allEquity []EquityPoint

	for i, window := range windows {
		fmt.Printf("Window %d/%d: Test %s to %s\n",
			i+1, len(windows),
			window.testStart.Format("2006-01-02"),
			window.testEnd.Format("2006-01-02"))

		// Create engine for test period
		testConfig := wf.baseConfig
		testConfig.StartTime = window.testStart
		testConfig.EndTime = window.testEnd

		engine := wf.engineFactory(testConfig)
		res, err := engine.Run()
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			continue
		}

		windowResult := WindowResult{
			TrainStart:  window.trainStart,
			TrainEnd:    window.trainEnd,
			TestStart:   window.testStart,
			TestEnd:     window.testEnd,
			TestMetrics: res.Metrics,
		}
		result.Windows = append(result.Windows, windowResult)

		// Collect trades and equity
		allTrades = append(allTrades, res.Trades...)
		allEquity = append(allEquity, res.Metrics.EquityCurve...)

		fmt.Printf("  Return: %.2f%% | Sharpe: %.2f | MaxDD: %.2f%%\n",
			res.Metrics.TotalReturn*100,
			res.Metrics.SharpeRatio,
			res.Metrics.MaxDrawdown*100)
	}

	// Calculate combined metrics
	mc := NewMetricsCalculator(wf.baseConfig)
	result.Combined = mc.Calculate(allTrades, allEquity)

	// Calculate stability score
	result.Stability = wf.calculateStability(result.Windows)

	// Generate summary
	result.Summary = wf.generateSummary(result)

	return result, nil
}

type window struct {
	trainStart time.Time
	trainEnd   time.Time
	testStart  time.Time
	testEnd    time.Time
}

// generateWindows creates train/test windows
func (wf *WalkForwardAnalyzer) generateWindows() []window {
	var windows []window

	start := wf.baseConfig.StartTime
	end := wf.baseConfig.EndTime

	// Need at least training + testing period
	minDuration := wf.wfConfig.TrainingPeriod + wf.wfConfig.TestingPeriod
	if end.Sub(start) < minDuration {
		return nil
	}

	if wf.wfConfig.Anchored {
		// Anchored: training starts from beginning, expands
		trainStart := start
		testStart := start.Add(wf.wfConfig.TrainingPeriod)

		for testStart.Before(end) {
			testEnd := testStart.Add(wf.wfConfig.TestingPeriod)
			if testEnd.After(end) {
				testEnd = end
			}

			windows = append(windows, window{
				trainStart: trainStart,
				trainEnd:   testStart,
				testStart:  testStart,
				testEnd:    testEnd,
			})

			testStart = testEnd
		}
	} else {
		// Rolling: training window moves forward
		trainStart := start

		for {
			trainEnd := trainStart.Add(wf.wfConfig.TrainingPeriod)
			testStart := trainEnd
			testEnd := testStart.Add(wf.wfConfig.TestingPeriod)

			if testEnd.After(end) {
				break
			}

			windows = append(windows, window{
				trainStart: trainStart,
				trainEnd:   trainEnd,
				testStart:  testStart,
				testEnd:    testEnd,
			})

			// Move training window forward by testing period
			trainStart = trainStart.Add(wf.wfConfig.TestingPeriod)
		}
	}

	return windows
}

// calculateStability computes consistency across windows
func (wf *WalkForwardAnalyzer) calculateStability(windows []WindowResult) float64 {
	if len(windows) < 2 {
		return 0
	}

	// Calculate what percentage of windows are profitable
	profitableCount := 0
	var sharpes []float64

	for _, w := range windows {
		if w.TestMetrics.TotalReturn > 0 {
			profitableCount++
		}
		sharpes = append(sharpes, w.TestMetrics.SharpeRatio)
	}

	profitability := float64(profitableCount) / float64(len(windows))

	// Calculate Sharpe consistency (inverse of coefficient of variation)
	if len(sharpes) > 1 {
		mean := 0.0
		for _, s := range sharpes {
			mean += s
		}
		mean /= float64(len(sharpes))

		variance := 0.0
		for _, s := range sharpes {
			variance += (s - mean) * (s - mean)
		}
		variance /= float64(len(sharpes))

		stdDev := 0.0
		if variance > 0 {
			stdDev = sqrt(variance)
		}

		// Consistency: low CV = high consistency
		cv := 0.0
		if mean != 0 {
			cv = stdDev / absFloat(mean)
		}
		consistency := 1.0 / (1.0 + cv) // Transform to 0-1 scale

		// Combine profitability and consistency
		return (profitability + consistency) / 2.0
	}

	return profitability
}

func (wf *WalkForwardAnalyzer) modeString() string {
	if wf.wfConfig.Anchored {
		return "Anchored (expanding window)"
	}
	return "Rolling (sliding window)"
}

func (wf *WalkForwardAnalyzer) generateSummary(result *WalkForwardResult) string {
	profitableWindows := 0
	for _, w := range result.Windows {
		if w.TestMetrics.TotalReturn > 0 {
			profitableWindows++
		}
	}

	summary := fmt.Sprintf(`
=== Walk-Forward Summary ===
Windows: %d total, %d profitable (%.0f%%)
Combined OOS Return: %.2f%%
Combined Sharpe: %.2f
Max Drawdown: %.2f%%
Stability Score: %.2f

Interpretation:
- Stability > 0.7: Strong evidence of robust strategy
- Stability 0.5-0.7: Moderate robustness, use caution
- Stability < 0.5: Strategy may be overfit
`,
		len(result.Windows),
		profitableWindows,
		float64(profitableWindows)/float64(len(result.Windows))*100,
		result.Combined.TotalReturn*100,
		result.Combined.SharpeRatio,
		result.Combined.MaxDrawdown*100,
		result.Stability,
	)

	return summary
}

// sqrt implementation without math import
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 100; i++ {
		z = (z + x/z) / 2
	}
	return z
}
