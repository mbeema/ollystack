"""
Anomaly Detection API

Provides endpoints for anomaly detection and scoring
on observability data, including seasonal pattern detection.
"""

import logging
from typing import Optional
from datetime import datetime

from fastapi import APIRouter, HTTPException, Request, Depends
from pydantic import BaseModel, Field

from ollystack_ai.anomaly.detector import AnomalyDetector

logger = logging.getLogger(__name__)
router = APIRouter()


# -----------------------------------------------------------------------------
# Request/Response Models
# -----------------------------------------------------------------------------


class DetectRequest(BaseModel):
    """Request for anomaly detection."""

    service: Optional[str] = Field(None, description="Service to analyze")
    metric: Optional[str] = Field(None, description="Specific metric to analyze")
    time_range: str = Field("1h", description="Time range for analysis")
    sensitivity: float = Field(0.8, description="Detection sensitivity (0-1)")


class AnomalyResponse(BaseModel):
    """Anomaly detection response."""

    anomalies: list[dict]
    total_count: int
    time_range: str
    summary: str


class ScoreRequest(BaseModel):
    """Request for anomaly scoring."""

    metric_name: str
    values: list[float]
    timestamps: Optional[list[datetime]] = None


class ScoreResponse(BaseModel):
    """Anomaly scoring response."""

    scores: list[float]
    anomaly_indices: list[int]
    threshold: float


def get_detector(request: Request) -> AnomalyDetector:
    """Dependency to get anomaly detector."""
    return AnomalyDetector(
        storage_service=request.app.state.storage_service,
        cache_service=request.app.state.cache_service,
    )


@router.post("/detect", response_model=AnomalyResponse)
async def detect_anomalies(
    request: DetectRequest,
    detector: AnomalyDetector = Depends(get_detector),
) -> AnomalyResponse:
    """
    Detect anomalies in observability data.

    Analyzes metrics, traces, and logs to identify anomalous patterns
    using statistical and ML-based methods.
    """
    try:
        result = await detector.detect(
            service=request.service,
            metric=request.metric,
            time_range=request.time_range,
            sensitivity=request.sensitivity,
        )

        return AnomalyResponse(
            anomalies=result.anomalies,
            total_count=len(result.anomalies),
            time_range=request.time_range,
            summary=result.summary,
        )

    except Exception as e:
        logger.exception("Anomaly detection failed")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/score", response_model=ScoreResponse)
async def score_values(
    request: ScoreRequest,
    detector: AnomalyDetector = Depends(get_detector),
) -> ScoreResponse:
    """
    Score a series of values for anomalies.

    Uses statistical methods to assign anomaly scores
    to each value in the series.
    """
    try:
        result = await detector.score(
            metric_name=request.metric_name,
            values=request.values,
            timestamps=request.timestamps,
        )

        return ScoreResponse(
            scores=result.scores,
            anomaly_indices=result.anomaly_indices,
            threshold=result.threshold,
        )

    except Exception as e:
        logger.exception("Anomaly scoring failed")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/baseline/{service}/{metric}")
async def get_baseline(
    service: str,
    metric: str,
    detector: AnomalyDetector = Depends(get_detector),
) -> dict:
    """
    Get baseline statistics for a metric.

    Returns mean, standard deviation, and percentiles
    calculated from historical data.
    """
    baseline = await detector.get_baseline(service, metric)
    return {
        "service": service,
        "metric": metric,
        "mean": baseline.mean,
        "std": baseline.std,
        "p50": baseline.p50,
        "p90": baseline.p90,
        "p99": baseline.p99,
        "sample_count": baseline.sample_count,
        "window": baseline.window,
    }


@router.get("/trends/{service}")
async def get_anomaly_trends(
    service: str,
    time_range: str = "24h",
    detector: AnomalyDetector = Depends(get_detector),
) -> dict:
    """
    Get anomaly trends for a service.

    Returns count of anomalies over time and by type.
    """
    trends = await detector.get_trends(service, time_range)
    return {
        "service": service,
        "time_range": time_range,
        "by_hour": trends.by_hour,
        "by_type": trends.by_type,
        "total_anomalies": trends.total,
    }


# -----------------------------------------------------------------------------
# Seasonal Detection Endpoints
# -----------------------------------------------------------------------------


class SeasonalDetectRequest(BaseModel):
    """Request for seasonal anomaly detection."""

    service: str = Field(..., description="Service to analyze")
    metric: str = Field(..., description="Metric to analyze")
    time_range: str = Field("1h", description="Time range for detection")
    baseline_window: str = Field("7d", description="Historical window for baseline")
    sensitivity: float = Field(0.8, description="Detection sensitivity (0-1)")


class SeasonalAnomalyResponse(BaseModel):
    """Seasonal anomaly detection response."""

    anomalies: list[dict]
    total_count: int
    seasonal_patterns: dict
    baseline_info: dict
    summary: str


class SeasonalityAnalysisResponse(BaseModel):
    """Response for seasonality analysis."""

    service: str
    metric: str
    has_hourly: bool
    has_daily: bool
    has_weekly: bool
    hourly_strength: float
    daily_strength: float
    weekly_strength: float
    dominant_period: str
    detected_periods: list[dict]


class SeasonalBaselineResponse(BaseModel):
    """Seasonal baseline response."""

    service: str
    metric: str
    hourly_means: list[float]
    hourly_stds: list[float]
    daily_means: list[float]
    daily_stds: list[float]
    weekly_pattern: Optional[list[float]]
    global_mean: float
    global_std: float
    sample_count: int
    last_updated: str


class CompareRequest(BaseModel):
    """Request to compare a value to baseline."""

    service: str
    metric: str
    value: float
    timestamp: Optional[datetime] = None


class HolidayRequest(BaseModel):
    """Request to add a holiday/event."""

    start: datetime
    end: datetime
    name: str


@router.post("/seasonal/detect", response_model=SeasonalAnomalyResponse)
async def detect_seasonal_anomalies(
    request: SeasonalDetectRequest,
    detector: AnomalyDetector = Depends(get_detector),
) -> SeasonalAnomalyResponse:
    """
    Detect anomalies using seasonal awareness.

    Compares current values against seasonal baselines (same hour of day,
    day of week, etc.) rather than simple statistical averages.

    This method is more accurate for metrics with predictable patterns like:
    - Traffic that peaks during business hours
    - Lower activity on weekends
    - Weekly or monthly cycles
    """
    try:
        result = await detector.detect_seasonal(
            service=request.service,
            metric=request.metric,
            time_range=request.time_range,
            baseline_window=request.baseline_window,
            sensitivity=request.sensitivity,
        )

        return SeasonalAnomalyResponse(
            anomalies=result.anomalies,
            total_count=len(result.anomalies),
            seasonal_patterns=result.seasonal_patterns,
            baseline_info=result.baseline_info,
            summary=result.summary,
        )

    except Exception as e:
        logger.exception("Seasonal anomaly detection failed")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/seasonal/analyze/{service}/{metric}", response_model=SeasonalityAnalysisResponse)
async def analyze_seasonality(
    service: str,
    metric: str,
    time_range: str = "7d",
    detector: AnomalyDetector = Depends(get_detector),
) -> SeasonalityAnalysisResponse:
    """
    Analyze what seasonal patterns exist in a metric.

    Returns information about detected hourly, daily, and weekly patterns,
    including their relative strengths. Use this to understand whether
    seasonal detection is appropriate for a given metric.
    """
    try:
        info = await detector.analyze_seasonality(service, metric, time_range)

        return SeasonalityAnalysisResponse(
            service=service,
            metric=metric,
            has_hourly=info.has_hourly,
            has_daily=info.has_daily,
            has_weekly=info.has_weekly,
            hourly_strength=info.hourly_strength,
            daily_strength=info.daily_strength,
            weekly_strength=info.weekly_strength,
            dominant_period=info.dominant_period,
            detected_periods=[
                {"period": p, "strength": s} for p, s in info.detected_periods
            ],
        )

    except Exception as e:
        logger.exception("Seasonality analysis failed")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/seasonal/baseline/{service}/{metric}", response_model=SeasonalBaselineResponse)
async def get_seasonal_baseline(
    service: str,
    metric: str,
    window: str = "7d",
    detector: AnomalyDetector = Depends(get_detector),
) -> SeasonalBaselineResponse:
    """
    Get seasonal baseline for a metric.

    Returns expected values and standard deviations for each:
    - Hour of day (0-23): What to expect at 9am vs 3am
    - Day of week (0-6): What to expect on Monday vs Sunday
    - Combined hour-of-week: Fine-grained seasonal pattern
    """
    try:
        baseline = await detector.get_seasonal_baseline(service, metric, window)

        return SeasonalBaselineResponse(
            service=service,
            metric=metric,
            hourly_means=baseline.hourly_means,
            hourly_stds=baseline.hourly_stds,
            daily_means=baseline.daily_means,
            daily_stds=baseline.daily_stds,
            weekly_pattern=baseline.weekly_pattern,
            global_mean=baseline.global_mean,
            global_std=baseline.global_std,
            sample_count=baseline.sample_count,
            last_updated=baseline.last_updated.isoformat(),
        )

    except Exception as e:
        logger.exception("Failed to get seasonal baseline")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/seasonal/decompose/{service}/{metric}")
async def decompose_metric(
    service: str,
    metric: str,
    time_range: str = "7d",
    detector: AnomalyDetector = Depends(get_detector),
) -> dict:
    """
    Decompose a metric into trend, seasonal, and residual components.

    Uses STL (Seasonal-Trend decomposition using LOESS) to separate:
    - Trend: Long-term direction of the metric
    - Seasonal: Repeating patterns (daily, weekly)
    - Residual: Noise after removing trend and seasonality

    Anomalies are best detected in the residual component.
    """
    try:
        result = await detector.decompose_metric(service, metric, time_range)
        return {
            "service": service,
            "metric": metric,
            "time_range": time_range,
            **result,
        }

    except Exception as e:
        logger.exception("Metric decomposition failed")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/seasonal/compare")
async def compare_to_baseline(
    request: CompareRequest,
    detector: AnomalyDetector = Depends(get_detector),
) -> dict:
    """
    Compare a single value to its seasonal baseline.

    Useful for real-time anomaly checking where you have a current
    value and want to know if it's anomalous for this time period.
    """
    try:
        result = await detector.compare_to_baseline(
            service=request.service,
            metric=request.metric,
            current_value=request.value,
            timestamp=request.timestamp,
        )
        return {
            "service": request.service,
            "metric": request.metric,
            **result,
        }

    except Exception as e:
        logger.exception("Baseline comparison failed")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/seasonal/holidays")
async def add_holiday(
    request: HolidayRequest,
    detector: AnomalyDetector = Depends(get_detector),
) -> dict:
    """
    Add a holiday or special event to the calendar.

    Holidays and special events are treated differently during
    anomaly detection - thresholds are more lenient since normal
    patterns don't apply.

    Examples:
    - Company holidays
    - Marketing campaigns
    - Known maintenance windows
    - Product launches
    """
    try:
        await detector.add_holiday(request.start, request.end, request.name)
        return {
            "status": "success",
            "message": f"Added holiday: {request.name}",
            "start": request.start.isoformat(),
            "end": request.end.isoformat(),
        }

    except Exception as e:
        logger.exception("Failed to add holiday")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/seasonal/expected/{service}/{metric}")
async def get_expected_value(
    service: str,
    metric: str,
    hour: Optional[int] = None,
    day_of_week: Optional[int] = None,
    detector: AnomalyDetector = Depends(get_detector),
) -> dict:
    """
    Get expected value for a metric at a specific time.

    If hour/day_of_week not provided, uses current time.
    Useful for understanding what "normal" looks like for a metric.
    """
    try:
        now = datetime.utcnow()
        if hour is None:
            hour = now.hour
        if day_of_week is None:
            day_of_week = now.weekday()

        baseline = await detector.get_seasonal_baseline(service, metric, "7d")

        hourly_expected = baseline.hourly_means[hour]
        hourly_std = baseline.hourly_stds[hour]
        daily_expected = baseline.daily_means[day_of_week]
        daily_std = baseline.daily_stds[day_of_week]

        # Weighted combination
        expected = 0.6 * hourly_expected + 0.4 * daily_expected
        expected_std = (0.6 * hourly_std**2 + 0.4 * daily_std**2) ** 0.5

        day_names = [
            "Monday", "Tuesday", "Wednesday", "Thursday",
            "Friday", "Saturday", "Sunday"
        ]

        return {
            "service": service,
            "metric": metric,
            "hour": hour,
            "day_of_week": day_of_week,
            "day_name": day_names[day_of_week],
            "expected_value": expected,
            "expected_std": expected_std,
            "confidence_interval": {
                "lower": expected - 2 * expected_std,
                "upper": expected + 2 * expected_std,
            },
            "hourly_expected": hourly_expected,
            "daily_expected": daily_expected,
        }

    except Exception as e:
        logger.exception("Failed to get expected value")
        raise HTTPException(status_code=500, detail=str(e))
