"""
RCA Models

Data models for root cause analysis results.
"""

from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, Any
from enum import Enum


class Severity(str, Enum):
    """Severity levels for issues."""

    CRITICAL = "critical"
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"
    INFO = "info"


class EvidenceType(str, Enum):
    """Types of evidence for RCA."""

    TRACE = "trace"
    METRIC = "metric"
    LOG = "log"
    TOPOLOGY = "topology"
    PATTERN = "pattern"


@dataclass
class RCARequest:
    """Request for root cause analysis."""

    anomaly_id: Optional[str] = None
    trace_id: Optional[str] = None
    service: Optional[str] = None
    symptom: Optional[str] = None
    time_window: Optional[tuple[datetime, datetime]] = None


@dataclass
class RootCause:
    """Identified root cause."""

    description: str
    service: str
    confidence: float
    severity: Severity
    category: str  # e.g., "database", "network", "resource", "code"
    details: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "description": self.description,
            "service": self.service,
            "confidence": self.confidence,
            "severity": self.severity.value,
            "category": self.category,
            "details": self.details,
        }


@dataclass
class ContributingFactor:
    """Factor that contributed to the issue."""

    description: str
    impact: float  # 0.0 to 1.0
    service: Optional[str] = None
    metric: Optional[str] = None
    details: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "description": self.description,
            "impact": self.impact,
            "service": self.service,
            "metric": self.metric,
            "details": self.details,
        }


@dataclass
class Evidence:
    """Evidence supporting the analysis."""

    type: EvidenceType
    description: str
    source: str  # e.g., trace_id, metric_name
    timestamp: Optional[datetime] = None
    value: Optional[Any] = None
    link: Optional[str] = None  # Deep link to UI

    def to_dict(self) -> dict:
        return {
            "type": self.type.value,
            "description": self.description,
            "source": self.source,
            "timestamp": self.timestamp.isoformat() if self.timestamp else None,
            "value": self.value,
            "link": self.link,
        }


@dataclass
class Recommendation:
    """Recommended action to fix or prevent the issue."""

    action: str
    priority: int  # 1-5, 1 being highest
    category: str  # e.g., "immediate", "short-term", "long-term"
    effort: str  # e.g., "low", "medium", "high"
    details: Optional[str] = None

    def to_dict(self) -> dict:
        return {
            "action": self.action,
            "priority": self.priority,
            "category": self.category,
            "effort": self.effort,
            "details": self.details,
        }


@dataclass
class RCAResult:
    """Complete root cause analysis result."""

    summary: str
    confidence: float
    root_causes: list[RootCause]
    contributing_factors: list[ContributingFactor]
    evidence: list[Evidence]
    recommendations: list[Recommendation]
    related_incidents: list[dict]
    timeline: list[dict]

    @classmethod
    def empty(cls, message: str = "No analysis available") -> "RCAResult":
        """Create an empty result."""
        return cls(
            summary=message,
            confidence=0.0,
            root_causes=[],
            contributing_factors=[],
            evidence=[],
            recommendations=[],
            related_incidents=[],
            timeline=[],
        )


@dataclass
class TraceAnalysisResult:
    """Result of trace-specific analysis."""

    total_duration_ms: float
    critical_path: list[dict]
    bottlenecks: list[dict]
    anomalous_spans: list[dict]
    suggestions: list[str]


@dataclass
class ErrorPatternResult:
    """Result of error pattern analysis."""

    summary: str
    clusters: list[dict]
    common_causes: list[str]
    affected_endpoints: list[dict]


@dataclass
class ImpactAnalysisResult:
    """Result of impact analysis."""

    upstream: list[dict]
    downstream: list[dict]
    total_affected: int
    user_impact: str
