# Delta-Go Trading Bot

A cryptocurrency trading bot for Delta Exchange with HMM-based market regime detection.

## Project Structure

```
delta-go/
├── go/                          # Go trading bot and strategy server
│   ├── cmd/
│   │   ├── bot/                 # Main trading bot binary
│   │   └── backtest/            # Strategy server for backtesting
│   ├── pkg/
│   │   ├── delta/               # Delta Exchange API client
│   │   ├── risk/                # Risk management
│   │   └── strategy/            # Trading strategies
│   ├── config/                  # Configuration
│   ├── go.mod
│   └── go.sum
│
├── python/                      # Python ML and analysis tools
│   ├── regime_ml/               # Shared ML library (installable package)
│   │   ├── src/regime_ml/
│   │   │   ├── hmm_detector.py  # HMM market regime detector
│   │   │   ├── data_fetcher.py  # Delta Exchange data fetcher
│   │   │   └── model_io.py      # Model save/load utilities
│   │   └── pyproject.toml
│   ├── training/                # Model training scripts
│   │   └── train_hmm.py         # Walk-forward HMM training
│   └── backtest/                # Backtesting engine
│       └── backtest.py          # Walk-forward backtester
│
├── cloud_function/              # GCP Cloud Run deployment
│   ├── app/
│   │   └── main.py              # Cloud Function entrypoint
│   ├── Dockerfile
│   └── requirements.txt
│
├── models/                      # Trained model artifacts (.pkl)
│   ├── hmm_model_BTCUSD.pkl
│   ├── hmm_model_ETHUSD.pkl
│   └── hmm_model_SOLUSD.pkl
│
└── .env.example                 # Environment variable template
```

## Components

### Go Trading Bot (`go/`)

The main trading bot written in Go. Connects to Delta Exchange via WebSocket and REST APIs.

```bash
cd go
go build -o bot ./cmd/bot
./bot
```

### Python ML (`python/`)

#### regime_ml - Shared Library

The core HMM-based market regime detection library. Install it locally:

```bash
cd python/regime_ml
pip install -e .
```

#### Training

Train HMM models with walk-forward validation:

```bash
cd python/training
uv run python train_hmm.py --symbol BTCUSD --months 12
```

#### Backtesting

Run walk-forward backtests:

```bash
cd python/backtest
uv run python backtest.py --symbols BTCUSD,ETHUSD,SOLUSD --months 6
```

### Cloud Function (`cloud_function/`)

Deployed to GCP Cloud Run for real-time regime detection.

Build and deploy from repo root:

```bash
# Build the Docker image
docker build -t hmm-detector -f cloud_function/Dockerfile .

# Deploy to Cloud Run
gcloud run deploy hmm-detector \
  --image gcr.io/PROJECT_ID/hmm-detector \
  --platform managed \
  --region us-central1
```

## Environment Variables

Copy `.env.example` to `.env` and configure:

```bash
DELTA_API_KEY=your_api_key
DELTA_API_SECRET=your_api_secret
DELTA_TESTNET=true
HMM_ENDPOINT=https://your-cloud-run-url
```

## Market Regimes

The HMM detector classifies markets into 5 regimes:

1. **Bull** - Strong uptrend, trend-following strategies
2. **Bear** - Strong downtrend, short strategies
3. **Ranging** - Sideways consolidation, mean-reversion
4. **High Volatility** - Explosive moves, breakout strategies
5. **Low Volatility** - Quiet market, preparation mode

## License

Private - All rights reserved
