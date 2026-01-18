package backtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kasyap/delta-go/go/pkg/delta"
)

// DataLoader handles fetching and caching historical data
type DataLoader struct {
	client   *delta.Client
	cacheDir string
}

// NewDataLoader creates a data loader with caching
func NewDataLoader(client *delta.Client, cacheDir string) *DataLoader {
	return &DataLoader{
		client:   client,
		cacheDir: cacheDir,
	}
}

// LoadCandles fetches candles for the given range, using cache if available
func (d *DataLoader) LoadCandles(symbol, resolution string, start, end time.Time) ([]delta.Candle, error) {
	// Try cache first
	cached, err := d.loadFromCache(symbol, resolution, start, end)
	if err == nil && len(cached) > 0 {
		return cached, nil
	}

	// Try fetching from Delta
	allCandles, err := d.fetchCandlesInChunks(symbol, resolution, start, end)
	if err == nil && len(allCandles) > 0 {
		// Save to cache
		d.saveToCache(symbol, resolution, start, end, allCandles)
		return allCandles, nil
	}

	// Fallback to Binance
	fmt.Printf("  Delta API failed or inaccessible, trying Binance fallback for %s...\n", symbol)
	allCandles, err = d.fetchFromBinance(symbol, resolution, start, end)
	if err != nil {
		return nil, fmt.Errorf("both Delta and Binance fetching failed for %s: %w", symbol, err)
	}

	// Save to cache
	if err := d.saveToCache(symbol, resolution, start, end, allCandles); err != nil {
		fmt.Printf("Warning: failed to cache data: %v\n", err)
	}

	return allCandles, nil
}

// fetchCandlesInChunks fetches data in chunks to avoid API limits
func (d *DataLoader) fetchCandlesInChunks(symbol, resolution string, start, end time.Time) ([]delta.Candle, error) {
	var allCandles []delta.Candle

	// Determine chunk size based on resolution
	chunkDuration := getChunkDuration(resolution)
	current := start

	for current.Before(end) {
		chunkEnd := current.Add(chunkDuration)
		if chunkEnd.After(end) {
			chunkEnd = end
		}

		candles, err := d.client.GetCandles(symbol, resolution, current, chunkEnd)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch candles for %s [%s - %s]: %w",
				symbol, current.Format("2006-01-02"), chunkEnd.Format("2006-01-02"), err)
		}

		allCandles = append(allCandles, candles...)
		current = chunkEnd

		// Rate limiting delay (100ms between requests)
		time.Sleep(100 * time.Millisecond)
	}

	// Sort by time
	sortCandles(allCandles)

	return allCandles, nil
}

// sortCandles sorts candles by timestamp
func sortCandles(candles []delta.Candle) {
	// Simple bubble sort (data is usually mostly sorted)
	for i := 0; i < len(candles)-1; i++ {
		for j := i + 1; j < len(candles); j++ {
			if candles[i].Time > candles[j].Time {
				candles[i], candles[j] = candles[j], candles[i]
			}
		}
	}
}

// fetchFromBinance fetches candles from Binance Futures public API
func (d *DataLoader) fetchFromBinance(symbol, resolution string, start, end time.Time) ([]delta.Candle, error) {
	var allCandles []delta.Candle

	// Map symbol and resolution
	binanceSymbol := mapToBinanceSymbol(symbol)
	binanceInterval := mapToBinanceInterval(resolution)

	baseURL := "https://fapi.binance.com/fapi/v1/klines"

	current := start
	client := &http.Client{Timeout: 10 * time.Second}

	for current.Before(end) {
		url := fmt.Sprintf("%s?symbol=%s&interval=%s&startTime=%d&endTime=%d&limit=1500",
			baseURL, binanceSymbol, binanceInterval, current.UnixMilli(), end.UnixMilli())

		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("binance API status: %d", resp.StatusCode)
		}

		var klines [][]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&klines); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		if len(klines) == 0 {
			break
		}

		for _, k := range klines {
			// Binance kline format: [ot, o, h, l, c, v, ct, qv, nt, tv, tq, _]
			open, _ := strconv.ParseFloat(k[1].(string), 64)
			high, _ := strconv.ParseFloat(k[2].(string), 64)
			low, _ := strconv.ParseFloat(k[3].(string), 64)
			close, _ := strconv.ParseFloat(k[4].(string), 64)
			volume, _ := strconv.ParseFloat(k[5].(string), 64)
			ot := int64(k[0].(float64))

			allCandles = append(allCandles, delta.Candle{
				Time:   ot / 1000,
				Open:   open,
				High:   high,
				Low:    low,
				Close:  close,
				Volume: volume,
			})
		}

		// Move forward
		lastTime := int64(klines[len(klines)-1][0].(float64))
		current = time.UnixMilli(lastTime).Add(time.Minute) // Add a buffer to skip the last candle

		// Avoid spamming
		time.Sleep(100 * time.Millisecond)
	}

	return allCandles, nil
}

func mapToBinanceSymbol(symbol string) string {
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

func mapToBinanceInterval(resolution string) string {
	switch resolution {
	case "1m":
		return "1m"
	case "5m":
		return "5m"
	case "15m":
		return "15m"
	case "1h":
		return "1h"
	case "4h":
		return "4h"
	case "1d":
		return "1d"
	default:
		return "5m"
	}
}

// getChunkDuration returns optimal chunk size for API calls
func getChunkDuration(resolution string) time.Duration {
	switch resolution {
	case "1m":
		return 24 * time.Hour // 1440 candles per day
	case "5m":
		return 7 * 24 * time.Hour // 2016 candles per week
	case "15m":
		return 14 * 24 * time.Hour
	case "1h":
		return 30 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

// Cache file naming
func (d *DataLoader) cacheFilePath(symbol, resolution string, start, end time.Time) string {
	filename := fmt.Sprintf("%s_%s_%s_%s.json",
		symbol, resolution,
		start.Format("20060102"),
		end.Format("20060102"))
	return filepath.Join(d.cacheDir, filename)
}

func (d *DataLoader) loadFromCache(symbol, resolution string, start, end time.Time) ([]delta.Candle, error) {
	path := d.cacheFilePath(symbol, resolution, start, end)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var candles []delta.Candle
	if err := json.Unmarshal(data, &candles); err != nil {
		return nil, err
	}

	return candles, nil
}

func (d *DataLoader) saveToCache(symbol, resolution string, start, end time.Time, candles []delta.Candle) error {
	// Ensure cache directory exists
	if err := os.MkdirAll(d.cacheDir, 0755); err != nil {
		return err
	}

	path := d.cacheFilePath(symbol, resolution, start, end)

	data, err := json.Marshal(candles)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// LoadMultiSymbol loads candles for multiple symbols
func (d *DataLoader) LoadMultiSymbol(symbols []string, resolution string, start, end time.Time) (map[string][]delta.Candle, error) {
	result := make(map[string][]delta.Candle)

	for _, symbol := range symbols {
		candles, err := d.LoadCandles(symbol, resolution, start, end)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", symbol, err)
		}
		result[symbol] = candles
	}

	return result, nil
}

// ClearCache removes all cached data
func (d *DataLoader) ClearCache() error {
	return os.RemoveAll(d.cacheDir)
}

// CacheInfo returns information about cached data
func (d *DataLoader) CacheInfo() (int, int64, error) {
	var fileCount int
	var totalSize int64

	err := filepath.Walk(d.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileCount++
			totalSize += info.Size()
		}
		return nil
	})

	return fileCount, totalSize, err
}
