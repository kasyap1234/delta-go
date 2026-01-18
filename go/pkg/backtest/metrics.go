package backtest

import (
	"math"
	"time"
)

// Metrics contains all backtest performance metrics
type Metrics struct {
	// Time period
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// Capital
	InitialCapital float64
	FinalEquity    float64

	// Returns
	TotalReturn      float64 // As decimal (0.45 = 45%)
	AnnualizedReturn float64

	// Risk metrics
	MaxDrawdown    float64 // As decimal (0.18 = 18%)
	MaxDrawdownDur time.Duration
	Volatility     float64 // Annualized volatility
	SharpeRatio    float64 // Risk-free rate assumed 0 for crypto
	SortinoRatio   float64 // Downside deviation only
	CalmarRatio    float64 // Return / MaxDrawdown

	// Trading statistics
	TotalTrades    int
	WinningTrades  int
	LosingTrades   int
	WinRate        float64
	ProfitFactor   float64 // Gross profit / Gross loss
	AvgWin         float64
	AvgLoss        float64
	LargestWin     float64
	LargestLoss    float64
	AvgHoldingTime time.Duration
	TradesPerDay   float64

	// Cost breakdown
	TotalFees     float64
	TotalSlippage float64
	TotalFunding  float64
	TotalCosts    float64
	CostPct       float64 // Costs as % of gross profits

	// Equity curve
	EquityCurve []EquityPoint
}

// MetricsCalculator computes performance metrics from trades
type MetricsCalculator struct {
	config       Config
	trades       []Trade
	equityCurve  []EquityPoint
	dailyReturns []float64
}

// NewMetricsCalculator creates a metrics calculator
func NewMetricsCalculator(config Config) *MetricsCalculator {
	return &MetricsCalculator{
		config: config,
	}
}

// Calculate computes all metrics from trades and equity curve
func (mc *MetricsCalculator) Calculate(trades []Trade, equityCurve []EquityPoint) Metrics {
	mc.trades = trades
	mc.equityCurve = equityCurve
	mc.dailyReturns = mc.computeDailyReturns()

	m := Metrics{
		InitialCapital: mc.config.InitialCapital,
		EquityCurve:    equityCurve,
	}

	if len(equityCurve) > 0 {
		m.StartTime = equityCurve[0].Timestamp
		m.EndTime = equityCurve[len(equityCurve)-1].Timestamp
		m.Duration = m.EndTime.Sub(m.StartTime)
		m.FinalEquity = equityCurve[len(equityCurve)-1].Equity
	}

	// Returns
	m.TotalReturn = mc.computeTotalReturn()
	m.AnnualizedReturn = mc.computeAnnualizedReturn(m.TotalReturn, m.Duration)

	// Risk
	m.MaxDrawdown, m.MaxDrawdownDur = mc.computeMaxDrawdown()
	m.Volatility = mc.computeVolatility()
	m.SharpeRatio = mc.computeSharpe()
	m.SortinoRatio = mc.computeSortino()
	m.CalmarRatio = mc.computeCalmar(m.AnnualizedReturn, m.MaxDrawdown)

	// Trading stats
	mc.computeTradingStats(&m)

	// Costs
	mc.computeCosts(&m)

	return m
}

func (mc *MetricsCalculator) computeTotalReturn() float64 {
	if len(mc.equityCurve) < 2 {
		return 0
	}
	initial := mc.equityCurve[0].Equity
	final := mc.equityCurve[len(mc.equityCurve)-1].Equity
	return (final - initial) / initial
}

func (mc *MetricsCalculator) computeAnnualizedReturn(totalReturn float64, duration time.Duration) float64 {
	years := duration.Hours() / (24 * 365)
	if years <= 0 {
		return 0
	}
	// Compound annual growth rate
	return math.Pow(1+totalReturn, 1/years) - 1
}

func (mc *MetricsCalculator) computeMaxDrawdown() (float64, time.Duration) {
	if len(mc.equityCurve) == 0 {
		return 0, 0
	}

	maxDD := 0.0
	maxDDDur := time.Duration(0)
	peak := mc.equityCurve[0].Equity
	peakTime := mc.equityCurve[0].Timestamp

	for _, point := range mc.equityCurve {
		if point.Equity > peak {
			peak = point.Equity
			peakTime = point.Timestamp
		}

		dd := (peak - point.Equity) / peak
		if dd > maxDD {
			maxDD = dd
			maxDDDur = point.Timestamp.Sub(peakTime)
		}
	}

	return maxDD, maxDDDur
}

func (mc *MetricsCalculator) computeDailyReturns() []float64 {
	if len(mc.equityCurve) < 2 {
		return nil
	}

	// Group by day
	dailyEquity := make(map[string]float64)
	for _, point := range mc.equityCurve {
		day := point.Timestamp.Format("2006-01-02")
		dailyEquity[day] = point.Equity
	}

	// Convert to sorted slice
	var days []string
	for day := range dailyEquity {
		days = append(days, day)
	}
	// Simple sort (Go 1.21+)
	for i := 0; i < len(days)-1; i++ {
		for j := i + 1; j < len(days); j++ {
			if days[i] > days[j] {
				days[i], days[j] = days[j], days[i]
			}
		}
	}

	// Compute daily returns
	var returns []float64
	for i := 1; i < len(days); i++ {
		prevEquity := dailyEquity[days[i-1]]
		currEquity := dailyEquity[days[i]]
		if prevEquity > 0 {
			returns = append(returns, (currEquity-prevEquity)/prevEquity)
		}
	}

	return returns
}

func (mc *MetricsCalculator) computeVolatility() float64 {
	if len(mc.dailyReturns) < 2 {
		return 0
	}

	// Mean
	sum := 0.0
	for _, r := range mc.dailyReturns {
		sum += r
	}
	mean := sum / float64(len(mc.dailyReturns))

	// Variance
	variance := 0.0
	for _, r := range mc.dailyReturns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(len(mc.dailyReturns))

	// Annualized volatility (sqrt(365) for crypto, 252 for stocks)
	return math.Sqrt(variance) * math.Sqrt(365)
}

func (mc *MetricsCalculator) computeSharpe() float64 {
	if len(mc.dailyReturns) < 2 {
		return 0
	}

	// Mean daily return
	sum := 0.0
	for _, r := range mc.dailyReturns {
		sum += r
	}
	meanDaily := sum / float64(len(mc.dailyReturns))

	// Standard deviation
	variance := 0.0
	for _, r := range mc.dailyReturns {
		variance += (r - meanDaily) * (r - meanDaily)
	}
	stdDev := math.Sqrt(variance / float64(len(mc.dailyReturns)))

	if stdDev == 0 {
		return 0
	}

	// Annualized Sharpe (risk-free rate = 0 for crypto)
	return (meanDaily / stdDev) * math.Sqrt(365)
}

func (mc *MetricsCalculator) computeSortino() float64 {
	if len(mc.dailyReturns) < 2 {
		return 0
	}

	// Mean daily return
	sum := 0.0
	for _, r := range mc.dailyReturns {
		sum += r
	}
	meanDaily := sum / float64(len(mc.dailyReturns))

	// Downside deviation (only negative returns)
	downsideSum := 0.0
	downsideCount := 0
	for _, r := range mc.dailyReturns {
		if r < 0 {
			downsideSum += r * r
			downsideCount++
		}
	}

	if downsideCount == 0 {
		return 0 // No downside, undefined
	}

	downsideDev := math.Sqrt(downsideSum / float64(downsideCount))
	if downsideDev == 0 {
		return 0
	}

	return (meanDaily / downsideDev) * math.Sqrt(365)
}

func (mc *MetricsCalculator) computeCalmar(annualizedReturn, maxDrawdown float64) float64 {
	if maxDrawdown == 0 {
		return 0
	}
	return annualizedReturn / maxDrawdown
}

func (mc *MetricsCalculator) computeTradingStats(m *Metrics) {
	if len(mc.trades) == 0 {
		return
	}

	m.TotalTrades = len(mc.trades)

	var grossProfit, grossLoss float64
	var totalWin, totalLoss float64
	var holdingSum time.Duration

	for _, t := range mc.trades {
		holdingSum += t.ExitTime.Sub(t.EntryTime)

		if t.NetPnL > 0 {
			m.WinningTrades++
			grossProfit += t.NetPnL
			totalWin += t.NetPnL
			if t.NetPnL > m.LargestWin {
				m.LargestWin = t.NetPnL
			}
		} else {
			m.LosingTrades++
			grossLoss += math.Abs(t.NetPnL)
			totalLoss += math.Abs(t.NetPnL)
			if t.NetPnL < m.LargestLoss {
				m.LargestLoss = t.NetPnL
			}
		}
	}

	if m.TotalTrades > 0 {
		m.WinRate = float64(m.WinningTrades) / float64(m.TotalTrades)
		m.AvgHoldingTime = holdingSum / time.Duration(m.TotalTrades)
	}

	if m.WinningTrades > 0 {
		m.AvgWin = totalWin / float64(m.WinningTrades)
	}
	if m.LosingTrades > 0 {
		m.AvgLoss = totalLoss / float64(m.LosingTrades)
	}

	if grossLoss > 0 {
		m.ProfitFactor = grossProfit / grossLoss
	}

	if m.Duration.Hours() > 24 {
		days := m.Duration.Hours() / 24
		m.TradesPerDay = float64(m.TotalTrades) / days
	}
}

func (mc *MetricsCalculator) computeCosts(m *Metrics) {
	for _, t := range mc.trades {
		m.TotalFees += t.EntryFee + t.ExitFee
		// Use slippage COSTS (in dollars), not slippage price deltas
		m.TotalSlippage += t.EntrySlipCost + t.ExitSlipCost
		m.TotalFunding += t.FundingPaid
	}
	m.TotalCosts = m.TotalFees + m.TotalSlippage + m.TotalFunding

	// Gross profit (before costs)
	grossProfit := 0.0
	for _, t := range mc.trades {
		if t.GrossPnL > 0 {
			grossProfit += t.GrossPnL
		}
	}
	if grossProfit > 0 {
		m.CostPct = m.TotalCosts / grossProfit
	}
}

// FormatReport creates a human-readable report
func (m *Metrics) FormatReport() string {
	return formatMetricsReport(m)
}

func formatMetricsReport(m *Metrics) string {
	// Helper for formatting
	pct := func(v float64) string {
		return formatPct(v * 100)
	}

	report := "===== BACKTEST RESULTS =====\n"
	report += formatLine("Period", m.StartTime.Format("2006-01-02")+" to "+m.EndTime.Format("2006-01-02"))
	report += formatLine("Initial Capital", formatMoney(m.InitialCapital))
	report += formatLine("Final Equity", formatMoney(m.FinalEquity))
	report += "\n"

	report += "PERFORMANCE\n"
	report += formatLine("  Total Return", pct(m.TotalReturn))
	report += formatLine("  Annualized Return", pct(m.AnnualizedReturn))
	report += formatLine("  Max Drawdown", pct(m.MaxDrawdown))
	report += formatLine("  Sharpe Ratio", formatFloat(m.SharpeRatio))
	report += formatLine("  Sortino Ratio", formatFloat(m.SortinoRatio))
	report += formatLine("  Calmar Ratio", formatFloat(m.CalmarRatio))
	report += "\n"

	report += "TRADING STATS\n"
	report += formatLine("  Total Trades", formatInt(m.TotalTrades))
	report += formatLine("  Win Rate", pct(m.WinRate))
	report += formatLine("  Profit Factor", formatFloat(m.ProfitFactor))
	report += formatLine("  Avg Win", formatMoney(m.AvgWin))
	report += formatLine("  Avg Loss", formatMoney(m.AvgLoss))
	report += formatLine("  Trades/Day", formatFloat(m.TradesPerDay))
	report += "\n"

	report += "COSTS BREAKDOWN\n"
	report += formatLine("  Total Fees", formatMoney(m.TotalFees))
	report += formatLine("  Total Slippage", formatMoney(m.TotalSlippage))
	report += formatLine("  Total Funding", formatMoney(m.TotalFunding))
	report += formatLine("  Total Costs", formatMoney(m.TotalCosts))

	return report
}

func formatLine(label, value string) string {
	return label + ": " + value + "\n"
}

func formatPct(v float64) string {
	sign := ""
	if v > 0 {
		sign = "+"
	}
	return sign + formatFloat(v) + "%"
}

func formatFloat(v float64) string {
	return floatToString(v, 2)
}

func formatMoney(v float64) string {
	sign := ""
	if v > 0 {
		sign = "+"
	} else if v < 0 {
		sign = "-"
		v = -v
	}
	return sign + "$" + floatToString(v, 2)
}

func formatInt(v int) string {
	return intToString(v)
}

func floatToString(v float64, decimals int) string {
	// Simple float formatting without fmt
	negative := v < 0
	if negative {
		v = -v
	}

	// Scale by decimals
	scale := math.Pow(10, float64(decimals))
	scaled := int64(v*scale + 0.5)

	intPart := scaled / int64(scale)
	decPart := scaled % int64(scale)

	result := intToString(int(intPart)) + "."
	decStr := intToString(int(decPart))
	for len(decStr) < decimals {
		decStr = "0" + decStr
	}
	result += decStr

	if negative {
		result = "-" + result
	}
	return result
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
