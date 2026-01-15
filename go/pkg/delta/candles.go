package delta

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// GetCandles fetches historical OHLC candles
// resolution: "1m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "1d", "7d", "30d"
func (c *Client) GetCandles(symbol string, resolution string, start, end time.Time) ([]Candle, error) {
	query := url.Values{}
	query.Set("symbol", symbol)
	query.Set("resolution", resolution)
	query.Set("start", strconv.FormatInt(start.Unix(), 10))
	query.Set("end", strconv.FormatInt(end.Unix(), 10))

	resp, err := c.Get("/history/candles", query)
	if err != nil {
		return nil, err
	}

	var candles []Candle
	if err := json.Unmarshal(resp.Result, &candles); err != nil {
		return nil, fmt.Errorf("failed to parse candles: %v", err)
	}

	return candles, nil
}

// GetRecentCandles fetches recent candles (last N)
func (c *Client) GetRecentCandles(symbol string, resolution string, count int) ([]Candle, error) {
	// Calculate time range based on resolution and count
	duration := resolutionToDuration(resolution) * time.Duration(count)
	end := time.Now()
	start := end.Add(-duration)

	return c.GetCandles(symbol, resolution, start, end)
}

// resolutionToDuration converts resolution string to duration
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
		return time.Minute
	}
}

// CandlesToHMMInput converts candles to format suitable for HMM processing
func CandlesToHMMInput(candles []Candle, symbol string) map[string]interface{} {
	opens := make([]float64, len(candles))
	highs := make([]float64, len(candles))
	lows := make([]float64, len(candles))
	closes := make([]float64, len(candles))
	volumes := make([]float64, len(candles))
	timestamps := make([]int64, len(candles))

	for i, c := range candles {
		opens[i] = c.Open
		highs[i] = c.High
		lows[i] = c.Low
		closes[i] = c.Close
		volumes[i] = c.Volume
		timestamps[i] = c.Time
	}

	return map[string]interface{}{
		"symbol":    symbol,
		"open":      opens,
		"high":      highs,
		"low":       lows,
		"close":     closes,
		"volume":    volumes,
		"timestamp": timestamps,
	}
}
