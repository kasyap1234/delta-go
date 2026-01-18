# Specification: Strategy Optimization

## Context
User requests higher profits and lower losses for Grid and Scalping strategies. The current implementations are relatively static.

## Goals
1.  **Dynamic Grid:** Implement "Recenter" logic. If current price deviates significantly from the grid center (e.g., > 20% of range), cancel existing grid and restart centered on new price. This prevents holding large losing bags in a trend.
2.  **Smart Scalp:**
    - **Volatility Filter:** Only scalp if `IV` or `HistoricalVol` is above a threshold (avoid chop-outs in dead markets).
    - **Trailing Stop:** (If feasible) or tighter initial stops.

## Scope
- Modify `go/pkg/strategy/grid_trading.go`
- Modify `go/pkg/strategy/scalper.go`
