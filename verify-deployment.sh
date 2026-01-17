#!/bin/bash
# Deployment Configuration Verification Script

set -e

echo "======================================"
echo "Deployment Configuration Verification"
echo "======================================"
echo ""

# Check if required model files exist
echo "1. Checking model files..."
MODELS_DIR="models"

if [ ! -f "$MODELS_DIR/hmm_model_BTCUSD.pkl" ]; then
    echo "  ❌ Missing: hmm_model_BTCUSD.pkl"
    exit 1
else
    echo "  ✅ Found: hmm_model_BTCUSD.pkl"
fi

if [ ! -f "$MODELS_DIR/hmm_model_ETHUSD.pkl" ]; then
    echo "  ❌ Missing: hmm_model_ETHUSD.pkl"
    exit 1
else
    echo "  ✅ Found: hmm_model_ETHUSD.pkl"
fi

if [ ! -f "$MODELS_DIR/hmm_model_SOLUSD.pkl" ]; then
    echo "  ❌ Missing: hmm_model_SOLUSD.pkl"
    exit 1
else
    echo "  ✅ Found: hmm_model_SOLUSD.pkl"
fi

if [ ! -f "$MODELS_DIR/trade_predictor.pkl" ]; then
    echo "  ❌ Missing: trade_predictor.pkl"
    exit 1
else
    echo "  ✅ Found: trade_predictor.pkl"
fi

echo ""

# Check Python version in Dockerfiles
echo "2. Checking Dockerfile Python versions..."

HMM_PYTHON=$(grep "FROM python:" cloud_function/Dockerfile | head -1)
if echo "$HMM_PYTHON" | grep -q "3.11"; then
    echo "  ✅ HMM Detector: $HMM_PYTHON"
else
    echo "  ❌ HMM Detector: $HMM_PYTHON (should be python:3.11-slim)"
    exit 1
fi

TRADE_PYTHON=$(grep "FROM python:" cloud_function/trade_predictor/Dockerfile | head -1)
if echo "$TRADE_PYTHON" | grep -q "3.11"; then
    echo "  ✅ Trade Predictor: $TRADE_PYTHON"
else
    echo "  ❌ Trade Predictor: $TRADE_PYTHON (should be python:3.11-slim)"
    exit 1
fi

echo ""

# Check GitHub Actions runtime
echo "3. Checking GitHub Actions Python runtime..."
if grep -q "python311" .github/workflows/deploy-gcp.yml; then
    echo "  ✅ GitHub Actions uses python311 runtime"
else
    echo "  ❌ GitHub Actions runtime incorrect (should be python311)"
    exit 1
fi

echo ""

# Check trade predictor deployment job exists
echo "4. Checking trade predictor deployment job..."
if grep -q "deploy-trade-predictor:" .github/workflows/deploy-gcp.yml; then
    echo "  ✅ Trade predictor deployment job exists"
else
    echo "  ❌ Trade predictor deployment job missing"
    exit 1
fi

echo ""

# Check model copying in Dockerfiles
echo "5. Checking model COPY commands..."

if grep -q "COPY models/" cloud_function/Dockerfile; then
    echo "  ✅ HMM Detector copies model files"
else
    echo "  ❌ HMM Detector missing model COPY"
    exit 1
fi

if grep -q "COPY models/trade_predictor.pkl" cloud_function/trade_predictor/Dockerfile; then
    echo "  ✅ Trade Predictor copies model file"
else
    echo "  ❌ Trade Predictor missing model COPY"
    exit 1
fi

echo ""

# Check PORT environment variable usage
echo "6. Checking PORT configuration..."

if grep -q "ENV PORT=8080" cloud_function/Dockerfile && \
   grep -q '\$PORT' cloud_function/Dockerfile; then
    echo "  ✅ HMM Detector uses PORT env var"
else
    echo "  ❌ HMM Detector PORT configuration incorrect"
    exit 1
fi

if grep -q "ENV PORT=8080" cloud_function/trade_predictor/Dockerfile && \
   grep -q '\$PORT' cloud_function/trade_predictor/Dockerfile; then
    echo "  ✅ Trade Predictor uses PORT env var"
else
    echo "  ❌ Trade Predictor PORT configuration incorrect"
    exit 1
fi

echo ""
echo "======================================"
echo "✅ All deployment checks passed!"
echo "======================================"
echo ""
echo "Next steps:"
echo "  1. Test Docker builds locally:"
echo "     docker build -t hmm-detector -f cloud_function/Dockerfile ."
echo "     docker build -t trade-predictor -f cloud_function/trade_predictor/Dockerfile ."
echo ""
echo "  2. Test containers locally:"
echo "     docker run -p 8080:8080 hmm-detector"
echo "     docker run -p 8080:8080 trade-predictor"
echo ""
echo "  3. Deploy to GCP via GitHub Actions workflow"
echo ""
