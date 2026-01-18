# Implementation Plan: Pure Arbitrage

## Phase 1: Signal Enhancement
- [ ] **Step 1**: Add `IsHedged` field to `strategy.Signal` struct in `go/pkg/strategy/strategy.go`.
- [ ] **Step 2**: Update `FundingArbitrageStrategy` in `go/pkg/strategy/funding_arbitrage.go` to set `IsHedged: true`.

## Phase 2: Execution Logic
- [ ] **Step 3**: Implement `GetSpotSymbol(perpSymbol string)` helper in `go/pkg/delta/client.go` or `util.go`.
- [ ] **Step 4**: Modify `executeFundingArbEntry` in `go/cmd/structural-bot/main.go`:
    - Check `signal.IsHedged`.
    - If true, resolve Spot symbol.
    - Calculate size for Spot (contracts * contract_value / spot_price).
    - Execute Spot Buy.
    - Execute Perp Short (only if Spot Buy succeeded).

## Phase 3: Verification
- [ ] **Step 5**: Build and verify.
