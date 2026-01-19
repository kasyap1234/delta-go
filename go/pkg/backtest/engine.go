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
	equity        float64
	peakEquity    float64
	positions     map[string]*Position
	trades        []Trade
	equityCurve   []EquityPoint
	pendingOrders map[string]PendingOrder
	prevTimestamp time.Time
	lastPrice     map[string]float64

	// Margin tracking
	usedMargin float64 // Total margin currently in use

	// Data
	candles      map[string][]delta.Candle
	fundingRates map[string][]FundingRate
}

// PendingOrder represents a signal to execute on the next bar
type PendingOrder struct {
	Signal     strategy.Signal
	SignalTime time.Time
	Symbol     string
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
		pendingOrders:  make(map[string]PendingOrder),
		lastPrice:      make(map[string]float64),
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
		e.prevTimestamp = ts

		// Progress update every 10%
		if i%(len(timestamps)/10+1) == 0 {
			progress := float64(i) / float64(len(timestamps)) * 100
			fmt.Printf("  Progress: %.0f%% | Equity: $%.2f | Trades: %d\n",
				progress, e.equity, len(e.trades))
		}
	}

	// Get the final mark-to-market equity from the equity curve for consistent reporting
	finalEquity := e.equity
	if len(e.equityCurve) > 0 {
		finalEquity = e.equityCurve[len(e.equityCurve)-1].Equity
	}

	fmt.Printf("\nSimulation complete. Final equity: $%.2f\n", finalEquity)
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
	// 1. FIRST: Process funding payments for positions that were open BEFORE this bar
	// This ensures positions held through funding time receive their payment
	if e.config.SimulateFunding && e.shouldProcessFunding(ts) {
		e.processFunding(ts)
	}

	// 2. Execute pending orders from previous bar at THIS bar's open
	e.executePendingOrders(ts)

	// 3. Check stop-loss and take-profit for open positions
	e.checkExits(ts)

	// 4. Generate signals for each symbol (will be executed NEXT bar)
	for _, symbol := range e.config.Symbols {
		candle := e.getCandleAt(symbol, ts)
		if candle == nil {
			continue
		}

		// Store last price for equity curve
		e.lastPrice[symbol] = candle.Close

		// Get signal from Strategy Manager
		candles := e.getRecentCandles(symbol, ts, 200)
		mf := e.buildMarketFeatures(symbol, candle, candles, ts)
		signal := e.strategyMgr.GetSignal(mf, candles)

		// Queue signal for execution on NEXT bar
		if signal.Action != strategy.ActionNone {
			e.pendingOrders[symbol] = PendingOrder{
				Signal:     signal,
				SignalTime: ts,
				Symbol:     symbol,
			}
		}
	}

	// 5. Update equity curve
	e.updateEquityCurve(ts)

	return nil
}

// executePendingOrders executes queued orders at the current bar's open
func (e *Engine) executePendingOrders(ts time.Time) {
	for symbol, order := range e.pendingOrders {
		candle := e.getCandleAt(symbol, ts)
		if candle == nil {
			continue // Keep order pending if no candle
		}

		// Execute at THIS bar's open (not close!)
		e.processSignalAtPrice(symbol, order.Signal, candle, ts, candle.Open)

		// Remove from pending
		delete(e.pendingOrders, symbol)
	}
}

// shouldProcessFunding checks if we crossed a funding boundary since last timestamp
func (e *Engine) shouldProcessFunding(ts time.Time) bool {
	if e.prevTimestamp.IsZero() {
		return false
	}
	return crossedFundingBoundary(e.prevTimestamp, ts)
}

// crossedFundingBoundary checks if we crossed 00:00, 08:00, or 16:00 UTC
func crossedFundingBoundary(prev, curr time.Time) bool {
	prevU := prev.UTC()
	currU := curr.UTC()

	fundingHours := []int{0, 8, 16}
	for _, h := range fundingHours {
		fundingTime := time.Date(currU.Year(), currU.Month(), currU.Day(), h, 0, 0, 0, time.UTC)
		if prevU.Before(fundingTime) && (currU.Equal(fundingTime) || currU.After(fundingTime)) {
			return true
		}
		// Handle day boundary for 00:00
		if h == 0 && prevU.Day() != currU.Day() {
			return true
		}
	}
	return false
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

		// Get notional value for funding calculation
		product := e.getProduct(symbol)
		contracts := int(pos.Size)
		// Use mark price (current candle close) for notional calculation
		candle := e.getCandleAt(symbol, ts)
		markPrice := pos.EntryPrice // Fallback to entry price
		if candle != nil {
			markPrice = candle.Close
		}
		notional, err := delta.ContractsToNotional(contracts, markPrice, product)
		if err != nil || notional <= 0 {
			continue
		}

		// Calculate funding payment based on notional value
		payment := notional * rate

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

// processSignalAtPrice handles a trading signal at a specific fill price
func (e *Engine) processSignalAtPrice(symbol string, signal strategy.Signal, candle *delta.Candle, ts time.Time, fillPrice float64) {
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
			e.closePositionAtPrice(symbol, fillPrice, ts, "signal_reversal", candle)
		}
		// Open new position
		e.openPositionAtPrice(symbol, signal, candle, ts, fillPrice)

	case strategy.ActionClose:
		if existingPos != nil {
			e.closePositionAtPrice(symbol, fillPrice, ts, "signal_close", candle)
		}
	}
}

// openPositionAtPrice opens a new position at a specific fill price
func (e *Engine) openPositionAtPrice(symbol string, signal strategy.Signal, candle *delta.Candle, ts time.Time, fillPrice float64) {
	// 1. Calculate position size in contracts based on equity and risk
	contracts := e.calculatePositionSize(symbol, fillPrice, signal.StopLoss)
	if contracts <= 0 {
		return
	}

	// 2. Convert contracts to notional for margin calculation
	product := e.getProduct(symbol)
	notional, err := delta.ContractsToNotional(contracts, fillPrice, product)
	if err != nil || notional <= 0 {
		return
	}

	// 3. Check if we have enough margin
	requiredMargin := e.calculateRequiredMargin(notional)
	if requiredMargin > e.getAvailableMargin() {
		return // Not enough margin
	}

	// 4. Calculate slippage based on ACTUAL size (use notional for slippage model)
	slippageAmt := e.slippage.Calculate(signal.Side, notional, *candle, 0)
	actualEntryPrice := ApplySlippage(fillPrice, slippageAmt, signal.Side)

	// 5. Calculate fee based on notional
	fee := CalculateFee(actualEntryPrice, notional, 1.0, e.config.TakerFeeBps)

	// 6. Reserve margin
	e.usedMargin += requiredMargin

	// Store size as contracts (int converted to float64 for Position struct compatibility)
	pos := &Position{
		Symbol:        symbol,
		Side:          signal.Side,
		Size:          float64(contracts), // Store contracts as Size
		EntryPrice:    actualEntryPrice,
		EntryTime:     ts,
		StopLoss:      signal.StopLoss,
		TakeProfit:    signal.TakeProfit,
		InitialMargin: requiredMargin,
		EntryFee:      fee,
		EntrySlip:     slippageAmt,
	}

	e.positions[symbol] = pos
	e.equity -= fee
}

// closePosition closes an existing position (used by checkExits)
func (e *Engine) closePosition(symbol string, exitPrice float64, ts time.Time, reason string) {
	candle := e.getCandleAt(symbol, ts)
	e.closePositionAtPrice(symbol, exitPrice, ts, reason, candle)
}

// closePositionAtPrice closes an existing position at a specific fill price
func (e *Engine) closePositionAtPrice(symbol string, exitPrice float64, ts time.Time, reason string, candle *delta.Candle) {
	pos := e.positions[symbol]
	if pos == nil {
		return
	}

	// Release margin
	e.usedMargin -= pos.InitialMargin

	// Apply slippage to exit
	exitSide := "sell"
	if pos.Side == "sell" {
		exitSide = "buy"
	}

	// Get product for notional conversion
	product := e.getProduct(symbol)
	contracts := int(pos.Size) // Size is stored as contracts (float64 for struct compatibility)

	// Convert to notional for slippage and fee calculations
	entryNotional, _ := delta.ContractsToNotional(contracts, pos.EntryPrice, product)

	slippageAmt := 0.0
	if candle != nil && entryNotional > 0 {
		slippageAmt = e.slippage.Calculate(exitSide, entryNotional, *candle, 0)
	}
	actualExitPrice := ApplySlippage(exitPrice, slippageAmt, exitSide)

	// Calculate exit notional and fee
	exitNotional, _ := delta.ContractsToNotional(contracts, actualExitPrice, product)
	exitFee := CalculateFee(actualExitPrice, exitNotional, 1.0, e.config.TakerFeeBps)

	// Calculate P&L based on notional difference
	// For linear futures: PnL = contracts * contractValue * (exitPrice - entryPrice) * direction
	cv, _ := delta.ParseContractValue(product)
	priceDiff := actualExitPrice - pos.EntryPrice

	// For a long: +price = +profit, for short: +price = -profit
	multiplier := 1.0
	if pos.Side == "sell" {
		multiplier = -1.0
	}

	// Gross P&L in USD
	grossPnL := float64(contracts) * cv * priceDiff * multiplier

	// Calculate slippage cost in dollars
	entrySlipCost := pos.EntrySlip * (entryNotional / pos.EntryPrice)
	exitSlipCost := slippageAmt * (exitNotional / actualExitPrice)

	// NetPnL includes ALL costs: exit fee, slippage costs
	// Note: EntryFee was deducted from equity at entry time
	// FundingPaid was also applied to equity in processFunding()
	netPnL := grossPnL - exitFee - entrySlipCost - exitSlipCost

	// Record trade
	trade := Trade{
		ID:            fmt.Sprintf("%s-%d", symbol, len(e.trades)),
		Symbol:        symbol,
		Side:          pos.Side,
		Size:          pos.Size, // Contracts
		EntryPrice:    pos.EntryPrice,
		EntryTime:     pos.EntryTime,
		EntryFee:      pos.EntryFee,
		EntrySlip:     pos.EntrySlip,
		ExitPrice:     actualExitPrice,
		ExitTime:      ts,
		ExitFee:       exitFee,
		ExitSlip:      slippageAmt,
		EntrySlipCost: entrySlipCost,
		ExitSlipCost:  exitSlipCost,
		FundingPaid:   pos.FundingPaid,
		GrossPnL:      grossPnL,
		NetPnL:        netPnL,
		Reason:        reason,
	}
	e.trades = append(e.trades, trade)

	// Update equity
	e.equity += netPnL

	// Remove position
	delete(e.positions, symbol)
}

// calculateRequiredMargin calculates initial margin for a position
func (e *Engine) calculateRequiredMargin(notional float64) float64 {
	return notional / float64(e.config.Leverage)
}

// getAvailableMargin returns the margin available for new positions
func (e *Engine) getAvailableMargin() float64 {
	return e.equity - e.usedMargin
}

// calculatePositionSize determines position size based on risk
// Returns the contract count (not USD notional) - aligned with Delta Exchange API
func (e *Engine) calculatePositionSize(symbol string, entryPrice, stopLoss float64) int {
	// Don't trade if equity is too low or negative
	if e.equity <= 10 {
		return 0
	}

	availableMargin := e.getAvailableMargin()
	if availableMargin <= 0 {
		return 0
	}

	// Risk 2% of equity per trade
	riskPct := 0.02
	riskAmount := e.equity * riskPct

	// Calculate max position value based on AVAILABLE margin and leverage
	maxPositionValue := availableMargin * float64(e.config.Leverage)

	var positionValue float64

	// Calculate position size based on stop distance
	if stopLoss > 0 && entryPrice > 0 {
		stopPct := absFloat(entryPrice-stopLoss) / entryPrice
		if stopPct > 0 {
			// Position value such that stopPct loss = riskAmount
			positionValue = riskAmount / stopPct

			// Cap at max leverage
			if positionValue > maxPositionValue {
				positionValue = maxPositionValue
			}
		}
	}

	// Default: 10% of available margin as position value
	if positionValue <= 0 {
		positionValue = availableMargin * 0.10 * float64(e.config.Leverage)
		if positionValue > maxPositionValue {
			positionValue = maxPositionValue
		}
	}

	// Convert notional to contracts using product metadata
	product := e.getProduct(symbol)
	contracts, err := delta.NotionalToContracts(positionValue, entryPrice, product)
	if err != nil || contracts < 1 {
		return 0
	}

	return contracts
}

// getProduct returns the product metadata for a symbol
func (e *Engine) getProduct(symbol string) *delta.Product {
	if e.config.Products != nil {
		if p, ok := e.config.Products[symbol]; ok {
			return p
		}
	}
	// Fallback to mock product
	return delta.MockProduct(symbol)
}

// updateEquityCurve records current equity point
func (e *Engine) updateEquityCurve(ts time.Time) {
	// Calculate mark-to-market equity
	totalEquity := e.equity

	for symbol, pos := range e.positions {
		candle := e.getCandleAt(symbol, ts)
		var markPrice float64
		if candle != nil {
			markPrice = candle.Close
			e.lastPrice[symbol] = markPrice
		} else if lastPrice, ok := e.lastPrice[symbol]; ok {
			// Use last known price if no candle at this timestamp
			markPrice = lastPrice
		} else {
			// Fallback to entry price if no price history
			markPrice = pos.EntryPrice
		}

		// Get contract value from product
		product := e.getProduct(symbol)
		cv, err := delta.ParseContractValue(product)
		if err != nil {
			cv = 0.001 // Default to BTC contract value
		}

		totalEquity += pos.UnrealizedPnL(markPrice, cv)
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

func (e *Engine) buildMarketFeatures(symbol string, candle *delta.Candle, candles []delta.Candle, ts time.Time) features.MarketFeatures {
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

	// Attach funding rate if available
	if e.config.SimulateFunding {
		ticker.FundingRate = GetFundingAtTime(e.fundingRates[symbol], ts)
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
