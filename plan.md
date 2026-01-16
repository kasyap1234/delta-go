Fix HMM Market Regime Model
The HMM model produces extremely poor results: high log-likelihood variance (-58527 to -2789), near-zero regime probabilities, and inconsistent distributions across folds.

Root Causes
No feature standardization - Features have different scales (returns ~0.001, RSI ~50)
Single random initialization - EM can get stuck in local optima
Low EM iterations (100) - May not converge
No covariance regularization - Can lead to singular matrices
No explicit K-Means init - Random initialization is unreliable
Proposed Changes
regime_ml Package
[MODIFY] 
hmm_detector.py
Add feature standardization:

from sklearn.preprocessing import StandardScaler
def __init__(self, ...):
    self.scaler: Optional[StandardScaler] = None
def _prepare_features(self, ...) -> np.ndarray:
    # ... existing feature computation ...
    if self.scaler is None:
        self.scaler = StandardScaler()
        features = self.scaler.fit_transform(features)
    else:
        features = self.scaler.transform(features)
    return features
Implement multiple random restarts:

def _train_model(self, features: np.ndarray, n_restarts: int = 10):
    best_model = None
    best_score = -np.inf
    
    for seed in range(n_restarts):
        model = GaussianHMM(
            n_components=self.n_states,
            covariance_type="full",
            n_iter=500,  # Increased from 100
            tol=1e-4,
            random_state=seed,
            init_params="stmc",
            verbose=False,
            covars_prior=1e-2,  # Regularization
        )
        model.fit(features)
        score = model.score(features)
        if score > best_score:
            best_score = score
            best_model = model
    
    self.model = best_model
Add K-Means initialization for means:

from sklearn.cluster import KMeans
def _init_means_with_kmeans(self, features: np.ndarray) -> np.ndarray:
    kmeans = KMeans(n_clusters=self.n_states, random_state=42, n_init=10)
    kmeans.fit(features)
    return kmeans.cluster_centers_
[MODIFY] 
model_io.py
Update save/load to persist the scaler:

def save_model(detector, path, ...):
    data = {
        'model': detector.model,
        'scaler': detector.scaler,  # Add scaler
        'state_to_regime': detector._state_to_regime,
        ...
    }
Verification Plan
Automated Test
Run training on BTCUSD with same parameters as before:

cd /home/tgt/Documents/projects/personal/delta-go/python/training
uv run python train_hmm.py --symbol BTCUSD --resolution 1h --months 36 --train-months 6 --val-months 1 --step-months 1
Success Criteria:

Avg log-likelihood variance (std) reduced significantly from 12464
No regime probability < 0.01 in any fold (was 0.00012)
Log-likelihood range should be more consistent (not 50x difference between folds)
All 5 regimes represented in each fold with meaningful probabilities