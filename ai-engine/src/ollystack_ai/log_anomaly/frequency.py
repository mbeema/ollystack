"""
Log Frequency Anomaly Detection

Detects anomalies in log pattern frequencies:
- Sudden spikes in error logs
- Unusual drops in expected log patterns
- Burst detection
- Trend changes
"""

import logging
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Optional
from collections import defaultdict
from enum import Enum

import numpy as np
from scipy import stats

logger = logging.getLogger(__name__)


class FrequencyAnomalyType(Enum):
    """Types of frequency anomalies."""

    SPIKE = "spike"  # Sudden increase
    DROP = "drop"  # Sudden decrease
    BURST = "burst"  # Rapid successive occurrences
    TREND_CHANGE = "trend_change"  # Gradual shift in baseline
    MISSING = "missing"  # Expected pattern not seen
    UNUSUAL_TIME = "unusual_time"  # Pattern at unexpected time


@dataclass
class FrequencyAnomaly:
    """Represents a detected frequency anomaly."""

    pattern_id: str
    pattern_template: str
    anomaly_type: FrequencyAnomalyType
    timestamp: datetime
    observed_count: int
    expected_count: float
    expected_std: float
    deviation_sigma: float
    score: float  # 0-1 anomaly score
    window_minutes: int
    description: str
    context: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "pattern_id": self.pattern_id,
            "pattern_template": self.pattern_template,
            "anomaly_type": self.anomaly_type.value,
            "timestamp": self.timestamp.isoformat(),
            "observed_count": self.observed_count,
            "expected_count": self.expected_count,
            "expected_std": self.expected_std,
            "deviation_sigma": self.deviation_sigma,
            "score": self.score,
            "window_minutes": self.window_minutes,
            "description": self.description,
            "context": self.context,
        }


@dataclass
class PatternFrequencyBaseline:
    """Baseline frequency statistics for a pattern."""

    pattern_id: str
    mean_per_minute: float
    std_per_minute: float
    hourly_means: list[float]  # 24 values
    hourly_stds: list[float]
    daily_means: list[float]  # 7 values (Mon-Sun)
    daily_stds: list[float]
    total_count: int
    last_updated: datetime


class FrequencyAnalyzer:
    """
    Analyzes log pattern frequencies for anomalies.

    Tracks pattern occurrences over time and detects:
    - Sudden frequency spikes or drops
    - Bursts (rapid successive occurrences)
    - Missing expected patterns
    - Unusual timing patterns
    """

    def __init__(
        self,
        window_minutes: int = 5,
        baseline_hours: int = 24,
        spike_threshold: float = 3.0,
        drop_threshold: float = 2.0,
        burst_window_seconds: int = 10,
        burst_threshold: int = 10,
        min_baseline_samples: int = 100,
    ):
        """
        Initialize the frequency analyzer.

        Args:
            window_minutes: Time window for frequency calculation
            baseline_hours: Hours of history for baseline calculation
            spike_threshold: Z-score threshold for spike detection
            drop_threshold: Z-score threshold for drop detection
            burst_window_seconds: Window for burst detection
            burst_threshold: Count threshold for burst detection
            min_baseline_samples: Minimum samples for reliable baseline
        """
        self.window_minutes = window_minutes
        self.baseline_hours = baseline_hours
        self.spike_threshold = spike_threshold
        self.drop_threshold = drop_threshold
        self.burst_window_seconds = burst_window_seconds
        self.burst_threshold = burst_threshold
        self.min_baseline_samples = min_baseline_samples

        # Pattern occurrence tracking
        # pattern_id -> list of timestamps
        self._occurrences: dict[str, list[datetime]] = defaultdict(list)

        # Pattern baselines
        self._baselines: dict[str, PatternFrequencyBaseline] = {}

        # Recent windows for fast lookup
        # pattern_id -> {window_start: count}
        self._window_counts: dict[str, dict[datetime, int]] = defaultdict(dict)

    def record_occurrence(
        self,
        pattern_id: str,
        timestamp: Optional[datetime] = None,
        pattern_template: str = "",
    ) -> Optional[FrequencyAnomaly]:
        """
        Record a pattern occurrence and check for anomalies.

        Args:
            pattern_id: The pattern identifier
            timestamp: When the pattern occurred (default: now)
            pattern_template: The pattern template for context

        Returns:
            FrequencyAnomaly if detected, None otherwise
        """
        if timestamp is None:
            timestamp = datetime.utcnow()

        # Record occurrence
        self._occurrences[pattern_id].append(timestamp)

        # Update window count
        window_start = self._get_window_start(timestamp)
        if window_start not in self._window_counts[pattern_id]:
            self._window_counts[pattern_id][window_start] = 0
        self._window_counts[pattern_id][window_start] += 1

        # Clean old data periodically
        if len(self._occurrences[pattern_id]) > 10000:
            self._cleanup_old_data(pattern_id)

        # Check for anomalies
        return self._check_anomalies(pattern_id, timestamp, pattern_template)

    def record_batch(
        self,
        occurrences: list[dict],
    ) -> list[FrequencyAnomaly]:
        """
        Record multiple occurrences and return all anomalies.

        Args:
            occurrences: List of dicts with pattern_id, timestamp, template

        Returns:
            List of detected anomalies
        """
        anomalies = []
        for occ in occurrences:
            anomaly = self.record_occurrence(
                pattern_id=occ["pattern_id"],
                timestamp=occ.get("timestamp"),
                pattern_template=occ.get("template", ""),
            )
            if anomaly:
                anomalies.append(anomaly)
        return anomalies

    def get_baseline(self, pattern_id: str) -> Optional[PatternFrequencyBaseline]:
        """Get the baseline for a pattern."""
        return self._baselines.get(pattern_id)

    def update_baseline(self, pattern_id: str) -> PatternFrequencyBaseline:
        """Update or create baseline for a pattern."""
        occurrences = self._occurrences.get(pattern_id, [])

        if not occurrences:
            baseline = PatternFrequencyBaseline(
                pattern_id=pattern_id,
                mean_per_minute=0,
                std_per_minute=1,
                hourly_means=[0] * 24,
                hourly_stds=[1] * 24,
                daily_means=[0] * 7,
                daily_stds=[1] * 7,
                total_count=0,
                last_updated=datetime.utcnow(),
            )
            self._baselines[pattern_id] = baseline
            return baseline

        # Filter to baseline window
        cutoff = datetime.utcnow() - timedelta(hours=self.baseline_hours)
        recent = [ts for ts in occurrences if ts >= cutoff]

        # Calculate per-minute rates
        minute_counts = defaultdict(int)
        hourly_counts: dict[int, list[int]] = defaultdict(list)
        daily_counts: dict[int, list[int]] = defaultdict(list)

        for ts in recent:
            minute_key = ts.replace(second=0, microsecond=0)
            minute_counts[minute_key] += 1

        # Aggregate to hourly/daily
        for minute_key, count in minute_counts.items():
            hour = minute_key.hour
            day = minute_key.weekday()
            hourly_counts[hour].append(count)
            daily_counts[day].append(count)

        # Calculate statistics
        all_counts = list(minute_counts.values())
        mean_per_minute = np.mean(all_counts) if all_counts else 0
        std_per_minute = np.std(all_counts) if len(all_counts) > 1 else 1

        hourly_means = []
        hourly_stds = []
        for h in range(24):
            counts = hourly_counts.get(h, [0])
            hourly_means.append(np.mean(counts))
            hourly_stds.append(np.std(counts) if len(counts) > 1 else 1)

        daily_means = []
        daily_stds = []
        for d in range(7):
            counts = daily_counts.get(d, [0])
            daily_means.append(np.mean(counts))
            daily_stds.append(np.std(counts) if len(counts) > 1 else 1)

        baseline = PatternFrequencyBaseline(
            pattern_id=pattern_id,
            mean_per_minute=mean_per_minute,
            std_per_minute=max(std_per_minute, 0.1),
            hourly_means=hourly_means,
            hourly_stds=hourly_stds,
            daily_means=daily_means,
            daily_stds=daily_stds,
            total_count=len(recent),
            last_updated=datetime.utcnow(),
        )
        self._baselines[pattern_id] = baseline
        return baseline

    def check_missing_patterns(
        self,
        expected_patterns: list[str],
        window_minutes: Optional[int] = None,
    ) -> list[FrequencyAnomaly]:
        """
        Check for patterns that should have occurred but didn't.

        Args:
            expected_patterns: List of pattern IDs expected to occur
            window_minutes: Time window to check (default: self.window_minutes)

        Returns:
            List of anomalies for missing patterns
        """
        if window_minutes is None:
            window_minutes = self.window_minutes

        anomalies = []
        cutoff = datetime.utcnow() - timedelta(minutes=window_minutes)

        for pattern_id in expected_patterns:
            baseline = self._baselines.get(pattern_id)
            if not baseline or baseline.total_count < self.min_baseline_samples:
                continue

            # Count recent occurrences
            recent = [
                ts for ts in self._occurrences.get(pattern_id, [])
                if ts >= cutoff
            ]

            # Expected count
            expected = baseline.mean_per_minute * window_minutes
            expected_std = baseline.std_per_minute * np.sqrt(window_minutes)

            if expected > 1 and len(recent) == 0:
                # Pattern expected but missing
                anomalies.append(FrequencyAnomaly(
                    pattern_id=pattern_id,
                    pattern_template="",
                    anomaly_type=FrequencyAnomalyType.MISSING,
                    timestamp=datetime.utcnow(),
                    observed_count=0,
                    expected_count=expected,
                    expected_std=expected_std,
                    deviation_sigma=expected / max(expected_std, 0.1),
                    score=min(expected / 10, 1.0),
                    window_minutes=window_minutes,
                    description=f"Expected pattern missing: ~{expected:.1f} occurrences expected",
                ))

        return anomalies

    def get_frequency_stats(self, pattern_id: str, window_minutes: int = 60) -> dict:
        """Get frequency statistics for a pattern."""
        cutoff = datetime.utcnow() - timedelta(minutes=window_minutes)
        recent = [
            ts for ts in self._occurrences.get(pattern_id, [])
            if ts >= cutoff
        ]

        if not recent:
            return {
                "count": 0,
                "rate_per_minute": 0,
                "window_minutes": window_minutes,
            }

        # Calculate rate
        rate = len(recent) / window_minutes

        # Calculate inter-arrival times
        sorted_times = sorted(recent)
        intervals = []
        for i in range(1, len(sorted_times)):
            interval = (sorted_times[i] - sorted_times[i - 1]).total_seconds()
            intervals.append(interval)

        return {
            "count": len(recent),
            "rate_per_minute": rate,
            "window_minutes": window_minutes,
            "mean_interval_seconds": np.mean(intervals) if intervals else 0,
            "min_interval_seconds": min(intervals) if intervals else 0,
            "max_interval_seconds": max(intervals) if intervals else 0,
        }

    def _check_anomalies(
        self,
        pattern_id: str,
        timestamp: datetime,
        pattern_template: str,
    ) -> Optional[FrequencyAnomaly]:
        """Check for various frequency anomalies."""
        # Check for burst
        burst_anomaly = self._check_burst(pattern_id, timestamp, pattern_template)
        if burst_anomaly:
            return burst_anomaly

        # Check for spike/drop (need baseline)
        baseline = self._baselines.get(pattern_id)
        if baseline and baseline.total_count >= self.min_baseline_samples:
            frequency_anomaly = self._check_frequency_anomaly(
                pattern_id, timestamp, pattern_template, baseline
            )
            if frequency_anomaly:
                return frequency_anomaly

            # Check for unusual timing
            timing_anomaly = self._check_timing_anomaly(
                pattern_id, timestamp, pattern_template, baseline
            )
            if timing_anomaly:
                return timing_anomaly

        return None

    def _check_burst(
        self,
        pattern_id: str,
        timestamp: datetime,
        pattern_template: str,
    ) -> Optional[FrequencyAnomaly]:
        """Check for burst of occurrences."""
        cutoff = timestamp - timedelta(seconds=self.burst_window_seconds)
        recent = [
            ts for ts in self._occurrences.get(pattern_id, [])
            if ts >= cutoff
        ]

        if len(recent) >= self.burst_threshold:
            return FrequencyAnomaly(
                pattern_id=pattern_id,
                pattern_template=pattern_template,
                anomaly_type=FrequencyAnomalyType.BURST,
                timestamp=timestamp,
                observed_count=len(recent),
                expected_count=self.burst_threshold / 2,
                expected_std=2,
                deviation_sigma=len(recent) / self.burst_threshold * 3,
                score=min(len(recent) / self.burst_threshold / 2, 1.0),
                window_minutes=self.burst_window_seconds // 60 or 1,
                description=f"Burst detected: {len(recent)} occurrences in {self.burst_window_seconds}s",
                context={"burst_count": len(recent)},
            )

        return None

    def _check_frequency_anomaly(
        self,
        pattern_id: str,
        timestamp: datetime,
        pattern_template: str,
        baseline: PatternFrequencyBaseline,
    ) -> Optional[FrequencyAnomaly]:
        """Check for frequency spike or drop."""
        window_start = self._get_window_start(timestamp)
        window_count = self._window_counts[pattern_id].get(window_start, 0)

        # Get hour-adjusted expected value
        hour = timestamp.hour
        expected = baseline.hourly_means[hour] * self.window_minutes
        expected_std = baseline.hourly_stds[hour] * np.sqrt(self.window_minutes)
        expected_std = max(expected_std, 0.5)  # Minimum std

        deviation = window_count - expected
        deviation_sigma = abs(deviation) / expected_std

        # Check for spike
        if deviation > 0 and deviation_sigma > self.spike_threshold:
            return FrequencyAnomaly(
                pattern_id=pattern_id,
                pattern_template=pattern_template,
                anomaly_type=FrequencyAnomalyType.SPIKE,
                timestamp=timestamp,
                observed_count=window_count,
                expected_count=expected,
                expected_std=expected_std,
                deviation_sigma=deviation_sigma,
                score=min(deviation_sigma / (self.spike_threshold * 2), 1.0),
                window_minutes=self.window_minutes,
                description=f"Frequency spike: {window_count} occurrences (expected ~{expected:.1f})",
            )

        # Check for drop (only if we expect significant occurrences)
        if (
            deviation < 0
            and deviation_sigma > self.drop_threshold
            and expected > 5
        ):
            return FrequencyAnomaly(
                pattern_id=pattern_id,
                pattern_template=pattern_template,
                anomaly_type=FrequencyAnomalyType.DROP,
                timestamp=timestamp,
                observed_count=window_count,
                expected_count=expected,
                expected_std=expected_std,
                deviation_sigma=deviation_sigma,
                score=min(deviation_sigma / (self.drop_threshold * 2), 1.0),
                window_minutes=self.window_minutes,
                description=f"Frequency drop: {window_count} occurrences (expected ~{expected:.1f})",
            )

        return None

    def _check_timing_anomaly(
        self,
        pattern_id: str,
        timestamp: datetime,
        pattern_template: str,
        baseline: PatternFrequencyBaseline,
    ) -> Optional[FrequencyAnomaly]:
        """Check for patterns occurring at unusual times."""
        hour = timestamp.hour
        day = timestamp.weekday()

        hourly_mean = baseline.hourly_means[hour]
        daily_mean = baseline.daily_means[day]

        # Pattern should be rare at this time
        if (
            hourly_mean < baseline.mean_per_minute * 0.1
            and baseline.mean_per_minute > 1
        ):
            return FrequencyAnomaly(
                pattern_id=pattern_id,
                pattern_template=pattern_template,
                anomaly_type=FrequencyAnomalyType.UNUSUAL_TIME,
                timestamp=timestamp,
                observed_count=1,
                expected_count=hourly_mean,
                expected_std=baseline.hourly_stds[hour],
                deviation_sigma=3.0,
                score=0.7,
                window_minutes=self.window_minutes,
                description=f"Pattern at unusual time: hour {hour} typically has low activity",
                context={"hour": hour, "typical_rate": hourly_mean},
            )

        return None

    def _get_window_start(self, timestamp: datetime) -> datetime:
        """Get the start of the time window containing timestamp."""
        minute = (timestamp.minute // self.window_minutes) * self.window_minutes
        return timestamp.replace(minute=minute, second=0, microsecond=0)

    def _cleanup_old_data(self, pattern_id: str) -> None:
        """Remove old occurrence data."""
        cutoff = datetime.utcnow() - timedelta(hours=self.baseline_hours * 2)
        self._occurrences[pattern_id] = [
            ts for ts in self._occurrences[pattern_id]
            if ts >= cutoff
        ]

        # Clean window counts
        window_cutoff = datetime.utcnow() - timedelta(hours=1)
        self._window_counts[pattern_id] = {
            k: v for k, v in self._window_counts[pattern_id].items()
            if k >= window_cutoff
        }
