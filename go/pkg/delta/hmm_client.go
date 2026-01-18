package delta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// HMMClient handles communication with the HMM Cloud Run function
type HMMClient struct {
	endpoint   string
	httpClient *http.Client
	tokenCache string
	tokenMu    sync.RWMutex
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

// getIdentityToken fetches an identity token from the GCE metadata server
func (c *HMMClient) getIdentityToken() (string, error) {
	c.tokenMu.RLock()
	cached := c.tokenCache
	c.tokenMu.RUnlock()
	if cached != "" {
		return cached, nil
	}

	metadataURL := fmt.Sprintf(
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=%s",
		c.endpoint,
	)

	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("create metadata request: %w", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Connection error to metadata server - likely not on GCE
		log.Printf("HMM auth: metadata server unreachable, skipping auth: %v", err)
		return "", nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// 404 means not on GCE or service account not configured
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		// Other errors (401, 403, 5xx) should be reported
		return "", fmt.Errorf("metadata server returned %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}

	c.tokenMu.Lock()
	c.tokenCache = string(token)
	c.tokenMu.Unlock()

	// Refresh token before expiry (tokens last ~1 hour, refresh at 50 min)
	go func() {
		time.Sleep(50 * time.Minute)
		c.tokenMu.Lock()
		c.tokenCache = ""
		c.tokenMu.Unlock()
	}()

	return string(token), nil
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

	// Add identity token for authenticated Cloud Functions
	if token, err := c.getIdentityToken(); err == nil && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

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
