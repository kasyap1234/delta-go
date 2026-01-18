# Specification: Pure Arbitrage

## Context
The user wants the bot to use a "pure arbitrage" strategy, specifically for funding arbitrage. This implies maintaining a Delta Neutral position (e.g., Short Perp + Long Spot) to capture funding rates without exposure to price movements.

## Goals
1.  **Hedged Signals:** Update `FundingArbitrageStrategy` to request hedged execution.
2.  **Execution Logic:** Update `StructuralBot` to execute the hedge leg (Spot Buy) when entering a Funding Arb position (Short Perp).
3.  **Symbol Resolution:** Implement logic to find the corresponding Spot symbol for a Perp (e.g., `BTCUSD` -> `BTCUSDT` or `BTC/USDT`).

## Constraints
- We assume Delta Exchange supports Spot trading via the same API.
- We assume the user has capital to fund both legs (requires 2x capital or splitting capital).

## Risks
- **Execution Risk:** One leg fills, the other doesn't (Legging risk). We will implement sequential execution (Spot first, then Perp) or parallel with retry.
- **Liquidation Risk:** Spot is not margined, but Perp is. If price moons, Perp PnL drops, but Spot value rises. User needs to manage margin to avoid Perp liquidation.
