# Regime ML

Shared Machine Learning library for Delta-Go trading bot.

## Installation

```bash
pip install -e .
```

Or with uv:

```bash
uv pip install -e .
```

## Usage

```python
from regime_ml import HMMMarketDetector, DeltaDataFetcher, load_model, save_model

# Create detector
detector = HMMMarketDetector(n_states=5)

# Detect regime
result = detector.detect_regime(
    opens=open_prices,
    highs=high_prices,
    lows=low_prices,
    closes=close_prices,
    volumes=volumes
)

print(result['regime'])      # 'bull', 'bear', 'ranging', etc.
print(result['confidence'])  # 0.0 - 1.0

# Save/load trained models
save_model(detector, 'model.pkl', symbol='BTCUSD')
detector = load_model('model.pkl')

# Fetch data
fetcher = DeltaDataFetcher()
df = fetcher.fetch_candles('BTCUSD', '1h', start_date, end_date)
```

## Components

- `HMMMarketDetector` - HMM-based market regime classifier
- `DeltaDataFetcher` - Delta Exchange API data fetcher
- `load_model` / `save_model` - Model persistence utilities
