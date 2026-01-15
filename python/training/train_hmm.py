"""
HMM Market Regime Model Training with Walk-Forward Validation

This script trains the Hidden Markov Model on historical data from Delta Exchange
using walk-forward (rolling window) validation to ensure robust regime detection.

Walk-Forward Process:
1. Train on window of N months
2. Validate on next M months
3. Roll window forward by M months
4. Repeat until all data is used
5. Save final model trained on all data
"""

import json
import argparse
from datetime import datetime, timedelta
from typing import List, Dict
import numpy as np
import pandas as pd
from dataclasses import dataclass

from regime_ml import HMMMarketDetector, DeltaDataFetcher, save_model


@dataclass
class WalkForwardResult:
    """Result from one walk-forward fold"""
    train_start: datetime
    train_end: datetime
    val_start: datetime
    val_end: datetime
    regime_distribution: Dict[str, float]
    log_likelihood: float
    transition_stability: float


class HMMWalkForwardTrainer:
    """
    Walk-Forward training for HMM Market Regime Model.
    
    Walk-forward validation ensures the model generalizes well to unseen data
    by simulating real trading conditions where we only have access to past data.
    """
    
    def __init__(
        self,
        train_months: int = 6,
        val_months: int = 1,
        step_months: int = 1,
        n_states: int = 5,
        min_train_samples: int = 1000
    ):
        self.train_months = train_months
        self.val_months = val_months
        self.step_months = step_months
        self.n_states = n_states
        self.min_train_samples = min_train_samples
        self.results: List[WalkForwardResult] = []
        
    def train(self, df: pd.DataFrame) -> HMMMarketDetector:
        """
        Perform walk-forward training and validation.
        
        Args:
            df: DataFrame with OHLCV data and 'timestamp' column
            
        Returns:
            Final trained HMMMarketDetector
        """
        df = df.sort_values('timestamp').reset_index(drop=True)
        
        start_date = df['timestamp'].min()
        end_date = df['timestamp'].max()
        
        print(f"Data range: {start_date} to {end_date}")
        print(f"Total samples: {len(df)}")
        print(f"Walk-forward config: train={self.train_months}mo, val={self.val_months}mo, step={self.step_months}mo")
        print("-" * 60)
        
        train_start = start_date
        fold = 0
        
        while True:
            train_end = train_start + pd.DateOffset(months=self.train_months)
            val_start = train_end
            val_end = val_start + pd.DateOffset(months=self.val_months)
            
            if val_end > end_date:
                print(f"Reached end of data at fold {fold}")
                break
                
            train_mask = (df['timestamp'] >= train_start) & (df['timestamp'] < train_end)
            val_mask = (df['timestamp'] >= val_start) & (df['timestamp'] < val_end)
            
            train_df = df[train_mask]
            val_df = df[val_mask]
            
            if len(train_df) < self.min_train_samples:
                print(f"Fold {fold}: Insufficient training samples ({len(train_df)}), skipping")
                train_start += pd.DateOffset(months=self.step_months)
                continue
                
            if len(val_df) < 100:
                print(f"Fold {fold}: Insufficient validation samples ({len(val_df)}), skipping")
                train_start += pd.DateOffset(months=self.step_months)
                continue
            
            result = self._train_fold(fold, train_df, val_df, train_start, train_end, val_start, val_end)
            self.results.append(result)
            
            print(f"Fold {fold}: Train {train_start.date()} to {train_end.date()} | "
                  f"Val {val_start.date()} to {val_end.date()}")
            print(f"  Log-likelihood: {result.log_likelihood:.4f} | "
                  f"Transition stability: {result.transition_stability:.4f}")
            print(f"  Regime distribution: {result.regime_distribution}")
            print()
            
            train_start += pd.DateOffset(months=self.step_months)
            fold += 1
        
        print("-" * 60)
        print("Training final model on all data...")
        final_detector = self._train_final_model(df)
        
        self._print_summary()
        
        return final_detector
    
    def _train_fold(
        self,
        fold: int,
        train_df: pd.DataFrame,
        val_df: pd.DataFrame,
        train_start: datetime,
        train_end: datetime,
        val_start: datetime,
        val_end: datetime
    ) -> WalkForwardResult:
        """Train and validate one fold"""
        
        detector = HMMMarketDetector(n_states=self.n_states)
        
        detector.detect_regime(
            opens=train_df['open'].values,
            highs=train_df['high'].values,
            lows=train_df['low'].values,
            closes=train_df['close'].values,
            volumes=train_df['volume'].values
        )
        
        val_result = detector.detect_regime(
            opens=val_df['open'].values,
            highs=val_df['high'].values,
            lows=val_df['low'].values,
            closes=val_df['close'].values,
            volumes=val_df['volume'].values
        )
        
        regime_dist = val_result.get('state_probabilities', {})
        log_likelihood = self._calculate_log_likelihood(detector, val_df)
        transition_stability = self._calculate_transition_stability(detector, val_df)
        
        return WalkForwardResult(
            train_start=train_start,
            train_end=train_end,
            val_start=val_start,
            val_end=val_end,
            regime_distribution=regime_dist,
            log_likelihood=log_likelihood,
            transition_stability=transition_stability
        )
    
    def _calculate_log_likelihood(self, detector: HMMMarketDetector, df: pd.DataFrame) -> float:
        """Calculate log-likelihood of validation data under the model"""
        try:
            features = detector._prepare_features(
                df['open'].values,
                df['high'].values,
                df['low'].values,
                df['close'].values,
                df['volume'].values
            )
            return float(detector.model.score(features))
        except:
            return 0.0
    
    def _calculate_transition_stability(self, detector: HMMMarketDetector, df: pd.DataFrame) -> float:
        """
        Calculate how stable the regime predictions are.
        Lower = more stable (fewer regime changes)
        """
        try:
            features = detector._prepare_features(
                df['open'].values,
                df['high'].values,
                df['low'].values,
                df['close'].values,
                df['volume'].values
            )
            states = detector.model.predict(features)
            
            transitions = np.sum(states[1:] != states[:-1])
            stability = 1.0 - (transitions / len(states))
            return float(stability)
        except:
            return 0.0
    
    def _train_final_model(self, df: pd.DataFrame) -> HMMMarketDetector:
        """Train final model on all data"""
        detector = HMMMarketDetector(n_states=self.n_states)
        
        detector.detect_regime(
            opens=df['open'].values,
            highs=df['high'].values,
            lows=df['low'].values,
            closes=df['close'].values,
            volumes=df['volume'].values
        )
        
        return detector
    
    def _print_summary(self):
        """Print walk-forward validation summary"""
        if not self.results:
            print("No results to summarize")
            return
            
        print("\n" + "=" * 60)
        print("WALK-FORWARD VALIDATION SUMMARY")
        print("=" * 60)
        
        log_likelihoods = [r.log_likelihood for r in self.results]
        stabilities = [r.transition_stability for r in self.results]
        
        print(f"Total folds: {len(self.results)}")
        print(f"Avg log-likelihood: {np.mean(log_likelihoods):.4f} (std: {np.std(log_likelihoods):.4f})")
        print(f"Avg stability: {np.mean(stabilities):.4f} (std: {np.std(stabilities):.4f})")
        
        all_regimes = {}
        for r in self.results:
            for regime, prob in r.regime_distribution.items():
                if regime not in all_regimes:
                    all_regimes[regime] = []
                all_regimes[regime].append(prob)
        
        print("\nAverage regime distribution:")
        for regime, probs in sorted(all_regimes.items()):
            print(f"  {regime}: {np.mean(probs):.3f}")


def main():
    parser = argparse.ArgumentParser(description='Train HMM Market Regime Model')
    parser.add_argument('--symbol', type=str, default='BTCUSD', help='Trading symbol')
    parser.add_argument('--resolution', type=str, default='1h', help='Candle resolution')
    parser.add_argument('--months', type=int, default=12, help='Months of historical data')
    parser.add_argument('--train-months', type=int, default=6, help='Training window months')
    parser.add_argument('--val-months', type=int, default=1, help='Validation window months')
    parser.add_argument('--step-months', type=int, default=1, help='Roll forward months')
    parser.add_argument('--n-states', type=int, default=5, help='Number of HMM states')
    parser.add_argument('--output-dir', type=str, default='../../models', help='Output directory')
    parser.add_argument('--csv', type=str, default=None, help='Load data from CSV instead of API')
    
    args = parser.parse_args()
    
    print("=" * 60)
    print("HMM MARKET REGIME MODEL TRAINING")
    print("=" * 60)
    print(f"Symbol: {args.symbol}")
    print(f"Resolution: {args.resolution}")
    print(f"Data period: {args.months} months")
    print()
    
    if args.csv:
        print(f"Loading data from {args.csv}")
        df = pd.read_csv(args.csv)
        df['timestamp'] = pd.to_datetime(df['timestamp'])
    else:
        print("Fetching data from Delta Exchange...")
        fetcher = DeltaDataFetcher()
        end_date = datetime.now()
        start_date = end_date - timedelta(days=args.months * 30)
        
        df = fetcher.fetch_candles(
            symbol=args.symbol,
            resolution=args.resolution,
            start=start_date,
            end=end_date
        )
    
    print(f"Loaded {len(df)} candles")
    print()
    
    trainer = HMMWalkForwardTrainer(
        train_months=args.train_months,
        val_months=args.val_months,
        step_months=args.step_months,
        n_states=args.n_states
    )
    
    final_model = trainer.train(df)
    
    output_path = f'{args.output_dir}/hmm_model_{args.symbol}.pkl'
    save_model(final_model, output_path, symbol=args.symbol)
    
    results_path = output_path.replace('.pkl', '_results.json')
    results_data = {
        'config': {
            'symbol': args.symbol,
            'resolution': args.resolution,
            'train_months': args.train_months,
            'val_months': args.val_months,
            'step_months': args.step_months,
            'n_states': args.n_states
        },
        'folds': [
            {
                'train_start': r.train_start.isoformat(),
                'train_end': r.train_end.isoformat(),
                'val_start': r.val_start.isoformat(),
                'val_end': r.val_end.isoformat(),
                'log_likelihood': r.log_likelihood,
                'transition_stability': r.transition_stability,
                'regime_distribution': r.regime_distribution
            }
            for r in trainer.results
        ]
    }
    
    with open(results_path, 'w') as f:
        json.dump(results_data, f, indent=2)
    
    print(f"Results saved to {results_path}")


if __name__ == "__main__":
    main()
