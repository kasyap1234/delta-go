package strategy

import (
	"math"

	"github.com/kasyap/delta-go/go/pkg/delta"
)

// CandleSeries holds extracted price data to avoid repeated allocations
type CandleSeries struct {
	Opens   []float64
	Highs   []float64
	Lows    []float64
	Closes  []float64
	Volumes []float64
}

// ExtractSeries extracts all price series from candles in a single pass
func ExtractSeries(candles []delta.Candle) CandleSeries {
	n := len(candles)
	s := CandleSeries{
		Opens:   make([]float64, n),
		Highs:   make([]float64, n),
		Lows:    make([]float64, n),
		Closes:  make([]float64, n),
		Volumes: make([]float64, n),
	}
	for i, c := range candles {
		s.Opens[i] = c.Open
		s.Highs[i] = c.High
		s.Lows[i] = c.Low
		s.Closes[i] = c.Close
		s.Volumes[i] = c.Volume
	}
	return s
}

// extractCloses extracts closing prices from candles
func extractCloses(candles []delta.Candle) []float64 {
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}
	return closes
}

// extractHighs extracts high prices from candles
func extractHighs(candles []delta.Candle) []float64 {
	highs := make([]float64, len(candles))
	for i, c := range candles {
		highs[i] = c.High
	}
	return highs
}

// extractLows extracts low prices from candles
func extractLows(candles []delta.Candle) []float64 {
	lows := make([]float64, len(candles))
	for i, c := range candles {
		lows[i] = c.Low
	}
	return lows
}

// extractVolumes extracts volumes from candles
func extractVolumes(candles []delta.Candle) []float64 {
	volumes := make([]float64, len(candles))
	for i, c := range candles {
		volumes[i] = c.Volume
	}
	return volumes
}

// average calculates the average of a slice
func average(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

// minSlice finds the minimum value in a slice (variadic for convenience)
func minSlice(data ...float64) float64 {
	if len(data) == 0 {
		return 0
	}
	minVal := data[0]
	for _, v := range data[1:] {
		minVal = math.Min(minVal, v)
	}
	return minVal
}

// maxSlice finds the maximum value in a slice (variadic for convenience)
func maxSlice(data ...float64) float64 {
	if len(data) == 0 {
		return 0
	}
	maxVal := data[0]
	for _, v := range data[1:] {
		maxVal = math.Max(maxVal, v)
	}
	return maxVal
}

// minOfSlice finds minimum in a slice (non-variadic, more efficient)
func minOfSlice(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	minVal := data[0]
	for _, v := range data[1:] {
		minVal = math.Min(minVal, v)
	}
	return minVal
}

// maxOfSlice finds maximum in a slice (non-variadic, more efficient)
func maxOfSlice(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	maxVal := data[0]
	for _, v := range data[1:] {
		maxVal = math.Max(maxVal, v)
	}
	return maxVal
}
