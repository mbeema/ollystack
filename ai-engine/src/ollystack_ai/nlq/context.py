"""
Query Context

Provides context for NLQ translation including time ranges,
service filters, and schema information.
"""

from dataclasses import dataclass, field
from typing import Optional
from datetime import datetime, timedelta


@dataclass
class QueryContext:
    """Context for query translation."""

    time_range: str = "1h"
    service_filter: Optional[str] = None
    available_services: list[str] = field(default_factory=list)
    available_metrics: list[str] = field(default_factory=list)
    user_timezone: str = "UTC"

    def get_time_filter(self) -> tuple[datetime, datetime]:
        """Convert time range to datetime bounds."""
        now = datetime.utcnow()
        duration = self._parse_duration(self.time_range)
        start = now - duration
        return start, now

    def get_time_filter_sql(self) -> str:
        """Get SQL time filter clause."""
        duration = self.time_range
        return f"Timestamp >= now() - INTERVAL {self._to_interval(duration)}"

    def _parse_duration(self, duration: str) -> timedelta:
        """Parse duration string to timedelta."""
        duration = duration.strip().lower()

        if duration.endswith("m"):
            return timedelta(minutes=int(duration[:-1]))
        elif duration.endswith("h"):
            return timedelta(hours=int(duration[:-1]))
        elif duration.endswith("d"):
            return timedelta(days=int(duration[:-1]))
        elif duration.endswith("w"):
            return timedelta(weeks=int(duration[:-1]))
        else:
            # Default to 1 hour
            return timedelta(hours=1)

    def _to_interval(self, duration: str) -> str:
        """Convert duration to SQL INTERVAL string."""
        duration = duration.strip().lower()

        if duration.endswith("m"):
            return f"{duration[:-1]} MINUTE"
        elif duration.endswith("h"):
            return f"{duration[:-1]} HOUR"
        elif duration.endswith("d"):
            return f"{duration[:-1]} DAY"
        elif duration.endswith("w"):
            weeks = int(duration[:-1])
            return f"{weeks * 7} DAY"
        else:
            return "1 HOUR"


@dataclass
class SchemaContext:
    """Database schema context for query generation."""

    tables: dict[str, list[str]] = field(default_factory=dict)

    @classmethod
    def default(cls) -> "SchemaContext":
        """Create default schema context for OllyStack."""
        return cls(
            tables={
                "otel_traces": [
                    "Timestamp",
                    "TraceId",
                    "SpanId",
                    "ParentSpanId",
                    "SpanName",
                    "SpanKind",
                    "ServiceName",
                    "Duration",
                    "StartTime",
                    "EndTime",
                    "StatusCode",
                    "StatusMessage",
                    "HttpMethod",
                    "HttpUrl",
                    "HttpStatusCode",
                    "HttpRoute",
                    "DbSystem",
                    "DbName",
                    "DbStatement",
                    "IsError",
                    "IsRootSpan",
                    "AnomalyScore",
                ],
                "otel_metrics": [
                    "Timestamp",
                    "MetricName",
                    "MetricType",
                    "MetricUnit",
                    "ServiceName",
                    "Value",
                    "HistogramCount",
                    "HistogramSum",
                    "AnomalyScore",
                ],
                "otel_logs": [
                    "Timestamp",
                    "TraceId",
                    "SpanId",
                    "SeverityText",
                    "SeverityNumber",
                    "Body",
                    "ServiceName",
                    "ErrorMessage",
                    "ErrorStack",
                ],
                "service_topology": [
                    "Timestamp",
                    "SourceService",
                    "TargetService",
                    "RequestCount",
                    "ErrorCount",
                    "LatencyP50",
                    "LatencyP90",
                    "LatencyP99",
                    "ErrorRate",
                ],
            }
        )

    def get_columns(self, table: str) -> list[str]:
        """Get columns for a table."""
        return self.tables.get(table, [])

    def has_column(self, table: str, column: str) -> bool:
        """Check if table has a column."""
        return column in self.tables.get(table, [])
