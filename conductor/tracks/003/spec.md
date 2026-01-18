# Specification: Strategy Tuning

## Context
Initial backtests showed extremely low activity (1 trade in 30 days). Investigation revealed that default parameters are too conservative for crypto markets and backtesting resolution.

## Issues
1.  **Backtest Engine Hardcoding:** `go/pkg/backtest/engine.go` has a hardcoded `50%` funding threshold that overrides the `StrategySelector`.
2.  **Scalper Latency:** `PersistenceSnapshots=5` at 5m resolution = 25m delay, missing scalps.
3.  **Grid Constraints:** `MaxVolatilityPct=50` is too low for active crypto periods (Jan 2024 ETF).

## Goals
1.  **Delegation:** Make `backtest/engine.go` fully delegate signal generation to `StrategySelector` (remove internal `getFundingArbitrageSignal` logic if it duplicates strategy).
2.  **Optimization:** Update default config in `strategy/*.go` to be more crypto-friendly:
    - Grid: Increase Max Volatility to 100%.
    - Scalper: Reduce Persistence to 2 snapshots.
    - Funding: Ensure 15% threshold is respected.
