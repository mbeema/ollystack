"""
Proactive AI Investigations API

Provides endpoints for managing AI-powered investigations that
automatically analyze anomalies, alerts, and performance issues.
"""

import logging
from typing import Optional
from datetime import datetime

from fastapi import APIRouter, HTTPException, Request, Depends, Query
from pydantic import BaseModel, Field

from ollystack_ai.investigations.engine import InvestigationEngine
from ollystack_ai.investigations.triggers import InvestigationTrigger
from ollystack_ai.investigations.models import (
    Investigation,
    Hypothesis,
    TimelineEvent,
    Evidence,
    TriggerType,
    InvestigationStatus,
    InvestigationTriggerConfig,
)

logger = logging.getLogger(__name__)
router = APIRouter()


# Request/Response Models

class StartInvestigationRequest(BaseModel):
    """Request to start a new investigation."""

    trigger_type: str = Field(
        default="manual",
        description="Type of trigger: anomaly, alert, slo_breach, error_spike, latency_spike, manual"
    )
    trigger_id: Optional[str] = Field(None, description="ID of the triggering event")
    service_name: Optional[str] = Field(None, description="Service to investigate")
    operation_name: Optional[str] = Field(None, description="Specific operation to investigate")
    investigation_window: str = Field(
        default="1h",
        description="Time window for investigation (e.g., 30m, 1h, 6h)"
    )
    description: Optional[str] = Field(None, description="Additional context for the investigation")


class InvestigationResponse(BaseModel):
    """Investigation response."""

    id: str
    created_at: str
    status: str
    phase: str
    progress_percent: int
    title: str
    summary: str
    severity: str
    service_name: Optional[str]
    affected_services: list[str]
    hypotheses: list[dict]
    timeline: list[dict]
    evidence_count: int
    overall_confidence: float


class InvestigationListResponse(BaseModel):
    """List of investigations."""

    investigations: list[dict]
    total: int
    page: int
    page_size: int


class HypothesisVerifyRequest(BaseModel):
    """Request to verify/reject a hypothesis."""

    verified: bool
    notes: Optional[str] = None


class TriggerConfigRequest(BaseModel):
    """Request to create/update a trigger configuration."""

    name: str
    description: Optional[str] = None
    trigger_type: str = Field(description="Type: anomaly, alert, slo_breach, error_spike, latency_spike")
    enabled: bool = True
    service_filter: Optional[str] = Field(None, description="Regex pattern to filter services")
    threshold: float = 0.0
    operator: str = Field(default="gt", description="Comparison: gt, lt, gte, lte, eq")
    duration: str = Field(default="5m", description="How long condition must persist")
    auto_start: bool = True
    investigation_window: str = Field(default="1h", description="Investigation time window")
    priority: str = Field(default="normal", description="Priority: high, normal, low")
    notify_on_start: bool = True
    notify_channels: list[str] = Field(default_factory=list)


# Dependencies

def get_investigation_engine(request: Request) -> InvestigationEngine:
    """Dependency to get investigation engine."""
    return InvestigationEngine(
        llm_service=request.app.state.llm_service,
        storage_service=request.app.state.storage_service,
        cache_service=request.app.state.cache_service,
    )


def get_investigation_trigger(request: Request) -> InvestigationTrigger:
    """Dependency to get investigation trigger manager."""
    if not hasattr(request.app.state, 'investigation_trigger'):
        engine = get_investigation_engine(request)
        request.app.state.investigation_trigger = InvestigationTrigger(
            engine=engine,
            storage=request.app.state.storage_service,
            cache=request.app.state.cache_service,
        )
    return request.app.state.investigation_trigger


# Investigation Endpoints

@router.post("/start", response_model=InvestigationResponse)
async def start_investigation(
    request: StartInvestigationRequest,
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> InvestigationResponse:
    """
    Start a new AI-powered investigation.

    The investigation will automatically:
    - Gather relevant traces, metrics, and logs
    - Analyze patterns and anomalies
    - Correlate events across services
    - Check recent deployments and config changes
    - Generate ranked hypotheses for root cause
    """
    try:
        # Parse trigger type
        try:
            trigger_type = TriggerType(request.trigger_type)
        except ValueError:
            trigger_type = TriggerType.MANUAL

        # Start investigation
        investigation = await engine.start_investigation(
            trigger_type=trigger_type,
            trigger_id=request.trigger_id,
            service_name=request.service_name,
            operation_name=request.operation_name,
            investigation_window=request.investigation_window,
            created_by="user",
        )

        return _investigation_to_response(investigation)

    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        logger.exception("Error starting investigation")
        raise HTTPException(status_code=500, detail=f"Failed to start investigation: {str(e)}")


@router.get("/{investigation_id}", response_model=InvestigationResponse)
async def get_investigation(
    investigation_id: str,
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> InvestigationResponse:
    """
    Get details of a specific investigation.
    """
    investigation = await engine.get_investigation(investigation_id)
    if not investigation:
        raise HTTPException(status_code=404, detail="Investigation not found")

    return _investigation_to_response(investigation)


@router.get("/", response_model=InvestigationListResponse)
async def list_investigations(
    status: Optional[str] = Query(None, description="Filter by status"),
    service: Optional[str] = Query(None, description="Filter by service"),
    trigger_type: Optional[str] = Query(None, description="Filter by trigger type"),
    page: int = Query(1, ge=1, description="Page number"),
    page_size: int = Query(20, ge=1, le=100, description="Items per page"),
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> InvestigationListResponse:
    """
    List investigations with optional filters.
    """
    investigations, total = await engine.list_investigations(
        status=status,
        service=service,
        trigger_type=trigger_type,
        limit=page_size,
        offset=(page - 1) * page_size,
    )

    return InvestigationListResponse(
        investigations=[inv.to_dict() for inv in investigations],
        total=total,
        page=page,
        page_size=page_size,
    )


@router.post("/{investigation_id}/cancel")
async def cancel_investigation(
    investigation_id: str,
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> dict:
    """
    Cancel a running investigation.
    """
    success = await engine.cancel_investigation(investigation_id)
    if not success:
        raise HTTPException(status_code=404, detail="Investigation not found or already completed")

    return {"status": "cancelled", "investigation_id": investigation_id}


@router.post("/{investigation_id}/resolve")
async def resolve_investigation(
    investigation_id: str,
    resolution: str,
    resolved_by: Optional[str] = None,
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> dict:
    """
    Mark an investigation as resolved.
    """
    success = await engine.resolve_investigation(
        investigation_id=investigation_id,
        resolution=resolution,
        resolved_by=resolved_by or "user",
    )
    if not success:
        raise HTTPException(status_code=404, detail="Investigation not found")

    return {"status": "resolved", "investigation_id": investigation_id}


# Hypothesis Endpoints

@router.get("/{investigation_id}/hypotheses")
async def get_hypotheses(
    investigation_id: str,
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> dict:
    """
    Get all hypotheses for an investigation.
    """
    investigation = await engine.get_investigation(investigation_id)
    if not investigation:
        raise HTTPException(status_code=404, detail="Investigation not found")

    return {
        "investigation_id": investigation_id,
        "hypotheses": [h.to_dict() for h in investigation.hypotheses],
    }


@router.post("/{investigation_id}/hypotheses/{hypothesis_id}/verify")
async def verify_hypothesis(
    investigation_id: str,
    hypothesis_id: str,
    request: HypothesisVerifyRequest,
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> dict:
    """
    Verify or reject a hypothesis.

    User feedback helps improve future investigations.
    """
    success = await engine.verify_hypothesis(
        investigation_id=investigation_id,
        hypothesis_id=hypothesis_id,
        verified=request.verified,
        verified_by="user",
        notes=request.notes,
    )
    if not success:
        raise HTTPException(status_code=404, detail="Investigation or hypothesis not found")

    return {
        "status": "updated",
        "hypothesis_id": hypothesis_id,
        "verified": request.verified,
    }


# Timeline Endpoints

@router.get("/{investigation_id}/timeline")
async def get_timeline(
    investigation_id: str,
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> dict:
    """
    Get the timeline of events for an investigation.
    """
    investigation = await engine.get_investigation(investigation_id)
    if not investigation:
        raise HTTPException(status_code=404, detail="Investigation not found")

    return {
        "investigation_id": investigation_id,
        "timeline": [t.to_dict() for t in investigation.timeline],
    }


# Evidence Endpoints

@router.get("/{investigation_id}/evidence")
async def get_evidence(
    investigation_id: str,
    hypothesis_id: Optional[str] = None,
    evidence_type: Optional[str] = None,
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> dict:
    """
    Get evidence collected during an investigation.
    """
    investigation = await engine.get_investigation(investigation_id)
    if not investigation:
        raise HTTPException(status_code=404, detail="Investigation not found")

    evidence = investigation.evidence

    # Filter by hypothesis
    if hypothesis_id:
        evidence = [e for e in evidence if e.hypothesis_id == hypothesis_id]

    # Filter by type
    if evidence_type:
        evidence = [e for e in evidence if e.evidence_type.value == evidence_type]

    return {
        "investigation_id": investigation_id,
        "evidence": [e.to_dict() for e in evidence],
        "total": len(evidence),
    }


# Trigger Management Endpoints

@router.get("/triggers/list")
async def list_triggers(
    trigger: InvestigationTrigger = Depends(get_investigation_trigger),
) -> dict:
    """
    List all configured investigation triggers.
    """
    triggers = trigger.list_triggers()
    return {
        "triggers": [t.to_dict() for t in triggers],
        "total": len(triggers),
    }


@router.get("/triggers/{trigger_id}")
async def get_trigger(
    trigger_id: str,
    trigger: InvestigationTrigger = Depends(get_investigation_trigger),
) -> dict:
    """
    Get a specific trigger configuration.
    """
    t = trigger.get_trigger(trigger_id)
    if not t:
        raise HTTPException(status_code=404, detail="Trigger not found")

    return t.to_dict()


@router.post("/triggers/create")
async def create_trigger(
    request: TriggerConfigRequest,
    trigger: InvestigationTrigger = Depends(get_investigation_trigger),
) -> dict:
    """
    Create a new investigation trigger.
    """
    try:
        trigger_type = TriggerType(request.trigger_type)
    except ValueError:
        raise HTTPException(status_code=400, detail=f"Invalid trigger type: {request.trigger_type}")

    config = InvestigationTriggerConfig(
        name=request.name,
        description=request.description or "",
        trigger_type=trigger_type,
        enabled=request.enabled,
        service_filter=request.service_filter,
        threshold=request.threshold,
        operator=request.operator,
        duration=request.duration,
        auto_start=request.auto_start,
        investigation_window=request.investigation_window,
        priority=request.priority,
        notify_on_start=request.notify_on_start,
        notify_channels=request.notify_channels,
    )

    trigger.add_trigger(config)

    return {"status": "created", "trigger": config.to_dict()}


@router.put("/triggers/{trigger_id}")
async def update_trigger(
    trigger_id: str,
    request: TriggerConfigRequest,
    trigger: InvestigationTrigger = Depends(get_investigation_trigger),
) -> dict:
    """
    Update an existing trigger configuration.
    """
    existing = trigger.get_trigger(trigger_id)
    if not existing:
        raise HTTPException(status_code=404, detail="Trigger not found")

    try:
        trigger_type = TriggerType(request.trigger_type)
    except ValueError:
        raise HTTPException(status_code=400, detail=f"Invalid trigger type: {request.trigger_type}")

    # Update fields
    existing.name = request.name
    existing.description = request.description or ""
    existing.trigger_type = trigger_type
    existing.enabled = request.enabled
    existing.service_filter = request.service_filter
    existing.threshold = request.threshold
    existing.operator = request.operator
    existing.duration = request.duration
    existing.auto_start = request.auto_start
    existing.investigation_window = request.investigation_window
    existing.priority = request.priority
    existing.notify_on_start = request.notify_on_start
    existing.notify_channels = request.notify_channels

    return {"status": "updated", "trigger": existing.to_dict()}


@router.delete("/triggers/{trigger_id}")
async def delete_trigger(
    trigger_id: str,
    trigger: InvestigationTrigger = Depends(get_investigation_trigger),
) -> dict:
    """
    Delete a trigger configuration.
    """
    success = trigger.remove_trigger(trigger_id)
    if not success:
        raise HTTPException(status_code=404, detail="Trigger not found")

    return {"status": "deleted", "trigger_id": trigger_id}


@router.post("/triggers/{trigger_id}/enable")
async def enable_trigger(
    trigger_id: str,
    trigger: InvestigationTrigger = Depends(get_investigation_trigger),
) -> dict:
    """
    Enable a trigger.
    """
    success = trigger.enable_trigger(trigger_id)
    if not success:
        raise HTTPException(status_code=404, detail="Trigger not found")

    return {"status": "enabled", "trigger_id": trigger_id}


@router.post("/triggers/{trigger_id}/disable")
async def disable_trigger(
    trigger_id: str,
    trigger: InvestigationTrigger = Depends(get_investigation_trigger),
) -> dict:
    """
    Disable a trigger.
    """
    success = trigger.disable_trigger(trigger_id)
    if not success:
        raise HTTPException(status_code=404, detail="Trigger not found")

    return {"status": "disabled", "trigger_id": trigger_id}


# Monitoring Control

@router.post("/triggers/monitoring/start")
async def start_monitoring(
    trigger: InvestigationTrigger = Depends(get_investigation_trigger),
) -> dict:
    """
    Start the background trigger monitoring loop.
    """
    await trigger.start()
    return {"status": "started", "message": "Trigger monitoring started"}


@router.post("/triggers/monitoring/stop")
async def stop_monitoring(
    trigger: InvestigationTrigger = Depends(get_investigation_trigger),
) -> dict:
    """
    Stop the background trigger monitoring loop.
    """
    await trigger.stop()
    return {"status": "stopped", "message": "Trigger monitoring stopped"}


# Stats & Analytics

@router.get("/stats/summary")
async def get_investigation_stats(
    time_range: str = Query("24h", description="Time range: 1h, 24h, 7d, 30d"),
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> dict:
    """
    Get investigation statistics and summary.
    """
    stats = await engine.get_investigation_stats(time_range)
    return stats


@router.get("/stats/trigger-performance")
async def get_trigger_performance(
    time_range: str = Query("7d", description="Time range for stats"),
    engine: InvestigationEngine = Depends(get_investigation_engine),
) -> dict:
    """
    Get trigger performance metrics.

    Shows how often each trigger fires and the accuracy of
    resulting investigations.
    """
    stats = await engine.get_trigger_stats(time_range)
    return stats


# Helper Functions

def _investigation_to_response(investigation: Investigation) -> InvestigationResponse:
    """Convert Investigation model to API response."""
    return InvestigationResponse(
        id=investigation.id,
        created_at=investigation.created_at.isoformat(),
        status=investigation.status.value,
        phase=investigation.phase.value,
        progress_percent=investigation.progress_percent,
        title=investigation.title,
        summary=investigation.summary,
        severity=investigation.severity.value,
        service_name=investigation.service_name,
        affected_services=investigation.affected_services,
        hypotheses=[h.to_dict() for h in investigation.hypotheses],
        timeline=[t.to_dict() for t in investigation.timeline],
        evidence_count=len(investigation.evidence),
        overall_confidence=investigation.overall_confidence,
    )
