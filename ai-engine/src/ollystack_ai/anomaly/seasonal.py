"""
Seasonal Anomaly Detection

Time-series aware anomaly detection with:
- STL decomposition (Seasonal-Trend using LOESS)
- Fourier analysis for periodicity detection
- Multi-period seasonality (hourly, daily, weekly)
- Holiday and special event handling
- Dynamic baselines by time period
"""

import logging
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Optional, Literal
from enum import Enum

import numpy as np
from scipy import stats, fft, signal
from scipy.interpolate import interp1d

logger = logging.getLogger(__name__)


class SeasonalPeriod(Enum):
    """Seasonal periods for pattern detection."""

    HOURLY = "hourly"  # Hour of day (0-23)
    DAILY = "daily"  # Day of week (0-6)
    WEEKLY = "weekly"  # Week patterns
    MONTHLY = "monthly"  # Day of month (1-31)


@dataclass
class SeasonalPattern:
    """Detected seasonal pattern."""

    period: SeasonalPeriod
    strength: float  # 0-1, how strong the pattern is
    pattern_values: list[float]  # Expected values for each period unit
    pattern_std: list[float]  # Standard deviation for each period unit
    confidence: float


@dataclass
class DecompositionResult:
    """Result of time series decomposition."""

    trend: np.ndarray
    seasonal: np.ndarray
    residual: np.ndarray
    period: int
    seasonal_strength: float


@dataclass
class SeasonalBaseline:
    """Seasonal baseline for a metric."""

    metric_name: str
    service_name: str
    hourly_means: list[float]  # 24 values, one per hour
    hourly_stds: list[float]
    daily_means: list[float]  # 7 values, one per day of week
    daily_stds: list[float]
    weekly_pattern: Optional[list[float]]  # 168 values (24*7)
    weekly_stds: Optional[list[float]]
    global_mean: float
    global_std: float
    sample_count: int
    last_updated: datetime


@dataclass
class SeasonalAnomalyResult:
    """Result of seasonal anomaly detection."""

    is_anomaly: bool
    score: float  # 0-1 anomaly score
    raw_value: float
    expected_value: float
    expected_std: float
    deviation_sigma: float  # Number of standard deviations
    seasonal_context: dict  # Hour, day, etc.
    contributing_factors: list[str]


class SeasonalDecomposer:
    """
    Decomposes time series into trend, seasonal, and residual components.

    Implements STL-like decomposition using moving averages and LOESS smoothing.
    """

    def __init__(
        self,
        period: int = 24,  # Default: hourly data, daily seasonality
        robust: bool = True,
        seasonal_window: int = 7,
        trend_window: Optional[int] = None,
    ):
        self.period = period
        self.robust = robust
        self.seasonal_window = seasonal_window
        self.trend_window = trend_window or (period * 2 + 1)

    def decompose(self, values: np.ndarray) -> DecompositionResult:
        """
        Decompose time series into trend, seasonal, and residual.

        Uses a simplified STL approach:
        1. Extract trend using moving average
        2. Detrend and extract seasonal component
        3. Residual = original - trend - seasonal
        """
        if len(values) < self.period * 2:
            # Not enough data for decomposition
            return DecompositionResult(
                trend=values.copy(),
                seasonal=np.zeros_like(values),
                residual=np.zeros_like(values),
                period=self.period,
                seasonal_strength=0.0,
            )

        # Step 1: Extract trend using centered moving average
        trend = self._moving_average(values, self.trend_window)

        # Step 2: Detrend
        detrended = values - trend

        # Step 3: Extract seasonal component by averaging each period position
        seasonal = self._extract_seasonal(detrended)

        # Step 4: Calculate residual
        residual = values - trend - seasonal

        # Step 5: Calculate seasonal strength
        # Strength = 1 - (Var(residual) / Var(detrended))
        var_residual = np.var(residual)
        var_detrended = np.var(detrended)
        if var_detrended > 0:
            seasonal_strength = max(0, 1 - var_residual / var_detrended)
        else:
            seasonal_strength = 0.0

        return DecompositionResult(
            trend=trend,
            seasonal=seasonal,
            residual=residual,
            period=self.period,
            seasonal_strength=seasonal_strength,
        )

    def _moving_average(self, values: np.ndarray, window: int) -> np.ndarray:
        """Calculate centered moving average."""
        if window % 2 == 0:
            window += 1

        half = window // 2
        result = np.zeros_like(values, dtype=float)

        # Use convolution for efficiency
        kernel = np.ones(window) / window
        smoothed = np.convolve(values, kernel, mode="same")

        # Handle edges with smaller windows
        for i in range(half):
            left_window = i + half + 1
            right_window = len(values) - i + half
            result[i] = np.mean(values[: i + half + 1])
            result[-(i + 1)] = np.mean(values[-(i + half + 1) :])

        result[half:-half] = smoothed[half:-half]
        return result

    def _extract_seasonal(self, detrended: np.ndarray) -> np.ndarray:
        """Extract seasonal component by averaging period positions."""
        n = len(detrended)
        seasonal = np.zeros(n)

        # Calculate average for each position in the period
        period_means = []
        for i in range(self.period):
            indices = np.arange(i, n, self.period)
            period_means.append(np.mean(detrended[indices]))

        # Center the seasonal component (subtract mean)
        period_means = np.array(period_means)
        period_means -= np.mean(period_means)

        # Apply to full series
        for i in range(n):
            seasonal[i] = period_means[i % self.period]

        return seasonal


class FourierAnalyzer:
    """
    Detects periodicities in time series using Fourier analysis.
    """

    def __init__(self, min_period: int = 2, max_period: int = 168):
        self.min_period = min_period
        self.max_period = max_period

    def detect_periods(
        self, values: np.ndarray, top_k: int = 3
    ) -> list[tuple[int, float]]:
        """
        Detect dominant periodicities in the data.

        Returns list of (period, strength) tuples.
        """
        if len(values) < self.min_period * 2:
            return []

        # Remove trend
        detrended = signal.detrend(values)

        # Apply FFT
        n = len(detrended)
        fft_result = fft.fft(detrended)
        power = np.abs(fft_result) ** 2

        # Get frequencies
        freqs = fft.fftfreq(n)

        # Only look at positive frequencies
        positive_mask = freqs > 0
        positive_freqs = freqs[positive_mask]
        positive_power = power[positive_mask]

        # Convert frequencies to periods
        periods = 1 / positive_freqs

        # Filter to valid period range
        valid_mask = (periods >= self.min_period) & (periods <= self.max_period)
        valid_periods = periods[valid_mask]
        valid_power = positive_power[valid_mask]

        if len(valid_periods) == 0:
            return []

        # Normalize power
        total_power = np.sum(valid_power)
        if total_power > 0:
            normalized_power = valid_power / total_power
        else:
            return []

        # Find top k periods
        top_indices = np.argsort(normalized_power)[-top_k:][::-1]

        results = []
        for idx in top_indices:
            period = int(round(valid_periods[idx]))
            strength = float(normalized_power[idx])
            if strength > 0.05:  # Only include significant periods
                results.append((period, strength))

        return results

    def get_period_strength(self, values: np.ndarray, period: int) -> float:
        """Calculate how well data fits a specific period."""
        if len(values) < period * 2:
            return 0.0

        # Reshape to periods
        n_periods = len(values) // period
        trimmed = values[: n_periods * period]
        reshaped = trimmed.reshape(n_periods, period)

        # Calculate correlation between consecutive periods
        if n_periods < 2:
            return 0.0

        correlations = []
        for i in range(n_periods - 1):
            corr, _ = stats.pearsonr(reshaped[i], reshaped[i + 1])
            if not np.isnan(corr):
                correlations.append(corr)

        if not correlations:
            return 0.0

        return float(np.mean(correlations))


class SeasonalAnomalyDetector:
    """
    Main seasonal anomaly detector.

    Combines multiple techniques:
    1. Seasonal baselines (compare to same hour/day/week)
    2. STL decomposition (analyze residuals)
    3. Fourier periodicity detection
    4. Adaptive thresholds based on seasonal variance
    """

    def __init__(
        self,
        hourly_weight: float = 0.5,
        daily_weight: float = 0.3,
        weekly_weight: float = 0.2,
        anomaly_threshold: float = 3.0,  # Standard deviations
        min_samples: int = 168,  # 1 week of hourly data
    ):
        self.hourly_weight = hourly_weight
        self.daily_weight = daily_weight
        self.weekly_weight = weekly_weight
        self.anomaly_threshold = anomaly_threshold
        self.min_samples = min_samples

        self.decomposer = SeasonalDecomposer(period=24)
        self.fourier = FourierAnalyzer()

    def build_baseline(
        self,
        values: np.ndarray,
        timestamps: list[datetime],
        metric_name: str = "",
        service_name: str = "",
    ) -> SeasonalBaseline:
        """
        Build seasonal baseline from historical data.

        Calculates expected values and standard deviations for each:
        - Hour of day (0-23)
        - Day of week (0-6, Monday=0)
        - Combined hour-of-week (0-167)
        """
        if len(values) != len(timestamps):
            raise ValueError("Values and timestamps must have same length")

        # Initialize accumulators
        hourly_values: list[list[float]] = [[] for _ in range(24)]
        daily_values: list[list[float]] = [[] for _ in range(7)]
        weekly_values: list[list[float]] = [[] for _ in range(168)]

        # Accumulate values by time period
        for value, ts in zip(values, timestamps):
            hour = ts.hour
            day = ts.weekday()
            week_idx = day * 24 + hour

            hourly_values[hour].append(value)
            daily_values[day].append(value)
            weekly_values[week_idx].append(value)

        # Calculate statistics
        def calc_stats(buckets: list[list[float]]) -> tuple[list[float], list[float]]:
            means = []
            stds = []
            for bucket in buckets:
                if bucket:
                    means.append(float(np.mean(bucket)))
                    stds.append(float(np.std(bucket)) if len(bucket) > 1 else 0.0)
                else:
                    means.append(0.0)
                    stds.append(0.0)
            return means, stds

        hourly_means, hourly_stds = calc_stats(hourly_values)
        daily_means, daily_stds = calc_stats(daily_values)
        weekly_means, weekly_stds = calc_stats(weekly_values)

        return SeasonalBaseline(
            metric_name=metric_name,
            service_name=service_name,
            hourly_means=hourly_means,
            hourly_stds=hourly_stds,
            daily_means=daily_means,
            daily_stds=daily_stds,
            weekly_pattern=weekly_means,
            weekly_stds=weekly_stds,
            global_mean=float(np.mean(values)),
            global_std=float(np.std(values)),
            sample_count=len(values),
            last_updated=datetime.utcnow(),
        )

    def detect_anomaly(
        self,
        value: float,
        timestamp: datetime,
        baseline: SeasonalBaseline,
        use_decomposition: bool = True,
        recent_values: Optional[np.ndarray] = None,
        recent_timestamps: Optional[list[datetime]] = None,
    ) -> SeasonalAnomalyResult:
        """
        Detect if a value is anomalous given seasonal patterns.

        Args:
            value: Current value to check
            timestamp: Timestamp of the value
            baseline: Pre-computed seasonal baseline
            use_decomposition: Whether to use STL decomposition
            recent_values: Recent historical values for decomposition
            recent_timestamps: Timestamps for recent values
        """
        hour = timestamp.hour
        day = timestamp.weekday()
        week_idx = day * 24 + hour

        contributing_factors = []

        # Get expected values from different seasonal components
        hourly_expected = baseline.hourly_means[hour]
        hourly_std = baseline.hourly_stds[hour] or baseline.global_std

        daily_expected = baseline.daily_means[day]
        daily_std = baseline.daily_stds[day] or baseline.global_std

        weekly_expected = baseline.global_mean
        weekly_std = baseline.global_std
        if baseline.weekly_pattern:
            weekly_expected = baseline.weekly_pattern[week_idx]
            if baseline.weekly_stds:
                weekly_std = baseline.weekly_stds[week_idx] or baseline.global_std

        # Combine expectations with weights
        expected = (
            self.hourly_weight * hourly_expected
            + self.daily_weight * daily_expected
            + self.weekly_weight * weekly_expected
        )

        # Combine standard deviations (weighted average of variances, then sqrt)
        expected_var = (
            self.hourly_weight * (hourly_std**2)
            + self.daily_weight * (daily_std**2)
            + self.weekly_weight * (weekly_std**2)
        )
        expected_std = np.sqrt(expected_var)

        # Ensure minimum std to avoid division by zero
        expected_std = max(expected_std, baseline.global_std * 0.1, 1e-10)

        # Calculate deviation
        deviation = value - expected
        deviation_sigma = abs(deviation) / expected_std

        # Check each component for anomalies
        hourly_sigma = abs(value - hourly_expected) / max(hourly_std, 1e-10)
        daily_sigma = abs(value - daily_expected) / max(daily_std, 1e-10)
        weekly_sigma = abs(value - weekly_expected) / max(weekly_std, 1e-10)

        if hourly_sigma > self.anomaly_threshold:
            contributing_factors.append(
                f"Unusual for hour {hour}:00 (expected ~{hourly_expected:.2f})"
            )
        if daily_sigma > self.anomaly_threshold:
            day_name = [
                "Monday",
                "Tuesday",
                "Wednesday",
                "Thursday",
                "Friday",
                "Saturday",
                "Sunday",
            ][day]
            contributing_factors.append(
                f"Unusual for {day_name} (expected ~{daily_expected:.2f})"
            )

        # Apply decomposition if we have recent data
        decomposition_score = 0.0
        if use_decomposition and recent_values is not None and len(recent_values) >= 48:
            decomp = self.decomposer.decompose(recent_values)
            if decomp.seasonal_strength > 0.3:
                # Strong seasonality - use residual for anomaly detection
                residual_std = np.std(decomp.residual)
                if residual_std > 0:
                    # Get the residual for the most recent value
                    recent_residual = decomp.residual[-1]
                    decomposition_score = abs(recent_residual) / residual_std
                    if decomposition_score > self.anomaly_threshold:
                        contributing_factors.append(
                            f"Unusual after seasonal adjustment (residual: {recent_residual:.2f})"
                        )

        # Combine scores
        base_score = deviation_sigma / self.anomaly_threshold
        if decomposition_score > 0:
            final_score = 0.7 * base_score + 0.3 * (
                decomposition_score / self.anomaly_threshold
            )
        else:
            final_score = base_score

        # Normalize to 0-1
        anomaly_score = min(final_score, 1.0)

        is_anomaly = anomaly_score > 0.7 or deviation_sigma > self.anomaly_threshold

        return SeasonalAnomalyResult(
            is_anomaly=is_anomaly,
            score=anomaly_score,
            raw_value=value,
            expected_value=expected,
            expected_std=expected_std,
            deviation_sigma=deviation_sigma,
            seasonal_context={
                "hour": hour,
                "day_of_week": day,
                "week_index": week_idx,
                "hourly_expected": hourly_expected,
                "daily_expected": daily_expected,
                "weekly_expected": weekly_expected,
            },
            contributing_factors=contributing_factors,
        )

    def detect_batch(
        self,
        values: np.ndarray,
        timestamps: list[datetime],
        baseline: SeasonalBaseline,
    ) -> list[SeasonalAnomalyResult]:
        """Detect anomalies in a batch of values."""
        results = []
        for i, (value, ts) in enumerate(zip(values, timestamps)):
            # Use preceding values for decomposition context
            recent_start = max(0, i - 168)  # Up to 1 week of context
            recent_values = values[recent_start:i] if i > 48 else None

            result = self.detect_anomaly(
                value=value,
                timestamp=ts,
                baseline=baseline,
                recent_values=recent_values,
            )
            results.append(result)

        return results


class HolidayCalendar:
    """
    Handles holiday and special event detection.

    Holidays typically show different patterns and shouldn't be compared
    to normal day baselines.
    """

    def __init__(self):
        # Default US holidays - can be extended
        self.fixed_holidays = [
            (1, 1),  # New Year's Day
            (7, 4),  # Independence Day
            (12, 25),  # Christmas
            (12, 31),  # New Year's Eve
        ]

        # Custom events (can be added dynamically)
        self.custom_events: list[tuple[datetime, datetime, str]] = []

    def add_event(self, start: datetime, end: datetime, name: str) -> None:
        """Add a custom event period."""
        self.custom_events.append((start, end, name))

    def is_holiday(self, dt: datetime) -> tuple[bool, Optional[str]]:
        """Check if a datetime is during a holiday or special event."""
        # Check fixed holidays
        for month, day in self.fixed_holidays:
            if dt.month == month and dt.day == day:
                return True, f"Holiday ({month}/{day})"

        # Check custom events
        for start, end, name in self.custom_events:
            if start <= dt <= end:
                return True, name

        return False, None

    def get_adjustment_factor(self, dt: datetime) -> float:
        """
        Get an adjustment factor for anomaly thresholds during special periods.

        Returns >1.0 to be more lenient during holidays.
        """
        is_hol, _ = self.is_holiday(dt)
        if is_hol:
            return 1.5  # 50% more lenient

        # Also be more lenient on weekends
        if dt.weekday() >= 5:
            return 1.2

        return 1.0


class AdaptiveThresholdCalculator:
    """
    Calculates adaptive anomaly thresholds based on:
    - Time of day patterns
    - Recent volatility
    - Trend direction
    """

    def __init__(
        self,
        base_threshold: float = 3.0,
        volatility_window: int = 24,
        trend_window: int = 6,
    ):
        self.base_threshold = base_threshold
        self.volatility_window = volatility_window
        self.trend_window = trend_window

    def calculate_threshold(
        self,
        recent_values: np.ndarray,
        baseline: SeasonalBaseline,
        timestamp: datetime,
        holiday_calendar: Optional[HolidayCalendar] = None,
    ) -> float:
        """
        Calculate adaptive threshold based on context.
        """
        threshold = self.base_threshold

        # Adjust for volatility
        if len(recent_values) >= self.volatility_window:
            recent_std = np.std(recent_values[-self.volatility_window :])
            historical_std = baseline.global_std
            if historical_std > 0:
                volatility_ratio = recent_std / historical_std
                # Higher volatility = more lenient threshold
                threshold *= max(1.0, min(volatility_ratio, 2.0))

        # Adjust for trend
        if len(recent_values) >= self.trend_window:
            recent = recent_values[-self.trend_window :]
            trend = np.polyfit(range(len(recent)), recent, 1)[0]
            trend_magnitude = abs(trend) / max(baseline.global_std, 1e-10)
            # Strong trend = more lenient for values in trend direction
            if trend_magnitude > 0.5:
                threshold *= 1.2

        # Adjust for holidays
        if holiday_calendar:
            threshold *= holiday_calendar.get_adjustment_factor(timestamp)

        return threshold


def detect_seasonality_type(
    values: np.ndarray, sample_interval_seconds: int = 3600
) -> list[SeasonalPeriod]:
    """
    Auto-detect what types of seasonality exist in the data.
    """
    fourier = FourierAnalyzer()
    detected = []

    # Convert sample interval to detect expected periods
    samples_per_hour = 3600 / sample_interval_seconds
    samples_per_day = samples_per_hour * 24
    samples_per_week = samples_per_day * 7

    periods = fourier.detect_periods(values, top_k=5)

    for period, strength in periods:
        if strength < 0.05:
            continue

        # Map to seasonal period types
        if abs(period - samples_per_day) / samples_per_day < 0.1:
            detected.append(SeasonalPeriod.DAILY)
        elif abs(period - samples_per_week) / samples_per_week < 0.1:
            detected.append(SeasonalPeriod.WEEKLY)
        elif abs(period - samples_per_hour * 12) / (samples_per_hour * 12) < 0.1:
            detected.append(SeasonalPeriod.HOURLY)

    return list(set(detected))
