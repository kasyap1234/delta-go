# Specification: Futures-Based Hedging

## Context
Delta Exchange India does not support Spot trading. To achieve "Pure Arbitrage" (Delta Neutrality) for the Funding Strategy, we must hedge the Short Perpetual position with a Long Futures position.

## Goals
1.  **Re-enable Hedging:** Restore `IsHedged` flag in signals.
2.  **Futures Resolution:** Implement logic to find the best matching Futures contract for a given Perpetual.
    - Preference: Nearest Quarterly or Monthly future with sufficient liquidity.
3.  **Execution:** Atomically (or sequentially) execute Long Future + Short Perp.

## Strategy
- **Entry:** Buy Future (Long) + Sell Perp (Short).
- **Exit:** Sell Future (Close Long) + Buy Perp (Close Short).
- **PnL:** Funding Income - (Future Entry Premium - Future Exit Premium).

## Risks
- **Basis Risk:** The premium on the future might shrink/expand unfavorably.
- **Liquidity:** Futures might have wider spreads than Perps.
