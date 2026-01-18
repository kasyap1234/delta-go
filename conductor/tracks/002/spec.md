# Specification: Multi-Strategy Integration

## Context
The user wants the bot (live and backtest) to dynamically switch between **Grid Trading**, **Funding Arbitrage**, and **Scalping** based on market conditions.

## Dependencies
- **Track 001 (Core System Hardening)**: MUST be completed first. We cannot implement complex multi-strategy logic on top of a broken execution engine (Size mismatch bugs).

## Goals
1.  **Unified Strategy Interface**: Refactor `Grid`, `Funding`, and `Scalper` to share a common `Strategy` interface that accepts `MarketFeatures` and returns a `Signal`.
2.  **Dynamic Selector**: Enhance `StrategySelector` to support weighted or fuzzy logic if needed, though the current priority queue is a good start.
3.  **Backtest Integration**: Ensure `go/pkg/backtest/engine.go` uses the `StrategySelector` instead of a single static strategy.
4.  **Live Bot Integration**: Ensure `go/cmd/structural-bot/main.go` uses the `StrategySelector`.

## Strategy Logic (Existing vs Desired)
- **Funding Arb**: Trigger when annualized basis > 15% (configurable).
- **Grid**: Trigger when Volatility < 30% (ranging).
- **Scalper**: Trigger when Volatility > 30% or as a default when no other conditions are met.
