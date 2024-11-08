"""
Isolation Forest Anomaly Detection

Multivariate anomaly detection using Isolation Forest algorithm.
Detects anomalies that span multiple metrics simultaneously,
which statistical methods often miss.

Key advantages over statistical methods:
- Catches correlations between metrics
- No assumptions about data distribution
- Works well with high-dimensional data
- Efficient O(n log n) complexity
"""

import logging
import pickle
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Optional
from pathlib import Path
import hashlib

import numpy as np
from sklearn.ensemble import IsolationForest
from sklearn.preprocessing import StandardScaler
from sklearn.exceptions import NotFittedError

logger = logging.getLogger(__name__)


@dataclass
class AnomalyResult:
    """Result of anomaly detection."""

    is_anomaly: bool
    anomaly_score: float  # -1 to 1, higher = more anomalous
    normalized_score: float  # 0 to 1
    timestamp: datetime
    feature_contributions: dict[str, float]  # Which features contributed most
    context: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "is_anomaly": self.is_anomaly,
            "anomaly_score": self.anomaly_score,
            "normalized_score": self.normalized_score,
            "timestamp": self.timestamp.isoformat(),
            "feature_contributions": self.feature_contributions,
            "context": self.context,
        }


@dataclass
class ModelMetadata:
    """Metadata about a trained model."""

    model_id: str
    service_name: str
    feature_names: list[str]
    trained_at: datetime
    training_samples: int
    contamination: float
    performance_metrics: dict = field(default_factory=dict)


class IsolationForestDetector:
    """
    Isolation Forest based anomaly detector for single service.

    Uses the principle that anomalies are few and different,
    making them easier to isolate in a tree structure.
    """

    def __init__(
        self,
        contamination: float = 0.01,  # Expected proportion of anomalies
        n_estimators: int = 100,
        max_samples: str = "auto",
        random_state: int = 42,
        threshold_percentile: float = 95,
    ):
        """
        Initialize the detector.

        Args:
            contamination: Expected proportion of anomalies (0.01 = 1%)
            n_estimators: Number of trees in the forest
            max_samples: Samples to draw for each tree
            random_state: Random seed for reproducibility
            threshold_percentile: Percentile for anomaly threshold
        """
        self.contamination = contamination
        self.n_estimators = n_estimators
        self.max_samples = max_samples
        self.random_state = random_state
        self.threshold_percentile = threshold_percentile

        self.model: Optional[IsolationForest] = None
        self.scaler: Optional[StandardScaler] = None
        self.feature_names: list[str] = []
        self.is_fitted = False
        self.training_stats: dict = {}

    def fit(
        self,
        data: np.ndarray,
        feature_names: Optional[list[str]] = None,
    ) -> "IsolationForestDetector":
        """
        Fit the model on training data.

        Args:
            data: Training data of shape (n_samples, n_features)
            feature_names: Names of features for interpretability

        Returns:
            self
        """
        if len(data) < 100:
            logger.warning(f"Training with only {len(data)} samples. Recommend 1000+.")

        # Store feature names
        if feature_names:
            self.feature_names = feature_names
        else:
            self.feature_names = [f"feature_{i}" for i in range(data.shape[1])]

        # Scale features
        self.scaler = StandardScaler()
        scaled_data = self.scaler.fit_transform(data)

        # Train Isolation Forest
        self.model = IsolationForest(
            contamination=self.contamination,
            n_estimators=self.n_estimators,
            max_samples=self.max_samples,
            random_state=self.random_state,
            n_jobs=-1,  # Use all cores
        )
        self.model.fit(scaled_data)

        # Calculate training statistics
        scores = self.model.decision_function(scaled_data)
        self.training_stats = {
            "mean_score": float(np.mean(scores)),
            "std_score": float(np.std(scores)),
            "min_score": float(np.min(scores)),
            "max_score": float(np.max(scores)),
            "threshold": float(np.percentile(scores, 100 - self.threshold_percentile)),
            "n_samples": len(data),
            "n_features": data.shape[1],
        }

        self.is_fitted = True
        logger.info(
            f"Trained Isolation Forest on {len(data)} samples, "
            f"{data.shape[1]} features"
        )

        return self

    def predict(
        self,
        data: np.ndarray,
        timestamps: Optional[list[datetime]] = None,
    ) -> list[AnomalyResult]:
        """
        Predict anomalies in new data.

        Args:
            data: Data of shape (n_samples, n_features)
            timestamps: Optional timestamps for each sample

        Returns:
            List of AnomalyResult for each sample
        """
        if not self.is_fitted:
            raise NotFittedError("Model not fitted. Call fit() first.")

        if timestamps is None:
            timestamps = [datetime.utcnow()] * len(data)

        # Scale data
        scaled_data = self.scaler.transform(data)

        # Get predictions (-1 = anomaly, 1 = normal)
        predictions = self.model.predict(scaled_data)

        # Get anomaly scores (lower = more anomalous)
        scores = self.model.decision_function(scaled_data)

        results = []
        for i, (pred, score, ts) in enumerate(zip(predictions, scores, timestamps)):
            # Normalize score to 0-1 (higher = more anomalous)
            normalized = self._normalize_score(score)

            # Calculate feature contributions
            contributions = self._calculate_contributions(scaled_data[i])

            results.append(AnomalyResult(
                is_anomaly=pred == -1,
                anomaly_score=float(score),
                normalized_score=normalized,
                timestamp=ts,
                feature_contributions=contributions,
                context={
                    "threshold": self.training_stats.get("threshold", 0),
                    "training_mean": self.training_stats.get("mean_score", 0),
                },
            ))

        return results

    def predict_single(
        self,
        features: dict[str, float],
        timestamp: Optional[datetime] = None,
    ) -> AnomalyResult:
        """
        Predict anomaly for a single sample given as feature dict.

        Args:
            features: Dict mapping feature names to values
            timestamp: Sample timestamp

        Returns:
            AnomalyResult
        """
        # Build feature vector in correct order
        vector = np.array([[features.get(name, 0) for name in self.feature_names]])
        results = self.predict(vector, [timestamp] if timestamp else None)
        return results[0]

    def score_samples(self, data: np.ndarray) -> np.ndarray:
        """Get raw anomaly scores for samples."""
        if not self.is_fitted:
            raise NotFittedError("Model not fitted. Call fit() first.")

        scaled_data = self.scaler.transform(data)
        return self.model.decision_function(scaled_data)

    def _normalize_score(self, score: float) -> float:
        """Normalize raw score to 0-1 range."""
        # Scores typically range from -0.5 (anomaly) to 0.5 (normal)
        # Normalize so that threshold maps to ~0.7
        mean = self.training_stats.get("mean_score", 0)
        std = self.training_stats.get("std_score", 1)

        if std == 0:
            return 0.5

        # Z-score normalization, then sigmoid-like mapping
        z = (mean - score) / std  # Flip so higher = more anomalous
        normalized = 1 / (1 + np.exp(-z))

        return float(np.clip(normalized, 0, 1))

    def _calculate_contributions(self, sample: np.ndarray) -> dict[str, float]:
        """
        Calculate which features contributed most to the anomaly score.

        Uses a simple approach: compare each feature to its mean,
        weighted by how far it deviates.
        """
        contributions = {}

        for i, name in enumerate(self.feature_names):
            # Deviation from mean (in scaled space)
            deviation = abs(sample[i])
            contributions[name] = float(deviation)

        # Normalize contributions to sum to 1
        total = sum(contributions.values())
        if total > 0:
            contributions = {k: v / total for k, v in contributions.items()}

        # Sort by contribution
        contributions = dict(
            sorted(contributions.items(), key=lambda x: x[1], reverse=True)
        )

        return contributions

    def save(self, path: str) -> None:
        """Save model to disk."""
        state = {
            "model": self.model,
            "scaler": self.scaler,
            "feature_names": self.feature_names,
            "training_stats": self.training_stats,
            "config": {
                "contamination": self.contamination,
                "n_estimators": self.n_estimators,
                "threshold_percentile": self.threshold_percentile,
            },
        }
        with open(path, "wb") as f:
            pickle.dump(state, f)
        logger.info(f"Saved model to {path}")

    def load(self, path: str) -> "IsolationForestDetector":
        """Load model from disk."""
        with open(path, "rb") as f:
            state = pickle.load(f)

        self.model = state["model"]
        self.scaler = state["scaler"]
        self.feature_names = state["feature_names"]
        self.training_stats = state["training_stats"]
        self.is_fitted = True

        config = state.get("config", {})
        self.contamination = config.get("contamination", self.contamination)
        self.n_estimators = config.get("n_estimators", self.n_estimators)

        logger.info(f"Loaded model from {path}")
        return self


class MultiMetricAnomalyDetector:
    """
    Manages multiple Isolation Forest models for different services/contexts.

    Provides:
    - Automatic model training and retraining
    - Model versioning and persistence
    - Online learning with periodic updates
    - Feature engineering helpers
    """

    # Common feature combinations for observability
    METRIC_GROUPS = {
        "system": ["cpu_usage", "memory_usage", "disk_io", "network_io"],
        "application": ["request_rate", "error_rate", "latency_p50", "latency_p99"],
        "database": ["query_rate", "query_latency", "connection_count", "lock_wait"],
        "jvm": ["heap_used", "gc_time", "thread_count", "class_count"],
    }

    def __init__(
        self,
        model_dir: str = "/tmp/ollystack_models",
        auto_retrain_hours: int = 24,
        min_training_samples: int = 1000,
    ):
        """
        Initialize the multi-metric detector.

        Args:
            model_dir: Directory to store trained models
            auto_retrain_hours: Hours between automatic retraining
            min_training_samples: Minimum samples required for training
        """
        self.model_dir = Path(model_dir)
        self.model_dir.mkdir(parents=True, exist_ok=True)
        self.auto_retrain_hours = auto_retrain_hours
        self.min_training_samples = min_training_samples

        # Service -> model mapping
        self._models: dict[str, IsolationForestDetector] = {}
        self._metadata: dict[str, ModelMetadata] = {}

        # Training data buffers
        self._training_buffers: dict[str, list[tuple[datetime, np.ndarray]]] = {}

    def get_or_create_model(
        self,
        service_name: str,
        feature_names: list[str],
    ) -> IsolationForestDetector:
        """Get existing model or create new one."""
        model_key = self._model_key(service_name, feature_names)

        if model_key not in self._models:
            # Try to load from disk
            model_path = self.model_dir / f"{model_key}.pkl"
            if model_path.exists():
                self._models[model_key] = IsolationForestDetector().load(str(model_path))
            else:
                self._models[model_key] = IsolationForestDetector()

        return self._models[model_key]

    def add_training_sample(
        self,
        service_name: str,
        features: dict[str, float],
        timestamp: Optional[datetime] = None,
    ) -> None:
        """
        Add a sample to the training buffer.

        Samples are accumulated and used for periodic retraining.
        """
        if timestamp is None:
            timestamp = datetime.utcnow()

        feature_names = sorted(features.keys())
        model_key = self._model_key(service_name, feature_names)

        if model_key not in self._training_buffers:
            self._training_buffers[model_key] = []

        vector = np.array([features[name] for name in feature_names])
        self._training_buffers[model_key].append((timestamp, vector))

        # Check if we should retrain
        self._maybe_retrain(service_name, feature_names)

    def detect(
        self,
        service_name: str,
        features: dict[str, float],
        timestamp: Optional[datetime] = None,
    ) -> Optional[AnomalyResult]:
        """
        Detect if the given metrics are anomalous.

        Args:
            service_name: Service identifier
            features: Dict of metric name -> value
            timestamp: Sample timestamp

        Returns:
            AnomalyResult if model is trained, None otherwise
        """
        feature_names = sorted(features.keys())
        model = self.get_or_create_model(service_name, feature_names)

        if not model.is_fitted:
            # Add to training buffer and return None
            self.add_training_sample(service_name, features, timestamp)
            return None

        # Ensure feature order matches model
        if set(feature_names) != set(model.feature_names):
            logger.warning(
                f"Feature mismatch for {service_name}. "
                f"Expected {model.feature_names}, got {feature_names}"
            )
            return None

        return model.predict_single(features, timestamp)

    def detect_batch(
        self,
        service_name: str,
        data: list[dict[str, float]],
        timestamps: Optional[list[datetime]] = None,
    ) -> list[Optional[AnomalyResult]]:
        """Detect anomalies in a batch of samples."""
        if not data:
            return []

        feature_names = sorted(data[0].keys())
        model = self.get_or_create_model(service_name, feature_names)

        if not model.is_fitted:
            for i, sample in enumerate(data):
                ts = timestamps[i] if timestamps else None
                self.add_training_sample(service_name, sample, ts)
            return [None] * len(data)

        # Build data matrix
        matrix = np.array([
            [sample.get(name, 0) for name in model.feature_names]
            for sample in data
        ])

        return model.predict(matrix, timestamps)

    def train_model(
        self,
        service_name: str,
        feature_names: list[str],
        data: Optional[np.ndarray] = None,
    ) -> bool:
        """
        Train or retrain a model.

        Args:
            service_name: Service identifier
            feature_names: Feature names
            data: Training data (uses buffer if None)

        Returns:
            True if training succeeded
        """
        model_key = self._model_key(service_name, feature_names)

        if data is None:
            # Use training buffer
            buffer = self._training_buffers.get(model_key, [])
            if len(buffer) < self.min_training_samples:
                logger.info(
                    f"Not enough samples for {service_name}: "
                    f"{len(buffer)} < {self.min_training_samples}"
                )
                return False

            data = np.array([sample for _, sample in buffer])

        # Create and train model
        model = IsolationForestDetector()
        model.fit(data, feature_names)

        # Save model
        model_path = self.model_dir / f"{model_key}.pkl"
        model.save(str(model_path))

        # Update registry
        self._models[model_key] = model
        self._metadata[model_key] = ModelMetadata(
            model_id=model_key,
            service_name=service_name,
            feature_names=feature_names,
            trained_at=datetime.utcnow(),
            training_samples=len(data),
            contamination=model.contamination,
            performance_metrics=model.training_stats,
        )

        # Clear training buffer
        self._training_buffers[model_key] = []

        logger.info(f"Trained model for {service_name} with {len(data)} samples")
        return True

    def get_model_info(self, service_name: str, feature_names: list[str]) -> Optional[dict]:
        """Get information about a model."""
        model_key = self._model_key(service_name, feature_names)
        metadata = self._metadata.get(model_key)

        if metadata is None:
            return None

        return {
            "model_id": metadata.model_id,
            "service_name": metadata.service_name,
            "feature_names": metadata.feature_names,
            "trained_at": metadata.trained_at.isoformat(),
            "training_samples": metadata.training_samples,
            "performance_metrics": metadata.performance_metrics,
        }

    def list_models(self) -> list[dict]:
        """List all trained models."""
        models = []
        for model_key, metadata in self._metadata.items():
            models.append({
                "model_id": metadata.model_id,
                "service_name": metadata.service_name,
                "feature_names": metadata.feature_names,
                "trained_at": metadata.trained_at.isoformat(),
                "training_samples": metadata.training_samples,
            })
        return models

    def _model_key(self, service_name: str, feature_names: list[str]) -> str:
        """Generate a unique key for a model."""
        features_str = ",".join(sorted(feature_names))
        key_str = f"{service_name}:{features_str}"
        return hashlib.md5(key_str.encode()).hexdigest()[:16]

    def _maybe_retrain(self, service_name: str, feature_names: list[str]) -> None:
        """Check if model should be retrained."""
        model_key = self._model_key(service_name, feature_names)
        metadata = self._metadata.get(model_key)

        buffer = self._training_buffers.get(model_key, [])
        buffer_size = len(buffer)

        # Check conditions for retraining
        should_retrain = False

        if metadata is None and buffer_size >= self.min_training_samples:
            # No model exists and we have enough data
            should_retrain = True
        elif metadata is not None:
            hours_since_training = (
                datetime.utcnow() - metadata.trained_at
            ).total_seconds() / 3600

            if (
                hours_since_training >= self.auto_retrain_hours
                and buffer_size >= self.min_training_samples
            ):
                should_retrain = True

        if should_retrain:
            self.train_model(service_name, feature_names)


class CorrelatedAnomalyDetector:
    """
    Detects anomalies that span multiple services.

    Useful for catching cascading failures or correlated issues
    that wouldn't be caught by per-service detection.
    """

    def __init__(
        self,
        correlation_window_seconds: int = 60,
        min_correlation: float = 0.7,
    ):
        self.correlation_window = correlation_window_seconds
        self.min_correlation = min_correlation
        self._recent_anomalies: list[tuple[datetime, str, AnomalyResult]] = []

    def record_anomaly(
        self,
        service_name: str,
        anomaly: AnomalyResult,
    ) -> list[dict]:
        """
        Record an anomaly and check for correlations.

        Returns list of correlated anomaly groups.
        """
        now = anomaly.timestamp
        self._recent_anomalies.append((now, service_name, anomaly))

        # Clean old anomalies
        cutoff = now - timedelta(seconds=self.correlation_window)
        self._recent_anomalies = [
            (ts, svc, a) for ts, svc, a in self._recent_anomalies
            if ts >= cutoff
        ]

        # Find correlations
        return self._find_correlations()

    def _find_correlations(self) -> list[dict]:
        """Find correlated anomalies across services."""
        if len(self._recent_anomalies) < 2:
            return []

        # Group by time windows
        groups = []
        used = set()

        for i, (ts1, svc1, a1) in enumerate(self._recent_anomalies):
            if i in used:
                continue

            group = [(ts1, svc1, a1)]
            used.add(i)

            for j, (ts2, svc2, a2) in enumerate(self._recent_anomalies):
                if j in used or svc1 == svc2:
                    continue

                # Check if within correlation window
                time_diff = abs((ts2 - ts1).total_seconds())
                if time_diff <= self.correlation_window:
                    group.append((ts2, svc2, a2))
                    used.add(j)

            if len(group) > 1:
                groups.append({
                    "services": [svc for _, svc, _ in group],
                    "timestamps": [ts.isoformat() for ts, _, _ in group],
                    "anomaly_scores": [a.normalized_score for _, _, a in group],
                    "correlation_window_seconds": self.correlation_window,
                })

        return groups
