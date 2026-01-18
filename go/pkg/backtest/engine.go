package backtest

import (
	"fmt"
	"time"

	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/features"
	"github.com/kasyap/delta-go/go/pkg/strategy"
)

// Engine is the main backtesting simulation engine
type Engine struct {
	config Config

	// Components
	dataLoader     *DataLoader
	fundingFetcher *FundingFetcher
	featuresEngine *features.Engine
	strategyMgr    *strategy.Manager
	slippage       SlippageModel

	// State
	equity      float64
	peakEquity  float64
	positions   map[string]*Position
	trades      []Trade
	equityCurve []EquityPoint

	// Data
	candles      map[string][]delta.Candle
	fundingRates map[string][]FundingRate
}

// NewEngine creates a new backtesting engine
func NewEngine(config Config, client *delta.Client) *Engine {
	return &Engine{
		config:         config,
		dataLoader:     NewDataLoader(client, config.DataCacheDir),
		fundingFetcher: NewFundingFetcher(config.DataCacheDir),
		featuresEngine: features.NewEngine(),
		strategyMgr:    strategy.NewManager(),
		slippage:       config.SlippageModel,
		equity:         config.InitialCapital,
		peakEquity:     config.InitialCapital,
		positions:      make(map[string]*Position),
		candles:        make(map[string][]delta.Candle),
		fundingRates:   make(map[string][]FundingRate),
	}
}

// RegisterStrategy adds a strategy to the backtest
func (e *Engine) RegisterStrategy(s strategy.Strategy) {
	e.strategyMgr.RegisterStrategy(s)
}

// Run executes the backtest and returns results
func (e *Engine) Run() (*Result, error) {
	fmt.Printf("=== Starting Backtest ===\n")
	fmt.Printf("Period: %s to %s\n", e.config.StartTime.Format("2006-01-02"), e.config.EndTime.Format("2006-01-02"))
	fmt.Printf("Symbols: %v\n", e.config.Symbols)
	fmt.Printf("Initial Capital: $%.2f\n", e.config.InitialCapital)
	fmt.Println()

	// Load data
	if err := e.loadData(); err != nil {
		return nil, fmt.Errorf("failed to load data: %w", err)
	}

	// Run simulation
	if err := e.simulate(); err != nil {
		return nil, fmt.Errorf("simulation failed: %w", err)
	}

	// Calculate metrics
	mc := NewMetricsCalculator(e.config)
	metrics := mc.Calculate(e.trades, e.equityCurve)

	return &Result{
		Metrics: metrics,
		Trades:  e.trades,
	}, nil
}

// Result contains backtest results
type Result struct {
	Metrics Metrics
	Trades  []Trade
}

// loadData fetches all historical data needed for backtest
func (e *Engine) loadData() error {
	fmt.Println("Loading historical data...")

	// Load candles for each symbol
	for _, symbol := range e.config.Symbols {
		fmt.Printf("  Loading %s candles...\n", symbol)
		candles, err := e.dataLoader.LoadCandles(
			symbol, e.config.Resolution,
			e.config.StartTime, e.config.EndTime,
		)
		if err != nil {
			return err
		}
		e.candles[symbol] = candles
		fmt.Printf("    Loaded %d candles\n", len(candles))
	}

	// Load funding rates
	if e.config.SimulateFunding {
		for _, symbol := range e.config.Symbols {
			fmt.Printf("  Loading %s funding rates...\n", symbol)
			rates, err := e.fundingFetcher.FetchFundingRates(
				symbol, e.config.StartTime, e.config.EndTime,
			)
			if err != nil {
				return err
			}
			e.fundingRates[symbol] = rates
			fmt.Printf("    Loaded %d funding rates\n", len(rates))
		}
	}

	fmt.Println("Data loading complete")
	return nil
}

// simulate runs the main simulation loop
func (e *Engine) simulate() error {
	fmt.Println("\nRunning simulation...")

	// Find all unique timestamps across symbols
	timestamps := e.getUniqueTimestamps()
	if len(timestamps) == 0 {
		return fmt.Errorf("no data to simulate")
	}

	fmt.Printf("Processing %d time steps...\n", len(timestamps))

	// Process each timestamp
	for i, ts := range timestamps {
		if err := e.processTimestamp(ts); err != nil {
			return fmt.Errorf("error at %v: %w", ts, err)
		}

		// Progress update every 10%
		if i%(len(timestamps)/10+1) == 0 {
			progress := float64(i) / float64(len(timestamps)) * 100
			fmt.Printf("  Progress: %.0f%% | Equity: $%.2f | Trades: %d\n",
				progress, e.equity, len(e.trades))
		}
	}

	fmt.Printf("\nSimulation complete. Final equity: $%.2f\n", e.equity)
	return nil
}

// getUniqueTimestamps collects all unique candle timestamps
func (e *Engine) getUniqueTimestamps() []time.Time {
	timeSet := make(map[int64]bool)

	for _, candles := range e.candles {
		for _, c := range candles {
			timeSet[c.Time] = true
		}
	}

	// Convert to sorted slice
	times := make([]time.Time, 0, len(timeSet))
	for ts := range timeSet {
		times = append(times, time.Unix(ts, 0))
	}

	// Sort
	for i := 0; i < len(times)-1; i++ {
		for j := i + 1; j < len(times); j++ {
			if times[i].After(times[j]) {
				times[i], times[j] = times[j], times[i]
			}
		}
	}

	return times
}

// processTimestamp handles all events at a single timestamp
func (e *Engine) processTimestamp(ts time.Time) error {
	// 1. Check funding payments
	if e.config.SimulateFunding && IsFundingTime(ts) {
		e.processFunding(ts)
	}

	// 2. Check stop-loss and take-profit for open positions
	e.checkExits(ts)

	// 3. Generate signals for each symbol
	for _, symbol := range e.config.Symbols {
		candle := e.getCandleAt(symbol, ts)
		if candle == nil {
			continue
		}

		// Get signal - try funding arbitrage first (uses historical funding rates)
		signal := e.getFundingArbitrageSignal(symbol, ts, candle)

		// If no funding signal, try standard strategy
		if signal.Action == strategy.ActionNone {
			candles := e.getRecentCandles(symbol, ts, 200)
			mf := e.buildMarketFeatures(symbol, candle, candles)
			signal = e.strategyMgr.GetSignal(candles, mf.HMMRegime)
		}

		// Process signal
		if signal.Action != strategy.ActionNone {
			e.processSignal(symbol, signal, candle, ts)
		}
	}

	// 4. Update equity curve
	e.updateEquityCurve(ts)

	return nil
}

// getFundingArbitrageSignal generates signals based on historical funding rates
func (e *Engine) getFundingArbitrageSignal(symbol string, ts time.Time, candle *delta.Candle) strategy.Signal {
	// Get current funding rate
	rate := GetFundingAtTime(e.fundingRates[symbol], ts)

	// Annualize: rate is per 8h, so multiply by 3*365
	annualizedRate := rate * 3 * 365

	// Check existing position
	pos := e.positions[symbol]

	// Entry threshold: 15% annualized
	entryThreshold := 0.15
	// Exit threshold: 5% annualized
	exitThreshold := 0.05

	// If we have a position, check exit conditions
	if pos != nil {
		// Exit if funding dropped below threshold
		if absFloat(annualizedRate) < exitThreshold {
			return strategy.Signal{
				Action:     strategy.ActionClose,
				Side:       oppositeSide(pos.Side),
				Confidence: 0.8,
				Reason:     "funding dropped below exit threshold",
			}
		}
		// Hold position
		return strategy.Signal{Action: strategy.ActionNone, Reason: "holding funding position"}
	}

	// Entry conditions: high absolute funding rate
	if absFloat(annualizedRate) > entryThreshold {
		side := "sell" // Positive funding -> short to earn
		action := strategy.ActionSell
		if annualizedRate < 0 {
			side = "buy" // Negative funding -> long to earn
			action = strategy.ActionBuy
		}

		// Set stop loss and take profit based on price
		stopDist := candle.Close * 0.02 // 2% stop
		takeDist := candle.Close * 0.04 // 4% take profit

		var stopLoss, takeProfit float64
		if side == "buy" {
			stopLoss = candle.Close - stopDist
			takeProfit = candle.Close + takeDist
		} else {
			stopLoss = candle.Close + stopDist
			takeProfit = candle.Close - takeDist
		}

		return strategy.Signal{
			Action:     action,
			Side:       side,
			Confidence: 0.65,
			Price:      candle.Close,
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Reason:     fmt.Sprintf("high funding rate: %.2f%% annualized", annualizedRate*100),
		}
	}

	return strategy.Signal{Action: strategy.ActionNone, Reason: "funding below threshold"}
}

func oppositeSide(side string) string {
	if side == "buy" {
		return "sell"
	}
	return "buy"
}

// processFunding applies funding payments to open positions
func (e *Engine) processFunding(ts time.Time) {
	for symbol, pos := range e.positions {
		rate := GetFundingAtTime(e.fundingRates[symbol], ts)
		if rate == 0 {
			continue
		}

		// Calculate funding payment - Size is NOTIONAL VALUE in dollars
		notionalValue := float64(pos.Size)
		payment := notionalValue * rate

		// Funding mechanics:
		// Positive rate: longs pay shorts
		// Negative rate: shorts pay longs
		if pos.Side == "buy" {
			// Long pays when rate is positive (payment > 0 means we lose)
			pos.FundingPaid += payment
			e.equity -= payment
		} else {
			// Short receives when rate is positive (payment > 0 means we earn)
			pos.FundingPaid -= payment // Negative FundingPaid = we earned
			e.equity += payment
		}
	}
}

// checkExits checks stop-loss and take-profit for all positions
func (e *Engine) checkExits(ts time.Time) {
	for symbol, pos := range e.positions {
		candle := e.getCandleAt(symbol, ts)
		if candle == nil {
			continue
		}

		var exitPrice float64
		var exitReason string

		if pos.Side == "buy" {
			// Long position
			if candle.Low <= pos.StopLoss && pos.StopLoss > 0 {
				exitPrice = pos.StopLoss
				exitReason = "stop_loss"
			} else if candle.High >= pos.TakeProfit && pos.TakeProfit > 0 {
				exitPrice = pos.TakeProfit
				exitReason = "take_profit"
			}
		} else {
			// Short position
			if candle.High >= pos.StopLoss && pos.StopLoss > 0 {
				exitPrice = pos.StopLoss
				exitReason = "stop_loss"
			} else if candle.Low <= pos.TakeProfit && pos.TakeProfit > 0 {
				exitPrice = pos.TakeProfit
				exitReason = "take_profit"
			}
		}

		if exitReason != "" {
			e.closePosition(symbol, exitPrice, ts, exitReason)
		}
	}
}

// processSignal handles a trading signal
func (e *Engine) processSignal(symbol string, signal strategy.Signal, candle *delta.Candle, ts time.Time) {
	// Check if we have an existing position
	existingPos := e.positions[symbol]

	switch signal.Action {
	case strategy.ActionBuy, strategy.ActionSell:
		if existingPos != nil {
			// Already have a position - check if same direction
			if (signal.Action == strategy.ActionBuy && existingPos.Side == "buy") ||
				(signal.Action == strategy.ActionSell && existingPos.Side == "sell") {
				return // Same direction, ignore
			}
			// Opposite direction - close first
			e.closePosition(symbol, candle.Close, ts, "signal_reversal")
		}
		// Open new position
		e.openPosition(symbol, signal, candle, ts)

	case strategy.ActionClose:
		if existingPos != nil {
			e.closePosition(symbol, candle.Close, ts, "signal_close")
		}
	}
}

// openPosition opens a new position
func (e *Engine) openPosition(symbol string, signal strategy.Signal, candle *delta.Candle, ts time.Time) {
	// 1. Calculate orientation first to get size
	entryPrice := candle.Close // Starting point

	// 2. Calculate position size based on equity and risk
	size := e.calculatePositionSize(entryPrice, signal.StopLoss)
	if size <= 0 {
		return
	}

	// 3. Calculate slippage based on ACTUAL size
	slippageAmt := e.slippage.Calculate(signal.Side, size, *candle, 0)
	actualEntryPrice := ApplySlippage(entryPrice, slippageAmt, signal.Side)

	// 4. Calculate fee based on ACTUAL size (notional)
	fee := CalculateFee(actualEntryPrice, size, 1.0, e.config.TakerFeeBps)

	pos := &Position{
		Symbol:     symbol,
		Side:       signal.Side,
		Size:       size,
		EntryPrice: actualEntryPrice,
		EntryTime:  ts,
		StopLoss:   signal.StopLoss,
		TakeProfit: signal.TakeProfit,
		EntryFee:   fee,
		EntrySlip:  slippageAmt,
	}

	e.positions[symbol] = pos
	e.equity -= fee
}

// closePosition closes an existing position
func (e *Engine) closePosition(symbol string, exitPrice float64, ts time.Time, reason string) {
	pos := e.positions[symbol]
	if pos == nil {
		return
	}

	// Apply slippage to exit
	exitSide := "sell"
	if pos.Side == "sell" {
		exitSide = "buy"
	}

	candle := e.getCandleAt(symbol, ts)
	slippageAmt := 0.0
	if candle != nil {
		slippageAmt = e.slippage.Calculate(exitSide, pos.Size, *candle, 0)
	}
	actualExitPrice := ApplySlippage(exitPrice, slippageAmt, exitSide)

	// Calculate exit fee
	exitFee := CalculateFee(actualExitPrice, pos.Size, 1.0, e.config.TakerFeeBps)

	// Calculate P&L based on percentage move
	// Size is already the NOTIONAL VALUE in dollars (not contract count)
	pricePctChange := (actualExitPrice - pos.EntryPrice) / pos.EntryPrice

	// For a long: +10% price = +10% of position value
	// For a short: +10% price = -10% of position value
	multiplier := 1.0
	if pos.Side == "sell" {
		multiplier = -1.0
	}

	// Size IS the notional value in dollars
	positionValue := float64(pos.Size)
	grossPnL := positionValue * pricePctChange * multiplier
	// Note: EntryFee already deducted at entry (line 420), FundingPaid already applied in processFunding()
	netPnL := grossPnL - exitFee

	// Record trade
	trade := Trade{
		ID:          fmt.Sprintf("%s-%d", symbol, len(e.trades)),
		Symbol:      symbol,
		Side:        pos.Side,
		Size:        pos.Size,
		EntryPrice:  pos.EntryPrice,
		EntryTime:   pos.EntryTime,
		EntryFee:    pos.EntryFee,
		EntrySlip:   pos.EntrySlip,
		ExitPrice:   actualExitPrice,
		ExitTime:    ts,
		ExitFee:     exitFee,
		ExitSlip:    slippageAmt,
		FundingPaid: pos.FundingPaid,
		GrossPnL:    grossPnL,
		NetPnL:      netPnL,
		Reason:      reason,
	}
	e.trades = append(e.trades, trade)

	// Update equity
	e.equity += netPnL

	// Remove position
	delete(e.positions, symbol)
}

// calculatePositionSize determines position size based on risk
// Returns the NOTIONAL VALUE in dollars (not contract count!)
// This value divided by entry price gives the "contract equivalent"
func (e *Engine) calculatePositionSize(entryPrice, stopLoss float64) int {
	// Don't trade if equity is too low or negative
	if e.equity <= 10 {
		return 0
	}

	// Risk 2% of equity per trade
	riskPct := 0.02
	riskAmount := e.equity * riskPct

	// Calculate max position value based on leverage
	maxPositionValue := e.equity * float64(e.config.Leverage)

	// Calculate position size based on stop distance
	if stopLoss > 0 && entryPrice > 0 {
		stopPct := absFloat(entryPrice-stopLoss) / entryPrice
		if stopPct > 0 {
			// Position value such that stopPct loss = riskAmount
			// positionValue * stopPct = riskAmount
			// positionValue = riskAmount / stopPct
			positionValue := riskAmount / stopPct

			// Cap at max leverage
			if positionValue > maxPositionValue {
				positionValue = maxPositionValue
			}

			// Return notional value (we'll treat Size as notional dollars)
			return int(positionValue)
		}
	}

	// Default: 10% of equity as position value
	defaultValue := e.equity * 0.10
	if defaultValue > maxPositionValue {
		defaultValue = maxPositionValue
	}
	return int(defaultValue)
}

// updateEquityCurve records current equity point
func (e *Engine) updateEquityCurve(ts time.Time) {
	// Calculate mark-to-market equity
	totalEquity := e.equity

	for symbol, pos := range e.positions {
		candle := e.getCandleAt(symbol, ts)
		if candle != nil {
			totalEquity += pos.UnrealizedPnL(candle.Close, 1.0)
		}
	}

	// Update peak
	if totalEquity > e.peakEquity {
		e.peakEquity = totalEquity
	}

	// Calculate drawdown
	drawdown := 0.0
	if e.peakEquity > 0 {
		drawdown = (e.peakEquity - totalEquity) / e.peakEquity
	}

	e.equityCurve = append(e.equityCurve, EquityPoint{
		Timestamp: ts,
		Equity:    totalEquity,
		Drawdown:  drawdown,
	})
}

// Helper methods

func (e *Engine) getCandleAt(symbol string, ts time.Time) *delta.Candle {
	candles := e.candles[symbol]
	targetTs := ts.Unix()

	for i := range candles {
		if candles[i].Time == targetTs {
			return &candles[i]
		}
	}
	return nil
}

func (e *Engine) getRecentCandles(symbol string, beforeTs time.Time, count int) []delta.Candle {
	candles := e.candles[symbol]
	var result []delta.Candle

	targetTs := beforeTs.Unix()
	for i := len(candles) - 1; i >= 0; i-- {
		if candles[i].Time < targetTs {
			result = append([]delta.Candle{candles[i]}, result...)
			if len(result) >= count {
				break
			}
		}
	}

	return result
}

func (e *Engine) buildMarketFeatures(symbol string, candle *delta.Candle, candles []delta.Candle) features.MarketFeatures {
	// Create synthetic ticker from candle
	ticker := &delta.Ticker{
		Symbol:    symbol,
		Close:     candle.Close,
		High:      candle.High,
		Low:       candle.Low,
		Open:      candle.Open,
		MarkPrice: candle.Close,
		Volume:    candle.Volume,
	}

	// Use features engine
	return e.featuresEngine.ComputeFeaturesWithFunding(nil, ticker, candles)
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
