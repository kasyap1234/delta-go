# Implementation Plan: Multi-Strategy Integration

## Phase 1: Interface Unification
- [ ] **Step 1**: Define a unified `Strategy` interface in `go/pkg/strategy/types.go` (or `strategy.go`) that accepts `features.MarketFeatures`.
- [ ] **Step 2**: Refactor `GridTradingStrategy`, `FundingArbitrageStrategy`, and `FeeAwareScalper` to implement this interface.

## Phase 2: Selector Enhancement
- [ ] **Step 3**: Update `StrategySelector` to use the unified interface.
- [ ] **Step 4**: Add configuration to `StrategySelector` to allow users to tune the switching thresholds (e.g., changing the 15% funding threshold via config).

## Phase 3: Integration
- [ ] **Step 5**: Wire `StrategySelector` into `go/pkg/backtest/engine.go`.
- [ ] **Step 6**: Wire `StrategySelector` into `go/cmd/structural-bot/main.go`.

## Phase 4: Verification
- [ ] **Step 7**: Create a backtest scenario that transitions through different regimes (Low Vol -> High Vol -> High Funding) to verify strategy switching.
