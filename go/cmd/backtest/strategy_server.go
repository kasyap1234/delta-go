package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/kasyap/delta-go/go/pkg/delta"
	"github.com/kasyap/delta-go/go/pkg/strategy"
)

// AnalyzeRequest is the request format for strategy analysis
type AnalyzeRequest struct {
	Candles []CandleData `json:"candles"`
	Regime  string       `json:"regime"`
	Symbol  string       `json:"symbol"`
}

// CandleData matches the Python format
type CandleData struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// AnalyzeResponse is returned by the strategy server
type AnalyzeResponse struct {
	Action     string  `json:"action"`
	Side       string  `json:"side"`
	Confidence float64 `json:"confidence"`
	Price      float64 `json:"price"`
	StopLoss   float64 `json:"stop_loss"`
	TakeProfit float64 `json:"take_profit"`
	Reason     string  `json:"reason"`
}

// StrategyServer serves strategy logic via HTTP for backtesting
type StrategyServer struct {
	strategyMgr *strategy.Manager
}

// NewStrategyServer creates a new strategy server
func NewStrategyServer() *StrategyServer {
	mgr := strategy.NewManager()

	// Register all strategies
	mgr.RegisterStrategy(strategy.NewBullTrendStrategy())
	mgr.RegisterStrategy(strategy.NewBearTrendStrategy())
	mgr.RegisterStrategy(strategy.NewRangingStrategy())
	mgr.RegisterStrategy(strategy.NewHighVolBreakoutStrategy())
	mgr.RegisterStrategy(strategy.NewLowVolPrepStrategy())

	// Map regimes to strategies
	mgr.SetRegimeStrategy(delta.RegimeBull, "bull_trend_following")
	mgr.SetRegimeStrategy(delta.RegimeBear, "bear_trend_following")
	mgr.SetRegimeStrategy(delta.RegimeRanging, "ranging_mean_reversion")
	mgr.SetRegimeStrategy(delta.RegimeHighVol, "high_vol_breakout")
	mgr.SetRegimeStrategy(delta.RegimeLowVol, "low_vol_preparation")

	return &StrategyServer{strategyMgr: mgr}
}

// HandleAnalyze handles POST /analyze requests
func (s *StrategyServer) HandleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Convert candles
	candles := make([]delta.Candle, len(req.Candles))
	for i, c := range req.Candles {
		candles[i] = delta.Candle{
			Time:   c.Time,
			Open:   c.Open,
			High:   c.High,
			Low:    c.Low,
			Close:  c.Close,
			Volume: c.Volume,
		}
	}

	// Convert regime string to type
	regime := stringToRegime(req.Regime)

	// Get signal from strategy
	signal := s.strategyMgr.GetSignal(candles, regime)

	// Build response
	resp := AnalyzeResponse{
		Action:     string(signal.Action),
		Side:       signal.Side,
		Confidence: signal.Confidence,
		Price:      signal.Price,
		StopLoss:   signal.StopLoss,
		TakeProfit: signal.TakeProfit,
		Reason:     signal.Reason,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleHealth handles health check
func (s *StrategyServer) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func stringToRegime(s string) delta.MarketRegime {
	switch s {
	case "bull":
		return delta.RegimeBull
	case "bear":
		return delta.RegimeBear
	case "ranging":
		return delta.RegimeRanging
	case "high_volatility":
		return delta.RegimeHighVol
	case "low_volatility":
		return delta.RegimeLowVol
	default:
		return delta.RegimeRanging
	}
}

func main() {
	server := NewStrategyServer()

	http.HandleFunc("/analyze", server.HandleAnalyze)
	http.HandleFunc("/health", server.HandleHealth)

	port := "8081"
	log.Printf("Strategy server starting on port %s", port)
	log.Printf("Endpoints: POST /analyze, GET /health")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
