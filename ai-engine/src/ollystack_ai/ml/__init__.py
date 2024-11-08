"""
Machine Learning Models for OllyStack

Local ML models for high-volume, low-latency inference:
- Isolation Forest for multivariate anomaly detection
- Prophet/NeuralProphet for forecasting
- Autoencoders for trace anomaly detection
"""

from ollystack_ai.ml.isolation_forest import (
    IsolationForestDetector,
    MultiMetricAnomalyDetector,
    AnomalyResult,
)
from ollystack_ai.ml.forecaster import (
    MetricForecaster,
    ForecastResult,
)

__all__ = [
    "IsolationForestDetector",
    "MultiMetricAnomalyDetector",
    "AnomalyResult",
    "MetricForecaster",
    "ForecastResult",
]
