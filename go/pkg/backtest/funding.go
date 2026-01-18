package backtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// FundingFetcher fetches historical funding rates from external sources
type FundingFetcher struct {
	cacheDir   string
	httpClient *http.Client
}

// NewFundingFetcher creates a funding rate fetcher
func NewFundingFetcher(cacheDir string) *FundingFetcher {
	return &FundingFetcher{
		cacheDir: cacheDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchFundingRates fetches historical funding rates for a symbol
// It tries multiple sources: Coinglass, Binance (as proxy for market funding)
func (f *FundingFetcher) FetchFundingRates(symbol string, start, end time.Time) ([]FundingRate, error) {
	// Try cache first
	cached, err := f.loadFromCache(symbol, start, end)
	if err == nil && len(cached) > 0 {
		return cached, nil
	}

	// Map symbol to external symbol format
	externalSymbol := mapToExternalSymbol(symbol)

	// Try Binance funding rates (free, no API key required)
	rates, err := f.fetchFromBinance(externalSymbol, start, end)
	if err == nil && len(rates) > 0 {
		sortFundingRates(rates)
		f.saveToCache(symbol, start, end, rates)
		return rates, nil
	}

	// Try Coinglass as fallback
	rates, err = f.fetchFromCoinglass(externalSymbol, start, end)
	if err == nil && len(rates) > 0 {
		sortFundingRates(rates)
		f.saveToCache(symbol, start, end, rates)
		return rates, nil
	}

	// If all external sources fail, generate synthetic funding rates
	// based on typical market conditions
	fmt.Printf("Warning: using synthetic funding rates for %s\n", symbol)
	rates = f.generateSyntheticRates(symbol, start, end)
	return rates, nil
}

func sortFundingRates(rates []FundingRate) {
	sort.Slice(rates, func(i, j int) bool {
		return rates[i].Timestamp.Before(rates[j].Timestamp)
	})
}

// mapToExternalSymbol converts Delta symbols to exchange symbols
func mapToExternalSymbol(symbol string) string {
	switch symbol {
	case "BTCUSD", "BTCINR":
		return "BTCUSDT"
	case "ETHUSD", "ETHINR":
		return "ETHUSDT"
	case "SOLUSD", "SOLINR":
		return "SOLUSDT"
	default:
		return symbol
	}
}

// fetchFromBinance fetches funding rates from Binance Futures API
func (f *FundingFetcher) fetchFromBinance(symbol string, start, end time.Time) ([]FundingRate, error) {
	var allRates []FundingRate

	// Binance funding rate endpoint (public, no auth required)
	baseURL := "https://fapi.binance.com/fapi/v1/fundingRate"

	current := start
	for current.Before(end) {
		url := fmt.Sprintf("%s?symbol=%s&startTime=%d&endTime=%d&limit=1000",
			baseURL, symbol, current.UnixMilli(), end.UnixMilli())

		resp, err := f.httpClient.Get(url)
		if err != nil {
			return nil, fmt.Errorf("binance request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("binance API error: %s", string(body))
		}

		var binanceRates []struct {
			Symbol      string `json:"symbol"`
			FundingTime int64  `json:"fundingTime"`
			FundingRate string `json:"fundingRate"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&binanceRates); err != nil {
			return nil, fmt.Errorf("failed to decode binance response: %w", err)
		}

		if len(binanceRates) == 0 {
			break
		}

		for _, br := range binanceRates {
			rate, _ := strconv.ParseFloat(br.FundingRate, 64)
			allRates = append(allRates, FundingRate{
				Timestamp: time.UnixMilli(br.FundingTime),
				Symbol:    symbol,
				Rate:      rate,
			})
		}

		// Move to next batch
		lastTime := time.UnixMilli(binanceRates[len(binanceRates)-1].FundingTime)
		current = lastTime.Add(time.Hour)

		// Rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	return allRates, nil
}

// fetchFromCoinglass fetches from Coinglass API (may require API key)
func (f *FundingFetcher) fetchFromCoinglass(symbol string, start, end time.Time) ([]FundingRate, error) {
	// Coinglass API endpoint
	baseURL := "https://open-api.coinglass.com/public/v2/funding"

	url := fmt.Sprintf("%s?symbol=%s&time_type=h8", baseURL, symbol)

	resp, err := f.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coinglass API returned status %d", resp.StatusCode)
	}

	var cgResp struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Data    []struct {
			CreateTime int64   `json:"create_time"`
			Rate       float64 `json:"rate"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&cgResp); err != nil {
		return nil, err
	}

	if cgResp.Code != 0 {
		return nil, fmt.Errorf("coinglass error: %s", cgResp.Message)
	}

	var rates []FundingRate
	for _, d := range cgResp.Data {
		t := time.Unix(d.CreateTime, 0)
		if t.After(start) && t.Before(end) {
			rates = append(rates, FundingRate{
				Timestamp: t,
				Symbol:    symbol,
				Rate:      d.Rate,
			})
		}
	}

	return rates, nil
}

// generateSyntheticRates creates realistic synthetic funding rates
// based on typical market behavior (positive bias during bull markets)
func (f *FundingFetcher) generateSyntheticRates(symbol string, start, end time.Time) []FundingRate {
	var rates []FundingRate

	// Generate 8-hourly rates
	interval := 8 * time.Hour
	current := start.Truncate(interval)

	// Base rate varies by asset (BTC tends to have higher funding)
	baseRate := 0.0001 // 0.01% per 8h (typical)
	switch symbol {
	case "BTCUSD", "BTCINR":
		baseRate = 0.00015
	case "ETHUSD", "ETHINR":
		baseRate = 0.0001
	case "SOLUSD", "SOLINR":
		baseRate = 0.00008
	}

	for current.Before(end) {
		// Add some variance (+/- 50% of base rate)
		// Using deterministic variance based on timestamp for reproducibility
		variance := (float64(current.Unix()%100) - 50) / 100 * baseRate * 0.5
		rate := baseRate + variance

		rates = append(rates, FundingRate{
			Timestamp: current,
			Symbol:    symbol,
			Rate:      rate,
		})

		current = current.Add(interval)
	}

	return rates
}

// Cache methods
func (f *FundingFetcher) cacheFilePath(symbol string, start, end time.Time) string {
	filename := fmt.Sprintf("funding_%s_%s_%s.json",
		symbol, start.Format("20060102"), end.Format("20060102"))
	return filepath.Join(f.cacheDir, filename)
}

func (f *FundingFetcher) loadFromCache(symbol string, start, end time.Time) ([]FundingRate, error) {
	path := f.cacheFilePath(symbol, start, end)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rates []FundingRate
	if err := json.Unmarshal(data, &rates); err != nil {
		return nil, err
	}

	return rates, nil
}

func (f *FundingFetcher) saveToCache(symbol string, start, end time.Time, rates []FundingRate) error {
	if err := os.MkdirAll(f.cacheDir, 0755); err != nil {
		return err
	}

	path := f.cacheFilePath(symbol, start, end)

	data, err := json.Marshal(rates)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GetFundingAtTime finds the applicable funding rate at a given time
func GetFundingAtTime(rates []FundingRate, t time.Time) float64 {
	// Find the most recent funding rate before or at time t
	var applicable FundingRate
	for _, rate := range rates {
		if rate.Timestamp.Before(t) || rate.Timestamp.Equal(t) {
			applicable = rate
		} else {
			break // Rates are sorted by time
		}
	}
	return applicable.Rate
}

// IsFundingTime checks if the given time is a funding payment time
// Funding is paid every 8 hours: 00:00, 08:00, 16:00 UTC
func IsFundingTime(t time.Time) bool {
	u := t.UTC()
	if !(u.Hour() == 0 || u.Hour() == 8 || u.Hour() == 16) {
		return false
	}
	return u.Minute() == 0 && u.Second() == 0
}

// IsFundingWindow checks if the time is within a 5-minute window of a funding time
func IsFundingWindow(t time.Time, prevTs time.Time) bool {
	u := t.UTC()
	if !(u.Hour() == 0 || u.Hour() == 8 || u.Hour() == 16) {
		return false
	}
	// Only trigger if we just crossed into this hour
	if u.Minute() > 5 {
		return false
	}
	// Check if previous timestamp was in a different hour
	return prevTs.UTC().Hour() != u.Hour()
}

// NextFundingTime returns the next funding time after t
func NextFundingTime(t time.Time) time.Time {
	t = t.UTC()
	hour := t.Hour()

	var nextHour int
	if hour < 8 {
		nextHour = 8
	} else if hour < 16 {
		nextHour = 16
	} else {
		nextHour = 24 // Next day 00:00
	}

	next := time.Date(t.Year(), t.Month(), t.Day(), nextHour%24, 0, 0, 0, time.UTC)
	if nextHour == 24 {
		next = next.AddDate(0, 0, 1)
	}

	return next
}
