package delta

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// GenerateSignature creates HMAC-SHA256 signature for Delta Exchange API
// Format: method + timestamp + path + queryString + body
func GenerateSignature(secret, method, timestamp, path, queryString, body string) string {
	message := method + timestamp + path
	if queryString != "" {
		message += "?" + queryString
	}
	message += body

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateTimestamp returns current Unix timestamp as string
func GenerateTimestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// AuthHeaders represents the authentication headers required by Delta Exchange
type AuthHeaders struct {
	APIKey    string
	Signature string
	Timestamp string
	UserAgent string
}

// NewAuthHeaders generates authentication headers for a request
func NewAuthHeaders(apiKey, apiSecret, method, path, queryString, body string) *AuthHeaders {
	timestamp := GenerateTimestamp()
	signature := GenerateSignature(apiSecret, method, timestamp, path, queryString, body)

	return &AuthHeaders{
		APIKey:    apiKey,
		Signature: signature,
		Timestamp: timestamp,
		UserAgent: "delta-go-bot/1.0",
	}
}

// Validate checks if the timestamp is within acceptable range (5 seconds)
func (a *AuthHeaders) Validate() error {
	ts, err := strconv.ParseInt(a.Timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %v", err)
	}

	now := time.Now().Unix()
	if now-ts > 5 {
		return fmt.Errorf("timestamp expired: %d seconds old", now-ts)
	}

	return nil
}
