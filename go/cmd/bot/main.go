package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/kasyap/delta-go/go/config"
	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/risk"
	"github.com/kasyap/delta-go/go/pkg/strategy"
)

// SignalFilter applies additional filters before trade execution
type SignalFilter struct {
	ti *strategy.TechnicalIndicators
}

// NewSignalFilter creates a new signal filter
func NewSignalFilter() *SignalFilter {
	return &SignalFilter{ti: &strategy.TechnicalIndicators{}}
}

// ShouldTrade applies 4H filter and long restrictions (matching Python backtest)
func (sf *SignalFilter) ShouldTrade(signal strategy.Signal, candles []delta.Candle, regime delta.MarketRegime) (bool, string) {
	if signal.Action == strategy.ActionNone {
		return false, "no signal"
	}

	// Extract closes for calculations
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}

	// 4H multi-timeframe filter (only for high_volatility regime)
	if regime == delta.RegimeHighVol && len(closes) >= 80 {
		// Sample every 4th candle to simulate 4H from 1H data
		closes4H := make([]float64, 0, len(closes)/4)
		for i := 0; i < len(closes); i += 4 {
			closes4H = append(closes4H, closes[i])
		}

		if len(closes4H) >= 20 {
			ema20 := sf.ti.EMALast(closes4H, 20)
			current4H := closes4H[len(closes4H)-1]
			trendUp := current4H > ema20

			if signal.Side == "buy" && !trendUp {
				return false, "4H trend down, skipping long in high_vol"
			}
			if signal.Side == "sell" && trendUp {
				return false, "4H trend up, skipping short in high_vol"
			}
		}
	}

	// Stricter long entry criteria (longs underperform historically)
	if signal.Side == "buy" {
		if signal.Confidence < 0.70 {
			return false, fmt.Sprintf("long confidence %.2f < 0.70 threshold", signal.Confidence)
		}
		if regime == delta.RegimeBear {
			return false, "no longs in bear regime"
		}
	}

	return true, ""
}

type PerformanceSnapshot struct {
	Timestamp     time.Time
	Equity        float64
	RealizedPnL   float64
	UnrealizedPnL float64
	Positions     int
}

type PerformanceTracker struct {
	mu          sync.RWMutex
	startEquity float64
	lastEquity  float64
	snapshots   []PerformanceSnapshot
	maxSamples  int
}

func NewPerformanceTracker(maxSamples int) *PerformanceTracker {
	if maxSamples <= 0 {
		maxSamples = 500
	}
	return &PerformanceTracker{maxSamples: maxSamples}
}

func (pt *PerformanceTracker) Record(s PerformanceSnapshot) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.startEquity == 0 {
		pt.startEquity = s.Equity
	}
	pt.lastEquity = s.Equity
	pt.snapshots = append(pt.snapshots, s)
	if len(pt.snapshots) > pt.maxSamples {
		pt.snapshots = pt.snapshots[len(pt.snapshots)-pt.maxSamples:]
	}
}

func (pt *PerformanceTracker) Report() map[string]interface{} {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	pnlAbs := pt.lastEquity - pt.startEquity
	pnlPct := 0.0
	if pt.startEquity != 0 {
		pnlPct = (pnlAbs / pt.startEquity) * 100
	}

	last := PerformanceSnapshot{}
	if len(pt.snapshots) > 0 {
		last = pt.snapshots[len(pt.snapshots)-1]
	}

	return map[string]interface{}{
		"start_equity":     pt.startEquity,
		"last_equity":      pt.lastEquity,
		"pnl_abs":          pnlAbs,
		"pnl_pct":          pnlPct,
		"last_timestamp":   last.Timestamp,
		"realized_pnl":     last.RealizedPnL,
		"unrealized_pnl":   last.UnrealizedPnL,
		"open_positions":   last.Positions,
		"snapshots_stored": len(pt.snapshots),
	}
}

// TradingBot is the main bot orchestrator
type TradingBot struct {
	cfg          *config.Config
	deltaClient  *delta.Client
	wsClient     *delta.WebSocketClient
	hmmClient    *delta.HMMClient
	riskManager  *risk.RiskManager
	strategyMgr  *strategy.Manager
	signalFilter *SignalFilter
	perfTracker  *PerformanceTracker

	// State
	mu             sync.RWMutex
	currentProduct *delta.Product
	currentRegime  delta.MarketRegime
	candles        []delta.Candle
	lastTicker     *delta.Ticker
	isRunning      bool
	stopChan       chan struct{}
	stopOnce       sync.Once
	lastPerfUpdate time.Time
	productCache   map[string]*delta.Product
}

// NewTradingBot creates a new trading bot
func NewTradingBot(cfg *config.Config) *TradingBot {
	bot := &TradingBot{
		cfg:           cfg,
		deltaClient:   delta.NewClient(cfg),
		wsClient:      delta.NewWebSocketClient(cfg),
		hmmClient:     delta.NewHMMClient(cfg.HMMEndpoint),
		riskManager:   risk.NewRiskManager(cfg),
		strategyMgr:   strategy.NewManager(),
		signalFilter:  NewSignalFilter(),
		perfTracker:   NewPerformanceTracker(500),
		currentRegime: delta.RegimeRanging,
		candles:       make([]delta.Candle, 0),
		stopChan:      make(chan struct{}),
		productCache:  make(map[string]*delta.Product),
	}

	// Register all regime-specific strategies
	bot.strategyMgr.RegisterStrategy(strategy.NewBullTrendStrategy())
	bot.strategyMgr.RegisterStrategy(strategy.NewBearTrendStrategy())
	bot.strategyMgr.RegisterStrategy(strategy.NewRangingStrategy())
	bot.strategyMgr.RegisterStrategy(strategy.NewHighVolBreakoutStrategy())
	bot.strategyMgr.RegisterStrategy(strategy.NewLowVolPrepStrategy())

	// Map regimes to strategies
	bot.strategyMgr.SetRegimeStrategy(delta.RegimeBull, "bull_trend_following")
	bot.strategyMgr.SetRegimeStrategy(delta.RegimeBear, "bear_trend_following")
	bot.strategyMgr.SetRegimeStrategy(delta.RegimeRanging, "ranging_mean_reversion")
	bot.strategyMgr.SetRegimeStrategy(delta.RegimeHighVol, "high_vol_breakout")
	bot.strategyMgr.SetRegimeStrategy(delta.RegimeLowVol, "low_vol_preparation")

	return bot
}

// Initialize sets up the bot
func (bot *TradingBot) Initialize() error {
	log.Println("Initializing trading bot...")

	// Get product info
	product, err := bot.deltaClient.GetProductBySymbol(bot.cfg.Symbol)
	if err != nil {
		return fmt.Errorf("failed to get product: %v", err)
	}
	bot.currentProduct = product
	bot.productCache[product.Symbol] = product
	log.Printf("Trading product: %s (ID: %d)", product.Symbol, product.ID)

	// Set leverage
	if err := bot.deltaClient.SetLeverage(product.ID, bot.cfg.Leverage); err != nil {
		log.Printf("Warning: failed to set leverage: %v", err)
	}

	// Load initial candles
	candles, err := bot.deltaClient.GetRecentCandles(bot.cfg.Symbol, bot.cfg.CandleInterval, 200)
	if err != nil {
		return fmt.Errorf("failed to get initial candles: %v", err)
	}
	bot.candles = candles
	log.Printf("Loaded %d historical candles", len(candles))

	// Get initial balance
	walletResp, err := bot.deltaClient.GetWalletBalances()
	if err != nil {
		log.Printf("Warning: failed to get wallet: %v", err)
	} else {
		settling := product.SettlingAsset.Symbol
		if settling == "" {
			settling = "USDT"
		}
		for _, w := range walletResp.Result {
			if w.AssetSymbol == settling {
				if balance, err := strconv.ParseFloat(w.AvailableBalance, 64); err == nil {
					bot.riskManager.UpdateBalance(balance)
					log.Printf("Available balance: %.2f %s", balance, w.AssetSymbol)
				}
			}
		}
	}

	// Initial regime detection
	if err := bot.updateMarketRegime(); err != nil {
		log.Printf("Warning: initial regime detection failed: %v", err)
		bot.currentRegime = delta.RegimeRanging
	}
	log.Printf("Initial market regime: %s", bot.currentRegime)

	bot.updatePerformanceIfDue(true, product)

	return nil
}

// Start begins the trading loop
func (bot *TradingBot) Start() error {
	log.Println("Starting trading bot...")

	// Connect WebSocket
	if err := bot.wsClient.Connect(); err != nil {
		return fmt.Errorf("websocket connection failed: %v", err)
	}

	// Set up WebSocket callbacks
	bot.wsClient.OnTicker(bot.handleTicker)
	bot.wsClient.OnCandle(bot.handleCandle)
	bot.wsClient.OnError(bot.handleWSError)

	// Subscribe to channels
	if err := bot.wsClient.SubscribeTicker(bot.cfg.Symbol); err != nil {
		log.Printf("Warning: failed to subscribe to ticker: %v", err)
	}
	if err := bot.wsClient.SubscribeCandles(bot.cfg.Symbol, bot.cfg.CandleInterval); err != nil {
		log.Printf("Warning: failed to subscribe to candles: %v", err)
	}

	bot.mu.Lock()
	bot.isRunning = true
	bot.mu.Unlock()

	// Start main loop
	go bot.mainLoop()

	// Start regime update loop
	go bot.regimeUpdateLoop()

	log.Println("Bot started successfully")
	return nil
}

// mainLoop is the main trading loop
func (bot *TradingBot) mainLoop() {
	loopPeriod := 10 * time.Second
	if d := resolutionToDuration(bot.cfg.CandleInterval); d > 0 {
		p := d / 3
		if p < 10*time.Second {
			p = 10 * time.Second
		}
		if p > 60*time.Second {
			p = 60 * time.Second
		}
		loopPeriod = p
	}

	ticker := time.NewTicker(loopPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-bot.stopChan:
			return
		case <-ticker.C:
			bot.tradingCycle()
		}
	}
}

// tradingCycle runs one iteration of the trading logic
func (bot *TradingBot) tradingCycle() {
	bot.updatePerformanceIfDue(false, bot.currentProduct)

	// Check if we can trade
	canTrade, reason := bot.riskManager.CanTrade()
	if !canTrade {
		log.Printf("Trading paused: %s", reason)
		return
	}

	// Multi-asset mode: scan all symbols and pick the best
	if bot.cfg.MultiAssetMode {
		bot.multiAssetTradingCycle()
		return
	}

	// Single-asset mode (original behavior)
	bot.mu.RLock()
	candles := make([]delta.Candle, len(bot.candles))
	copy(candles, bot.candles)
	regime := bot.currentRegime
	bot.mu.RUnlock()

	// Get trading signal from strategy
	signal := bot.strategyMgr.GetSignal(candles, regime)

	if signal.Action == strategy.ActionNone {
		return
	}

	// Apply 4H filter and long restrictions (matching Python backtest)
	if canTrade, filterReason := bot.signalFilter.ShouldTrade(signal, candles, regime); !canTrade {
		log.Printf("Signal filtered: %s", filterReason)
		return
	}

	log.Printf("Signal: %s (Side: %s, Confidence: %.2f, Reason: %s)",
		signal.Action, signal.Side, signal.Confidence, signal.Reason)

	// Execute the signal
	bot.executeSignalForSymbol(signal, regime, bot.cfg.Symbol, bot.currentProduct)
}

// multiAssetTradingCycle evaluates all symbols and trades the best opportunity
func (bot *TradingBot) multiAssetTradingCycle() {
	log.Printf("Multi-asset scan: evaluating %d symbols...", len(bot.cfg.Symbols))

	// Collect data for all symbols
	var assets []strategy.AssetData
	var products = make(map[string]*delta.Product)

	for _, symbol := range bot.cfg.Symbols {
		// Fetch candles for this symbol
		candles, err := bot.deltaClient.GetRecentCandles(symbol, bot.cfg.CandleInterval, 200)
		if err != nil {
			log.Printf("  %s: failed to get candles: %v", symbol, err)
			continue
		}

		if len(candles) < 50 {
			log.Printf("  %s: insufficient candles (%d)", symbol, len(candles))
			continue
		}

		product := bot.getProductCached(symbol)
		if product == nil {
			p, err := bot.deltaClient.GetProductBySymbol(symbol)
			if err != nil {
				log.Printf("  %s: failed to get product: %v", symbol, err)
				continue
			}
			bot.setProductCached(symbol, p)
			product = p
		}
		products[symbol] = product

		// Detect regime for this symbol
		hmmResp, err := bot.hmmClient.DetectRegime(candles, symbol)
		if err != nil {
			log.Printf("  %s: HMM detection failed: %v", symbol, err)
			continue
		}

		log.Printf("  %s: regime=%s, hmm_conf=%.2f", symbol, hmmResp.Regime, hmmResp.Confidence)

		assets = append(assets, strategy.AssetData{
			Symbol:  symbol,
			Candles: candles,
			Regime:  hmmResp.Regime,
			HMMConf: hmmResp.Confidence,
		})
	}

	if len(assets) == 0 {
		log.Println("No valid assets to evaluate")
		return
	}

	// Build asset lookup for candles
	assetMap := make(map[string]strategy.AssetData)
	for _, a := range assets {
		assetMap[a.Symbol] = a
	}

	// Create aggregator and find best signal
	aggregator := strategy.NewSignalAggregator(bot.strategyMgr)
	bestSignal := aggregator.SelectBestSignal(assets)

	if bestSignal == nil {
		log.Println("No qualifying signals found across all assets")
		return
	}

	// Apply 4H filter and long restrictions (matching Python backtest)
	assetData := assetMap[bestSignal.Symbol]
	if canTrade, filterReason := bot.signalFilter.ShouldTrade(bestSignal.Signal, assetData.Candles, bestSignal.Regime); !canTrade {
		log.Printf("Signal filtered for %s: %s", bestSignal.Symbol, filterReason)
		return
	}

	// Execute the best signal
	product := products[bestSignal.Symbol]
	if product == nil {
		log.Printf("Product not found for %s", bestSignal.Symbol)
		return
	}

	log.Printf("EXECUTING: %s %s (score: %.3f, regime: %s)",
		bestSignal.Symbol, bestSignal.Signal.Side, bestSignal.TotalScore, bestSignal.Regime)

	bot.executeSignalForSymbol(bestSignal.Signal, bestSignal.Regime, bestSignal.Symbol, product)
}

// executeSignalForSymbol executes a trading signal for a specific symbol
func (bot *TradingBot) executeSignalForSymbol(signal strategy.Signal, regime delta.MarketRegime, symbol string, product *delta.Product) {
	// Get current balance for position sizing
	settling := "USDT"
	if product != nil && product.SettlingAsset.Symbol != "" {
		settling = product.SettlingAsset.Symbol
	}
	balance, err := bot.deltaClient.GetAvailableBalance(settling)
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
		return
	}
	bot.riskManager.UpdateBalance(balance)

	// Minimum confidence threshold
	minConfidence := 0.5
	if regime == delta.RegimeHighVol {
		minConfidence = 0.6 // Higher bar for volatile markets
	}

	if signal.Confidence < minConfidence {
		log.Printf("Signal confidence %.2f below threshold %.2f, skipping", signal.Confidence, minConfidence)
		return
	}

	switch signal.Action {
	case strategy.ActionBuy, strategy.ActionSell:
		bot.executeTradeForSymbol(signal, regime, balance, symbol, product)
	case strategy.ActionClose:
		bot.closePositions(signal.Side)
	case strategy.ActionReduceSize:
		log.Printf("Capital preservation mode - reducing exposure")
		// Could implement partial position close here
	}
}

// executeTradeForSymbol places a trade for a specific symbol
func (bot *TradingBot) executeTradeForSymbol(signal strategy.Signal, regime delta.MarketRegime, balance float64, symbol string, product *delta.Product) {
	stopLoss := signal.StopLoss
	if stopLoss <= 0 {
		stopLoss = bot.riskManager.CalculateStopLoss(signal.Price, signal.Side, 0, regime)
	}
	takeProfit := signal.TakeProfit
	if takeProfit <= 0 {
		takeProfit = bot.riskManager.CalculateTakeProfit(signal.Price, stopLoss, signal.Side, regime)
	}

	// Calculate position size using risk manager
	size := bot.riskManager.CalculatePositionSize(
		balance,
		signal.Price,
		stopLoss,
		regime,
		product,
	)

	if size <= 0 {
		log.Printf("Calculated position size is 0, skipping trade")
		return
	}

	// Set leverage for this product
	if err := bot.deltaClient.SetLeverage(product.ID, bot.cfg.Leverage); err != nil {
		log.Printf("Warning: failed to set leverage for %s: %v", symbol, err)
	}

	// Round SL/TP prices to valid tick size
	slPrice, _ := delta.RoundToTickSize(stopLoss, product.TickSize)
	tpPrice, _ := delta.RoundToTickSize(takeProfit, product.TickSize)

	// Create order request with bracket (stop loss + take profit)
	// NOTE: Delta API expects only one of product_id or product_symbol, not both
	order := &delta.OrderRequest{
		ProductID:              product.ID,
		Size:                   size,
		Side:                   signal.Side,
		BracketStopLossPrice:   slPrice,
		BracketTakeProfitPrice: tpPrice,
	}

	log.Printf("Placing limit order: %s %s, size=%d, SL=%.2f, TP=%.2f",
		symbol, signal.Side, size, stopLoss, takeProfit)

	// Use limit order with 5-second timeout, fallback to market
	result, err := bot.deltaClient.PlaceLimitOrderWithFallback(order, symbol, 5)
	if err != nil {
		log.Printf("Failed to place order: %v", err)
		return
	}

	orderType := "limit"
	if result.OrderType == "market_order" {
		orderType = "market (fallback)"
	}
	log.Printf("Order placed successfully: ID=%d, Type=%s, State=%s", result.ID, orderType, result.State)
	bot.riskManager.RecordTrade()
}

// closePositions closes all positions for a side
func (bot *TradingBot) closePositions(side string) {
	positions, err := bot.deltaClient.GetPositions()
	if err != nil {
		log.Printf("Failed to get positions: %v", err)
		return
	}

	for _, pos := range positions {
		if pos.Size == 0 {
			continue
		}
		if side == "buy" && pos.Size > 0 {
			continue
		}
		if side == "sell" && pos.Size < 0 {
			continue
		}

		if pos.Size != 0 {
			log.Printf("Closing position: %s size=%d", pos.ProductSymbol, pos.Size)

			// Pass position side (not close side) - ClosePosition will determine the correct close side
			positionSide := "buy" // long position
			if pos.Size < 0 {
				positionSide = "sell" // short position
			}

			err := bot.deltaClient.ClosePosition(
				pos.ProductSymbol,
				pos.ProductID,
				absInt(pos.Size),
				positionSide,
			)
			if err != nil {
				log.Printf("Failed to close position: %v", err)
			}
		}
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// regimeUpdateLoop periodically updates the market regime
func (bot *TradingBot) regimeUpdateLoop() {
	ticker := time.NewTicker(bot.cfg.RegimeCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-bot.stopChan:
			return
		case <-ticker.C:
			if err := bot.updateMarketRegime(); err != nil {
				log.Printf("Failed to update regime: %v", err)
			}
		}
	}
}

// updateMarketRegime calls HMM to detect current market regime
func (bot *TradingBot) updateMarketRegime() error {
	bot.mu.RLock()
	candles := make([]delta.Candle, len(bot.candles))
	copy(candles, bot.candles)
	bot.mu.RUnlock()

	if len(candles) < 50 {
		return fmt.Errorf("insufficient candles for regime detection")
	}

	resp, err := bot.hmmClient.DetectRegimeWithRetry(candles, bot.cfg.Symbol, 3)
	if err != nil {
		return err
	}

	bot.mu.Lock()
	bot.currentRegime = resp.Regime
	bot.mu.Unlock()

	log.Printf("Market regime updated: %s (confidence: %.2f, volatility: %.4f, trend: %.4f)",
		resp.Regime, resp.Confidence, resp.Features.Volatility, resp.Features.Trend)

	return nil
}

// handleTicker handles incoming ticker updates
func (bot *TradingBot) handleTicker(ticker delta.Ticker) {
	bot.mu.Lock()
	bot.lastTicker = &ticker
	bot.mu.Unlock()
}

// handleCandle handles incoming candle updates
func (bot *TradingBot) handleCandle(candle delta.Candle) {
	bot.mu.Lock()
	defer bot.mu.Unlock()

	// Add or update candle
	if len(bot.candles) > 0 {
		lastCandle := &bot.candles[len(bot.candles)-1]
		if candle.Time == lastCandle.Time {
			// Update existing candle
			bot.candles[len(bot.candles)-1] = candle
		} else if candle.Time > lastCandle.Time {
			// New candle
			bot.candles = append(bot.candles, candle)
			// Keep only last 500 candles
			if len(bot.candles) > 500 {
				bot.candles = bot.candles[len(bot.candles)-500:]
			}
		}
	} else {
		bot.candles = append(bot.candles, candle)
	}
}

// handleWSError handles WebSocket errors
func (bot *TradingBot) handleWSError(err error) {
	log.Printf("WebSocket error: %v", err)
}

// Stop gracefully stops the bot (idempotent - safe to call multiple times)
func (bot *TradingBot) Stop() {
	bot.stopOnce.Do(func() {
		log.Println("Stopping trading bot...")
		bot.mu.Lock()
		bot.isRunning = false
		bot.mu.Unlock()
		close(bot.stopChan)
		bot.wsClient.Close()
		bot.deltaClient.Close()
		log.Println("Bot stopped")
	})
}

// GetStatus returns current bot status
func (bot *TradingBot) GetStatus() map[string]interface{} {
	bot.mu.RLock()
	defer bot.mu.RUnlock()

	return map[string]interface{}{
		"is_running":    bot.isRunning,
		"symbol":        bot.cfg.Symbol,
		"regime":        string(bot.currentRegime),
		"candles_count": len(bot.candles),
		"ws_connected":  bot.wsClient.IsConnected(),
		"risk_metrics":  bot.riskManager.GetRiskMetrics(),
		"performance":   bot.perfTracker.Report(),
	}
}

func (bot *TradingBot) updatePerformanceIfDue(force bool, product *delta.Product) {
	if !force && time.Since(bot.lastPerfUpdate) < 60*time.Second {
		return
	}

	equity, eqErr := bot.deltaClient.GetNetEquity()
	if eqErr != nil {
		settling := "USDT"
		if product != nil && product.SettlingAsset.Symbol != "" {
			settling = product.SettlingAsset.Symbol
		}
		if bal, err := bot.deltaClient.GetAvailableBalance(settling); err == nil {
			equity = bal
		} else {
			return
		}
	}

	positions, err := bot.deltaClient.GetPositions()
	if err != nil {
		return
	}

	realized := 0.0
	unrealized := 0.0
	open := 0
	for _, p := range positions {
		if p.Size != 0 {
			open++
		}
		realized += parseFloatOrZero(p.RealizedPnL)
		unrealized += parseFloatOrZero(p.UnrealizedPnL)
	}

	bot.perfTracker.Record(PerformanceSnapshot{
		Timestamp:     time.Now(),
		Equity:        equity,
		RealizedPnL:   realized,
		UnrealizedPnL: unrealized,
		Positions:     open,
	})
	bot.lastPerfUpdate = time.Now()

	report := bot.perfTracker.Report()
	log.Printf("Performance: equity=%.2f pnl=%.2f (%.2f%%) positions=%d", report["last_equity"], report["pnl_abs"], report["pnl_pct"], open)
}

func (bot *TradingBot) getProductCached(symbol string) *delta.Product {
	bot.mu.RLock()
	defer bot.mu.RUnlock()
	return bot.productCache[symbol]
}

func (bot *TradingBot) setProductCached(symbol string, p *delta.Product) {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	bot.productCache[symbol] = p
}

func parseFloatOrZero(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func resolutionToDuration(resolution string) time.Duration {
	switch resolution {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "2h":
		return 2 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "1d":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	default:
		return 0
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Delta Exchange Trading Bot v1.0")

	// Load configuration
	cfg := config.LoadConfig()

	if cfg.APIKey == "" || cfg.APISecret == "" {
		log.Fatal("DELTA_API_KEY and DELTA_API_SECRET environment variables are required")
	}

	// Create and initialize bot
	bot := NewTradingBot(cfg)
	if err := bot.Initialize(); err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Start trading
	if err := bot.Start(); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Graceful shutdown
	bot.Stop()
}
