# Delta-Go Trading Bot

## Build & Test Commands

```bash
cd go
go build ./...      # Build all packages
go test ./...       # Run all tests
```

## Project Structure

- `go/pkg/backtest/` - Backtesting engine
- `go/pkg/strategy/` - Trading strategies
- `go/pkg/delta/` - Delta Exchange API client
- `go/pkg/features/` - Feature engineering
- `go/pkg/risk/` - Risk management
- `go/cmd/backtest/` - Backtest CLI
- `go/cmd/structural-bot/` - Live trading bot

## Code Conventions

- Position `Size` is NOTIONAL VALUE in dollars, not contract count
- Slippage costs tracked in dollars (`EntrySlipCost`, `ExitSlipCost`)
- All timestamps should be in UTC
- Use `float64` for monetary values
