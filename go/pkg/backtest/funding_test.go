package backtest

import (
	"testing"
	"time"
)

func TestIsFundingTime(t *testing.T) {
	tests := []struct {
		hour     int
		expected bool
	}{
		{0, true},
		{1, false},
		{7, false},
		{8, true},
		{12, false},
		{16, true},
		{20, false},
	}

	for _, tt := range tests {
		testTime := time.Date(2024, 1, 1, tt.hour, 0, 0, 0, time.UTC)
		result := IsFundingTime(testTime)
		if result != tt.expected {
			t.Errorf("IsFundingTime(%d:00) = %v, expected %v", tt.hour, result, tt.expected)
		}
	}
}

func TestNextFundingTime(t *testing.T) {
	tests := []struct {
		inputHour    int
		expectedHour int
	}{
		{1, 8},
		{7, 8},
		{9, 16},
		{15, 16},
		{17, 0}, // Next day
		{23, 0}, // Next day
	}

	for _, tt := range tests {
		testTime := time.Date(2024, 1, 1, tt.inputHour, 30, 0, 0, time.UTC)
		result := NextFundingTime(testTime)

		if result.Hour() != tt.expectedHour {
			t.Errorf("NextFundingTime(%d:30) hour = %d, expected %d",
				tt.inputHour, result.Hour(), tt.expectedHour)
		}
	}
}

func TestGetFundingAtTime(t *testing.T) {
	rates := []FundingRate{
		{Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Rate: 0.0001},
		{Timestamp: time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC), Rate: 0.0002},
		{Timestamp: time.Date(2024, 1, 1, 16, 0, 0, 0, time.UTC), Rate: 0.0003},
	}

	// At 12:00, should use 8:00 rate
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	result := GetFundingAtTime(rates, testTime)

	if result != 0.0002 {
		t.Errorf("GetFundingAtTime at 12:00 = %f, expected 0.0002", result)
	}

	// At 20:00, should use 16:00 rate
	testTime = time.Date(2024, 1, 1, 20, 0, 0, 0, time.UTC)
	result = GetFundingAtTime(rates, testTime)

	if result != 0.0003 {
		t.Errorf("GetFundingAtTime at 20:00 = %f, expected 0.0003", result)
	}
}

func TestMapToExternalSymbol(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"BTCUSD", "BTCUSDT"},
		{"ETHUSD", "ETHUSDT"},
		{"SOLUSD", "SOLUSDT"},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		result := mapToExternalSymbol(tt.input)
		if result != tt.expected {
			t.Errorf("mapToExternalSymbol(%s) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}
