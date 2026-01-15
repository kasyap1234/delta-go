"""
Regime ML - Shared Machine Learning library for Delta-Go trading bot

This package provides:
- HMMMarketDetector: Hidden Markov Model for market regime detection
- DeltaDataFetcher: Data fetching from Delta Exchange API
- Model I/O utilities for loading/saving trained models
"""

from .hmm_detector import HMMMarketDetector
from .data_fetcher import DeltaDataFetcher
from .model_io import load_model, save_model

__all__ = ['HMMMarketDetector', 'DeltaDataFetcher', 'load_model', 'save_model']
__version__ = '0.1.0'
