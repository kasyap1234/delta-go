package delta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HMMClient handles communication with the HMM Cloud Run function
type HMMClient struct {
	endpoint   string
	httpClient *http.Client
}

// NewHMMClient creates a new HMM client
func NewHMMClient(endpoint string) *HMMClient {
	return &HMMClient{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// DetectRegime calls the HMM Cloud Run function to detect market regime
// symbol is used to select the appropriate per-coin model (BTCUSD, ETHUSD, SOLUSD)
func (c *HMMClient) DetectRegime(candles []Candle, symbol string) (*HMMResponse, error) {
	input := CandlesToHMMInput(candles, symbol)

	jsonData, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal candle data: %w", err)
	}

	req, err := http.NewRequest("POST", c.endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HMM request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HMM http %d: %s", resp.StatusCode, string(body))
	}

	var hmmResp HMMResponse
	if err := json.Unmarshal(body, &hmmResp); err != nil {
		return nil, fmt.Errorf("parse HMM response: %w", err)
	}

	return &hmmResp, nil
}

// DetectRegimeWithRetry calls DetectRegime with retry logic
func (c *HMMClient) DetectRegimeWithRetry(candles []Candle, symbol string, maxRetries int) (*HMMResponse, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		resp, err := c.DetectRegime(candles, symbol)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	return nil, fmt.Errorf("HMM detection failed after %d retries: %v", maxRetries, lastErr)
}
