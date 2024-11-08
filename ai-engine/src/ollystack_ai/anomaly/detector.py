"""
Anomaly Detector

Multi-method anomaly detection for observability data:
- Statistical (z-score, IQR)
- Isolation Forest
- Time-series decomposition
- Seasonal pattern detection (hourly, daily, weekly)
- Holiday and special event awareness
"""

import logging
from dataclasses import dataclass, field
from typing import Optional
from datetime import datetime, timedelta
import json

import numpy as np
from scipy import stats

from ollystack_ai.services.storage import StorageService
from ollystack_ai.services.cache import CacheService
from ollystack_ai.anomaly.seasonal import (
    SeasonalAnomalyDetector,
    SeasonalBaseline,
    SeasonalDecomposer,
    FourierAnalyzer,
    HolidayCalendar,
    AdaptiveThresholdCalculator,
    SeasonalAnomalyResult,
    detect_seasonality_type,
    SeasonalPeriod,
)

logger = logging.getLogger(__name__)


@dataclass
class DetectionResult:
    """Result of anomaly detection."""

    anomalies: list[dict]
    summary: str


@dataclass
class ScoringResult:
    """Result of anomaly scoring."""

    scores: list[float]
    anomaly_indices: list[int]
    threshold: float


@dataclass
class Baseline:
    """Baseline statistics for a metric."""

    mean: float
    std: float
    p50: float
    p90: float
    p99: float
    sample_count: int
    window: str


@dataclass
class TrendResult:
    """Anomaly trend data."""

    by_hour: list[dict]
    by_type: dict[str, int]
    total: int


@dataclass
class SeasonalDetectionResult:
    """Result of seasonal anomaly detection."""

    anomalies: list[dict]
    seasonal_patterns: dict  # Detected seasonality info
    baseline_info: dict  # Baseline statistics
    summary: str


@dataclass
class SeasonalityInfo:
    """Information about detected seasonality in a metric."""

    has_hourly: bool
    has_daily: bool
    has_weekly: bool
    hourly_strength: float
    daily_strength: float
    weekly_strength: float
    dominant_period: str
    detected_periods: list[tuple[int, float]]  # (period, strength)


class AnomalyDetector:
    """
    Multi-method anomaly detector for observability data.

    Detection methods:
    1. Z-score: Statistical deviation from mean
    2. IQR: Interquartile range outliers
    3. Isolation Forest: ML-based anomaly detection
    4. Seasonal decomposition: Time-series aware detection
    """

    def __init__(
        self,
        storage_service: StorageService,
        cache_service: Optional[CacheService] = None,
        z_threshold: float = 3.0,
        iqr_multiplier: float = 1.5,
        enable_seasonality: bool = True,
    ):
        self.storage = storage_service
        self.cache = cache_service
        self.z_threshold = z_threshold
        self.iqr_multiplier = iqr_multiplier
        self.enable_seasonality = enable_seasonality

        # Seasonal detection components
        self.seasonal_detector = SeasonalAnomalyDetector(
            hourly_weight=0.5,
            daily_weight=0.3,
            weekly_weight=0.2,
            anomaly_threshold=z_threshold,
        )
        self.decomposer = SeasonalDecomposer(period=24)
        self.fourier = FourierAnalyzer()
        self.holiday_calendar = HolidayCalendar()
        self.adaptive_threshold = AdaptiveThresholdCalculator(
            base_threshold=z_threshold
        )

    async def detect(
        self,
        service: Optional[str] = None,
        metric: Optional[str] = None,
        time_range: str = "1h",
        sensitivity: float = 0.8,
    ) -> DetectionResult:
        """
        Detect anomalies in observability data.

        Args:
            service: Optional service filter
            metric: Optional metric filter
            time_range: Time range for analysis
            sensitivity: Detection sensitivity (0-1)

        Returns:
            DetectionResult with list of anomalies
        """
        anomalies = []

        # Adjust threshold based on sensitivity
        z_thresh = self.z_threshold * (2 - sensitivity)

        # Detect metric anomalies
        metric_anomalies = await self._detect_metric_anomalies(
            service, metric, time_range, z_thresh
        )
        anomalies.extend(metric_anomalies)

        # Detect latency anomalies
        latency_anomalies = await self._detect_latency_anomalies(
            service, time_range, z_thresh
        )
        anomalies.extend(latency_anomalies)

        # Detect error rate anomalies
        error_anomalies = await self._detect_error_anomalies(
            service, time_range, z_thresh
        )
        anomalies.extend(error_anomalies)

        # Sort by score
        anomalies.sort(key=lambda x: x.get("score", 0), reverse=True)

        summary = self._generate_summary(anomalies)

        return DetectionResult(anomalies=anomalies, summary=summary)

    async def score(
        self,
        metric_name: str,
        values: list[float],
        timestamps: Optional[list[datetime]] = None,
    ) -> ScoringResult:
        """
        Score a series of values for anomalies.

        Uses multiple methods and combines scores.
        """
        if not values:
            return ScoringResult(scores=[], anomaly_indices=[], threshold=0.0)

        values_array = np.array(values)

        # Z-score method
        z_scores = self._z_score(values_array)

        # IQR method
        iqr_scores = self._iqr_score(values_array)

        # Combine scores (average)
        combined = (np.abs(z_scores) / self.z_threshold + iqr_scores) / 2
        combined = np.clip(combined, 0, 1)

        # Find anomaly indices
        threshold = 0.7
        anomaly_indices = list(np.where(combined > threshold)[0])

        return ScoringResult(
            scores=combined.tolist(),
            anomaly_indices=anomaly_indices,
            threshold=threshold,
        )

    async def get_baseline(self, service: str, metric: str) -> Baseline:
        """
        Get baseline statistics for a metric.

        Calculates statistics from historical data.
        """
        # Check cache first
        cache_key = f"baseline:{service}:{metric}"
        if self.cache:
            cached = await self.cache.get(cache_key)
            if cached:
                return Baseline(**cached)

        # Query historical data
        query = f"""
        SELECT
            avg(Value) as mean,
            stddevPop(Value) as std,
            quantile(0.50)(Value) as p50,
            quantile(0.90)(Value) as p90,
            quantile(0.99)(Value) as p99,
            count() as sample_count
        FROM otel_metrics
        WHERE ServiceName = '{service}'
          AND MetricName = '{metric}'
          AND Timestamp >= now() - INTERVAL 7 DAY
        """

        result = await self.storage.execute_query(query)
        rows = result[0] if result else []

        if not rows:
            return Baseline(
                mean=0, std=1, p50=0, p90=0, p99=0, sample_count=0, window="7d"
            )

        row = rows[0]
        baseline = Baseline(
            mean=float(row.get("mean", 0) or 0),
            std=float(row.get("std", 1) or 1),
            p50=float(row.get("p50", 0) or 0),
            p90=float(row.get("p90", 0) or 0),
            p99=float(row.get("p99", 0) or 0),
            sample_count=int(row.get("sample_count", 0) or 0),
            window="7d",
        )

        # Cache for 1 hour
        if self.cache:
            await self.cache.set(cache_key, baseline.__dict__, ttl=3600)

        return baseline

    async def get_trends(self, service: str, time_range: str) -> TrendResult:
        """Get anomaly trends for a service."""
        # Query anomalies by hour
        query = f"""
        SELECT
            toStartOfHour(Timestamp) as hour,
            AnomalyType,
            count() as count
        FROM anomalies
        WHERE ServiceName = '{service}'
          AND Timestamp >= now() - INTERVAL {time_range}
        GROUP BY hour, AnomalyType
        ORDER BY hour
        """

        result = await self.storage.execute_query(query)
        rows = result[0] if result else []

        # Aggregate by hour
        by_hour = {}
        by_type: dict[str, int] = {}
        total = 0

        for row in rows:
            hour = str(row.get("hour"))
            atype = row.get("AnomalyType", "unknown")
            count = int(row.get("count", 0))

            if hour not in by_hour:
                by_hour[hour] = {"hour": hour, "count": 0}
            by_hour[hour]["count"] += count

            by_type[atype] = by_type.get(atype, 0) + count
            total += count

        return TrendResult(
            by_hour=list(by_hour.values()),
            by_type=by_type,
            total=total,
        )

    # -------------------------------------------------------------------------
    # Seasonal Detection Methods
    # -------------------------------------------------------------------------

    async def detect_seasonal(
        self,
        service: str,
        metric: str,
        time_range: str = "1h",
        baseline_window: str = "7d",
        sensitivity: float = 0.8,
    ) -> SeasonalDetectionResult:
        """
        Detect anomalies using seasonal awareness.

        Compares current values against seasonal baselines (same hour of day,
        day of week, etc.) rather than simple statistical averages.

        Args:
            service: Service name
            metric: Metric name
            time_range: Time range for detection
            baseline_window: Historical window for baseline calculation
            sensitivity: Detection sensitivity (0-1)

        Returns:
            SeasonalDetectionResult with anomalies and seasonal context
        """
        # Get or build seasonal baseline
        baseline = await self._get_seasonal_baseline(service, metric, baseline_window)

        if baseline.sample_count < 168:  # Less than 1 week of hourly data
            # Fall back to standard detection
            result = await self.detect(service, metric, time_range, sensitivity)
            return SeasonalDetectionResult(
                anomalies=result.anomalies,
                seasonal_patterns={"insufficient_data": True},
                baseline_info={"sample_count": baseline.sample_count},
                summary=result.summary + " (insufficient data for seasonal analysis)",
            )

        # Get current values for analysis
        values, timestamps = await self._get_metric_series(
            service, metric, time_range
        )

        if not values:
            return SeasonalDetectionResult(
                anomalies=[],
                seasonal_patterns={},
                baseline_info={},
                summary="No data found for analysis",
            )

        # Adjust threshold based on sensitivity
        adaptive_threshold = self.z_threshold * (2 - sensitivity)

        # Detect anomalies with seasonal context
        anomalies = []
        for i, (value, ts) in enumerate(zip(values, timestamps)):
            # Get recent context for decomposition
            recent_start = max(0, i - 168)
            recent_values = np.array(values[recent_start:i]) if i > 48 else None

            # Calculate adaptive threshold
            current_threshold = adaptive_threshold
            if recent_values is not None and len(recent_values) > 24:
                current_threshold = self.adaptive_threshold.calculate_threshold(
                    recent_values=recent_values,
                    baseline=baseline,
                    timestamp=ts,
                    holiday_calendar=self.holiday_calendar,
                )

            # Check for holiday/special event
            is_holiday, holiday_name = self.holiday_calendar.is_holiday(ts)

            # Detect anomaly with seasonal awareness
            result = self.seasonal_detector.detect_anomaly(
                value=value,
                timestamp=ts,
                baseline=baseline,
                use_decomposition=True,
                recent_values=recent_values,
            )

            if result.is_anomaly and result.deviation_sigma > current_threshold:
                anomaly = {
                    "type": "seasonal_metric",
                    "timestamp": ts.isoformat(),
                    "service": service,
                    "metric": metric,
                    "value": value,
                    "expected_value": result.expected_value,
                    "expected_std": result.expected_std,
                    "deviation_sigma": result.deviation_sigma,
                    "score": result.score,
                    "seasonal_context": result.seasonal_context,
                    "contributing_factors": result.contributing_factors,
                    "is_holiday": is_holiday,
                    "holiday_name": holiday_name,
                    "description": self._build_seasonal_description(result, ts),
                }
                anomalies.append(anomaly)

        # Sort by score
        anomalies.sort(key=lambda x: x.get("score", 0), reverse=True)

        # Detect seasonality patterns
        seasonality_info = await self._analyze_seasonality(values, timestamps)

        return SeasonalDetectionResult(
            anomalies=anomalies,
            seasonal_patterns={
                "has_hourly": seasonality_info.has_hourly,
                "has_daily": seasonality_info.has_daily,
                "has_weekly": seasonality_info.has_weekly,
                "hourly_strength": seasonality_info.hourly_strength,
                "daily_strength": seasonality_info.daily_strength,
                "weekly_strength": seasonality_info.weekly_strength,
                "dominant_period": seasonality_info.dominant_period,
            },
            baseline_info={
                "sample_count": baseline.sample_count,
                "global_mean": baseline.global_mean,
                "global_std": baseline.global_std,
                "last_updated": baseline.last_updated.isoformat(),
            },
            summary=self._generate_seasonal_summary(anomalies, seasonality_info),
        )

    async def get_seasonal_baseline(
        self, service: str, metric: str, window: str = "7d"
    ) -> SeasonalBaseline:
        """
        Get or compute seasonal baseline for a metric.

        Public method to retrieve seasonal baselines.
        """
        return await self._get_seasonal_baseline(service, metric, window)

    async def analyze_seasonality(
        self, service: str, metric: str, time_range: str = "7d"
    ) -> SeasonalityInfo:
        """
        Analyze what seasonal patterns exist in a metric.

        Returns information about detected hourly, daily, and weekly patterns.
        """
        values, timestamps = await self._get_metric_series(
            service, metric, time_range
        )

        if not values:
            return SeasonalityInfo(
                has_hourly=False,
                has_daily=False,
                has_weekly=False,
                hourly_strength=0.0,
                daily_strength=0.0,
                weekly_strength=0.0,
                dominant_period="none",
                detected_periods=[],
            )

        return await self._analyze_seasonality(values, timestamps)

    async def decompose_metric(
        self, service: str, metric: str, time_range: str = "7d"
    ) -> dict:
        """
        Decompose a metric into trend, seasonal, and residual components.

        Useful for understanding underlying patterns in the data.
        """
        values, timestamps = await self._get_metric_series(
            service, metric, time_range
        )

        if not values or len(values) < 48:
            return {
                "error": "Insufficient data for decomposition",
                "required_samples": 48,
                "actual_samples": len(values) if values else 0,
            }

        values_array = np.array(values)

        # Daily seasonality decomposition
        decomp_daily = self.decomposer.decompose(values_array)

        # Weekly decomposition if enough data
        weekly_decomp = None
        if len(values) >= 336:  # 2 weeks of hourly data
            weekly_decomposer = SeasonalDecomposer(period=168)
            weekly_decomp = weekly_decomposer.decompose(values_array)

        result = {
            "timestamps": [ts.isoformat() for ts in timestamps],
            "original": values,
            "trend": decomp_daily.trend.tolist(),
            "seasonal_daily": decomp_daily.seasonal.tolist(),
            "residual": decomp_daily.residual.tolist(),
            "seasonal_strength_daily": decomp_daily.seasonal_strength,
        }

        if weekly_decomp:
            result["seasonal_weekly"] = weekly_decomp.seasonal.tolist()
            result["seasonal_strength_weekly"] = weekly_decomp.seasonal_strength

        return result

    async def compare_to_baseline(
        self,
        service: str,
        metric: str,
        current_value: float,
        timestamp: Optional[datetime] = None,
    ) -> dict:
        """
        Compare a single value to its seasonal baseline.

        Returns expected value, deviation, and whether it's anomalous.
        """
        if timestamp is None:
            timestamp = datetime.utcnow()

        baseline = await self._get_seasonal_baseline(service, metric, "7d")

        result = self.seasonal_detector.detect_anomaly(
            value=current_value,
            timestamp=timestamp,
            baseline=baseline,
            use_decomposition=False,
        )

        return {
            "value": current_value,
            "expected_value": result.expected_value,
            "expected_std": result.expected_std,
            "deviation_sigma": result.deviation_sigma,
            "is_anomaly": result.is_anomaly,
            "score": result.score,
            "seasonal_context": result.seasonal_context,
            "contributing_factors": result.contributing_factors,
        }

    async def add_holiday(
        self, start: datetime, end: datetime, name: str
    ) -> None:
        """Add a custom holiday or special event to the calendar."""
        self.holiday_calendar.add_event(start, end, name)

    async def _get_seasonal_baseline(
        self, service: str, metric: str, window: str
    ) -> SeasonalBaseline:
        """Get or compute seasonal baseline."""
        cache_key = f"seasonal_baseline:{service}:{metric}:{window}"

        # Check cache
        if self.cache:
            cached = await self.cache.get(cache_key)
            if cached:
                return SeasonalBaseline(
                    metric_name=cached["metric_name"],
                    service_name=cached["service_name"],
                    hourly_means=cached["hourly_means"],
                    hourly_stds=cached["hourly_stds"],
                    daily_means=cached["daily_means"],
                    daily_stds=cached["daily_stds"],
                    weekly_pattern=cached.get("weekly_pattern"),
                    weekly_stds=cached.get("weekly_stds"),
                    global_mean=cached["global_mean"],
                    global_std=cached["global_std"],
                    sample_count=cached["sample_count"],
                    last_updated=datetime.fromisoformat(cached["last_updated"]),
                )

        # Query historical data
        values, timestamps = await self._get_metric_series(service, metric, window)

        if not values:
            return SeasonalBaseline(
                metric_name=metric,
                service_name=service,
                hourly_means=[0.0] * 24,
                hourly_stds=[1.0] * 24,
                daily_means=[0.0] * 7,
                daily_stds=[1.0] * 7,
                weekly_pattern=None,
                weekly_stds=None,
                global_mean=0.0,
                global_std=1.0,
                sample_count=0,
                last_updated=datetime.utcnow(),
            )

        # Build baseline
        baseline = self.seasonal_detector.build_baseline(
            values=np.array(values),
            timestamps=timestamps,
            metric_name=metric,
            service_name=service,
        )

        # Cache for 1 hour
        if self.cache:
            cache_data = {
                "metric_name": baseline.metric_name,
                "service_name": baseline.service_name,
                "hourly_means": baseline.hourly_means,
                "hourly_stds": baseline.hourly_stds,
                "daily_means": baseline.daily_means,
                "daily_stds": baseline.daily_stds,
                "weekly_pattern": baseline.weekly_pattern,
                "weekly_stds": baseline.weekly_stds,
                "global_mean": baseline.global_mean,
                "global_std": baseline.global_std,
                "sample_count": baseline.sample_count,
                "last_updated": baseline.last_updated.isoformat(),
            }
            await self.cache.set(cache_key, cache_data, ttl=3600)

        return baseline

    async def _get_metric_series(
        self, service: str, metric: str, time_range: str
    ) -> tuple[list[float], list[datetime]]:
        """Fetch metric time series from storage."""
        query = f"""
        SELECT
            toStartOfHour(Timestamp) as hour,
            avg(Value) as value
        FROM otel_metrics
        WHERE ServiceName = '{service}'
          AND MetricName = '{metric}'
          AND Timestamp >= now() - INTERVAL {time_range}
        GROUP BY hour
        ORDER BY hour
        """

        result = await self.storage.execute_query(query)
        rows = result[0] if result else []

        values = []
        timestamps = []
        for row in rows:
            values.append(float(row.get("value", 0)))
            ts = row.get("hour")
            if isinstance(ts, str):
                ts = datetime.fromisoformat(ts.replace("Z", "+00:00"))
            timestamps.append(ts)

        return values, timestamps

    async def _analyze_seasonality(
        self, values: list[float], timestamps: list[datetime]
    ) -> SeasonalityInfo:
        """Analyze seasonality patterns in data."""
        if not values or len(values) < 48:
            return SeasonalityInfo(
                has_hourly=False,
                has_daily=False,
                has_weekly=False,
                hourly_strength=0.0,
                daily_strength=0.0,
                weekly_strength=0.0,
                dominant_period="none",
                detected_periods=[],
            )

        values_array = np.array(values)

        # Detect periods using Fourier analysis
        detected_periods = self.fourier.detect_periods(values_array, top_k=5)

        # Calculate strength for known periods
        hourly_strength = self.fourier.get_period_strength(values_array, 24)
        daily_strength = self.fourier.get_period_strength(values_array, 24)  # Daily = 24 hourly samples
        weekly_strength = 0.0
        if len(values) >= 336:  # Need 2 weeks for weekly analysis
            weekly_strength = self.fourier.get_period_strength(values_array, 168)

        # Determine if patterns are significant
        has_hourly = any(
            abs(p - 12) <= 2 and s > 0.1 for p, s in detected_periods
        )  # ~12 hour pattern
        has_daily = any(
            abs(p - 24) <= 3 and s > 0.1 for p, s in detected_periods
        )
        has_weekly = any(
            abs(p - 168) <= 20 and s > 0.1 for p, s in detected_periods
        )

        # Determine dominant period
        dominant_period = "none"
        if detected_periods:
            top_period, top_strength = detected_periods[0]
            if top_strength > 0.1:
                if abs(top_period - 24) <= 3:
                    dominant_period = "daily"
                elif abs(top_period - 168) <= 20:
                    dominant_period = "weekly"
                elif abs(top_period - 12) <= 2:
                    dominant_period = "12-hour"
                else:
                    dominant_period = f"{top_period}-hour"

        return SeasonalityInfo(
            has_hourly=has_hourly,
            has_daily=has_daily,
            has_weekly=has_weekly,
            hourly_strength=hourly_strength,
            daily_strength=daily_strength,
            weekly_strength=weekly_strength,
            dominant_period=dominant_period,
            detected_periods=detected_periods,
        )

    def _build_seasonal_description(
        self, result: SeasonalAnomalyResult, timestamp: datetime
    ) -> str:
        """Build human-readable description for seasonal anomaly."""
        hour = timestamp.hour
        day_name = [
            "Monday", "Tuesday", "Wednesday", "Thursday",
            "Friday", "Saturday", "Sunday"
        ][timestamp.weekday()]

        parts = [
            f"Value {result.raw_value:.2f} is {result.deviation_sigma:.1f}σ "
            f"from expected {result.expected_value:.2f}"
        ]

        if result.contributing_factors:
            parts.append(f"Factors: {'; '.join(result.contributing_factors)}")

        parts.append(f"Time: {day_name} {hour:02d}:00")

        return ". ".join(parts)

    def _generate_seasonal_summary(
        self, anomalies: list[dict], seasonality: SeasonalityInfo
    ) -> str:
        """Generate summary of seasonal detection results."""
        parts = []

        if anomalies:
            parts.append(f"Detected {len(anomalies)} seasonal anomalies")
        else:
            parts.append("No seasonal anomalies detected")

        # Add seasonality info
        patterns = []
        if seasonality.has_daily:
            patterns.append(f"daily (strength: {seasonality.daily_strength:.2f})")
        if seasonality.has_weekly:
            patterns.append(f"weekly (strength: {seasonality.weekly_strength:.2f})")

        if patterns:
            parts.append(f"Seasonal patterns: {', '.join(patterns)}")
        else:
            parts.append("No significant seasonal patterns detected")

        return ". ".join(parts)

    async def _detect_metric_anomalies(
        self,
        service: Optional[str],
        metric: Optional[str],
        time_range: str,
        z_threshold: float,
    ) -> list[dict]:
        """Detect anomalies in metrics."""
        service_filter = f"AND ServiceName = '{service}'" if service else ""
        metric_filter = f"AND MetricName = '{metric}'" if metric else ""

        query = f"""
        SELECT
            Timestamp,
            ServiceName,
            MetricName,
            Value,
            AnomalyScore
        FROM otel_metrics
        WHERE Timestamp >= now() - INTERVAL {time_range}
          AND AnomalyScore > 0.7
          {service_filter}
          {metric_filter}
        ORDER BY AnomalyScore DESC
        LIMIT 50
        """

        result = await self.storage.execute_query(query)
        rows = result[0] if result else []

        anomalies = []
        for row in rows:
            anomalies.append({
                "type": "metric",
                "timestamp": str(row.get("Timestamp")),
                "service": row.get("ServiceName"),
                "metric": row.get("MetricName"),
                "value": row.get("Value"),
                "score": row.get("AnomalyScore", 0.8),
                "description": f"Anomalous value for {row.get('MetricName')}",
            })

        return anomalies

    async def _detect_latency_anomalies(
        self,
        service: Optional[str],
        time_range: str,
        z_threshold: float,
    ) -> list[dict]:
        """Detect latency anomalies in traces."""
        service_filter = f"AND ServiceName = '{service}'" if service else ""

        # Get latency statistics
        stats_query = f"""
        SELECT
            ServiceName,
            SpanName,
            avg(Duration) as avg_duration,
            stddevPop(Duration) as std_duration,
            quantile(0.99)(Duration) as p99_duration
        FROM otel_traces
        WHERE Timestamp >= now() - INTERVAL 7 DAY
          {service_filter}
        GROUP BY ServiceName, SpanName
        """

        stats_result = await self.storage.execute_query(stats_query)
        stats = {
            f"{r['ServiceName']}:{r['SpanName']}": r
            for r in (stats_result[0] if stats_result else [])
        }

        # Find anomalous traces
        query = f"""
        SELECT
            Timestamp,
            TraceId,
            ServiceName,
            SpanName,
            Duration
        FROM otel_traces
        WHERE Timestamp >= now() - INTERVAL {time_range}
          AND Duration > 1000000000
          {service_filter}
        ORDER BY Duration DESC
        LIMIT 50
        """

        result = await self.storage.execute_query(query)
        rows = result[0] if result else []

        anomalies = []
        for row in rows:
            key = f"{row.get('ServiceName')}:{row.get('SpanName')}"
            baseline = stats.get(key, {})

            avg = baseline.get("avg_duration", 0) or 0
            std = baseline.get("std_duration", 1) or 1
            duration = row.get("Duration", 0)

            if std > 0:
                z_score = abs(duration - avg) / std
                if z_score > z_threshold:
                    anomalies.append({
                        "type": "latency",
                        "timestamp": str(row.get("Timestamp")),
                        "service": row.get("ServiceName"),
                        "operation": row.get("SpanName"),
                        "trace_id": row.get("TraceId"),
                        "duration_ms": duration / 1_000_000,
                        "score": min(z_score / (z_threshold * 2), 1.0),
                        "description": f"High latency: {duration / 1_000_000:.0f}ms (avg: {avg / 1_000_000:.0f}ms)",
                    })

        return anomalies

    async def _detect_error_anomalies(
        self,
        service: Optional[str],
        time_range: str,
        z_threshold: float,
    ) -> list[dict]:
        """Detect error rate anomalies."""
        service_filter = f"AND ServiceName = '{service}'" if service else ""

        query = f"""
        SELECT
            toStartOfMinute(Timestamp) as minute,
            ServiceName,
            count() as total,
            countIf(StatusCode = 'ERROR') as errors
        FROM otel_traces
        WHERE Timestamp >= now() - INTERVAL {time_range}
          {service_filter}
        GROUP BY minute, ServiceName
        HAVING errors > 0
        ORDER BY errors DESC
        LIMIT 50
        """

        result = await self.storage.execute_query(query)
        rows = result[0] if result else []

        anomalies = []
        for row in rows:
            total = row.get("total", 1)
            errors = row.get("errors", 0)
            error_rate = errors / total if total > 0 else 0

            if error_rate > 0.1:  # > 10% error rate
                anomalies.append({
                    "type": "error_rate",
                    "timestamp": str(row.get("minute")),
                    "service": row.get("ServiceName"),
                    "error_count": errors,
                    "total_count": total,
                    "error_rate": error_rate,
                    "score": min(error_rate * 2, 1.0),
                    "description": f"High error rate: {error_rate * 100:.1f}% ({errors}/{total})",
                })

        return anomalies

    def _z_score(self, values: np.ndarray) -> np.ndarray:
        """Calculate z-scores for values."""
        mean = np.mean(values)
        std = np.std(values)
        if std == 0:
            return np.zeros_like(values)
        return (values - mean) / std

    def _iqr_score(self, values: np.ndarray) -> np.ndarray:
        """Calculate IQR-based anomaly scores."""
        q1 = np.percentile(values, 25)
        q3 = np.percentile(values, 75)
        iqr = q3 - q1

        if iqr == 0:
            return np.zeros_like(values)

        lower = q1 - self.iqr_multiplier * iqr
        upper = q3 + self.iqr_multiplier * iqr

        scores = np.zeros_like(values, dtype=float)
        below = values < lower
        above = values > upper

        if np.any(below):
            scores[below] = (lower - values[below]) / iqr
        if np.any(above):
            scores[above] = (values[above] - upper) / iqr

        return np.clip(scores / 3, 0, 1)  # Normalize

    def _generate_summary(self, anomalies: list[dict]) -> str:
        """Generate summary of detected anomalies."""
        if not anomalies:
            return "No anomalies detected"

        by_type = {}
        for a in anomalies:
            t = a.get("type", "unknown")
            by_type[t] = by_type.get(t, 0) + 1

        parts = [f"{count} {atype}" for atype, count in by_type.items()]
        return f"Detected {len(anomalies)} anomalies: {', '.join(parts)}"
