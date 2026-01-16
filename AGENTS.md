# Delta-Go Development Guide

## Build Commands

### Go
```bash
cd go
go build ./...          # Build all
go vet ./...            # Lint
go test ./...           # Test
go build -o bot ./cmd/bot                    # Build trading bot
go build -o strategy-server ./cmd/backtest   # Build strategy server
```

### Python
```bash
cd python/regime_ml
pip install -e .        # Install shared library

cd python/training
uv run python train_hmm.py --symbol BTCUSD

cd python/backtest
uv run python backtest.py --symbols BTCUSD,ETHUSD,SOLUSD
```

### Cloud Function

**HMM Detector** (Market regime detection):
```bash
# Build from repo root (required for shared library access)
docker build -t hmm-detector -f cloud_function/Dockerfile .
```

**Trade Predictor** (XGBoost-based trade signal prediction):
```bash
# Build from repo root
docker build -t trade-predictor -f cloud_function/trade_predictor/Dockerfile .
```

## Project Structure

- `go/` - Go trading bot, strategy server, Delta API client
- `python/regime_ml/` - Shared ML library (HMM detector, data fetcher)
- `python/training/` - Model training scripts
- `python/backtest/` - Backtesting engine
- `cloud_function/` - GCP Cloud Run deployments
  - `app/` - HMM market regime detector
  - `trade_predictor/` - XGBoost trade signal predictor
- `models/` - Trained HMM model artifacts (.pkl) and trade prediction models

## Code Style

- Go: Standard gofmt formatting
- Python: Black formatter, type hints preferred
- No comments unless code is complex

## Key Patterns

- Go strategies implement `Strategy` interface in `pkg/strategy/strategy.go`
- Python ML code imports from `regime_ml` package
- HTTP clients use `%w` error wrapping for proper error chains
- Indicators have `*Last` variants for single-value computation (performance)
