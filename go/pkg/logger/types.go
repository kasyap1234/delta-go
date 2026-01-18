package logger

import (
	"time"
)

// Standard log keys
const (
	KeyTraceID     = "trace_id"
	KeyComponent   = "component"
	KeyEnvironment = "environment"
)

// TradeEvent represents a trade execution or update
type TradeEvent struct {
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Timestamp time.Time `json:"timestamp"`
	OrderID   string    `json:"order_id"`
}

// SystemHealthEvent represents a snapshot of a component's health
type SystemHealthEvent struct {
	Component   string        `json:"component"`
	Status      string        `json:"status"`
	Latency     time.Duration `json:"latency_ms"` // serialized as nanoseconds by default
	MemoryUsage int64         `json:"memory_bytes"`
	Timestamp   time.Time     `json:"timestamp"`
}
