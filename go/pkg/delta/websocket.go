package delta

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kasyap/delta-go/go/config"
)

type subscription struct {
	name    string
	symbols []string
}

// WebSocketClient handles real-time market data from Delta Exchange
type WebSocketClient struct {
	cfg  *config.Config
	conn *websocket.Conn
	url  string

	// Subscriptions
	subscriptions []subscription

	// Callbacks
	onTicker           func(Ticker)
	onCandle           func(Candle)
	onCandleWithSymbol func(symbol string, candle Candle) // Enhanced callback with symbol
	onOrderbook        func(json.RawMessage)
	onFundingRate      func(FundingRateUpdate)
	onError            func(error)

	// State
	mu           sync.RWMutex
	isConnected  bool
	stopChan     chan struct{}
	reconnecting bool
	closeOnce    sync.Once
	writeMu      sync.Mutex
	started      bool
}

// FundingRateUpdate represents a funding rate update message
type FundingRateUpdate struct {
	Symbol      string  `json:"symbol"`
	FundingRate float64 `json:"funding_rate"`
	Timestamp   int64   `json:"timestamp"`
}

// WebSocketMessage represents a message from Delta Exchange WebSocket
type WebSocketMessage struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel,omitempty"`
	Symbol  string          `json:"symbol,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// NewWebSocketClient creates a new WebSocket client
func NewWebSocketClient(cfg *config.Config) *WebSocketClient {
	return &WebSocketClient{
		cfg:           cfg,
		url:           cfg.WebSocketURL,
		subscriptions: []subscription{},
		stopChan:      make(chan struct{}),
	}
}

// OnTicker sets the ticker callback
func (ws *WebSocketClient) OnTicker(callback func(Ticker)) {
	ws.onTicker = callback
}

// OnCandle sets the candle callback
func (ws *WebSocketClient) OnCandle(callback func(Candle)) {
	ws.onCandle = callback
}

// OnCandleWithSymbol sets the candle callback with symbol context
func (ws *WebSocketClient) OnCandleWithSymbol(callback func(symbol string, candle Candle)) {
	ws.onCandleWithSymbol = callback
}

// OnOrderbook sets the orderbook callback
func (ws *WebSocketClient) OnOrderbook(callback func(json.RawMessage)) {
	ws.onOrderbook = callback
}

func (ws *WebSocketClient) OnFundingRate(callback func(FundingRateUpdate)) {
	ws.onFundingRate = callback
}

// OnError sets the error callback
func (ws *WebSocketClient) OnError(callback func(error)) {
	ws.onError = callback
}

// Connect establishes WebSocket connection
func (ws *WebSocketClient) Connect() error {
	// Create custom TLS config that forces HTTP/1.1 (disables ALPN for HTTP/2)
	// This is required because CDNs like CloudFront will negotiate HTTP/2 via ALPN
	// but websocket upgrade requires HTTP/1.1
	tlsConfig := &tls.Config{
		NextProtos: []string{"http/1.1"}, // Force HTTP/1.1 only
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  tlsConfig,
	}

	// Add Origin and User-Agent headers which many exchanges/CDNs require
	// IMPORTANT: Origin must match the websocket host, not the REST API host
	headers := make(http.Header)
	if u, err := url.Parse(ws.url); err == nil {
		headers.Add("Origin", "https://"+u.Host)
	} else {
		headers.Add("Origin", "https://india.delta.exchange")
	}
	headers.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	headers.Add("Accept-Language", "en-US,en;q=0.9")

	conn, resp, err := dialer.Dial(ws.url, headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("websocket dial failed with status %d: %v", resp.StatusCode, err)
		}
		return fmt.Errorf("websocket dial failed: %v", err)
	}

	ws.mu.Lock()
	oldConn := ws.conn
	ws.conn = conn
	ws.isConnected = true
	startLoops := !ws.started
	ws.started = true
	subs := make([]subscription, len(ws.subscriptions))
	copy(subs, ws.subscriptions)
	ws.mu.Unlock()

	if oldConn != nil {
		_ = oldConn.Close()
	}

	if startLoops {
		go ws.readMessages()
		go ws.heartbeat()
	}

	// Resubscribe to channels
	for _, sub := range subs {
		_ = ws.sendSubscribe(sub)
	}

	log.Printf("WebSocket connected to %s", ws.url)
	return nil
}

// Subscribe subscribes to a channel
func (ws *WebSocketClient) Subscribe(channel string, symbols []string) error {
	ws.mu.Lock()
	for _, existing := range ws.subscriptions {
		if existing.name == channel && equalStringSlice(existing.symbols, symbols) {
			ws.mu.Unlock()
			return nil
		}
	}
	ws.subscriptions = append(ws.subscriptions, subscription{name: channel, symbols: append([]string(nil), symbols...)})
	isConnected := ws.isConnected
	ws.mu.Unlock()

	if isConnected {
		return ws.sendSubscribe(subscription{name: channel, symbols: symbols})
	}
	return nil
}

// SubscribeTicker subscribes to ticker updates for a symbol
func (ws *WebSocketClient) SubscribeTicker(symbol string) error {
	return ws.Subscribe("v2/ticker", []string{symbol})
}

// SubscribeCandles subscribes to candle updates
func (ws *WebSocketClient) SubscribeCandles(symbol, resolution string) error {
	return ws.Subscribe(fmt.Sprintf("candlestick_%s", resolution), []string{symbol})
}

// SubscribeOrderbook subscribes to L2 orderbook
func (ws *WebSocketClient) SubscribeOrderbook(symbol string) error {
	return ws.Subscribe("l2_orderbook", []string{symbol})
}

func (ws *WebSocketClient) SubscribeFundingRate(symbols []string) error {
	return ws.Subscribe("funding_rate", symbols)
}

// sendSubscribe sends subscription message
func (ws *WebSocketClient) sendSubscribe(sub subscription) error {
	var symbolsPayload interface{} = "all"
	if len(sub.symbols) > 0 {
		symbolsPayload = sub.symbols
	}

	msg := map[string]interface{}{
		"type": "subscribe",
		"payload": map[string]interface{}{
			"channels": []map[string]interface{}{
				{"name": sub.name, "symbols": symbolsPayload},
			},
		},
	}

	return ws.sendJSON(msg)
}

// sendJSON sends a JSON message
func (ws *WebSocketClient) sendJSON(msg interface{}) error {
	ws.mu.RLock()
	if ws.conn == nil {
		ws.mu.RUnlock()
		return fmt.Errorf("websocket not connected")
	}
	conn := ws.conn
	ws.mu.RUnlock()

	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	return conn.WriteJSON(msg)
}

// readMessages handles incoming WebSocket messages
func (ws *WebSocketClient) readMessages() {
	for {
		select {
		case <-ws.stopChan:
			return
		default:
			ws.mu.RLock()
			conn := ws.conn
			ws.mu.RUnlock()
			if conn == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				if ws.onError != nil {
					ws.onError(err)
				}
				ws.reconnect()
				continue
			}

			ws.handleMessage(message)
		}
	}
}

// handleMessage processes incoming messages
func (ws *WebSocketClient) handleMessage(data []byte) {
	var msg WebSocketMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Failed to parse WebSocket message: %v", err)
		return
	}

	switch {
	case msg.Type == "v2/ticker" || msg.Channel == "v2/ticker" || containsSubstr(msg.Type, "ticker") || containsSubstr(msg.Channel, "ticker"):
		if ws.onTicker != nil {
			var ticker Ticker
			if err := json.Unmarshal(msg.Data, &ticker); err == nil {
				ws.onTicker(ticker)
			}
		}

	case containsSubstr(msg.Type, "candlestick") || containsSubstr(msg.Channel, "candlestick"):
		if ws.onCandle != nil || ws.onCandleWithSymbol != nil {
			var candle Candle
			if err := json.Unmarshal(msg.Data, &candle); err == nil {
				if ws.onCandle != nil {
					ws.onCandle(candle)
				}
				if ws.onCandleWithSymbol != nil {
					ws.onCandleWithSymbol(msg.Symbol, candle)
				}
			}
		}

	case containsSubstr(msg.Type, "l2_orderbook") || containsSubstr(msg.Channel, "l2_orderbook"):
		if ws.onOrderbook != nil {
			ws.onOrderbook(msg.Data)
		}

	case msg.Type == "subscribed":
		log.Printf("Subscribed to: %s", msg.Channel)

	case msg.Type == "error":
		log.Printf("WebSocket error: %s", string(data))
		if ws.onError != nil {
			ws.onError(fmt.Errorf("websocket error: %s", string(data)))
		}
	}
}

// heartbeat sends periodic pings to keep connection alive
func (ws *WebSocketClient) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ws.stopChan:
			return
		case <-ticker.C:
			ws.mu.RLock()
			conn := ws.conn
			isConnected := ws.isConnected
			ws.mu.RUnlock()
			if conn == nil || !isConnected {
				continue
			}
			ws.writeMu.Lock()
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			ws.writeMu.Unlock()
			if err != nil {
				log.Printf("Heartbeat ping failed: %v", err)
			}
		}
	}
}

// reconnect attempts to reconnect with exponential backoff
func (ws *WebSocketClient) reconnect() {
	ws.mu.Lock()
	if ws.reconnecting {
		ws.mu.Unlock()
		return
	}
	ws.reconnecting = true
	ws.isConnected = false
	ws.mu.Unlock()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ws.stopChan:
			return
		default:
			log.Printf("Attempting to reconnect in %v...", backoff)
			time.Sleep(backoff)

			if err := ws.Connect(); err != nil {
				log.Printf("Reconnection failed: %v", err)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}

			ws.mu.Lock()
			ws.reconnecting = false
			ws.mu.Unlock()

			log.Println("Successfully reconnected")
			return
		}
	}
}

// Close closes the WebSocket connection (idempotent - safe to call multiple times)
func (ws *WebSocketClient) Close() {
	ws.closeOnce.Do(func() {
		close(ws.stopChan)
	})

	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.conn != nil {
		ws.conn.Close()
		ws.conn = nil
	}
	ws.isConnected = false
}

// IsConnected returns connection status
func (ws *WebSocketClient) IsConnected() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.isConnected
}

// helper function
func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
