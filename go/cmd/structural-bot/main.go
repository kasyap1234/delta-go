package main

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/kasyap/delta-go/go/config"
	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/features"
	"github.com/kasyap/delta-go/go/pkg/logger"
	"github.com/kasyap/delta-go/go/pkg/risk"
	"github.com/kasyap/delta-go/go/pkg/strategy"
)

type ScalpPosition struct {
	Symbol     string
	Side       string
	Size       int
	EntryTime  time.Time
	EntryPrice float64
	OrderID    int64
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

type StructuralBot struct {
	cfg            *config.Config
	deltaClient    *delta.Client
	wsClient       *delta.WebSocketClient
	riskManager    *risk.RiskManager
	driverSelector *strategy.DriverSelector
	perfTracker    *PerformanceTracker

	mu                  sync.RWMutex
	currentProduct      *delta.Product
	candles             map[string][]delta.Candle
	lastTickers         map[string]*delta.Ticker
	lastOrderbooks      map[string]*delta.Orderbook
	lastFeatures        map[string]features.MarketFeatures
	scalpPositions      map[string]*ScalpPosition
	basisPositions      map[string]bool
	gridOrderIDToSymbol map[int64]string
	activeGridSymbol    string
	isRunning           bool
	stopChan            chan struct{}
	stopOnce            sync.Once
	lastPerfUpdate      time.Time
	productCache        map[string]*delta.Product
}

func NewStructuralBot(cfg *config.Config) *StructuralBot {
	driverCfg := strategy.DriverSelectorConfig{
		ScalperConfig: strategy.ScalperConfig{
			ImbalanceThreshold:   cfg.ScalpImbalanceThreshold,
			PersistenceSnapshots: cfg.ScalpPersistenceCount,
			TargetProfitBps:      cfg.ScalpTargetBps,
			MaxLossBps:           cfg.ScalpMaxLossBps,
			MinSpreadBps:         1.0,
			MaxSpreadBps:         10.0,
			ScalpWindowBTC:       30 * time.Minute,
			ScalpWindowOther:     15 * time.Minute,
			ConfirmationPricePct: 0.02,
			Enabled:              cfg.ScalperEnabled,
		},
		FundingConfig: strategy.FundingArbitrageConfig{
			EntryThresholdAnnualized: cfg.BasisEntryThreshold,
			ExitThresholdAnnualized:  cfg.BasisExitThreshold,
			MaxHoldingHours:          24,
			MaxPositionPct:           33.0,
			Enabled:                  cfg.BasisTradeEnabled,
		},
		GridConfig: strategy.DefaultGridConfig(),
	}

	return &StructuralBot{
		cfg:                 cfg,
		deltaClient:         delta.NewClient(cfg),
		wsClient:            delta.NewWebSocketClient(cfg),
		riskManager:         risk.NewRiskManager(cfg),
		driverSelector:      strategy.NewDriverSelector(driverCfg),
		perfTracker:         NewPerformanceTracker(500),
		candles:             make(map[string][]delta.Candle),
		lastTickers:         make(map[string]*delta.Ticker),
		lastOrderbooks:      make(map[string]*delta.Orderbook),
		lastFeatures:        make(map[string]features.MarketFeatures),
		scalpPositions:      make(map[string]*ScalpPosition),
		basisPositions:      make(map[string]bool),
		gridOrderIDToSymbol: make(map[int64]string),
		activeGridSymbol:    "",
		stopChan:            make(chan struct{}),
		productCache:        make(map[string]*delta.Product),
	}
}

func (bot *StructuralBot) Initialize() error {
	log.Println("Initializing structural trading bot...")

	for _, symbol := range bot.cfg.Symbols {
		product, err := bot.deltaClient.GetProductBySymbol(symbol)
		if err != nil {
			log.Printf("Warning: failed to get product for %s: %v", symbol, err)
			continue
		}

		bot.productCache[symbol] = product
		if bot.currentProduct == nil {
			bot.currentProduct = product
		}
		log.Printf("Loaded product: %s (ID: %d)", symbol, product.ID)

		if err := bot.deltaClient.SetLeverage(product.ID, bot.cfg.Leverage); err != nil {
			log.Printf("Warning: failed to set leverage for %s: %v", symbol, err)
		}

		candles, err := bot.deltaClient.GetRecentCandles(symbol, bot.cfg.CandleInterval, 200)
		if err != nil {
			log.Printf("Warning: failed to get initial candles for %s: %v", symbol, err)
			continue
		}
		bot.candles[symbol] = candles

		orderbook, err := bot.deltaClient.GetOrderbook(symbol)
		if err == nil {
			bot.lastOrderbooks[symbol] = orderbook
		}

		ticker, err := bot.deltaClient.GetTicker(symbol)
		if err == nil {
			bot.lastTickers[symbol] = ticker
		}
	}

	if len(bot.productCache) == 0 {
		return fmt.Errorf("failed to initialize any products")
	}

	return nil
}

func (bot *StructuralBot) Start() error {
	bot.mu.Lock()
	if bot.isRunning {
		bot.mu.Unlock()
		return fmt.Errorf("bot already running")
	}
	bot.isRunning = true
	bot.mu.Unlock()

	bot.wsClient.OnTicker(bot.handleTicker)
	bot.wsClient.OnCandleWithSymbol(bot.handleCandleWithSymbol)
	bot.wsClient.OnOrderbook(bot.handleOrderbook)
	bot.wsClient.OnError(bot.handleWSError)

	if err := bot.wsClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect websocket: %w", err)
	}

	for _, symbol := range bot.cfg.Symbols {
		bot.wsClient.SubscribeTicker(symbol)
		bot.wsClient.SubscribeCandles(symbol, bot.cfg.CandleInterval)
		bot.wsClient.SubscribeOrderbook(symbol)
		bot.wsClient.SubscribeFundingRate([]string{symbol})
	}

	go bot.tradingLoop()
	go bot.featureUpdateLoop()
	go bot.scalpExitMonitor()
	go bot.gridFillMonitor()

	log.Printf("Structural bot started - Symbols: %v", bot.cfg.Symbols)
	return nil
}

func (bot *StructuralBot) tradingLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-bot.stopChan:
			return
		case <-ticker.C:
			bot.evaluateAndTrade()
		}
	}
}

func (bot *StructuralBot) featureUpdateLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-bot.stopChan:
			return
		case <-ticker.C:
			bot.updateFeatures()
		}
	}
}

func (bot *StructuralBot) scalpExitMonitor() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-bot.stopChan:
			return
		case <-ticker.C:
			bot.checkScalpExits()
		}
	}
}

func (bot *StructuralBot) gridFillMonitor() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-bot.stopChan:
			return
		case <-ticker.C:
			bot.checkGridFills()
		}
	}
}

func (bot *StructuralBot) updateFeatures() {
	bot.mu.RLock()
	tickersMap := make(map[string]*delta.Ticker)
	orderbooksMap := make(map[string]*delta.Orderbook)
	candlesMap := make(map[string][]delta.Candle)
	for sym, tick := range bot.lastTickers {
		tickersMap[sym] = tick
	}
	for sym, ob := range bot.lastOrderbooks {
		orderbooksMap[sym] = ob
	}
	for sym, candles := range bot.candles {
		candlesMap[sym] = make([]delta.Candle, len(candles))
		copy(candlesMap[sym], candles)
	}
	bot.mu.RUnlock()

	engine := bot.driverSelector.GetFeatureEngine()
	for _, symbol := range bot.cfg.Symbols {
		tick := tickersMap[symbol]
		ob := orderbooksMap[symbol]
		candles := candlesMap[symbol]

		if tick == nil || len(candles) < 20 {
			continue
		}

		f := engine.ComputeFeaturesWithFunding(ob, tick, candles)

		bot.mu.Lock()
		bot.lastFeatures[symbol] = f
		bot.mu.Unlock()
	}
}

func (bot *StructuralBot) evaluateAndTrade() {
	bot.mu.RLock()
	featuresMap := make(map[string]features.MarketFeatures)
	candlesMap := make(map[string][]delta.Candle)
	productsMap := make(map[string]*delta.Product)
	scalpHasPosition := len(bot.scalpPositions) > 0
	basisHasPosition := len(bot.basisPositions) > 0
	for sym, f := range bot.lastFeatures {
		featuresMap[sym] = f
	}
	for sym, candles := range bot.candles {
		candlesMap[sym] = make([]delta.Candle, len(candles))
		copy(candlesMap[sym], candles)
	}
	for sym, prod := range bot.productCache {
		productsMap[sym] = prod
	}
	bot.mu.RUnlock()

	canTrade, reason := bot.riskManager.CanTrade()
	if !canTrade {
		log.Printf("Trading blocked: %s", reason)
		return
	}

	for _, symbol := range bot.cfg.Symbols {
		f, ok := featuresMap[symbol]
		if !ok || len(candlesMap[symbol]) < 50 {
			continue
		}

		product, ok := productsMap[symbol]
		if !ok {
			continue
		}

		if scalpHasPosition || basisHasPosition {
			continue
		}

		candles := candlesMap[symbol]
		selected, signal := bot.driverSelector.SelectStrategy(f, candles)

		if signal.Action == strategy.ActionNone {
			continue
		}

		log.Printf("[%s] Signal: %s %s (strategy=%s, driver=%s, confidence=%.2f)",
			symbol, signal.Action, signal.Side, selected.Name, selected.Driver, signal.Confidence)

		switch selected.Name {
		case "fee_aware_scalper":
			bot.executeScalpEntry(signal, product, symbol)
		case "funding_arbitrage":
			bot.executeFundingArbEntry(signal, product, symbol)
		case "grid_trading":
			bot.executeGridEntry(signal, product, symbol)
		}

		bot.updatePerformanceIfDue(false, product)
		return
	}
}

func (bot *StructuralBot) executeScalpEntry(signal strategy.Signal, product *delta.Product, symbol string) {
	scalper := bot.driverSelector.GetScalper()
	if scalper == nil || !scalper.IsEnabled() {
		return
	}

	balance, err := bot.deltaClient.GetAvailableBalance("USDT")
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
		return
	}

	positionValue := balance * (bot.cfg.MaxPositionPct / 100) * float64(bot.cfg.Leverage)
	size, err := delta.NotionalToContracts(positionValue, signal.Price, product)
	if err != nil {
		log.Printf("Failed to calculate scalp size: %v", err)
		return
	}
	if size < 1 {
		size = 1
	}

	slPrice, _ := delta.RoundToTickSize(signal.StopLoss, product.TickSize)
	tpPrice, _ := delta.RoundToTickSize(signal.TakeProfit, product.TickSize)

	req := &delta.OrderRequest{
		ProductID:              product.ID,
		Size:                   size,
		Side:                   signal.Side,
		OrderType:              "limit_order",
		LimitPrice:             fmt.Sprintf("%.2f", signal.Price),
		BracketStopLossPrice:   slPrice,
		BracketTakeProfitPrice: tpPrice,
		TimeInForce:            "gtc",
	}

	order, err := bot.deltaClient.PlaceOrder(req)
	if err != nil {
		log.Printf("Failed to place scalp order: %v", err)
		return
	}

	bot.mu.Lock()
	bot.scalpPositions[symbol] = &ScalpPosition{
		Symbol:     symbol,
		Side:       signal.Side,
		Size:       size,
		EntryTime:  time.Now(),
		EntryPrice: signal.Price,
		OrderID:    order.ID,
	}
	bot.mu.Unlock()

	// Track entry in scalper for fee windows
	scalper.RecordEntry(symbol)

	log.Printf("[%s] Scalp entry: %s %d contracts @ %.2f (SL: %s, TP: %s)",
		symbol, signal.Side, size, signal.Price, slPrice, tpPrice)
}

func (bot *StructuralBot) executeFundingArbEntry(signal strategy.Signal, product *delta.Product, symbol string) {
	fundingArb := bot.driverSelector.GetFundingArb()
	if fundingArb == nil || !fundingArb.IsEnabled() {
		return
	}

	balance, err := bot.deltaClient.GetAvailableBalance("USDT")
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
		return
	}

	targetNotional := balance * (bot.cfg.MaxPositionPct / 100) * 5.0
	perpSize, err := delta.NotionalToContracts(targetNotional, signal.Price, product)
	if err != nil {
		log.Printf("Failed to calculate funding arb size: %v", err)
		return
	}
	if perpSize < 1 {
		perpSize = 1
	}

	// Hedge Execution Logic (Futures Basis Arb)
	if signal.IsHedged {
		futureProduct, err := bot.deltaClient.GetFuturesProductForPerp(product.Symbol)
		if err != nil {
			log.Printf("PURE ARBITRAGE BLOCKED: Could not find futures product for %s: %v", product.Symbol, err)
			return // ABORT to ensure pure arbitrage
		}

		futureSize, err := delta.NotionalToContracts(targetNotional, signal.Price, futureProduct)
		if err != nil {
			log.Printf("PURE ARBITRAGE BLOCKED: Failed to calculate future size: %v", err)
			return
		}

		// Place Future Long Order
		// Marketable limit order
		futurePriceStr, _ := delta.RoundToTickSize(signal.Price*1.01, futureProduct.TickSize)
		futureReq := &delta.OrderRequest{
			ProductID:   futureProduct.ID,
			Size:        futureSize,
			Side:        "buy",
			OrderType:   "limit_order",
			LimitPrice:  futurePriceStr,
			TimeInForce: "ioc",
		}

		futureOrder, err := bot.deltaClient.PlaceOrder(futureReq)
		if err != nil {
			log.Printf("PURE ARBITRAGE BLOCKED: Failed to place Future Buy: %v", err)
			return
		}
		log.Printf("[%s] HEDGE: Future Buy Placed: %d contracts (Order: %d)", futureProduct.Symbol, futureSize, futureOrder.ID)
	}

	req := &delta.OrderRequest{
		ProductID:   product.ID,
		Size:        perpSize,
		Side:        signal.Side,
		OrderType:   "limit_order",
		LimitPrice:  fmt.Sprintf("%.2f", signal.Price),
		TimeInForce: "gtc",
	}

	order, err := bot.deltaClient.PlaceOrder(req)
	if err != nil {
		log.Printf("Failed to place funding arb order: %v", err)
		return
	}

	bot.mu.Lock()
	bot.basisPositions[symbol] = true
	bot.mu.Unlock()

	fundingArb.RecordEntry(symbol, signal.Side, 0.0)
	log.Printf("[%s] Funding Arb entry: %s %d contracts @ %.2f (Order ID: %d)", symbol, signal.Side, perpSize, signal.Price, order.ID)
}

func (bot *StructuralBot) executeGridEntry(signal strategy.Signal, product *delta.Product, symbol string) {
	gridTrader := bot.driverSelector.GetGridTrader()
	if gridTrader == nil || !gridTrader.IsEnabled() {
		return
	}

	levels := gridTrader.GetLevels()
	if len(levels) == 0 {
		log.Printf("[%s] Grid trading activated but no levels calculated", symbol)
		return
	}

	balance, err := bot.deltaClient.GetAvailableBalance("USDT")
	if err != nil {
		log.Printf("Failed to get balance for grid: %v", err)
		return
	}

	totalGridNotional := balance * 0.05 * float64(bot.cfg.Leverage)
	sizePerLevel, err := delta.NotionalToContracts(totalGridNotional, levels[0].Price, product)
	if err != nil {
		log.Printf("Failed to calculate grid size: %v", err)
		return
	}
	if sizePerLevel < 1 {
		sizePerLevel = 1
	}

	placedOrders := 0
	for _, level := range levels {
		if !level.IsActive {
			continue
		}

		priceStr, _ := delta.RoundToTickSize(level.Price, product.TickSize)

		req := &delta.OrderRequest{
			ProductID:   product.ID,
			Size:        sizePerLevel,
			Side:        level.Side,
			OrderType:   "limit_order",
			LimitPrice:  priceStr,
			TimeInForce: "gtc",
		}

		order, err := bot.deltaClient.PlaceOrder(req)
		if err != nil {
			log.Printf("[%s] Failed to place grid order at %s: %v", symbol, priceStr, err)
			continue
		}

		bot.mu.Lock()
		bot.gridOrderIDToSymbol[order.ID] = symbol
		bot.activeGridSymbol = symbol
		bot.mu.Unlock()
		placedOrders++
	}

	log.Printf("[%s] Grid trading activated: placed %d/%d orders (size: %d contracts)", symbol, placedOrders, len(levels), sizePerLevel)
}

func (bot *StructuralBot) checkScalpExits() {
	scalper := bot.driverSelector.GetScalper()
	if scalper == nil {
		return
	}

	bot.mu.RLock()
	positions := make([]*ScalpPosition, 0, len(bot.scalpPositions))
	for _, p := range bot.scalpPositions {
		positions = append(positions, p)
	}
	bot.mu.RUnlock()

	for _, pos := range positions {
		feeWindowActive := scalper.ShouldCloseForFees(pos.Symbol)
		timeRemaining := scalper.GetFeeWindow(pos.Symbol) - time.Since(pos.EntryTime)

		if timeRemaining < 30*time.Second && timeRemaining > 0 && feeWindowActive {
			log.Printf("Fee window expiring in %v for %s - consider closing", timeRemaining, pos.Symbol)
		}
	}
}

func (bot *StructuralBot) checkGridFills() {
	bot.mu.RLock()
	gridOrderIDs := make([]int64, 0, len(bot.gridOrderIDToSymbol))
	for orderID := range bot.gridOrderIDToSymbol {
		gridOrderIDs = append(gridOrderIDs, orderID)
	}
	bot.mu.RUnlock()

	if len(gridOrderIDs) == 0 {
		return
	}

	gridTrader := bot.driverSelector.GetGridTrader()
	if gridTrader == nil {
		return
	}

	for _, orderID := range gridOrderIDs {
		order, err := bot.deltaClient.GetOrderByID(orderID)
		if err != nil {
			log.Printf("Failed to get grid order %d: %v", orderID, err)
			continue
		}

		if order.State == "filled" || order.State == "closed" {
			signal := gridTrader.OnFill(orderID)
			bot.mu.Lock()
			delete(bot.gridOrderIDToSymbol, orderID)
			bot.mu.Unlock()

			if signal.Action != strategy.ActionNone {
				log.Printf("[GRID] Order %d filled: %s", orderID, signal.Reason)
			}
		}
	}
}

func (bot *StructuralBot) handleTicker(ticker delta.Ticker) {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	bot.lastTickers[ticker.Symbol] = &ticker
}

func (bot *StructuralBot) handleCandleWithSymbol(symbol string, candle delta.Candle) {
	bot.mu.Lock()
	defer bot.mu.Unlock()

	if _, ok := bot.candles[symbol]; !ok {
		bot.candles[symbol] = make([]delta.Candle, 0)
	}

	candles := bot.candles[symbol]
	if len(candles) > 0 {
		lastCandle := &candles[len(candles)-1]
		if candle.Time == lastCandle.Time {
			candles[len(candles)-1] = candle
		} else if candle.Time > lastCandle.Time {
			candles = append(candles, candle)
			if len(candles) > 500 {
				candles = candles[len(candles)-500:]
			}
		}
	} else {
		candles = append(candles, candle)
	}
	bot.candles[symbol] = candles
}

func (bot *StructuralBot) handleOrderbook(data json.RawMessage) {
	var ob delta.Orderbook
	if err := json.Unmarshal(data, &ob); err != nil {
		return
	}
	bot.mu.Lock()
	defer bot.mu.Unlock()
	bot.lastOrderbooks[ob.Symbol] = &ob
}

func (bot *StructuralBot) handleWSError(err error) {
	log.Printf("WebSocket error: %v", err)
}

func (bot *StructuralBot) Stop() {
	bot.stopOnce.Do(func() {
		log.Println("Stopping structural bot...")
		bot.mu.Lock()
		bot.isRunning = false
		bot.mu.Unlock()
		close(bot.stopChan)
		bot.wsClient.Close()
		bot.deltaClient.Close()
		log.Println("Bot stopped")
	})
}

func (bot *StructuralBot) updatePerformanceIfDue(force bool, product *delta.Product) {
	if !force && time.Since(bot.lastPerfUpdate) < 60*time.Second {
		return
	}

	equity, err := bot.deltaClient.GetNetEquity()
	if err != nil {
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

	realized, unrealized := 0.0, 0.0
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
}

func parseFloatOrZero(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func main() {
	cfg := config.LoadConfig()

	// Initialize structured logger
	logCfg := logger.Config{
		FilePath: cfg.LogPath,
		Level:    cfg.LogLevel,
	}
	l, err := logger.New(logCfg)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	slog.SetDefault(l)
	// Disable standard log flags as slog handles them
	log.SetFlags(0)

	slog.Info("Delta Exchange Structural Trading Bot v2.0", "strategy", "Real-time structural drivers (no ML)")

	if cfg.APIKey == "" || cfg.APISecret == "" {
		log.Fatal("DELTA_API_KEY and DELTA_API_SECRET environment variables are required")
	}

	bot := NewStructuralBot(cfg)
	if err := bot.Initialize(); err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	if err := bot.Start(); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	bot.Stop()
}
