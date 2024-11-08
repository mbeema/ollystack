"""
Investigation Models

Data models for proactive AI investigations.
"""

from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, Any
from enum import Enum
import uuid


class InvestigationStatus(str, Enum):
    """Status of an investigation."""
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class InvestigationPhase(str, Enum):
    """Current phase of investigation."""
    INITIALIZING = "initializing"
    GATHERING_DATA = "gathering_data"
    ANALYZING_TRACES = "analyzing_traces"
    ANALYZING_METRICS = "analyzing_metrics"
    ANALYZING_LOGS = "analyzing_logs"
    CORRELATING = "correlating"
    CHECKING_DEPLOYMENTS = "checking_deployments"
    GENERATING_HYPOTHESES = "generating_hypotheses"
    COMPLETE = "complete"


class TriggerType(str, Enum):
    """Types of investigation triggers."""
    ANOMALY = "anomaly"
    ALERT = "alert"
    SLO_BREACH = "slo_breach"
    ERROR_SPIKE = "error_spike"
    LATENCY_SPIKE = "latency_spike"
    MANUAL = "manual"


class Severity(str, Enum):
    """Severity levels."""
    CRITICAL = "critical"
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"
    INFO = "info"


class HypothesisCategory(str, Enum):
    """Categories for root cause hypotheses."""
    INFRASTRUCTURE = "infrastructure"
    CODE = "code"
    DEPENDENCY = "dependency"
    CONFIGURATION = "configuration"
    CAPACITY = "capacity"
    EXTERNAL = "external"
    DATABASE = "database"
    NETWORK = "network"


class EvidenceType(str, Enum):
    """Types of evidence."""
    TRACE = "trace"
    METRIC = "metric"
    LOG = "log"
    DEPLOYMENT = "deployment"
    CONFIG_CHANGE = "config_change"
    TOPOLOGY = "topology"
    ALERT = "alert"


@dataclass
class Investigation:
    """Represents a proactive investigation."""

    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    created_at: datetime = field(default_factory=datetime.utcnow)
    updated_at: datetime = field(default_factory=datetime.utcnow)

    # Trigger info
    trigger_type: TriggerType = TriggerType.MANUAL
    trigger_id: Optional[str] = None
    trigger_timestamp: Optional[datetime] = None

    # Scope
    service_name: Optional[str] = None
    operation_name: Optional[str] = None
    environment: str = "production"

    # Time window
    investigation_start: Optional[datetime] = None
    investigation_end: Optional[datetime] = None

    # Status
    status: InvestigationStatus = InvestigationStatus.PENDING
    phase: InvestigationPhase = InvestigationPhase.INITIALIZING
    progress_percent: int = 0

    # AI-generated summary
    title: str = ""
    summary: str = ""
    severity: Severity = Severity.MEDIUM

    # Impact
    affected_services: list[str] = field(default_factory=list)
    affected_endpoints: list[str] = field(default_factory=list)
    estimated_user_impact: str = ""
    error_count: int = 0
    affected_trace_count: int = 0

    # Results
    hypotheses: list["Hypothesis"] = field(default_factory=list)
    timeline: list["TimelineEvent"] = field(default_factory=list)
    evidence: list["Evidence"] = field(default_factory=list)

    # Confidence
    overall_confidence: float = 0.0

    # Metadata
    labels: dict[str, str] = field(default_factory=dict)
    created_by: str = "system"

    # Resolution
    resolved_at: Optional[datetime] = None
    resolved_by: Optional[str] = None
    resolution: Optional[str] = None

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "id": self.id,
            "created_at": self.created_at.isoformat(),
            "updated_at": self.updated_at.isoformat(),
            "trigger_type": self.trigger_type.value,
            "trigger_id": self.trigger_id,
            "trigger_timestamp": self.trigger_timestamp.isoformat() if self.trigger_timestamp else None,
            "service_name": self.service_name,
            "operation_name": self.operation_name,
            "environment": self.environment,
            "investigation_start": self.investigation_start.isoformat() if self.investigation_start else None,
            "investigation_end": self.investigation_end.isoformat() if self.investigation_end else None,
            "status": self.status.value,
            "phase": self.phase.value,
            "progress_percent": self.progress_percent,
            "title": self.title,
            "summary": self.summary,
            "severity": self.severity.value,
            "affected_services": self.affected_services,
            "affected_endpoints": self.affected_endpoints,
            "estimated_user_impact": self.estimated_user_impact,
            "error_count": self.error_count,
            "affected_trace_count": self.affected_trace_count,
            "hypotheses": [h.to_dict() for h in self.hypotheses],
            "timeline": [t.to_dict() for t in self.timeline],
            "evidence_count": len(self.evidence),
            "overall_confidence": self.overall_confidence,
            "labels": self.labels,
            "created_by": self.created_by,
            "resolved_at": self.resolved_at.isoformat() if self.resolved_at else None,
            "resolved_by": self.resolved_by,
            "resolution": self.resolution,
        }


@dataclass
class Hypothesis:
    """A root cause hypothesis."""

    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    investigation_id: str = ""
    created_at: datetime = field(default_factory=datetime.utcnow)

    # Ranking
    rank: int = 1  # 1 = most likely

    # Details
    title: str = ""
    description: str = ""
    category: HypothesisCategory = HypothesisCategory.CODE

    # Confidence
    confidence: float = 0.0
    reasoning: str = ""

    # Related entities
    related_services: list[str] = field(default_factory=list)
    related_trace_ids: list[str] = field(default_factory=list)
    related_metrics: list[str] = field(default_factory=list)
    related_logs: list[str] = field(default_factory=list)
    related_deployments: list[str] = field(default_factory=list)

    # Actions
    suggested_actions: list[str] = field(default_factory=list)
    runbook_url: Optional[str] = None

    # Verification
    verified: bool = False
    verified_by: Optional[str] = None
    verified_at: Optional[datetime] = None
    verification_notes: Optional[str] = None

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "id": self.id,
            "rank": self.rank,
            "title": self.title,
            "description": self.description,
            "category": self.category.value,
            "confidence": self.confidence,
            "reasoning": self.reasoning,
            "related_services": self.related_services,
            "related_trace_ids": self.related_trace_ids[:5],  # Limit for response size
            "related_metrics": self.related_metrics,
            "related_logs": self.related_logs[:5],
            "related_deployments": self.related_deployments,
            "suggested_actions": self.suggested_actions,
            "runbook_url": self.runbook_url,
            "verified": self.verified,
            "verified_by": self.verified_by,
            "verified_at": self.verified_at.isoformat() if self.verified_at else None,
            "verification_notes": self.verification_notes,
        }


@dataclass
class TimelineEvent:
    """An event in the investigation timeline."""

    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    investigation_id: str = ""
    timestamp: datetime = field(default_factory=datetime.utcnow)

    # Event details
    event_type: str = ""  # anomaly, error, deployment, config_change, metric_spike, etc.
    event_source: str = ""  # traces, metrics, logs, deployments
    title: str = ""
    description: str = ""

    # Severity
    severity: Severity = Severity.INFO
    impact_score: float = 0.0

    # Related data
    service_name: Optional[str] = None
    trace_id: Optional[str] = None
    span_id: Optional[str] = None
    metric_name: Optional[str] = None
    metric_value: Optional[float] = None
    log_message: Optional[str] = None

    # Links
    deep_link_url: Optional[str] = None
    raw_data: Optional[dict] = None

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "id": self.id,
            "timestamp": self.timestamp.isoformat(),
            "event_type": self.event_type,
            "event_source": self.event_source,
            "title": self.title,
            "description": self.description,
            "severity": self.severity.value,
            "impact_score": self.impact_score,
            "service_name": self.service_name,
            "trace_id": self.trace_id,
            "metric_name": self.metric_name,
            "metric_value": self.metric_value,
            "deep_link_url": self.deep_link_url,
        }


@dataclass
class Evidence:
    """Evidence supporting an investigation or hypothesis."""

    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    investigation_id: str = ""
    hypothesis_id: Optional[str] = None
    created_at: datetime = field(default_factory=datetime.utcnow)

    # Evidence details
    evidence_type: EvidenceType = EvidenceType.TRACE
    title: str = ""
    description: str = ""

    # Relevance
    relevance_score: float = 0.0
    is_supporting: bool = True  # True = supports hypothesis, False = contradicts

    # Source data
    source_timestamp: Optional[datetime] = None
    source_service: Optional[str] = None
    trace_id: Optional[str] = None
    span_id: Optional[str] = None
    metric_name: Optional[str] = None
    metric_value: Optional[float] = None
    log_body: Optional[str] = None

    # Raw data
    raw_data: Optional[dict] = None

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "id": self.id,
            "evidence_type": self.evidence_type.value,
            "title": self.title,
            "description": self.description,
            "relevance_score": self.relevance_score,
            "is_supporting": self.is_supporting,
            "source_timestamp": self.source_timestamp.isoformat() if self.source_timestamp else None,
            "source_service": self.source_service,
            "trace_id": self.trace_id,
            "metric_name": self.metric_name,
            "metric_value": self.metric_value,
        }


@dataclass
class InvestigationTriggerConfig:
    """Configuration for automatic investigation triggers."""

    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    name: str = ""
    description: str = ""
    trigger_type: TriggerType = TriggerType.ANOMALY
    enabled: bool = True

    # Conditions
    service_filter: Optional[str] = None  # Regex pattern
    threshold: float = 0.0
    operator: str = "gt"  # gt, lt, gte, lte, eq
    duration: str = "5m"  # How long condition must persist

    # Investigation settings
    auto_start: bool = True
    investigation_window: str = "1h"  # How far back to look
    priority: str = "normal"  # high, normal, low

    # Notification
    notify_on_start: bool = True
    notify_channels: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "id": self.id,
            "name": self.name,
            "description": self.description,
            "trigger_type": self.trigger_type.value,
            "enabled": self.enabled,
            "service_filter": self.service_filter,
            "threshold": self.threshold,
            "operator": self.operator,
            "duration": self.duration,
            "auto_start": self.auto_start,
            "investigation_window": self.investigation_window,
            "priority": self.priority,
            "notify_on_start": self.notify_on_start,
            "notify_channels": self.notify_channels,
        }
