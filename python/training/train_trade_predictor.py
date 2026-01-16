"""
Train Trade Predictor Model
Uses historical trade data from backtest to train XGBoost classifier
"""
import json
import pickle
import argparse
from pathlib import Path
import numpy as np
from sklearn.model_selection import train_test_split
from sklearn.metrics import classification_report, accuracy_score
import xgboost as xgb


def prepare_features(trade: dict) -> list:
    """Extract features from a trade record."""
    # These would normally come from the backtest data
    # For now, we derive what we can
    regime = trade.get('regime', 'ranging')
    regime_encoded = {
        'ranging': 0, 'bull': 1, 'bear': 2, 
        'high_volatility': 3, 'low_volatility': 4
    }.get(regime, 0)
    
    # Derive features from available data
    confidence = trade.get('confidence', 0.5)
    
    return [
        50,  # RSI placeholder (would need indicator data)
        0,   # BB position placeholder
        0.02,  # ATR pct placeholder
        1.0,  # Volume ratio placeholder
        1,   # Consecutive candles placeholder
        regime_encoded
    ]


def load_backtest_data(filepath: str) -> tuple:
    """Load and prepare training data from backtest results."""
    with open(filepath, 'r') as f:
        data = json.load(f)
    
    trades = data.get('trades', [])
    
    X = []
    y = []
    
    for trade in trades:
        features = prepare_features(trade)
        X.append(features)
        
        # Label: 1 if profitable, 0 if not
        pnl = trade.get('pnl', 0)
        y.append(1 if pnl > 0 else 0)
    
    return np.array(X), np.array(y)


def train_model(X: np.ndarray, y: np.ndarray) -> xgb.XGBClassifier:
    """Train XGBoost classifier."""
    # Split data
    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=42
    )
    
    # Train model
    model = xgb.XGBClassifier(
        n_estimators=100,
        max_depth=4,
        learning_rate=0.1,
        random_state=42,
        use_label_encoder=False,
        eval_metric='logloss'
    )
    
    model.fit(X_train, y_train)
    
    # Evaluate
    y_pred = model.predict(X_test)
    print("\n=== Model Evaluation ===")
    print(f"Accuracy: {accuracy_score(y_test, y_pred):.2%}")
    print("\nClassification Report:")
    print(classification_report(y_test, y_pred, target_names=['Loss', 'Win']))
    
    return model


def main():
    parser = argparse.ArgumentParser(description='Train trade predictor model')
    parser.add_argument('--data', type=str, default='backtest_results.json',
                        help='Path to backtest results JSON')
    parser.add_argument('--output', type=str, default='../models/trade_predictor.pkl',
                        help='Output model path')
    args = parser.parse_args()
    
    # Load data
    print(f"Loading data from {args.data}...")
    X, y = load_backtest_data(args.data)
    print(f"Loaded {len(y)} trades ({sum(y)} wins, {len(y) - sum(y)} losses)")
    
    if len(y) < 20:
        print("WARNING: Not enough trades for reliable training. Need at least 20.")
        return
    
    # Train model
    print("\nTraining XGBoost model...")
    model = train_model(X, y)
    
    # Save model
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    
    with open(output_path, 'wb') as f:
        pickle.dump(model, f)
    
    print(f"\nModel saved to {output_path}")


if __name__ == '__main__':
    main()
