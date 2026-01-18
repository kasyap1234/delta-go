"""
Trade Predictor Cloud Function
Predicts trade profitability based on regime + technical indicators
Uses XGBoost model trained on historical trade outcomes
"""
import os
import pickle
from pathlib import Path
import functions_framework
from flask import jsonify, request
import numpy as np

# Model cache
predictor_model = None
# Use absolute path derived from this file's location as fallback
_DEFAULT_MODEL_PATH = str(Path(__file__).parent.parent / "models" / "trade_predictor.pkl")
MODEL_PATH = os.environ.get('TRADE_PREDICTOR_MODEL') or _DEFAULT_MODEL_PATH
# Ensure absolute path
if not os.path.isabs(MODEL_PATH):
    MODEL_PATH = os.path.abspath(MODEL_PATH)


def load_model():
    """Load pre-trained XGBoost model from disk."""
    global predictor_model
    if predictor_model is not None:
        return predictor_model
    
    if os.path.exists(MODEL_PATH):
        try:
            with open(MODEL_PATH, 'rb') as f:
                predictor_model = pickle.load(f)
            print(f"Loaded trade predictor model from {MODEL_PATH}")
            return predictor_model
        except Exception as e:
            print(f"Failed to load model: {e}")
    
    # Return None if no model - will use fallback heuristics
    print("No pre-trained model found, using heuristic fallback")
    return None


def heuristic_prediction(data: dict) -> dict:
    """
    Fallback heuristic when no ML model is available.
    Uses simple rules based on research for each regime.
    """
    regime = data.get('regime', 'ranging')
    rsi = data.get('rsi', 50)
    bb_position = data.get('bb_position', 0)  # -1 to 1 (below/above middle)
    atr_pct = data.get('atr_pct', 0.02)  # ATR as % of price
    volume_ratio = data.get('volume_ratio', 1.0)
    consecutive_candles = data.get('consecutive_candles', 0)  # Same direction
    
    # Initialize
    should_trade = False
    confidence = 0.5
    reasons = []
    
    if regime == 'ranging':
        # Mean reversion - require extreme RSI + BB confirmation
        if rsi < 25 and bb_position < -0.8:
            should_trade = True
            confidence = 0.7
            reasons.append("Strong oversold + below lower BB")
        elif rsi > 75 and bb_position > 0.8:
            should_trade = True
            confidence = 0.7
            reasons.append("Strong overbought + above upper BB")
        else:
            confidence = 0.3
            
    elif regime == 'bull':
        # Trend following - require pullback + momentum confirmation
        if 30 <= rsi <= 45 and consecutive_candles >= 2 and volume_ratio > 1.2:
            should_trade = True
            confidence = 0.75
            reasons.append("RSI pullback + consecutive bullish + volume")
        elif rsi < 30:
            # Too oversold in bull market, wait
            confidence = 0.4
        else:
            confidence = 0.5
            
    elif regime == 'bear':
        # Short on rallies - require overextension + rejection
        if 55 <= rsi <= 70 and consecutive_candles >= 2 and volume_ratio > 1.0:
            should_trade = True
            confidence = 0.7
            reasons.append("RSI rally + consecutive bearish rejection")
        else:
            confidence = 0.4
            
    elif regime == 'high_volatility':
        # Breakout - require strong volume
        if volume_ratio >= 2.0 and atr_pct > 0.025:
            should_trade = True
            confidence = 0.65
            reasons.append("Volume spike + high ATR breakout")
        else:
            confidence = 0.35
            
    elif regime == 'low_volatility':
        # Don't trade, wait for expansion
        should_trade = False
        confidence = 0.2
        reasons.append("Low volatility - monitoring only")
    
    return {
        'should_trade': should_trade,
        'confidence': confidence,
        'reason': ' + '.join(reasons) if reasons else f"Regime: {regime}, no strong signal"
    }


def ml_prediction(model, data: dict) -> dict:
    """Make prediction using trained XGBoost model."""
    # Prepare features in expected order
    features = np.array([[
        data.get('rsi', 50),
        data.get('bb_position', 0),
        data.get('atr_pct', 0.02),
        data.get('volume_ratio', 1.0),
        data.get('consecutive_candles', 0),
        # Encode regime as numeric
        {'ranging': 0, 'bull': 1, 'bear': 2, 'high_volatility': 3, 'low_volatility': 4}.get(
            data.get('regime', 'ranging'), 0
        )
    ]])
    
    # Get probability prediction
    proba = model.predict_proba(features)[0]
    should_trade = bool(proba[1] > 0.5)
    
    return {
        'should_trade': should_trade,
        'confidence': float(proba[1]) if should_trade else float(proba[0]),
        'reason': f"ML prediction (prob: {proba[1]:.2f})"
    }


@functions_framework.http
def predict_trade(request):
    """
    HTTP Cloud Function to predict trade profitability.
    
    Request JSON:
    {
        "regime": "bull" | "bear" | "ranging" | "high_volatility" | "low_volatility",
        "symbol": "BTCUSD",
        "rsi": float (0-100),
        "bb_position": float (-1 to 1),
        "atr_pct": float (ATR / price),
        "volume_ratio": float (current / avg),
        "consecutive_candles": int (same direction)
    }
    
    Response JSON:
    {
        "should_trade": bool,
        "confidence": float (0-1),
        "reason": str
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
        
        # Validate required fields
        required_fields = ['regime']
        for field in required_fields:
            if field not in data:
                return jsonify({"error": f"Missing required field: {field}"}), 400, headers
        
        # Validate regime value
        valid_regimes = {'bull', 'bear', 'ranging', 'high_volatility', 'low_volatility'}
        regime = data.get('regime', '').lower()
        if regime not in valid_regimes:
            return jsonify({"error": f"Invalid regime '{regime}', must be one of: {valid_regimes}"}), 400, headers
        
        # Validate numeric fields if provided
        numeric_fields = ['rsi', 'bb_position', 'atr_pct', 'volume_ratio', 'consecutive_candles']
        for field in numeric_fields:
            if field in data:
                try:
                    float(data[field])
                except (ValueError, TypeError):
                    return jsonify({"error": f"Field '{field}' must be numeric"}), 400, headers
        
        # Try ML model first, fallback to heuristics
        model = load_model()
        
        if model is not None:
            result = ml_prediction(model, data)
        else:
            result = heuristic_prediction(data)
        
        result['symbol'] = data.get('symbol', 'UNKNOWN')
        result['regime'] = data.get('regime', 'unknown')
        
        return jsonify(result), 200, headers

    except Exception as e:
        return jsonify({"error": str(e)}), 500, headers


if __name__ == "__main__":
    os.environ['FUNCTION_TARGET'] = 'predict_trade'
    
    from flask import Flask
    app = Flask(__name__)
    
    @app.route('/', methods=['POST', 'OPTIONS'])
    def handle():
        return predict_trade(request)
    
    @app.route('/health', methods=['GET'])
    def health():
        return jsonify({"status": "healthy"})
    
    app.run(host='0.0.0.0', port=8082, debug=True)
