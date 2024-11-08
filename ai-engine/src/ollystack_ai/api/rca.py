"""
Root Cause Analysis (RCA) API

Provides AI-powered root cause analysis for anomalies, errors,
and performance issues in the observability data.
"""

import logging
from typing import Optional
from datetime import datetime

from fastapi import APIRouter, HTTPException, Request, Depends
from pydantic import BaseModel, Field

from ollystack_ai.rca.analyzer import RCAAnalyzer
from ollystack_ai.rca.models import (
    RCARequest,
    RCAResult,
    ContributingFactor,
    Evidence,
    Recommendation,
)

logger = logging.getLogger(__name__)
router = APIRouter()


class AnalyzeRequest(BaseModel):
    """Request for root cause analysis."""

    anomaly_id: Optional[str] = Field(None, description="Anomaly ID to analyze")
    trace_id: Optional[str] = Field(None, description="Trace ID to analyze")
    service: Optional[str] = Field(None, description="Service to analyze")
    symptom: Optional[str] = Field(None, description="Description of the symptom")
    time_start: Optional[datetime] = Field(None, description="Start of incident window")
    time_end: Optional[datetime] = Field(None, description="End of incident window")
    include_recommendations: bool = Field(True, description="Include fix recommendations")


class AnalyzeResponse(BaseModel):
    """Root cause analysis response."""

    summary: str
    confidence: float
    root_causes: list[dict]
    contributing_factors: list[dict]
    evidence: list[dict]
    recommendations: list[dict]
    related_incidents: list[dict]
    timeline: list[dict]


def get_analyzer(request: Request) -> RCAAnalyzer:
    """Dependency to get RCA analyzer."""
    return RCAAnalyzer(
        llm_service=request.app.state.llm_service,
        storage_service=request.app.state.storage_service,
        cache_service=request.app.state.cache_service,
    )


@router.post("/analyze", response_model=AnalyzeResponse)
async def analyze_root_cause(
    request: AnalyzeRequest,
    analyzer: RCAAnalyzer = Depends(get_analyzer),
) -> AnalyzeResponse:
    """
    Perform root cause analysis.

    Analyzes anomalies, errors, or performance issues to identify
    the most likely root cause and contributing factors.

    The analysis includes:
    - Correlation of traces, metrics, and logs
    - Service dependency analysis
    - Temporal pattern detection
    - Similar historical incident matching
    """
    try:
        # Build RCA request
        rca_request = RCARequest(
            anomaly_id=request.anomaly_id,
            trace_id=request.trace_id,
            service=request.service,
            symptom=request.symptom,
            time_window=(request.time_start, request.time_end)
            if request.time_start and request.time_end
            else None,
        )

        # Perform analysis
        result = await analyzer.analyze(rca_request)

        return AnalyzeResponse(
            summary=result.summary,
            confidence=result.confidence,
            root_causes=[rc.to_dict() for rc in result.root_causes],
            contributing_factors=[cf.to_dict() for cf in result.contributing_factors],
            evidence=[e.to_dict() for e in result.evidence],
            recommendations=[r.to_dict() for r in result.recommendations]
            if request.include_recommendations
            else [],
            related_incidents=[ri for ri in result.related_incidents],
            timeline=result.timeline,
        )

    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        logger.exception("Error performing RCA")
        raise HTTPException(status_code=500, detail=f"Analysis failed: {str(e)}")


@router.post("/analyze-trace/{trace_id}")
async def analyze_trace(
    trace_id: str,
    analyzer: RCAAnalyzer = Depends(get_analyzer),
) -> dict:
    """
    Analyze a specific trace for performance issues.

    Returns detailed analysis of where time was spent
    and potential bottlenecks.
    """
    result = await analyzer.analyze_trace(trace_id)
    return {
        "trace_id": trace_id,
        "total_duration_ms": result.total_duration_ms,
        "critical_path": result.critical_path,
        "bottlenecks": result.bottlenecks,
        "anomalous_spans": result.anomalous_spans,
        "suggestions": result.suggestions,
    }


@router.post("/analyze-error-pattern")
async def analyze_error_pattern(
    service: str,
    time_range: str = "1h",
    analyzer: RCAAnalyzer = Depends(get_analyzer),
) -> dict:
    """
    Analyze error patterns for a service.

    Identifies common error types, their frequency,
    and potential causes.
    """
    result = await analyzer.analyze_error_pattern(service, time_range)
    return {
        "service": service,
        "time_range": time_range,
        "error_summary": result.summary,
        "error_clusters": result.clusters,
        "common_causes": result.common_causes,
        "affected_endpoints": result.affected_endpoints,
    }


@router.get("/impact/{service}")
async def analyze_impact(
    service: str,
    analyzer: RCAAnalyzer = Depends(get_analyzer),
) -> dict:
    """
    Analyze the impact of issues in a service.

    Returns which upstream and downstream services
    are affected and to what degree.
    """
    result = await analyzer.analyze_impact(service)
    return {
        "service": service,
        "upstream_impact": result.upstream,
        "downstream_impact": result.downstream,
        "total_affected_services": result.total_affected,
        "estimated_user_impact": result.user_impact,
    }


@router.get("/similar-incidents")
async def find_similar_incidents(
    anomaly_id: Optional[str] = None,
    service: Optional[str] = None,
    symptom: Optional[str] = None,
    limit: int = 5,
    analyzer: RCAAnalyzer = Depends(get_analyzer),
) -> dict:
    """
    Find similar historical incidents.

    Useful for learning from past incidents and
    applying known fixes.
    """
    incidents = await analyzer.find_similar_incidents(
        anomaly_id=anomaly_id,
        service=service,
        symptom=symptom,
        limit=limit,
    )
    return {
        "similar_incidents": incidents,
        "common_patterns": analyzer.extract_common_patterns(incidents),
    }
