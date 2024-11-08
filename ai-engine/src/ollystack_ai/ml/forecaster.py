"""
Metric Forecasting

Time-series forecasting for capacity planning and predictive alerting.
Uses statistical decomposition with optional Prophet-style features.
"""

import logging
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Optional, Literal
import pickle

import numpy as np
from scipy import stats, signal, optimize

logger = logging.getLogger(__name__)


@dataclass
class ForecastResult:
    """Result of a forecast."""

    timestamps: list[datetime]
    values: list[float]
    lower_bound: list[float]  # Lower confidence interval
    upper_bound: list[float]  # Upper confidence interval
    confidence_level: float
    model_info: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "timestamps": [ts.isoformat() for ts in self.timestamps],
            "values": self.values,
            "lower_bound": self.lower_bound,
            "upper_bound": self.upper_bound,
            "confidence_level": self.confidence_level,
            "model_info": self.model_info,
        }


@dataclass
class ChangePoint:
    """Detected change point in time series."""

    timestamp: datetime
    index: int
    confidence: float
    change_type: str  # 'level_shift', 'trend_change', 'variance_change'
    magnitude: float


class MetricForecaster:
    """
    Time-series forecaster for observability metrics.

    Implements a Prophet-inspired approach:
    1. Trend component (linear or logistic)
    2. Seasonal components (daily, weekly)
    3. Holiday/event effects
    4. Changepoint detection

    Lighter weight than Prophet but captures similar patterns.
    """

    def __init__(
        self,
        seasonality_mode: Literal["additive", "multiplicative"] = "additive",
        daily_seasonality: bool = True,
        weekly_seasonality: bool = True,
        changepoint_prior_scale: float = 0.05,
        n_changepoints: int = 25,
        confidence_level: float = 0.95,
    ):
        """
        Initialize the forecaster.

        Args:
            seasonality_mode: How seasonality combines with trend
            daily_seasonality: Include daily patterns
            weekly_seasonality: Include weekly patterns
            changepoint_prior_scale: Flexibility of trend changes
            n_changepoints: Number of potential changepoints
            confidence_level: Confidence level for prediction intervals
        """
        self.seasonality_mode = seasonality_mode
        self.daily_seasonality = daily_seasonality
        self.weekly_seasonality = weekly_seasonality
        self.changepoint_prior_scale = changepoint_prior_scale
        self.n_changepoints = n_changepoints
        self.confidence_level = confidence_level

        # Fitted parameters
        self._trend_params: Optional[dict] = None
        self._seasonal_params: Optional[dict] = None
        self._residual_std: float = 1.0
        self._is_fitted: bool = False
        self._training_timestamps: list[datetime] = []
        self._changepoints: list[ChangePoint] = []

    def fit(
        self,
        timestamps: list[datetime],
        values: list[float],
        holidays: Optional[list[tuple[datetime, str]]] = None,
    ) -> "MetricForecaster":
        """
        Fit the forecaster on historical data.

        Args:
            timestamps: Timestamps for each value
            values: Metric values
            holidays: Optional list of (timestamp, name) for holidays

        Returns:
            self
        """
        if len(timestamps) < 48:  # Need at least 2 days of hourly data
            logger.warning("Limited data for forecasting. Results may be inaccurate.")

        self._training_timestamps = timestamps
        values_array = np.array(values)

        # Convert timestamps to numeric (hours since start)
        t = self._timestamps_to_numeric(timestamps)

        # Detect changepoints
        self._changepoints = self._detect_changepoints(t, values_array)

        # Fit trend (piecewise linear with changepoints)
        self._trend_params = self._fit_trend(t, values_array)

        # Remove trend
        trend = self._predict_trend(t)
        detrended = values_array - trend

        # Fit seasonal components
        self._seasonal_params = self._fit_seasonality(timestamps, detrended)

        # Calculate residual standard deviation
        seasonal = self._predict_seasonality(timestamps)
        residuals = detrended - seasonal
        self._residual_std = float(np.std(residuals))

        self._is_fitted = True

        logger.info(
            f"Fitted forecaster on {len(values)} samples. "
            f"Detected {len(self._changepoints)} changepoints."
        )

        return self

    def forecast(
        self,
        periods: int,
        freq_hours: float = 1.0,
        return_components: bool = False,
    ) -> ForecastResult:
        """
        Generate forecast for future periods.

        Args:
            periods: Number of periods to forecast
            freq_hours: Hours between each period
            return_components: Include trend/seasonal breakdown

        Returns:
            ForecastResult with predictions and intervals
        """
        if not self._is_fitted:
            raise ValueError("Forecaster not fitted. Call fit() first.")

        # Generate future timestamps
        last_ts = self._training_timestamps[-1]
        future_timestamps = [
            last_ts + timedelta(hours=freq_hours * (i + 1))
            for i in range(periods)
        ]

        # Get predictions
        t = self._timestamps_to_numeric(
            self._training_timestamps + future_timestamps
        )[-periods:]

        trend = self._predict_trend(t)
        seasonal = self._predict_seasonality(future_timestamps)

        if self.seasonality_mode == "multiplicative":
            predictions = trend * (1 + seasonal)
        else:
            predictions = trend + seasonal

        # Calculate prediction intervals
        z = stats.norm.ppf((1 + self.confidence_level) / 2)
        margin = z * self._residual_std * np.sqrt(1 + np.arange(periods) * 0.01)

        lower = predictions - margin
        upper = predictions + margin

        result = ForecastResult(
            timestamps=future_timestamps,
            values=predictions.tolist(),
            lower_bound=lower.tolist(),
            upper_bound=upper.tolist(),
            confidence_level=self.confidence_level,
            model_info={
                "trend_type": "piecewise_linear",
                "n_changepoints": len(self._changepoints),
                "residual_std": self._residual_std,
            },
        )

        if return_components:
            result.model_info["trend"] = trend.tolist()
            result.model_info["seasonal"] = seasonal.tolist()

        return result

    def predict(
        self,
        timestamps: list[datetime],
    ) -> np.ndarray:
        """Predict values for given timestamps."""
        if not self._is_fitted:
            raise ValueError("Forecaster not fitted. Call fit() first.")

        t = self._timestamps_to_numeric(timestamps)
        trend = self._predict_trend(t)
        seasonal = self._predict_seasonality(timestamps)

        if self.seasonality_mode == "multiplicative":
            return trend * (1 + seasonal)
        return trend + seasonal

    def detect_anomalies(
        self,
        timestamps: list[datetime],
        values: list[float],
        threshold_sigma: float = 3.0,
    ) -> list[dict]:
        """
        Detect anomalies based on forecast residuals.

        Points that deviate significantly from forecast are flagged.
        """
        if not self._is_fitted:
            raise ValueError("Forecaster not fitted. Call fit() first.")

        predicted = self.predict(timestamps)
        residuals = np.array(values) - predicted

        anomalies = []
        for i, (ts, val, pred, resid) in enumerate(
            zip(timestamps, values, predicted, residuals)
        ):
            sigma = abs(resid) / self._residual_std
            if sigma > threshold_sigma:
                anomalies.append({
                    "timestamp": ts.isoformat(),
                    "actual": val,
                    "predicted": float(pred),
                    "residual": float(resid),
                    "sigma": float(sigma),
                    "direction": "above" if resid > 0 else "below",
                })

        return anomalies

    def get_changepoints(self) -> list[dict]:
        """Get detected changepoints."""
        return [
            {
                "timestamp": cp.timestamp.isoformat(),
                "confidence": cp.confidence,
                "change_type": cp.change_type,
                "magnitude": cp.magnitude,
            }
            for cp in self._changepoints
        ]

    def _timestamps_to_numeric(self, timestamps: list[datetime]) -> np.ndarray:
        """Convert timestamps to numeric values (hours since first timestamp)."""
        if not timestamps:
            return np.array([])

        base = self._training_timestamps[0] if self._training_timestamps else timestamps[0]
        return np.array([
            (ts - base).total_seconds() / 3600
            for ts in timestamps
        ])

    def _fit_trend(self, t: np.ndarray, values: np.ndarray) -> dict:
        """Fit piecewise linear trend with changepoints."""
        if len(self._changepoints) == 0:
            # Simple linear trend
            slope, intercept, _, _, _ = stats.linregress(t, values)
            return {
                "type": "linear",
                "intercept": float(intercept),
                "slope": float(slope),
            }

        # Piecewise linear
        changepoint_indices = [cp.index for cp in self._changepoints]

        # Fit each segment
        segments = []
        prev_idx = 0
        for cp_idx in changepoint_indices + [len(t)]:
            if cp_idx - prev_idx < 3:
                prev_idx = cp_idx
                continue

            segment_t = t[prev_idx:cp_idx]
            segment_v = values[prev_idx:cp_idx]

            if len(segment_t) >= 2:
                slope, intercept, _, _, _ = stats.linregress(segment_t, segment_v)
                segments.append({
                    "start_idx": prev_idx,
                    "end_idx": cp_idx,
                    "start_t": float(t[prev_idx]),
                    "slope": float(slope),
                    "intercept": float(intercept),
                })

            prev_idx = cp_idx

        return {
            "type": "piecewise",
            "segments": segments,
        }

    def _predict_trend(self, t: np.ndarray) -> np.ndarray:
        """Predict trend component."""
        if self._trend_params["type"] == "linear":
            return (
                self._trend_params["intercept"]
                + self._trend_params["slope"] * t
            )

        # Piecewise linear
        result = np.zeros_like(t)
        segments = self._trend_params["segments"]

        for i, t_val in enumerate(t):
            # Find applicable segment
            applicable = None
            for seg in segments:
                if t_val >= seg["start_t"]:
                    applicable = seg

            if applicable:
                result[i] = applicable["intercept"] + applicable["slope"] * t_val
            elif segments:
                # Extrapolate from last segment
                last = segments[-1]
                result[i] = last["intercept"] + last["slope"] * t_val

        return result

    def _fit_seasonality(
        self,
        timestamps: list[datetime],
        detrended: np.ndarray,
    ) -> dict:
        """Fit seasonal components using Fourier series."""
        params = {}

        if self.daily_seasonality:
            # Fit daily pattern (24-hour period)
            params["daily"] = self._fit_fourier(
                timestamps, detrended, period_hours=24, n_terms=5
            )

        if self.weekly_seasonality:
            # Fit weekly pattern (168-hour period)
            params["weekly"] = self._fit_fourier(
                timestamps, detrended, period_hours=168, n_terms=3
            )

        return params

    def _fit_fourier(
        self,
        timestamps: list[datetime],
        values: np.ndarray,
        period_hours: float,
        n_terms: int,
    ) -> dict:
        """Fit Fourier series for periodic pattern."""
        t = self._timestamps_to_numeric(timestamps)

        # Create Fourier features
        features = []
        for i in range(1, n_terms + 1):
            features.append(np.sin(2 * np.pi * i * t / period_hours))
            features.append(np.cos(2 * np.pi * i * t / period_hours))

        X = np.column_stack(features)

        # Fit with ridge regression (regularized)
        XtX = X.T @ X + 0.1 * np.eye(X.shape[1])
        Xty = X.T @ values
        coeffs = np.linalg.solve(XtX, Xty)

        return {
            "period_hours": period_hours,
            "n_terms": n_terms,
            "coefficients": coeffs.tolist(),
        }

    def _predict_seasonality(self, timestamps: list[datetime]) -> np.ndarray:
        """Predict seasonal component."""
        if not self._seasonal_params:
            return np.zeros(len(timestamps))

        t = self._timestamps_to_numeric(timestamps)
        result = np.zeros(len(t))

        for component_name, params in self._seasonal_params.items():
            period = params["period_hours"]
            n_terms = params["n_terms"]
            coeffs = np.array(params["coefficients"])

            features = []
            for i in range(1, n_terms + 1):
                features.append(np.sin(2 * np.pi * i * t / period))
                features.append(np.cos(2 * np.pi * i * t / period))

            X = np.column_stack(features)
            result += X @ coeffs

        return result

    def _detect_changepoints(
        self,
        t: np.ndarray,
        values: np.ndarray,
    ) -> list[ChangePoint]:
        """Detect changepoints using PELT-like algorithm."""
        if len(values) < 50:
            return []

        changepoints = []

        # Simple approach: sliding window variance change detection
        window_size = max(24, len(values) // 20)  # At least 24 points

        variances = []
        for i in range(len(values) - window_size):
            var = np.var(values[i : i + window_size])
            variances.append(var)

        if not variances:
            return []

        variances = np.array(variances)
        mean_var = np.mean(variances)
        std_var = np.std(variances)

        if std_var == 0:
            return []

        # Find significant variance changes
        z_scores = (variances - mean_var) / std_var

        for i, z in enumerate(z_scores):
            if abs(z) > 2.5:  # Significant change
                # Check if this is a local maximum
                is_local_max = True
                for j in range(max(0, i - 5), min(len(z_scores), i + 6)):
                    if j != i and abs(z_scores[j]) > abs(z):
                        is_local_max = False
                        break

                if is_local_max and len(changepoints) < self.n_changepoints:
                    idx = i + window_size // 2
                    changepoints.append(ChangePoint(
                        timestamp=self._training_timestamps[idx] if idx < len(self._training_timestamps) else datetime.utcnow(),
                        index=idx,
                        confidence=min(abs(z) / 3, 1.0),
                        change_type="variance_change",
                        magnitude=float(z),
                    ))

        return changepoints

    def save(self, path: str) -> None:
        """Save model to disk."""
        state = {
            "trend_params": self._trend_params,
            "seasonal_params": self._seasonal_params,
            "residual_std": self._residual_std,
            "training_timestamps": self._training_timestamps,
            "changepoints": [
                {
                    "timestamp": cp.timestamp,
                    "index": cp.index,
                    "confidence": cp.confidence,
                    "change_type": cp.change_type,
                    "magnitude": cp.magnitude,
                }
                for cp in self._changepoints
            ],
            "config": {
                "seasonality_mode": self.seasonality_mode,
                "daily_seasonality": self.daily_seasonality,
                "weekly_seasonality": self.weekly_seasonality,
                "confidence_level": self.confidence_level,
            },
        }
        with open(path, "wb") as f:
            pickle.dump(state, f)

    def load(self, path: str) -> "MetricForecaster":
        """Load model from disk."""
        with open(path, "rb") as f:
            state = pickle.load(f)

        self._trend_params = state["trend_params"]
        self._seasonal_params = state["seasonal_params"]
        self._residual_std = state["residual_std"]
        self._training_timestamps = state["training_timestamps"]
        self._changepoints = [
            ChangePoint(**cp) for cp in state["changepoints"]
        ]
        self._is_fitted = True

        config = state.get("config", {})
        self.seasonality_mode = config.get("seasonality_mode", self.seasonality_mode)
        self.daily_seasonality = config.get("daily_seasonality", self.daily_seasonality)
        self.weekly_seasonality = config.get("weekly_seasonality", self.weekly_seasonality)

        return self


class CapacityPlanner:
    """
    Uses forecasting for capacity planning.

    Predicts when resources will be exhausted and recommends scaling.
    """

    def __init__(self, forecaster: Optional[MetricForecaster] = None):
        self.forecaster = forecaster or MetricForecaster()

    def predict_exhaustion(
        self,
        current_usage: float,
        capacity: float,
        historical_values: list[float],
        historical_timestamps: list[datetime],
        threshold_percent: float = 90,
    ) -> Optional[dict]:
        """
        Predict when resource will hit threshold.

        Args:
            current_usage: Current usage value
            capacity: Maximum capacity
            historical_values: Historical usage values
            historical_timestamps: Timestamps for historical values
            threshold_percent: Warning threshold (e.g., 90%)

        Returns:
            Dict with exhaustion prediction or None if not predicted
        """
        # Fit forecaster
        self.forecaster.fit(historical_timestamps, historical_values)

        # Forecast next 7 days
        forecast = self.forecaster.forecast(periods=24 * 7, freq_hours=1)

        threshold_value = capacity * threshold_percent / 100

        # Find when upper bound crosses threshold
        for i, (ts, upper) in enumerate(
            zip(forecast.timestamps, forecast.upper_bound)
        ):
            if upper >= threshold_value:
                return {
                    "exhaustion_predicted": True,
                    "predicted_timestamp": ts.isoformat(),
                    "hours_until_exhaustion": i + 1,
                    "predicted_value": forecast.values[i],
                    "upper_bound": upper,
                    "threshold": threshold_value,
                    "capacity": capacity,
                    "recommendation": self._get_recommendation(
                        upper, capacity, i + 1
                    ),
                }

        return {
            "exhaustion_predicted": False,
            "forecast_horizon_hours": 24 * 7,
            "max_predicted_value": max(forecast.upper_bound),
            "threshold": threshold_value,
        }

    def _get_recommendation(
        self,
        predicted_value: float,
        capacity: float,
        hours_until: int,
    ) -> str:
        """Generate capacity recommendation."""
        overage = predicted_value / capacity

        if hours_until < 24:
            urgency = "URGENT"
        elif hours_until < 72:
            urgency = "Soon"
        else:
            urgency = "Plan ahead"

        if overage > 1.5:
            scale = "significantly (50%+)"
        elif overage > 1.2:
            scale = "moderately (20-50%)"
        else:
            scale = "slightly (10-20%)"

        return f"{urgency}: Scale capacity {scale} within {hours_until} hours"
