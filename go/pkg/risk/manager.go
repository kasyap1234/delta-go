package risk

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/kasyap/delta-go/go/config"
	"github.com/kasyap/delta-go/go/pkg/delta"
)

// RiskManager handles position sizing and risk controls
type RiskManager struct {
	cfg *config.Config

	// State tracking
	mu              sync.RWMutex
	peakBalance     float64
	currentBalance  float64
	currentDrawdown float64
	lastTradeTime   time.Time

	// Daily loss tracking (Fix #1)
	dailyStartBalance float64
	dailyPnL          float64
	dailyLossLimit    float64 // -5% default
	currentDay        time.Time

	// Circuit breaker
	isCircuitBroken     bool
	circuitBrokenAt     time.Time
	isDailyLimitHit     bool
	dailyLimitResetTime time.Time
}

// NewRiskManager creates a new risk manager
func NewRiskManager(cfg *config.Config) *RiskManager {
	return &RiskManager{
		cfg:            cfg,
		dailyLossLimit: -5.0, // -5% daily loss limit
		currentDay:     time.Now().Truncate(24 * time.Hour),
	}
}

// UpdateBalance updates the current balance and calculates drawdown
func (rm *RiskManager) UpdateBalance(balance float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if new day
	today := time.Now().Truncate(24 * time.Hour)
	if today.After(rm.currentDay) {
		// New day - reset daily tracking
		rm.currentDay = today
		rm.dailyStartBalance = balance
		rm.dailyPnL = 0
		rm.isDailyLimitHit = false
		log.Printf("New trading day started. Daily balance reset to %.2f", balance)
	}

	// Initialize daily start balance if not set
	if rm.dailyStartBalance == 0 {
		rm.dailyStartBalance = balance
	}

	rm.currentBalance = balance

	// Calculate daily P&L percentage
	if rm.dailyStartBalance > 0 {
		rm.dailyPnL = ((balance - rm.dailyStartBalance) / rm.dailyStartBalance) * 100
	}

	// Check daily loss limit
	if rm.dailyPnL <= rm.dailyLossLimit && !rm.isDailyLimitHit {
		rm.isDailyLimitHit = true
		rm.dailyLimitResetTime = today.Add(24 * time.Hour)
		log.Printf("DAILY LOSS LIMIT HIT: Daily P&L %.2f%% exceeds limit %.2f%%. Trading paused until %v",
			rm.dailyPnL, rm.dailyLossLimit, rm.dailyLimitResetTime)
	}

	if balance > rm.peakBalance {
		rm.peakBalance = balance
	}

	if rm.peakBalance > 0 {
		rm.currentDrawdown = (rm.peakBalance - balance) / rm.peakBalance * 100
	}

	// Check overall circuit breaker
	if rm.currentDrawdown >= rm.cfg.MaxDrawdownPct {
		if !rm.isCircuitBroken {
			rm.isCircuitBroken = true
			rm.circuitBrokenAt = time.Now()
			log.Printf("CIRCUIT BREAKER TRIGGERED: Drawdown %.2f%% exceeds max %.2f%%",
				rm.currentDrawdown, rm.cfg.MaxDrawdownPct)
		}
	}
}

// CanTrade checks if trading is allowed
func (rm *RiskManager) CanTrade() (bool, string) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Check daily loss limit first (Fix #1)
	if rm.isDailyLimitHit {
		if time.Now().After(rm.dailyLimitResetTime) {
			// Reset at start of new day
			return true, ""
		}
		hoursRemaining := rm.dailyLimitResetTime.Sub(time.Now()).Hours()
		return false, fmt.Sprintf("daily loss limit hit (%.2f%%), resets in %.1f hours",
			rm.dailyPnL, hoursRemaining)
	}

	if rm.isCircuitBroken {
		// Auto-reset after 24 hours
		if time.Since(rm.circuitBrokenAt) > 24*time.Hour {
			return true, ""
		}
		return false, fmt.Sprintf("circuit breaker active (%.1f hours remaining)",
			24-time.Since(rm.circuitBrokenAt).Hours())
	}

	return true, ""
}

// CalculatePositionSize calculates the position size based on risk parameters and market regime
func (rm *RiskManager) CalculatePositionSize(
	balance float64,
	entryPrice float64,
	stopLossPrice float64,
	regime delta.MarketRegime,
	product *delta.Product,
) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Base risk per trade
	riskAmount := balance * (rm.cfg.RiskPerTradePct / 100)

	// Adjust risk based on regime
	regimeMultiplier := rm.getRegimeMultiplier(regime)
	adjustedRisk := riskAmount * regimeMultiplier

	// Calculate risk per contract
	riskPerContract := math.Abs(entryPrice - stopLossPrice)
	if riskPerContract <= 0 {
		// Use default stop loss percentage if no stop provided
		riskPerContract = entryPrice * (rm.cfg.StopLossPct / 100)
	}

	// Calculate number of contracts
	contracts := adjustedRisk / riskPerContract

	// Apply leverage
	contracts = contracts * float64(rm.cfg.Leverage)

	// Round down to integer
	size := int(math.Floor(contracts))

	// Apply max position limit
	maxSize := rm.calculateMaxSize(balance, entryPrice, product)
	if size > maxSize {
		size = maxSize
	}

	// Minimum size
	if size < 1 {
		size = 1
	}

	return size
}

// getRegimeMultiplier returns position size multiplier based on market regime
func (rm *RiskManager) getRegimeMultiplier(regime delta.MarketRegime) float64 {
	switch regime {
	case delta.RegimeBull:
		return 1.2 // Slightly larger in bull markets
	case delta.RegimeBear:
		return 0.8 // More conservative in bear markets
	case delta.RegimeRanging:
		return 1.0 // Normal in ranging
	case delta.RegimeHighVol:
		return 0.5 // Much smaller in high volatility
	case delta.RegimeLowVol:
		return 1.0 // Normal in low volatility
	default:
		return 1.0
	}
}

// calculateMaxSize calculates maximum position size based on account limits
func (rm *RiskManager) calculateMaxSize(balance float64, price float64, product *delta.Product) int {
	// Max position as percentage of balance
	maxValue := balance * (rm.cfg.MaxPositionPct / 100) * float64(rm.cfg.Leverage)

	// Parse contract value
	contractValue := 1.0
	if product != nil && product.ContractValue != "" {
		if cv, err := strconv.ParseFloat(product.ContractValue, 64); err == nil {
			contractValue = cv
		}
	}

	// Calculate max contracts
	maxSize := int(maxValue / (price * contractValue))

	return maxSize
}

// CalculateStopLoss calculates stop loss price based on ATR or percentage
func (rm *RiskManager) CalculateStopLoss(
	entryPrice float64,
	side string, // "buy" or "sell"
	atr float64, // Average True Range
	regime delta.MarketRegime,
) float64 {
	// Base stop distance
	stopPct := rm.cfg.StopLossPct / 100

	// Adjust based on regime
	switch regime {
	case delta.RegimeHighVol:
		stopPct *= 1.5 // Wider stops in high volatility
	case delta.RegimeLowVol:
		stopPct *= 0.8 // Tighter stops in low volatility
	}

	// Use ATR if provided (typical: 2-3x ATR)
	if atr > 0 {
		atrStop := (2.0 * atr) / entryPrice
		if atrStop > stopPct {
			stopPct = atrStop
		}
	}

	if side == "buy" {
		return entryPrice * (1 - stopPct)
	}
	return entryPrice * (1 + stopPct)
}

// CalculateTakeProfit calculates take profit price
func (rm *RiskManager) CalculateTakeProfit(
	entryPrice float64,
	stopLossPrice float64,
	side string,
	regime delta.MarketRegime,
) float64 {
	// Calculate reward/risk ratio based on regime
	rewardRatio := 2.0 // Default 2:1

	switch regime {
	case delta.RegimeBull:
		if side == "buy" {
			rewardRatio = 3.0 // Let winners run in bull
		} else {
			rewardRatio = 1.5 // Shorter targets for shorts
		}
	case delta.RegimeBear:
		if side == "sell" {
			rewardRatio = 3.0 // Let winners run in bear
		} else {
			rewardRatio = 1.5 // Shorter targets for longs
		}
	case delta.RegimeRanging:
		rewardRatio = 1.5 // Smaller targets in ranging
	case delta.RegimeHighVol:
		rewardRatio = 1.0 // Quick profits in high vol
	}

	// Calculate take profit
	riskDistance := math.Abs(entryPrice - stopLossPrice)
	rewardDistance := riskDistance * rewardRatio

	if side == "buy" {
		return entryPrice + rewardDistance
	}
	return entryPrice - rewardDistance
}

// GetRiskMetrics returns current risk metrics
func (rm *RiskManager) GetRiskMetrics() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return map[string]interface{}{
		"current_balance":  rm.currentBalance,
		"peak_balance":     rm.peakBalance,
		"current_drawdown": rm.currentDrawdown,
		"max_drawdown":     rm.cfg.MaxDrawdownPct,
		"circuit_broken":   rm.isCircuitBroken,
		"last_trade_time":  rm.lastTradeTime,
	}
}

// RecordTrade records a trade execution
func (rm *RiskManager) RecordTrade() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.lastTradeTime = time.Now()
}

// ResetCircuitBreaker manually resets the circuit breaker
func (rm *RiskManager) ResetCircuitBreaker() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.isCircuitBroken = false
	rm.peakBalance = rm.currentBalance
	log.Println("Circuit breaker manually reset")
}
