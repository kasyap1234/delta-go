"""
Model I/O utilities for HMM Market Detector

Provides functions for saving and loading trained HMM models.
"""

import pickle
from datetime import datetime
from typing import Optional

from .hmm_detector import HMMMarketDetector


def save_model(
    detector: HMMMarketDetector, 
    output_path: str, 
    symbol: Optional[str] = None
) -> None:
    """
    Save trained model to disk.
    
    Args:
        detector: Trained HMMMarketDetector instance
        output_path: Path to save the model (.pkl)
        symbol: Optional symbol name for metadata
    """
    model_data = {
        'model': detector.model,
        'scaler': detector.scaler,  # Persist scaler for correct feature transformation
        'n_states': detector.n_states,
        'state_to_regime': detector._state_to_regime,
        'symbol': symbol,
        'trained_at': datetime.now().isoformat()
    }
    
    with open(output_path, 'wb') as f:
        pickle.dump(model_data, f)
    
    print(f"Model saved to {output_path}")


def load_model(model_path: str) -> HMMMarketDetector:
    """
    Load trained model from disk.
    
    Args:
        model_path: Path to the saved model (.pkl)
        
    Returns:
        Loaded HMMMarketDetector instance
    """
    with open(model_path, 'rb') as f:
        model_data = pickle.load(f)
    
    detector = HMMMarketDetector(n_states=model_data['n_states'])
    detector.model = model_data['model']
    detector.scaler = model_data.get('scaler')  # Restore scaler for correct feature transformation
    detector._state_to_regime = model_data['state_to_regime']
    detector._is_trained = True
    
    return detector
