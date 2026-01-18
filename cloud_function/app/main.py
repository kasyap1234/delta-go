"""
HMM Market Regime Detector for Trading Bot
Deployed as a Cloud Run function

Supports per-coin models: BTCUSD, ETHUSD, SOLUSD, etc.
"""
import os
import functions_framework
from flask import jsonify, request
import numpy as np

from pathlib import Path
from regime_ml import HMMMarketDetector, load_model

detectors = {}
# Use absolute path derived from this file's location as fallback
_DEFAULT_MODEL_DIR = str(Path(__file__).parent / "models")
MODEL_DIR = os.environ.get('HMM_MODEL_DIR') or _DEFAULT_MODEL_DIR
# Ensure absolute path
if not os.path.isabs(MODEL_DIR):
    MODEL_DIR = os.path.abspath(MODEL_DIR)


def get_detector(symbol: str) -> HMMMarketDetector:
    """
    Load pre-trained model for specific symbol or create new detector.
    Models are cached in memory after first load.
    
    Model naming convention: hmm_model_{symbol}.pkl
    """
    global detectors
    
    symbol = symbol.upper().strip()
    
    if symbol in detectors:
        return detectors[symbol]
    
    model_path = os.path.join(MODEL_DIR, f'hmm_model_{symbol}.pkl')
    
    if os.path.exists(model_path):
        try:
            detector = load_model(model_path)
            detectors[symbol] = detector
            print(f"Loaded pre-trained model for {symbol} from {model_path}")
            return detector
        except Exception as e:
            print(f"Failed to load model for {symbol}: {e}")
    
    generic_path = os.path.join(MODEL_DIR, 'hmm_model.pkl')
    if os.path.exists(generic_path):
        try:
            detector = load_model(generic_path)
            detectors[symbol] = detector
            print(f"Using generic model for {symbol}")
            return detector
        except Exception as e:
            print(f"Failed to load generic model: {e}")
    
    print(f"No pre-trained model for {symbol}, will train on first request")
    detector = HMMMarketDetector(n_states=5)
    detectors[symbol] = detector
    return detector


@functions_framework.http
def detect_regime(request):
    """
    HTTP Cloud Function to detect market regime.
    
    Request JSON:
    {
        "symbol": "BTCUSD" | "ETHUSD" | "SOLUSD",
        "open": [float, ...],
        "high": [float, ...],
        "low": [float, ...],
        "close": [float, ...],
        "volume": [float, ...]
    }
    
    Response JSON:
    {
        "regime": "bull" | "bear" | "ranging" | "high_volatility" | "low_volatility",
        "confidence": float,
        "symbol": str,
        "features": {...}
    }
    """
    if request.method == 'OPTIONS':
        headers = {
            'Access-Control-Allow-Origin': '*',
            'Access-Control-Allow-Methods': 'POST',
            'Access-Control-Allow-Headers': 'Content-Type',
            'Access-Control-Max-Age': '3600'
        }
        return ('', 204, headers)

    headers = {'Access-Control-Allow-Origin': '*'}

    try:
        data = request.get_json()
        
        if not data:
            return jsonify({"error": "No data provided"}), 400, headers
        
        symbol = data.get('symbol', 'BTCUSD')
        
        required_fields = ['open', 'high', 'low', 'close', 'volume']
        for field in required_fields:
            if field not in data:
                return jsonify({"error": f"Missing field: {field}"}), 400, headers
        
        # Validate array lengths match
        lengths = {field: len(data[field]) for field in required_fields}
        unique_lengths = set(lengths.values())
        if len(unique_lengths) > 1:
            return jsonify({"error": f"Array length mismatch: {lengths}"}), 400, headers
        
        # Validate minimum data points
        data_len = lengths['close']
        if data_len < 50:
            return jsonify({"error": f"Insufficient data: {data_len} points, need at least 50"}), 400, headers
        
        # Validate numeric data
        try:
            opens = np.array(data['open'], dtype=np.float64)
            highs = np.array(data['high'], dtype=np.float64)
            lows = np.array(data['low'], dtype=np.float64)
            closes = np.array(data['close'], dtype=np.float64)
            volumes = np.array(data['volume'], dtype=np.float64)
        except (ValueError, TypeError) as e:
            return jsonify({"error": f"Invalid numeric data: {e}"}), 400, headers
        
        det = get_detector(symbol)
        
        result = det.detect_regime(
            opens=opens,
            highs=highs,
            lows=lows,
            closes=closes,
            volumes=volumes
        )
        
        result['symbol'] = symbol
        
        return jsonify(result), 200, headers

    except Exception as e:
        return jsonify({"error": str(e)}), 500, headers


if __name__ == "__main__":
    os.environ['FUNCTION_TARGET'] = 'detect_regime'
    
    from flask import Flask
    app = Flask(__name__)
    
    @app.route('/', methods=['POST', 'OPTIONS'])
    def handle():
        return detect_regime(request)
    
    app.run(host='0.0.0.0', port=8080, debug=True)
