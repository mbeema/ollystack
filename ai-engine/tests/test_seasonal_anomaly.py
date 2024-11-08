"""
Tests for Seasonal Anomaly Detection
"""

import pytest
import numpy as np
from datetime import datetime, timedelta

from ollystack_ai.anomaly.seasonal import (
    SeasonalDecomposer,
    FourierAnalyzer,
    SeasonalAnomalyDetector,
    SeasonalBaseline,
    HolidayCalendar,
    AdaptiveThresholdCalculator,
    detect_seasonality_type,
    SeasonalPeriod,
)


class TestSeasonalDecomposer:
    """Tests for STL-like decomposition."""

    def test_decompose_with_clear_seasonality(self):
        """Test decomposition of data with clear daily pattern."""
        # Create data with daily seasonality (24-hour period)
        n_days = 14
        hours = np.arange(n_days * 24)

        # Trend: slight upward
        trend = hours * 0.01

        # Seasonal: peak at noon, low at midnight
        seasonal = 10 * np.sin(2 * np.pi * hours / 24)

        # Noise
        np.random.seed(42)
        noise = np.random.normal(0, 1, len(hours))

        values = trend + seasonal + noise + 50  # Baseline of 50

        decomposer = SeasonalDecomposer(period=24)
        result = decomposer.decompose(values)

        # Check that seasonal strength is high
        assert result.seasonal_strength > 0.5
        assert result.period == 24
        assert len(result.trend) == len(values)
        assert len(result.seasonal) == len(values)
        assert len(result.residual) == len(values)

    def test_decompose_no_seasonality(self):
        """Test decomposition of random data."""
        np.random.seed(42)
        values = np.random.normal(100, 10, 168)

        decomposer = SeasonalDecomposer(period=24)
        result = decomposer.decompose(values)

        # Seasonal strength should be low for random data
        assert result.seasonal_strength < 0.3

    def test_decompose_insufficient_data(self):
        """Test decomposition with insufficient data."""
        values = np.array([1, 2, 3, 4, 5])

        decomposer = SeasonalDecomposer(period=24)
        result = decomposer.decompose(values)

        # Should return zeros for seasonal
        assert result.seasonal_strength == 0.0


class TestFourierAnalyzer:
    """Tests for Fourier-based periodicity detection."""

    def test_detect_24hour_period(self):
        """Test detection of 24-hour periodicity."""
        hours = np.arange(168)  # 1 week
        values = 10 * np.sin(2 * np.pi * hours / 24) + 50

        analyzer = FourierAnalyzer()
        periods = analyzer.detect_periods(values, top_k=3)

        # Should detect period around 24
        assert len(periods) > 0
        detected_period, strength = periods[0]
        assert 22 <= detected_period <= 26  # Allow some tolerance

    def test_detect_weekly_period(self):
        """Test detection of weekly periodicity."""
        hours = np.arange(672)  # 4 weeks
        values = 10 * np.sin(2 * np.pi * hours / 168) + 50

        analyzer = FourierAnalyzer()
        periods = analyzer.detect_periods(values, top_k=3)

        # Should detect period around 168
        assert len(periods) > 0
        # Check if weekly period is in top periods
        weekly_found = any(150 <= p <= 190 for p, _ in periods)
        assert weekly_found

    def test_period_strength(self):
        """Test calculation of period strength."""
        hours = np.arange(168)
        values = 10 * np.sin(2 * np.pi * hours / 24) + 50

        analyzer = FourierAnalyzer()
        strength = analyzer.get_period_strength(values, 24)

        # Should have high strength for matching period
        assert strength > 0.5


class TestSeasonalAnomalyDetector:
    """Tests for the main seasonal anomaly detector."""

    @pytest.fixture
    def sample_baseline(self):
        """Create a sample baseline for testing."""
        return SeasonalBaseline(
            metric_name="cpu_usage",
            service_name="api-server",
            hourly_means=[50 + 20 * np.sin(2 * np.pi * h / 24) for h in range(24)],
            hourly_stds=[5.0] * 24,
            daily_means=[50, 52, 54, 53, 51, 45, 43],  # Mon-Sun
            daily_stds=[5.0] * 7,
            weekly_pattern=None,
            weekly_stds=None,
            global_mean=50.0,
            global_std=10.0,
            sample_count=1000,
            last_updated=datetime.utcnow(),
        )

    def test_detect_normal_value(self, sample_baseline):
        """Test that normal values are not flagged."""
        detector = SeasonalAnomalyDetector()

        # Monday at noon - expected to be around 50 + 20*sin(pi) = 50
        ts = datetime(2024, 1, 8, 12, 0, 0)  # Monday noon
        value = 52  # Close to expected

        result = detector.detect_anomaly(
            value=value,
            timestamp=ts,
            baseline=sample_baseline,
        )

        assert not result.is_anomaly
        assert result.score < 0.7

    def test_detect_anomalous_value(self, sample_baseline):
        """Test that anomalous values are flagged."""
        detector = SeasonalAnomalyDetector()

        # Monday at noon - expected ~50, give value way off
        ts = datetime(2024, 1, 8, 12, 0, 0)
        value = 100  # Way above expected

        result = detector.detect_anomaly(
            value=value,
            timestamp=ts,
            baseline=sample_baseline,
        )

        assert result.is_anomaly
        assert result.score > 0.7
        assert result.deviation_sigma > 3.0

    def test_seasonal_context(self, sample_baseline):
        """Test that seasonal context is properly included."""
        detector = SeasonalAnomalyDetector()

        ts = datetime(2024, 1, 8, 14, 0, 0)  # Monday 2pm

        result = detector.detect_anomaly(
            value=55,
            timestamp=ts,
            baseline=sample_baseline,
        )

        assert result.seasonal_context["hour"] == 14
        assert result.seasonal_context["day_of_week"] == 0  # Monday

    def test_build_baseline(self):
        """Test baseline construction from data."""
        detector = SeasonalAnomalyDetector()

        # Generate synthetic data with patterns
        n = 168 * 2  # 2 weeks
        timestamps = [datetime(2024, 1, 1) + timedelta(hours=i) for i in range(n)]
        values = np.array([
            50 + 10 * np.sin(2 * np.pi * i / 24)  # Daily pattern
            + np.random.normal(0, 2)
            for i in range(n)
        ])

        baseline = detector.build_baseline(
            values=values,
            timestamps=timestamps,
            metric_name="test_metric",
            service_name="test_service",
        )

        assert baseline.sample_count == n
        assert len(baseline.hourly_means) == 24
        assert len(baseline.daily_means) == 7
        assert baseline.global_mean > 0

    def test_batch_detection(self, sample_baseline):
        """Test batch anomaly detection."""
        detector = SeasonalAnomalyDetector()

        n = 48
        timestamps = [datetime(2024, 1, 8) + timedelta(hours=i) for i in range(n)]

        # Mostly normal values with one anomaly
        values = np.array([55 + np.random.normal(0, 3) for _ in range(n)])
        values[24] = 150  # Inject anomaly at hour 24

        results = detector.detect_batch(
            values=values,
            timestamps=timestamps,
            baseline=sample_baseline,
        )

        assert len(results) == n
        # The injected anomaly should be detected
        assert results[24].is_anomaly


class TestHolidayCalendar:
    """Tests for holiday calendar functionality."""

    def test_is_fixed_holiday(self):
        """Test detection of fixed holidays."""
        calendar = HolidayCalendar()

        # Christmas
        is_holiday, name = calendar.is_holiday(datetime(2024, 12, 25, 14, 0))
        assert is_holiday
        assert "12/25" in name

    def test_custom_event(self):
        """Test adding and detecting custom events."""
        calendar = HolidayCalendar()

        start = datetime(2024, 3, 1)
        end = datetime(2024, 3, 3)
        calendar.add_event(start, end, "Company Offsite")

        # During event
        is_holiday, name = calendar.is_holiday(datetime(2024, 3, 2, 10, 0))
        assert is_holiday
        assert name == "Company Offsite"

        # Outside event
        is_holiday, name = calendar.is_holiday(datetime(2024, 3, 5, 10, 0))
        assert not is_holiday

    def test_adjustment_factor(self):
        """Test threshold adjustment factor."""
        calendar = HolidayCalendar()

        # Holiday should have higher factor
        factor = calendar.get_adjustment_factor(datetime(2024, 12, 25, 12, 0))
        assert factor == 1.5

        # Weekend should have moderate factor
        factor = calendar.get_adjustment_factor(datetime(2024, 1, 6, 12, 0))  # Saturday
        assert factor == 1.2

        # Normal weekday
        factor = calendar.get_adjustment_factor(datetime(2024, 1, 8, 12, 0))  # Monday
        assert factor == 1.0


class TestAdaptiveThresholdCalculator:
    """Tests for adaptive threshold calculation."""

    @pytest.fixture
    def sample_baseline(self):
        return SeasonalBaseline(
            metric_name="test",
            service_name="test",
            hourly_means=[50.0] * 24,
            hourly_stds=[5.0] * 24,
            daily_means=[50.0] * 7,
            daily_stds=[5.0] * 7,
            weekly_pattern=None,
            weekly_stds=None,
            global_mean=50.0,
            global_std=5.0,
            sample_count=1000,
            last_updated=datetime.utcnow(),
        )

    def test_base_threshold(self, sample_baseline):
        """Test that base threshold is returned for normal conditions."""
        calculator = AdaptiveThresholdCalculator(base_threshold=3.0)

        # Normal variance data
        recent = np.array([50 + np.random.normal(0, 5) for _ in range(24)])

        threshold = calculator.calculate_threshold(
            recent_values=recent,
            baseline=sample_baseline,
            timestamp=datetime(2024, 1, 8, 12, 0),
        )

        # Should be close to base threshold
        assert 2.5 < threshold < 4.0

    def test_high_volatility_adjustment(self, sample_baseline):
        """Test that threshold increases with volatility."""
        calculator = AdaptiveThresholdCalculator(base_threshold=3.0)

        # High variance data
        recent = np.array([50 + np.random.normal(0, 20) for _ in range(24)])

        threshold = calculator.calculate_threshold(
            recent_values=recent,
            baseline=sample_baseline,
            timestamp=datetime(2024, 1, 8, 12, 0),
        )

        # Should be higher than base threshold
        assert threshold > 3.0


class TestDetectSeasonalityType:
    """Tests for auto-detection of seasonality types."""

    def test_detect_daily_pattern(self):
        """Test detection of daily seasonality."""
        hours = np.arange(168)
        values = 10 * np.sin(2 * np.pi * hours / 24) + 50

        detected = detect_seasonality_type(values, sample_interval_seconds=3600)

        assert SeasonalPeriod.DAILY in detected

    def test_detect_no_pattern(self):
        """Test with random data."""
        np.random.seed(42)
        values = np.random.normal(50, 5, 168)

        detected = detect_seasonality_type(values, sample_interval_seconds=3600)

        # Should detect minimal or no patterns
        assert len(detected) <= 1


class TestIntegration:
    """Integration tests for the full seasonal detection pipeline."""

    def test_full_pipeline(self):
        """Test the complete detection pipeline."""
        # Generate realistic data
        n_weeks = 4
        n_points = n_weeks * 168
        timestamps = [datetime(2024, 1, 1) + timedelta(hours=i) for i in range(n_points)]

        # Create pattern: higher during business hours, lower on weekends
        values = []
        for i, ts in enumerate(timestamps):
            base = 50

            # Hourly pattern (business hours peak)
            hour = ts.hour
            if 9 <= hour <= 17:
                hourly_effect = 20
            else:
                hourly_effect = -10

            # Daily pattern (weekends lower)
            if ts.weekday() >= 5:
                daily_effect = -15
            else:
                daily_effect = 5

            noise = np.random.normal(0, 3)
            values.append(base + hourly_effect + daily_effect + noise)

        values = np.array(values)

        # Build baseline from first 3 weeks
        detector = SeasonalAnomalyDetector()
        baseline = detector.build_baseline(
            values=values[:504],  # 3 weeks
            timestamps=timestamps[:504],
            metric_name="requests_per_sec",
            service_name="web-app",
        )

        # Inject anomaly in week 4
        anomaly_idx = 504 + 24  # Tuesday noon in week 4
        values[anomaly_idx] = 150  # Way above expected

        # Detect in week 4
        results = detector.detect_batch(
            values=values[504:],
            timestamps=timestamps[504:],
            baseline=baseline,
        )

        # Should detect the injected anomaly
        anomaly_result = results[24]  # Index 24 in the test data
        assert anomaly_result.is_anomaly
        assert anomaly_result.score > 0.8

    def test_with_decomposition(self):
        """Test anomaly detection with STL decomposition enabled."""
        n = 168 * 2
        timestamps = [datetime(2024, 1, 1) + timedelta(hours=i) for i in range(n)]

        # Create data with trend and seasonality
        values = np.array([
            50 + 0.05 * i  # Upward trend
            + 15 * np.sin(2 * np.pi * i / 24)  # Daily seasonality
            + np.random.normal(0, 2)
            for i in range(n)
        ])

        detector = SeasonalAnomalyDetector()
        baseline = detector.build_baseline(
            values=values[:168],
            timestamps=timestamps[:168],
        )

        # Test with recent context for decomposition
        result = detector.detect_anomaly(
            value=100,  # Anomalous
            timestamp=timestamps[200],
            baseline=baseline,
            use_decomposition=True,
            recent_values=values[150:200],
        )

        assert result.is_anomaly
        assert len(result.contributing_factors) > 0


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
