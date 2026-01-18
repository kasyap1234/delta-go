# Implementation Plan: Futures-Based Hedging

## Phase 1: Restore Signal Flags
- [ ] **Step 1**: Re-add `IsHedged` bool to `strategy.Signal` in `go/pkg/strategy/strategy.go`.
- [ ] **Step 2**: Re-enable `IsHedged: true` in `FundingArbitrageStrategy` (`go/pkg/strategy/funding_arbitrage.go`).

## Phase 2: Futures Resolution
- [ ] **Step 3**: Implement `GetFuturesProductForPerp(perpSymbol string)` in `go/pkg/delta/client.go`.
    - Logic: List products, filter for `product_type="futures"` (or similar), match underlying asset (e.g., "BTC"), select the one with earliest expiry > 7 days (to avoid immediate rollover).

## Phase 3: Execution Update
- [ ] **Step 4**: Update `executeFundingArbEntry` in `go/cmd/structural-bot/main.go`.
    - Call `GetFuturesProductForPerp`.
    - Calculate size (Notional -> Contracts) for Future.
    - Execute Long Future.
    - Execute Short Perp.

## Phase 4: Verification
- [ ] **Step 5**: Build and verify.
