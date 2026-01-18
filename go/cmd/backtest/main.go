// Backtest CLI - Run backtests on historical data
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	botconfig "github.com/kasyap/delta-go/go/config"
	"github.com/kasyap/delta-go/go/pkg/backtest"
	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/features"
	"github.com/kasyap/delta-go/go/pkg/strategy"
)

func main() {
	// Parse command line flags
	symbolsFlag := flag.String("symbols", "BTCUSD,ETHUSD,SOLUSD", "Comma-separated list of symbols to backtest")
	startFlag := flag.String("start", "2024-01-01", "Start date (YYYY-MM-DD)")
	endFlag := flag.String("end", "2025-01-01", "End date (YYYY-MM-DD)")
	capitalFlag := flag.Float64("capital", 200, "Initial capital in USD")
	leverageFlag := flag.Int("leverage", 10, "Leverage to use")
	resolutionFlag := flag.String("resolution", "5m", "Candle resolution (1m, 5m, 15m, 1h)")
	strategyFlag := flag.String("strategy", "all", "Strategy: scalper, funding, grid, all")
	walkforwardFlag := flag.Bool("walkforward", false, "Enable walk-forward analysis")
	jsonOutputFlag := flag.Bool("json", false, "Output results as JSON")
	cacheDirFlag := flag.String("cache", ".backtest_cache", "Directory for cached data")
	flag.Parse()

	// Parse dates
	start, err := time.Parse("2006-01-02", *startFlag)
	if err != nil {
		fmt.Printf("Error parsing start date: %v\n", err)
		os.Exit(1)
	}
	end, err := time.Parse("2006-01-02", *endFlag)
	if err != nil {
		fmt.Printf("Error parsing end date: %v\n", err)
		os.Exit(1)
	}

	// Parse symbols
	symbols := strings.Split(*symbolsFlag, ",")
	for i := range symbols {
		symbols[i] = strings.TrimSpace(symbols[i])
	}

	// Create backtest config
	btConfig := backtest.Config{
		StartTime:       start,
		EndTime:         end,
		Symbols:         symbols,
		Resolution:      *resolutionFlag,
		InitialCapital:  *capitalFlag,
		Leverage:        *leverageFlag,
		MakerFeeBps:     2.0,
		TakerFeeBps:     5.0,
		SlippageModel:   backtest.NewVolatilitySlippage(1.5, 0.5),
		LatencyMs:       50,
		SimulateFunding: true,
		DataCacheDir:    *cacheDirFlag,
	}

	// Create Delta client (for data fetching - using default config)
	deltaCfg := botconfig.LoadConfig()
	client := delta.NewClient(deltaCfg)

	// Create engine factory
	engineFactory := func(cfg backtest.Config) *backtest.Engine {
		engine := backtest.NewEngine(cfg, client)
		registerStrategies(engine, *strategyFlag)
		return engine
	}

	if *walkforwardFlag {
		// Walk-forward analysis
		wfConfig := backtest.DefaultWalkForwardConfig()
		analyzer := backtest.NewWalkForwardAnalyzer(btConfig, wfConfig, engineFactory)

		result, err := analyzer.Run()
		if err != nil {
			fmt.Printf("Walk-forward analysis failed: %v\n", err)
			os.Exit(1)
		}

		if *jsonOutputFlag {
			outputJSON(result)
		} else {
			fmt.Println(result.Summary)
		}
	} else {
		// Single backtest
		engine := engineFactory(btConfig)
		result, err := engine.Run()
		if err != nil {
			fmt.Printf("Backtest failed: %v\n", err)
			os.Exit(1)
		}

		if *jsonOutputFlag {
			outputJSON(result)
		} else {
			fmt.Println(result.Metrics.FormatReport())
		}
	}
}

// registerStrategies adds strategies to the engine based on flag
func registerStrategies(engine *backtest.Engine, strategyType string) {
	featuresEngine := features.NewEngine()

	switch strategyType {
	case "scalper":
		scalper := strategy.NewFeeAwareScalper(strategy.DefaultScalperConfig(), featuresEngine)
		engine.RegisterStrategy(scalper)

	case "funding":
		funding := strategy.NewFundingArbitrageStrategy(strategy.DefaultFundingArbitrageConfig())
		// Wrap to implement Strategy interface
		engine.RegisterStrategy(&fundingWrapper{funding})

	case "grid":
		grid := strategy.NewGridTradingStrategy(strategy.DefaultGridConfig(), "BTCUSD")
		engine.RegisterStrategy(&gridWrapper{grid})

	case "all":
		// Register all strategies
		scalper := strategy.NewFeeAwareScalper(strategy.DefaultScalperConfig(), featuresEngine)
		engine.RegisterStrategy(scalper)

		funding := strategy.NewFundingArbitrageStrategy(strategy.DefaultFundingArbitrageConfig())
		engine.RegisterStrategy(&fundingWrapper{funding})

	default:
		fmt.Printf("Unknown strategy: %s\n", strategyType)
		os.Exit(1)
	}
}

// Wrappers to adapt strategies to the Strategy interface

type fundingWrapper struct {
	*strategy.FundingArbitrageStrategy
}

func (w *fundingWrapper) Analyze(candles []delta.Candle, regime delta.MarketRegime) strategy.Signal {
	// Funding strategy needs features, not just candles
	// Return no signal when used via standard interface
	return strategy.Signal{Action: strategy.ActionNone, Reason: "funding requires features"}
}

func (w *fundingWrapper) UpdateParams(params map[string]interface{}) {
	w.FundingArbitrageStrategy.UpdateParams(params)
}

type gridWrapper struct {
	*strategy.GridTradingStrategy
}

func (w *gridWrapper) Analyze(candles []delta.Candle, regime delta.MarketRegime) strategy.Signal {
	// Grid strategy needs features, not just candles
	return strategy.Signal{Action: strategy.ActionNone, Reason: "grid requires features"}
}

func (w *gridWrapper) UpdateParams(params map[string]interface{}) {
	w.GridTradingStrategy.UpdateParams(params)
}

func outputJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Printf("Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}
