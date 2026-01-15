package delta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kasyap/delta-go/go/config"
)

// Client is the Delta Exchange API client
type Client struct {
	cfg        *config.Config
	httpClient *http.Client
	baseURL    string
	limiter    *time.Ticker
}

// NewClient creates a new Delta Exchange API client
func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg:     cfg,
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		limiter: time.NewTicker(100 * time.Millisecond), // 10 rps rate limit
	}
}

// APIResponse is the base response structure from Delta Exchange
type APIResponse struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Error   *APIError       `json:"error,omitempty"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

// APIError represents an error from the API
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// doRequest performs an authenticated HTTP request with proper retry logic
func (c *Client) doRequest(method, path string, query url.Values, body interface{}) (*APIResponse, error) {
	<-c.limiter.C // Rate limiting without locks

	fullURL := c.baseURL + path
	queryString := ""
	if len(query) > 0 {
		queryString = query.Encode()
		fullURL += "?" + queryString
	}

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
	}

	bodyStr := ""
	if len(bodyBytes) > 0 {
		bodyStr = string(bodyBytes)
	}

	authHeaders := NewAuthHeaders(c.cfg.APIKey, c.cfg.APISecret, method, path, queryString, bodyStr)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest(method, fullURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("api-key", authHeaders.APIKey)
		req.Header.Set("signature", authHeaders.Signature)
		req.Header.Set("timestamp", authHeaders.Timestamp)
		req.Header.Set("User-Agent", authHeaders.UserAgent)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		// Retry on rate limit or server errors
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		// Non-retryable HTTP errors
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp APIResponse
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			return nil, fmt.Errorf("parse response: %w (body=%q)", err, respBody)
		}

		if !apiResp.Success {
			if apiResp.Error != nil {
				return nil, fmt.Errorf("API error %s: %s", apiResp.Error.Code, apiResp.Error.Message)
			}
			return nil, fmt.Errorf("API error: %s", string(respBody))
		}

		return &apiResp, nil
	}

	return nil, fmt.Errorf("request failed after retries: %w", lastErr)
}

// Get performs a GET request
func (c *Client) Get(path string, query url.Values) (*APIResponse, error) {
	return c.doRequest("GET", path, query, nil)
}

// Post performs a POST request
func (c *Client) Post(path string, body interface{}) (*APIResponse, error) {
	return c.doRequest("POST", path, nil, body)
}

// Delete performs a DELETE request
func (c *Client) Delete(path string, query url.Values) (*APIResponse, error) {
	return c.doRequest("DELETE", path, query, nil)
}

// Put performs a PUT request
func (c *Client) Put(path string, body interface{}) (*APIResponse, error) {
	return c.doRequest("PUT", path, nil, body)
}
