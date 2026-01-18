# Implementation Plan: Strategy Tuning

## Phase 1: Backtest Engine Cleanup
- [ ] **Step 1**: Modify `go/pkg/backtest/engine.go` to remove the hardcoded `getFundingArbitrageSignal` call in `processTimestamp`. Rely purely on `e.strategyMgr.GetSignal`.

## Phase 2: Parameter Updates
- [ ] **Step 2**: Update `go/pkg/strategy/grid_trading.go`: Set `MaxVolatilityPct` to `100.0`.
- [ ] **Step 3**: Update `go/pkg/strategy/scalper.go`: Set `PersistenceSnapshots` to `2`.

## Phase 3: Verification
- [ ] **Step 4**: Re-run the Jan 2024 backtest and expect significantly more trades.
