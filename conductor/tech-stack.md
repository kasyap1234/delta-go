# Tech Stack

## Core Language & Runtime
- **Language:** Go (Golang)
- **Version:** 1.22.0
- **Rationale:** High-performance, excellent concurrency primitives (goroutines/channels) for high-frequency trading, and a strong standard library.

## Exchange Integration
- **Platform:** Delta Exchange India
- **API Connectivity:** REST API and WebSockets for real-time market data and order management.
- **Key Library:** `github.com/gorilla/websocket` for reliable WebSocket communication.

## System Components
- **Strategy Engine:** Custom Go-based implementation for Scalper, Grid Trading, and Funding Arbitrage strategies.
- **Risk Management:** Centralized risk manager for position sizing, stop-loss/take-profit logic, and exposure control.
- **Backtesting Engine:** Integrated Go framework for high-fidelity strategy validation using historical data.

## Infrastructure & Tooling
- **Dependency Management:** Go Modules (`go.mod`)
- **Testing:** Go's built-in `testing` package for unit and integration tests.
