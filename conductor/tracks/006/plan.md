# Implementation Plan: Strategy Optimization

## Phase 1: Grid Optimization (Recenter)
- [ ] **Step 1**: Modify `GridTradingStrategy.Analyze` in `go/pkg/strategy/grid_trading.go`.
    - Check distance between `currentPrice` and `gridCenter`.
    - If distance > threshold, return `ActionClose` (or special `ActionReset`) to clear state, then next tick will recenter.

## Phase 2: Scalper Optimization (Filters)
- [ ] **Step 2**: Modify `FeeAwareScalper.Analyze` in `go/pkg/strategy/scalper.go`.
    - Add check: `if f.HistoricalVol < minScalpVol { return ActionNone }`.
    - Tweak `ImbalanceThreshold` or `ConfirmationPricePct` for better selectivity.

## Phase 3: Verification
- [ ] **Step 3**: Run backtest to see if "Trades" metric improves (or at least quality of trade logic).
