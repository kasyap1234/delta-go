"""
Hidden Markov Model for Market Regime Detection

Uses GaussianHMM from hmmlearn to classify market states:
1. Bull Market (strong uptrend)
2. Bear Market (strong downtrend)
3. Ranging Market (sideways/consolidation)
4. High Volatility (explosive moves)
5. Low Volatility (quiet market)
"""
import numpy as np
from hmmlearn.hmm import GaussianHMM
from sklearn.preprocessing import StandardScaler
from sklearn.cluster import KMeans
from typing import Dict, Optional
import warnings

warnings.filterwarnings('ignore')


class HMMMarketDetector:
    """
    Hidden Markov Model based market regime detector.
    
    Features used:
    - Log returns
    - Rolling volatility (20-period)
    - Rolling Sharpe ratio (trend indicator)
    - RSI (14-period)
    - Volume change ratio
    
    Improvements:
    - Feature standardization via StandardScaler
    - Multiple random restarts to escape local optima
    - Covariance regularization to prevent singular matrices
    - K-Means initialization for better starting points
    """
    
    REGIME_NAMES = {
        0: "bull",
        1: "bear", 
        2: "ranging",
        3: "high_volatility",
        4: "low_volatility"
    }
    
    def __init__(self, n_states: int = 5, lookback: int = 100, n_restarts: int = 20):
        """
        Initialize the HMM detector.
        
        Args:
            n_states: Number of hidden states (market regimes)
            lookback: Number of periods for rolling calculations
            n_restarts: Number of random restarts for training (more = better but slower)
        """
        self.n_states = n_states
        self.lookback = lookback
        self.n_restarts = n_restarts
        self.model: Optional[GaussianHMM] = None
        self.scaler: Optional[StandardScaler] = None
        self._is_trained = False
        self._state_to_regime: Dict[int, str] = {}
        
    def _calculate_returns(self, closes: np.ndarray) -> np.ndarray:
        """Calculate log returns."""
        returns = np.diff(np.log(closes))
        return np.insert(returns, 0, 0)
    
    def _calculate_volatility(self, returns: np.ndarray, window: int = 20) -> np.ndarray:
        """Calculate rolling volatility (standard deviation of returns)."""
        vol = np.zeros(len(returns))
        for i in range(window, len(returns)):
            vol[i] = np.std(returns[i-window:i])
        vol[:window] = vol[window] if len(vol) > window else 0
        return vol
    
    def _calculate_rsi(self, closes: np.ndarray, period: int = 14) -> np.ndarray:
        """Calculate Relative Strength Index."""
        deltas = np.diff(closes)
        gains = np.where(deltas > 0, deltas, 0)
        losses = np.where(deltas < 0, -deltas, 0)
        
        rsi = np.zeros(len(closes))
        
        if len(closes) < period + 1:
            return rsi
        
        avg_gain = np.mean(gains[:period])
        avg_loss = np.mean(losses[:period])
        
        for i in range(period, len(closes)):
            if i > period:
                avg_gain = (avg_gain * (period - 1) + gains[i-1]) / period
                avg_loss = (avg_loss * (period - 1) + losses[i-1]) / period
            
            if avg_loss == 0:
                rsi[i] = 100
            else:
                rs = avg_gain / avg_loss
                rsi[i] = 100 - (100 / (1 + rs))
        
        rsi[:period] = 50
        return rsi
    
    def _calculate_trend(self, returns: np.ndarray, window: int = 20) -> np.ndarray:
        """Calculate trend indicator (rolling mean of returns / volatility)."""
        trend = np.zeros(len(returns))
        for i in range(window, len(returns)):
            mean_ret = np.mean(returns[i-window:i])
            std_ret = np.std(returns[i-window:i])
            if std_ret > 0:
                trend[i] = mean_ret / std_ret * np.sqrt(252)
            else:
                trend[i] = 0
        trend[:window] = 0
        return trend
    
    def _calculate_volume_ratio(self, volumes: np.ndarray, window: int = 20) -> np.ndarray:
        """Calculate volume change ratio."""
        vol_ratio = np.zeros(len(volumes))
        for i in range(window, len(volumes)):
            avg_vol = np.mean(volumes[i-window:i])
            if avg_vol > 0:
                vol_ratio[i] = volumes[i] / avg_vol - 1
            else:
                vol_ratio[i] = 0
        vol_ratio[:window] = 0
        return vol_ratio
    
    def _rolling_mean(self, data: np.ndarray, window: int) -> np.ndarray:
        """Calculate rolling mean."""
        result = np.zeros(len(data))
        for i in range(window, len(data)):
            result[i] = np.mean(data[i-window:i])
        result[:window] = result[window] if len(result) > window else 0
        return result
    
    def _prepare_features(
        self,
        opens: np.ndarray,
        highs: np.ndarray,
        lows: np.ndarray,
        closes: np.ndarray,
        volumes: np.ndarray,
        fit_scaler: bool = False
    ) -> np.ndarray:
        """
        Prepare feature matrix for HMM.
        
        Args:
            fit_scaler: If True, fit the scaler on this data (training).
                       If False, use existing scaler (inference).
        
        Returns:
            Feature matrix of shape (n_samples, n_features), standardized
        """
        returns = self._calculate_returns(closes)
        volatility = self._calculate_volatility(returns)
        rsi = self._calculate_rsi(closes) / 100
        trend = self._calculate_trend(returns)
        vol_ratio = self._calculate_volume_ratio(volumes)
        
        tr = np.maximum(
            highs - lows,
            np.maximum(
                np.abs(highs - np.roll(closes, 1)),
                np.abs(lows - np.roll(closes, 1))
            )
        )
        tr[0] = highs[0] - lows[0]
        atr = self._rolling_mean(tr, 14) / closes
        
        features = np.column_stack([
            returns,
            volatility,
            rsi,
            trend,
            vol_ratio,
            atr
        ])
        
        features = np.nan_to_num(features, nan=0, posinf=0, neginf=0)
        
        # Standardize features for better HMM convergence
        if fit_scaler:
            self.scaler = StandardScaler()
            features = self.scaler.fit_transform(features)
        elif self.scaler is not None:
            features = self.scaler.transform(features)
        
        # Clip outliers to ±3 standard deviations to prevent extreme values
        # from distorting the HMM learning process
        features = np.clip(features, -3.0, 3.0)
        
        return features
    
    def _init_means_with_kmeans(self, features: np.ndarray, random_state: int = 42) -> np.ndarray:
        """Initialize means using K-Means clustering for better starting points.
        
        Args:
            features: Feature matrix for clustering
            random_state: Random seed for K-Means (use different seeds per restart)
        """
        kmeans = KMeans(
            n_clusters=self.n_states,
            random_state=random_state,
            n_init=10,
            max_iter=300
        )
        kmeans.fit(features)
        return kmeans.cluster_centers_
    
    def _train_model(self, features: np.ndarray):
        """
        Train the HMM on feature data with multiple random restarts.
        
        Uses multiple restarts to escape local optima and selects
        the model with the best log-likelihood score.
        """
        best_model = None
        best_score = -np.inf
        
        # Note: K-Means means are now computed inside each restart loop
        # with different random seeds to provide diverse starting points
        
        for restart in range(self.n_restarts):
            try:
                # Get K-Means initialized means with different seed per restart
                # This provides diversity across restarts to escape local optima
                kmeans_means = self._init_means_with_kmeans(features, random_state=restart * 7 + 42)
                
                model = GaussianHMM(
                    n_components=self.n_states,
                    covariance_type="tied",  # Tied is more stable than full
                    n_iter=1000,  # Increased for better convergence
                    tol=1e-6,  # Tighter tolerance
                    random_state=restart,
                    init_params="stc",  # Don't init means, we set them
                    params="stmc",  # But update all during training
                    verbose=False,
                    min_covar=1e-1,  # Strong regularization to prevent regime collapse
                )
                
                # Set K-Means initialized means
                model.means_ = kmeans_means.copy()
                
                # Add small regularization to covariances
                # This prevents singular covariance matrices
                model.fit(features)
                
                score = model.score(features)
                if score > best_score:
                    best_score = score
                    best_model = model
            except Exception:
                # Skip failed fits (e.g., singular covariance)
                continue
        
        if best_model is None:
            # Fallback to simple model if all restarts failed
            best_model = GaussianHMM(
                n_components=self.n_states,
                covariance_type="diag",  # Simpler, more stable
                n_iter=1000,
                random_state=42,
                min_covar=1e-1,  # Strong regularization for fallback too
            )
            best_model.fit(features)
        
        self.model = best_model
        self._is_trained = True
        self._map_states_to_regimes(features)
    
    def _map_states_to_regimes(self, features: np.ndarray):
        """Map hidden states to meaningful regime names based on state characteristics."""
        if not self._is_trained or self.model is None:
            return
        
        means = self.model.means_
        return_idx, vol_idx, rsi_idx, trend_idx = 0, 1, 2, 3
        
        state_chars = []
        for i in range(self.n_states):
            char = {
                'state': i,
                'return': means[i, return_idx],
                'volatility': means[i, vol_idx],
                'trend': means[i, trend_idx],
                'rsi': means[i, rsi_idx]
            }
            state_chars.append(char)
        
        sorted_by_return = sorted(state_chars, key=lambda x: x['return'])
        sorted_by_vol = sorted(state_chars, key=lambda x: x['volatility'])
        
        self._state_to_regime = {}
        used_regimes = set()
        
        high_vol_state = sorted_by_vol[-1]['state']
        self._state_to_regime[high_vol_state] = "high_volatility"
        used_regimes.add(high_vol_state)
        
        low_vol_state = sorted_by_vol[0]['state']
        if low_vol_state not in used_regimes:
            self._state_to_regime[low_vol_state] = "low_volatility"
            used_regimes.add(low_vol_state)
        
        for s in reversed(sorted_by_return):
            if s['state'] not in used_regimes:
                self._state_to_regime[s['state']] = "bull"
                used_regimes.add(s['state'])
                break
        
        for s in sorted_by_return:
            if s['state'] not in used_regimes:
                self._state_to_regime[s['state']] = "bear"
                used_regimes.add(s['state'])
                break
        
        for i in range(self.n_states):
            if i not in used_regimes:
                self._state_to_regime[i] = "ranging"
    
    def detect_regime(
        self,
        opens: np.ndarray,
        highs: np.ndarray,
        lows: np.ndarray,
        closes: np.ndarray,
        volumes: np.ndarray
    ) -> Dict:
        """
        Detect the current market regime.
        
        Args:
            opens: Array of open prices
            highs: Array of high prices
            lows: Array of low prices
            closes: Array of close prices
            volumes: Array of volumes
        
        Returns:
            Dict with regime, confidence, and features
        """
        if len(closes) < 50:
            return {
                "regime": "insufficient_data",
                "confidence": 0.0,
                "features": {}
            }
        
        # When training (not yet trained), fit the scaler
        fit_scaler = not self._is_trained
        features = self._prepare_features(opens, highs, lows, closes, volumes, fit_scaler=fit_scaler)
        
        if not self._is_trained:
            self._train_model(features)
        
        try:
            probs = self.model.predict_proba(features)
            last_probs = probs[-1]
            predicted_state = np.argmax(last_probs)
            confidence = last_probs[predicted_state]
            
            regime = self._state_to_regime.get(predicted_state, "ranging")
            
            current_features = {
                "volatility": float(features[-1, 1]),
                "trend": float(features[-1, 3]),
                "rsi": float(features[-1, 2] * 100),
                "returns": float(features[-1, 0])
            }
            
            return {
                "regime": regime,
                "confidence": float(confidence),
                "features": current_features,
                "state_probabilities": {
                    self._state_to_regime.get(i, f"state_{i}"): float(p) 
                    for i, p in enumerate(last_probs)
                }
            }
            
        except Exception as e:
            return {
                "regime": "error",
                "confidence": 0.0,
                "features": {},
                "error": str(e)
            }
    
    def retrain(
        self,
        opens: np.ndarray,
        highs: np.ndarray,
        lows: np.ndarray,
        closes: np.ndarray,
        volumes: np.ndarray
    ):
        """Force retrain the model on new data."""
        # Reset scaler and train fresh
        self.scaler = None
        self._is_trained = False
        features = self._prepare_features(opens, highs, lows, closes, volumes, fit_scaler=True)
        self._train_model(features)
