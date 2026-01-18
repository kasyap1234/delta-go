# Specification: Core System Hardening

## Context
A codebase investigation revealed critical discrepancies between the project's conventions and the implementation of the Delta Exchange API client, as well as potential risks in order execution logic.

## Issues

### 1. Size Convention Mismatch (CRITICAL)
- **Convention**: `AGENTS.md` states "Position Size is NOTIONAL VALUE in dollars".
- **Backtest**: `go/pkg/backtest/engine.go` treats `Size` as `float64` (USD).
- **Implementation**: `go/pkg/delta/types.go` defines `Size` as `int` (Contract Count) for `Order`, `Position`, and `OrderRequest`.
- **Impact**: Sending a dollar amount (e.g., 500.0) to an API expecting contracts (e.g., 5) could result in unintended massive orders or failures.

### 2. Partial Fill Risk
- **Location**: `go/pkg/delta/orders.go` in `PlaceLimitOrderWithFallback`.
- **Issue**: If a limit order is partially filled, the logic places a market order for the remainder. However, it explicitly avoids attaching bracket orders (Stop Loss/Take Profit) to this subsequent market order, potentially leaving the position unprotected.

## Goals
1.  **Unify Size Type**: Update `go/pkg/delta/types.go` and related logic to handle the conversion between "Notional USD" (internal convention) and "Contracts" (API requirement).
2.  **Fix Order Logic**: Ensure `PlaceLimitOrderWithFallback` correctly handles bracket orders for all fills, including partials.
