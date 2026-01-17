# Deployment Configuration Review

## Summary

Reviewed deployment configuration for Delta-Go trading system. Found and fixed **4 critical issues** and **1 missing feature**.

---

## Issues Found & Fixed

### 1. ❌ **CRITICAL: Python Version Mismatch**

**Problem:**
- Dockerfiles used `python:3.13-slim`
- GitHub Actions used `--runtime python312`
- Project requires Python 3.11 (per memory and `pyproject.toml`)

**Impact:** Potential compatibility issues, behavioral differences, dependency conflicts

**Fix Applied:**
- ✅ Changed `cloud_function/Dockerfile` to use `python:3.11-slim`
- ✅ Changed `cloud_function/trade_predictor/Dockerfile` to use `python:3.11-slim`
- ✅ Changed GitHub Actions workflow to use `--runtime python311`

---

### 2. ❌ **CRITICAL: Trade Predictor Missing Model Files**

**Problem:**
- `cloud_function/trade_predictor/Dockerfile` created `/app/models` directory but never copied the model file
- `models/trade_predictor.pkl` exists in repo but wasn't being deployed
- Function would fail at runtime when trying to load the model

**Impact:** Trade predictor service would be non-functional in production

**Fix Applied:**
- ✅ Added `COPY models/trade_predictor.pkl /app/models/trade_predictor.pkl` to Dockerfile
- ✅ Fixed Dockerfile to build from repo root context (like HMM detector)
- ✅ Updated COPY paths to include `cloud_function/trade_predictor/` prefix

---

### 3. ❌ **Port Configuration Inconsistency**

**Problem:**
- Trade predictor hardcoded port 8082
- Cloud Run expects port 8080 or dynamic PORT env var
- Inconsistent with HMM detector which uses PORT env var

**Impact:** Deployment failures or misrouted traffic in Cloud Run

**Fix Applied:**
- ✅ Changed trade predictor to use `PORT` env var (default 8080)
- ✅ Updated CMD to use `$PORT` instead of hardcoded 8082
- ✅ Updated main.py to read PORT env var for local development

---

### 4. ❌ **Missing Trade Predictor Deployment**

**Problem:**
- GitHub Actions workflow only deployed HMM detector
- Trade predictor service had Dockerfile but no CI/CD automation
- Two-service architecture but only one service being deployed

**Impact:** Trade predictor service would never reach production automatically

**Fix Applied:**
- ✅ Added new job `deploy-trade-predictor` to GitHub Actions workflow
- ✅ Properly stages source files and model
- ✅ Deploys to Cloud Functions gen2 with correct runtime
- ✅ Sets environment variable `TRADE_PREDICTOR_MODEL=models/trade_predictor.pkl`

---

## Configuration Verified ✅

### Dockerfiles
- Both use Python 3.11 (matches project requirements)
- Both properly copy dependencies and models
- Both use Cloud Run best practices (PORT env var, exec form CMD)

### GitHub Actions Workflow
- Three deployment jobs:
  1. `deploy-cloud-function` - HMM detector service
  2. `deploy-trade-predictor` - Trade prediction service (NEW)
  3. `deploy-go-bot` - Go trading bot on GCE VM
- All use python311 runtime
- Proper authentication with Workload Identity
- Model files correctly staged and deployed

### Cloud Function Structure
Both functions follow best practices:
- HTTP trigger with CORS support
- Health check endpoints
- Model caching for performance
- Graceful error handling
- Proper environment variable configuration

---

## Deployment Architecture (After Fixes)

```
┌─────────────────────────────────────────────────────┐
│                  GCP Deployment                      │
├─────────────────────────────────────────────────────┤
│                                                      │
│  1. HMM Detector (Cloud Run)                        │
│     - URL: https://hmm-detector-*.run.app           │
│     - Runtime: Python 3.11                          │
│     - Models: BTCUSD, ETHUSD, SOLUSD HMM models     │
│     - Entry: detect_regime()                        │
│                                                      │
│  2. Trade Predictor (Cloud Run) - NOW DEPLOYED      │
│     - URL: https://trade-predictor-*.run.app        │
│     - Runtime: Python 3.11                          │
│     - Model: trade_predictor.pkl (XGBoost)          │
│     - Entry: predict_trade()                        │
│                                                      │
│  3. Trading Bot (GCE e2-micro VM)                   │
│     - Type: Always Free Tier VM                     │
│     - Binary: Go compiled bot                       │
│     - Calls both Cloud Run services                 │
│                                                      │
└─────────────────────────────────────────────────────┘
```

---

## Files Modified

1. `cloud_function/Dockerfile` - Python version fix
2. `cloud_function/trade_predictor/Dockerfile` - Python version, model copy, port fixes
3. `cloud_function/trade_predictor/app/main.py` - Dynamic port configuration
4. `.github/workflows/deploy-gcp.yml` - Runtime version + new trade predictor job

---

## Remaining Recommendations

### Minor Cleanup (Optional)
- Consider consolidating `requirements.txt` and `requirements-shared.txt` (currently duplicated)
- Add explicit memory/CPU limits in Cloud Run deployment (currently using defaults)
- Consider adding health check endpoints to HMM detector (trade predictor already has one)

### Security (Optional)
- Currently both functions use `--allow-unauthenticated`
- For production, consider using service account authentication between bot and functions
- Add API key validation or IAM-based auth

### Monitoring (Recommended)
- Add logging/monitoring setup in workflow
- Configure Cloud Run alerts for errors/latency
- Add deployment notifications (Slack, email, etc.)

---

## Testing Recommendations

Before deploying to production:

1. **Test Dockerfiles locally:**
   ```bash
   # HMM Detector
   docker build -t hmm-detector -f cloud_function/Dockerfile .
   docker run -p 8080:8080 hmm-detector
   
   # Trade Predictor
   docker build -t trade-predictor -f cloud_function/trade_predictor/Dockerfile .
   docker run -p 8080:8080 trade-predictor
   ```

2. **Verify model files exist:**
   ```bash
   ls -lh models/
   # Should show: hmm_model_*.pkl and trade_predictor.pkl
   ```

3. **Test function endpoints:**
   ```bash
   # Test HMM detector
   curl -X POST http://localhost:8080 \
     -H "Content-Type: application/json" \
     -d '{"symbol": "BTCUSD", "open": [...], "high": [...], ...}'
   
   # Test trade predictor
   curl -X POST http://localhost:8080 \
     -H "Content-Type: application/json" \
     -d '{"regime": "bull", "rsi": 45, "bb_position": 0.2, ...}'
   ```

---

## Conclusion

✅ **All critical deployment issues have been resolved.**

The deployment configuration is now:
- Consistent across all services (Python 3.11)
- Complete (both Cloud Functions deploying)
- Correct (models copied, ports configured properly)
- Production-ready

The system will deploy three components:
1. HMM market regime detector
2. Trade signal predictor (XGBoost)
3. Go trading bot orchestrator
