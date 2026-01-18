# Implementation Plan: Core System Hardening

## Phase 1: Size Convention Alignment

- [ ] **Step 1**: Analyze `go/pkg/delta/types.go` and identify all `Size` fields requiring abstraction.
- [ ] **Step 2**: Implement a `SizeConverter` or helper functions in `go/pkg/delta` to convert Notional USD -> Contract Count based on market price and contract value.
- [ ] **Step 3**: Update `OrderRequest` struct or the sending logic to perform this conversion before sending to the API.
- [ ] **Step 4**: Update `Position` and `Order` structs (or their parsing logic) to convert received Contract Counts back to Notional USD for internal use.
- [ ] **Step 5**: Verify backtest compatibility (ensure it still runs correctly with these changes, though it likely mocks the API).

## Phase 2: Order Execution Safety

- [ ] **Step 6**: Refactor `PlaceLimitOrderWithFallback` in `go/pkg/delta/orders.go`.
    - Ensure that if a cleanup market order is placed, it attaches the necessary Bracket Orders (SL/TP).
    - Or, ensure that the original Bracket Orders are updated/replaced to cover the new total position.
- [ ] **Step 7**: Add unit tests for `PlaceLimitOrderWithFallback` simulating partial fills to verify protection.

## Phase 3: Verification

- [ ] **Step 8**: Run `go build ./...` and `go test ./...` to ensure no regressions.
