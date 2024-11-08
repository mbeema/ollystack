"""
Log Anomaly Detection API

Provides endpoints for log pattern extraction, anomaly detection,
and analysis.
"""

import logging
from typing import Optional
from datetime import datetime, timedelta

from fastapi import APIRouter, HTTPException, Request, Depends
from pydantic import BaseModel, Field

from ollystack_ai.log_anomaly.detector import LogAnomalyDetector, AnomalyType
from ollystack_ai.log_anomaly.pattern_extractor import LogPatternExtractor

logger = logging.getLogger(__name__)
router = APIRouter()

# Store detectors per service
_detectors: dict[str, LogAnomalyDetector] = {}


def get_detector(service_name: str = "default") -> LogAnomalyDetector:
    """Get or create a detector for a service."""
    if service_name not in _detectors:
        _detectors[service_name] = LogAnomalyDetector(service_name=service_name)
    return _detectors[service_name]


# -----------------------------------------------------------------------------
# Request/Response Models
# -----------------------------------------------------------------------------


class LogEntry(BaseModel):
    """A single log entry."""

    message: str = Field(..., description="The log message")
    timestamp: Optional[datetime] = Field(None, description="Log timestamp")
    severity: str = Field("INFO", description="Log severity level")
    service: str = Field("default", description="Service name")
    session_id: str = Field("default", description="Session/request ID")
    metadata: Optional[dict] = Field(None, description="Additional metadata")


class AnalyzeRequest(BaseModel):
    """Request for log analysis."""

    logs: list[LogEntry] = Field(..., description="Logs to analyze")
    session_id: str = Field("default", description="Session identifier")


class LogAnomalyResponse(BaseModel):
    """Response for a detected anomaly."""

    anomaly_id: str
    anomaly_type: str
    timestamp: str
    service_name: str
    log_message: str
    pattern_id: Optional[str]
    pattern_template: Optional[str]
    score: float
    severity: str
    description: str
    details: dict


class AnalyzeResponse(BaseModel):
    """Response for log analysis."""

    anomalies: list[LogAnomalyResponse]
    total_anomalies: int
    patterns_analyzed: int
    new_patterns: int
    summary: str


class PatternResponse(BaseModel):
    """Response for a log pattern."""

    pattern_id: str
    template: str
    count: int
    first_seen: str
    last_seen: str
    sample_logs: list[str]
    severity_distribution: dict


class StatisticsResponse(BaseModel):
    """Response for detection statistics."""

    total_logs_analyzed: int
    total_anomalies_detected: int
    anomaly_rate: float
    unique_patterns: int
    compression_ratio: float


class SequenceRuleRequest(BaseModel):
    """Request to add a sequence rule."""

    from_template: str = Field(..., description="Template of triggering pattern")
    valid_next_templates: list[str] = Field(..., description="Valid next patterns")


# -----------------------------------------------------------------------------
# Endpoints
# -----------------------------------------------------------------------------


@router.post("/analyze", response_model=AnalyzeResponse)
async def analyze_logs(request: AnalyzeRequest) -> AnalyzeResponse:
    """
    Analyze a batch of logs for anomalies.

    Detects various types of anomalies:
    - New/rare patterns
    - Frequency spikes or drops
    - Unusual sequences
    - Sensitive data exposure
    """
    try:
        # Group logs by service
        by_service: dict[str, list[dict]] = {}
        for log in request.logs:
            service = log.service
            if service not in by_service:
                by_service[service] = []
            by_service[service].append({
                "message": log.message,
                "timestamp": log.timestamp,
                "severity": log.severity,
            })

        # Analyze each service
        all_anomalies = []
        total_patterns = 0
        total_new = 0

        for service, logs in by_service.items():
            detector = get_detector(service)
            result = detector.analyze_batch(logs, session_id=request.session_id)

            all_anomalies.extend(result.anomalies)
            total_patterns += result.patterns_analyzed
            total_new += result.new_patterns_count

        # Convert to response format
        anomaly_responses = [
            LogAnomalyResponse(
                anomaly_id=a.anomaly_id,
                anomaly_type=a.anomaly_type.value,
                timestamp=a.timestamp.isoformat(),
                service_name=a.service_name,
                log_message=a.log_message[:500],
                pattern_id=a.pattern_id,
                pattern_template=a.pattern_template,
                score=a.score,
                severity=a.severity,
                description=a.description,
                details=a.details,
            )
            for a in all_anomalies
        ]

        # Sort by score
        anomaly_responses.sort(key=lambda a: a.score, reverse=True)

        return AnalyzeResponse(
            anomalies=anomaly_responses,
            total_anomalies=len(anomaly_responses),
            patterns_analyzed=total_patterns,
            new_patterns=total_new,
            summary=_generate_summary(anomaly_responses),
        )

    except Exception as e:
        logger.exception("Log analysis failed")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/analyze/single")
async def analyze_single_log(log: LogEntry) -> dict:
    """
    Analyze a single log entry for anomalies.

    Returns immediate detection results.
    """
    try:
        detector = get_detector(log.service)
        anomalies = detector.analyze(
            log_message=log.message,
            timestamp=log.timestamp,
            severity=log.severity,
            session_id=log.session_id,
            metadata=log.metadata,
        )

        return {
            "anomalies": [a.to_dict() for a in anomalies],
            "anomaly_count": len(anomalies),
            "is_anomalous": len(anomalies) > 0,
            "max_score": max((a.score for a in anomalies), default=0),
        }

    except Exception as e:
        logger.exception("Single log analysis failed")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/patterns/{service}")
async def get_patterns(
    service: str,
    top_n: int = 50,
    sort_by: str = "count",
) -> dict:
    """
    Get extracted log patterns for a service.

    Patterns are log message templates with variables replaced by <*>.
    """
    try:
        detector = get_detector(service)
        patterns = detector.pattern_extractor.get_all_patterns()

        # Sort
        if sort_by == "count":
            patterns.sort(key=lambda p: p.count, reverse=True)
        elif sort_by == "recent":
            patterns.sort(key=lambda p: p.last_seen, reverse=True)
        elif sort_by == "first_seen":
            patterns.sort(key=lambda p: p.first_seen, reverse=True)

        patterns = patterns[:top_n]

        return {
            "service": service,
            "patterns": [p.to_dict() for p in patterns],
            "total_patterns": len(detector.pattern_extractor.get_all_patterns()),
        }

    except Exception as e:
        logger.exception("Failed to get patterns")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/patterns/{service}/new")
async def get_new_patterns(
    service: str,
    hours: int = 24,
) -> dict:
    """
    Get newly discovered log patterns.

    Useful for detecting new types of errors or unexpected behavior.
    """
    try:
        detector = get_detector(service)
        patterns = detector.get_new_patterns(hours=hours)

        return {
            "service": service,
            "hours": hours,
            "new_patterns": patterns,
            "count": len(patterns),
        }

    except Exception as e:
        logger.exception("Failed to get new patterns")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/patterns/{service}/rare")
async def get_rare_patterns(
    service: str,
    threshold: int = 5,
) -> dict:
    """
    Get rare log patterns.

    Rare patterns may indicate unusual events or edge cases.
    """
    try:
        detector = get_detector(service)
        patterns = detector.get_rare_patterns(threshold=threshold)

        return {
            "service": service,
            "threshold": threshold,
            "rare_patterns": patterns,
            "count": len(patterns),
        }

    except Exception as e:
        logger.exception("Failed to get rare patterns")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/patterns/{service}/errors")
async def get_error_patterns(service: str) -> dict:
    """
    Get log patterns associated with errors.

    Returns patterns that frequently appear with ERROR/FATAL severity.
    """
    try:
        detector = get_detector(service)
        patterns = detector.get_error_patterns()

        return {
            "service": service,
            "error_patterns": patterns,
            "count": len(patterns),
        }

    except Exception as e:
        logger.exception("Failed to get error patterns")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/statistics/{service}", response_model=StatisticsResponse)
async def get_statistics(service: str) -> StatisticsResponse:
    """
    Get detection statistics for a service.
    """
    try:
        detector = get_detector(service)
        stats = detector.get_statistics()
        pattern_stats = stats.get("pattern_stats", {})

        return StatisticsResponse(
            total_logs_analyzed=stats.get("total_logs_analyzed", 0),
            total_anomalies_detected=stats.get("total_anomalies_detected", 0),
            anomaly_rate=stats.get("anomaly_rate", 0),
            unique_patterns=pattern_stats.get("unique_patterns", 0),
            compression_ratio=pattern_stats.get("compression_ratio", 0),
        )

    except Exception as e:
        logger.exception("Failed to get statistics")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/frequency/{service}/{pattern_id}")
async def get_frequency_stats(
    service: str,
    pattern_id: str,
    window_minutes: int = 60,
) -> dict:
    """
    Get frequency statistics for a specific pattern.
    """
    try:
        detector = get_detector(service)
        stats = detector.frequency_analyzer.get_frequency_stats(
            pattern_id=pattern_id,
            window_minutes=window_minutes,
        )

        return {
            "service": service,
            "pattern_id": pattern_id,
            **stats,
        }

    except Exception as e:
        logger.exception("Failed to get frequency stats")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/sequence/{service}/{session_id}")
async def analyze_session_sequence(
    service: str,
    session_id: str,
) -> dict:
    """
    Analyze the sequence of events in a session.

    Returns sequence statistics and any detected sequence anomalies.
    """
    try:
        detector = get_detector(service)
        analysis = detector.sequence_analyzer.analyze_session(session_id)

        return {
            "service": service,
            "session_id": session_id,
            **analysis,
        }

    except Exception as e:
        logger.exception("Failed to analyze session sequence")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/transitions/{service}")
async def get_transition_matrix(service: str) -> dict:
    """
    Get the pattern transition probability matrix.

    Shows how likely pattern B is to follow pattern A.
    """
    try:
        detector = get_detector(service)
        matrix = detector.sequence_analyzer.get_transition_matrix()

        # Get pattern templates for readability
        pattern_templates = {}
        for pattern in detector.pattern_extractor.get_all_patterns():
            pattern_templates[pattern.pattern_id] = pattern.template[:50]

        return {
            "service": service,
            "transition_matrix": matrix,
            "pattern_templates": pattern_templates,
        }

    except Exception as e:
        logger.exception("Failed to get transition matrix")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/likely-next/{service}/{pattern_id}")
async def get_likely_next_patterns(
    service: str,
    pattern_id: str,
    top_k: int = 5,
) -> dict:
    """
    Get the most likely next patterns after a given pattern.
    """
    try:
        detector = get_detector(service)
        likely = detector.sequence_analyzer.get_likely_next(pattern_id, top_k)

        # Enrich with templates
        results = []
        for pid, prob in likely:
            pattern = detector.pattern_extractor.get_pattern(pid)
            results.append({
                "pattern_id": pid,
                "template": pattern.template if pattern else "Unknown",
                "probability": prob,
            })

        return {
            "service": service,
            "pattern_id": pattern_id,
            "likely_next": results,
        }

    except Exception as e:
        logger.exception("Failed to get likely next patterns")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/rules/sequence")
async def add_sequence_rule(
    service: str,
    request: SequenceRuleRequest,
) -> dict:
    """
    Add a sequence rule for state machine validation.

    This allows you to define expected event sequences and detect violations.
    """
    try:
        detector = get_detector(service)
        detector.add_sequence_rule(
            from_pattern_template=request.from_template,
            valid_next_templates=request.valid_next_templates,
        )

        return {
            "status": "success",
            "message": f"Added sequence rule for pattern: {request.from_template[:50]}",
        }

    except Exception as e:
        logger.exception("Failed to add sequence rule")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/baselines/{service}/update")
async def update_baselines(service: str) -> dict:
    """
    Update frequency baselines for all patterns.

    Should be called periodically to adapt to changing patterns.
    """
    try:
        detector = get_detector(service)
        detector.update_baselines()

        return {
            "status": "success",
            "message": "Baselines updated",
            "patterns_updated": len(detector.pattern_extractor.get_all_patterns()),
        }

    except Exception as e:
        logger.exception("Failed to update baselines")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/anomaly-types")
async def get_anomaly_types() -> dict:
    """
    Get all supported anomaly types and their descriptions.
    """
    return {
        "anomaly_types": [
            {
                "type": "new_pattern",
                "description": "A log pattern that has never been seen before",
                "severity": "medium",
            },
            {
                "type": "rare_pattern",
                "description": "A log pattern that occurs very infrequently",
                "severity": "low",
            },
            {
                "type": "error_pattern",
                "description": "A pattern frequently associated with errors",
                "severity": "high",
            },
            {
                "type": "frequency_spike",
                "description": "Sudden increase in pattern frequency",
                "severity": "medium",
            },
            {
                "type": "frequency_drop",
                "description": "Sudden decrease in expected pattern frequency",
                "severity": "medium",
            },
            {
                "type": "burst",
                "description": "Rapid succession of the same pattern",
                "severity": "high",
            },
            {
                "type": "missing_pattern",
                "description": "Expected pattern not seen within time window",
                "severity": "medium",
            },
            {
                "type": "unexpected_sequence",
                "description": "Unusual sequence of log events",
                "severity": "medium",
            },
            {
                "type": "missing_followup",
                "description": "Expected follow-up event not observed",
                "severity": "medium",
            },
            {
                "type": "state_violation",
                "description": "Violation of defined state machine rules",
                "severity": "high",
            },
            {
                "type": "sensitive_data",
                "description": "Sensitive data detected in logs",
                "severity": "critical",
            },
        ]
    }


def _generate_summary(anomalies: list[LogAnomalyResponse]) -> str:
    """Generate a summary of detected anomalies."""
    if not anomalies:
        return "No anomalies detected"

    type_counts = {}
    for a in anomalies:
        type_counts[a.anomaly_type] = type_counts.get(a.anomaly_type, 0) + 1

    parts = [f"{count} {atype}" for atype, count in type_counts.items()]

    critical = [a for a in anomalies if a.score > 0.8]
    summary = f"Detected {len(anomalies)} anomalies: {', '.join(parts)}"

    if critical:
        summary += f". {len(critical)} require immediate attention."

    return summary
