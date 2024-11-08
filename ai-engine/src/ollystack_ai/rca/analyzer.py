"""
RCA Analyzer

Performs root cause analysis using multiple techniques:
- Trace correlation
- Metric anomaly correlation
- Log pattern analysis
- Service dependency analysis
- LLM-powered reasoning
"""

import logging
from typing import Optional
from datetime import datetime, timedelta

import networkx as nx

from ollystack_ai.services.llm import LLMService
from ollystack_ai.services.storage import StorageService
from ollystack_ai.services.cache import CacheService
from ollystack_ai.rca.models import (
    RCARequest,
    RCAResult,
    RootCause,
    ContributingFactor,
    Evidence,
    Recommendation,
    Severity,
    EvidenceType,
    TraceAnalysisResult,
    ErrorPatternResult,
    ImpactAnalysisResult,
)

logger = logging.getLogger(__name__)


class RCAAnalyzer:
    """
    Root Cause Analysis engine.

    Combines multiple analysis techniques to identify root causes:
    1. Statistical correlation of metrics
    2. Trace-based causal analysis
    3. Log pattern clustering
    4. Service dependency graph analysis
    5. LLM-powered reasoning
    """

    def __init__(
        self,
        llm_service: LLMService,
        storage_service: StorageService,
        cache_service: Optional[CacheService] = None,
    ):
        self.llm = llm_service
        self.storage = storage_service
        self.cache = cache_service

    async def analyze(self, request: RCARequest) -> RCAResult:
        """
        Perform comprehensive root cause analysis.

        Args:
            request: RCA request with context

        Returns:
            RCAResult with root causes, evidence, and recommendations
        """
        try:
            # Gather data for analysis
            context = await self._gather_context(request)

            if not context:
                return RCAResult.empty("Insufficient data for analysis")

            # Run multiple analysis techniques in parallel
            trace_analysis = await self._analyze_traces(context)
            metric_analysis = await self._analyze_metrics(context)
            log_analysis = await self._analyze_logs(context)
            topology_analysis = await self._analyze_topology(context)

            # Combine findings
            combined_evidence = (
                trace_analysis.get("evidence", [])
                + metric_analysis.get("evidence", [])
                + log_analysis.get("evidence", [])
                + topology_analysis.get("evidence", [])
            )

            # Use LLM to synthesize findings
            synthesis = await self._synthesize_findings(
                request=request,
                trace_findings=trace_analysis,
                metric_findings=metric_analysis,
                log_findings=log_analysis,
                topology_findings=topology_analysis,
            )

            # Find similar historical incidents
            related = await self._find_related_incidents(
                request, synthesis.get("root_causes", [])
            )

            # Build timeline
            timeline = self._build_timeline(combined_evidence)

            # Generate recommendations
            recommendations = await self._generate_recommendations(
                synthesis, related
            )

            return RCAResult(
                summary=synthesis.get("summary", "Analysis complete"),
                confidence=synthesis.get("confidence", 0.5),
                root_causes=synthesis.get("root_causes", []),
                contributing_factors=synthesis.get("contributing_factors", []),
                evidence=[Evidence(**e) if isinstance(e, dict) else e for e in combined_evidence],
                recommendations=recommendations,
                related_incidents=related,
                timeline=timeline,
            )

        except Exception as e:
            logger.exception("RCA analysis failed")
            return RCAResult.empty(f"Analysis failed: {str(e)}")

    async def _gather_context(self, request: RCARequest) -> Optional[dict]:
        """Gather relevant data for analysis."""
        context = {
            "request": request,
            "traces": [],
            "metrics": [],
            "logs": [],
            "topology": [],
        }

        # Determine time window
        if request.time_window:
            start, end = request.time_window
        else:
            end = datetime.utcnow()
            start = end - timedelta(hours=1)

        time_filter = f"Timestamp BETWEEN '{start.isoformat()}' AND '{end.isoformat()}'"

        # Get traces
        if request.trace_id:
            trace_query = f"""
            SELECT * FROM otel_traces
            WHERE TraceId = '{request.trace_id}'
            ORDER BY StartTime
            """
        elif request.service:
            trace_query = f"""
            SELECT * FROM otel_traces
            WHERE ServiceName = '{request.service}'
              AND {time_filter}
              AND (StatusCode = 'ERROR' OR Duration > (
                SELECT quantile(0.95)(Duration)
                FROM otel_traces
                WHERE ServiceName = '{request.service}'
                  AND {time_filter}
              ))
            ORDER BY Timestamp DESC
            LIMIT 100
            """
        else:
            trace_query = f"""
            SELECT * FROM otel_traces
            WHERE {time_filter}
              AND StatusCode = 'ERROR'
            ORDER BY Timestamp DESC
            LIMIT 100
            """

        traces_result = await self.storage.execute_query(trace_query)
        context["traces"] = traces_result[0] if traces_result else []

        # Get metrics (anomalies)
        if request.service:
            metrics_query = f"""
            SELECT * FROM otel_metrics
            WHERE ServiceName = '{request.service}'
              AND {time_filter}
              AND AnomalyScore > 0.7
            ORDER BY AnomalyScore DESC
            LIMIT 50
            """
        else:
            metrics_query = f"""
            SELECT * FROM otel_metrics
            WHERE {time_filter}
              AND AnomalyScore > 0.7
            ORDER BY AnomalyScore DESC
            LIMIT 50
            """

        metrics_result = await self.storage.execute_query(metrics_query)
        context["metrics"] = metrics_result[0] if metrics_result else []

        # Get logs (errors)
        service_filter = f"AND ServiceName = '{request.service}'" if request.service else ""
        logs_query = f"""
        SELECT * FROM otel_logs
        WHERE {time_filter}
          AND SeverityText IN ('ERROR', 'FATAL')
          {service_filter}
        ORDER BY Timestamp DESC
        LIMIT 100
        """

        logs_result = await self.storage.execute_query(logs_query)
        context["logs"] = logs_result[0] if logs_result else []

        # Get topology
        topology_query = f"""
        SELECT * FROM service_topology
        WHERE {time_filter}
        """

        topology_result = await self.storage.execute_query(topology_query)
        context["topology"] = topology_result[0] if topology_result else []

        return context

    async def _analyze_traces(self, context: dict) -> dict:
        """Analyze traces for root cause indicators."""
        traces = context.get("traces", [])
        if not traces:
            return {"evidence": [], "findings": []}

        evidence = []
        findings = []

        # Group by trace ID
        trace_groups = {}
        for trace in traces:
            tid = trace.get("TraceId")
            if tid not in trace_groups:
                trace_groups[tid] = []
            trace_groups[tid].append(trace)

        # Analyze each trace
        for trace_id, spans in trace_groups.items():
            # Find error spans
            error_spans = [s for s in spans if s.get("StatusCode") == "ERROR"]
            if error_spans:
                for span in error_spans:
                    evidence.append({
                        "type": EvidenceType.TRACE.value,
                        "description": f"Error in {span.get('ServiceName')}: {span.get('StatusMessage', 'Unknown error')}",
                        "source": trace_id,
                        "timestamp": span.get("Timestamp"),
                        "value": {
                            "service": span.get("ServiceName"),
                            "operation": span.get("SpanName"),
                            "duration_ms": span.get("Duration", 0) / 1_000_000,
                        },
                    })

            # Find slow spans
            slow_spans = [s for s in spans if s.get("Duration", 0) > 1_000_000_000]  # > 1s
            for span in slow_spans:
                evidence.append({
                    "type": EvidenceType.TRACE.value,
                    "description": f"Slow span in {span.get('ServiceName')}: {span.get('SpanName')}",
                    "source": trace_id,
                    "timestamp": span.get("Timestamp"),
                    "value": {
                        "service": span.get("ServiceName"),
                        "operation": span.get("SpanName"),
                        "duration_ms": span.get("Duration", 0) / 1_000_000,
                    },
                })

        return {"evidence": evidence, "findings": findings}

    async def _analyze_metrics(self, context: dict) -> dict:
        """Analyze metrics for anomalies."""
        metrics = context.get("metrics", [])
        if not metrics:
            return {"evidence": [], "findings": []}

        evidence = []

        for metric in metrics:
            if metric.get("AnomalyScore", 0) > 0.7:
                evidence.append({
                    "type": EvidenceType.METRIC.value,
                    "description": f"Anomaly in {metric.get('MetricName')} for {metric.get('ServiceName')}",
                    "source": metric.get("MetricName"),
                    "timestamp": metric.get("Timestamp"),
                    "value": {
                        "value": metric.get("Value"),
                        "baseline": metric.get("Baseline"),
                        "anomaly_score": metric.get("AnomalyScore"),
                    },
                })

        return {"evidence": evidence, "findings": []}

    async def _analyze_logs(self, context: dict) -> dict:
        """Analyze logs for error patterns."""
        logs = context.get("logs", [])
        if not logs:
            return {"evidence": [], "findings": []}

        evidence = []

        # Group errors by message pattern
        error_patterns = {}
        for log in logs:
            body = log.get("Body", "")
            # Simple pattern extraction (first 100 chars)
            pattern = body[:100] if body else "Unknown"
            if pattern not in error_patterns:
                error_patterns[pattern] = []
            error_patterns[pattern].append(log)

        # Report significant patterns
        for pattern, logs_in_pattern in error_patterns.items():
            if len(logs_in_pattern) >= 3:  # At least 3 occurrences
                evidence.append({
                    "type": EvidenceType.LOG.value,
                    "description": f"Repeated error pattern ({len(logs_in_pattern)} occurrences)",
                    "source": logs_in_pattern[0].get("ServiceName", "unknown"),
                    "timestamp": logs_in_pattern[0].get("Timestamp"),
                    "value": {
                        "pattern": pattern,
                        "count": len(logs_in_pattern),
                        "services": list(set(l.get("ServiceName") for l in logs_in_pattern)),
                    },
                })

        return {"evidence": evidence, "findings": []}

    async def _analyze_topology(self, context: dict) -> dict:
        """Analyze service topology for dependency issues."""
        topology = context.get("topology", [])
        if not topology:
            return {"evidence": [], "findings": []}

        evidence = []

        # Build dependency graph
        G = nx.DiGraph()
        for edge in topology:
            src = edge.get("SourceService")
            tgt = edge.get("TargetService")
            G.add_edge(src, tgt, **edge)

        # Find services with high error rates
        for edge in topology:
            error_rate = edge.get("ErrorRate", 0)
            if error_rate > 0.1:  # > 10% error rate
                evidence.append({
                    "type": EvidenceType.TOPOLOGY.value,
                    "description": f"High error rate between {edge.get('SourceService')} → {edge.get('TargetService')}",
                    "source": f"{edge.get('SourceService')}->{edge.get('TargetService')}",
                    "timestamp": edge.get("Timestamp"),
                    "value": {
                        "error_rate": error_rate,
                        "request_count": edge.get("RequestCount"),
                        "p99_latency_ms": edge.get("LatencyP99"),
                    },
                })

        return {"evidence": evidence, "findings": []}

    async def _synthesize_findings(
        self,
        request: RCARequest,
        trace_findings: dict,
        metric_findings: dict,
        log_findings: dict,
        topology_findings: dict,
    ) -> dict:
        """Use LLM to synthesize findings into root causes."""
        # Prepare summary for LLM
        summary = f"""
Analyze the following observability data to identify root causes:

Request Context:
- Service: {request.service or 'All services'}
- Symptom: {request.symptom or 'Unknown'}

Trace Evidence:
{self._format_evidence(trace_findings.get('evidence', []))}

Metric Anomalies:
{self._format_evidence(metric_findings.get('evidence', []))}

Log Errors:
{self._format_evidence(log_findings.get('evidence', []))}

Topology Issues:
{self._format_evidence(topology_findings.get('evidence', []))}

Identify:
1. Most likely root cause(s)
2. Contributing factors
3. Confidence level (0-1)
"""

        response = await self.llm.complete(
            system_prompt="You are an expert SRE performing root cause analysis. Be concise and specific.",
            user_prompt=summary,
            temperature=0.3,
        )

        # Parse LLM response
        return self._parse_synthesis(response, request)

    def _format_evidence(self, evidence: list) -> str:
        """Format evidence for LLM prompt."""
        if not evidence:
            return "No significant findings"

        lines = []
        for e in evidence[:10]:  # Limit to top 10
            lines.append(f"- {e.get('description', 'Unknown')}")
        return "\n".join(lines)

    def _parse_synthesis(self, response: str, request: RCARequest) -> dict:
        """Parse LLM synthesis response."""
        # Default result
        result = {
            "summary": response[:500],
            "confidence": 0.6,
            "root_causes": [],
            "contributing_factors": [],
        }

        # Extract confidence if mentioned
        if "confidence" in response.lower():
            import re
            match = re.search(r"confidence[:\s]+(\d+\.?\d*)", response.lower())
            if match:
                try:
                    result["confidence"] = min(float(match.group(1)), 1.0)
                except ValueError:
                    pass

        # Create root cause from response
        if request.service:
            result["root_causes"].append(
                RootCause(
                    description=response[:200],
                    service=request.service,
                    confidence=result["confidence"],
                    severity=Severity.HIGH,
                    category="unknown",
                )
            )

        return result

    async def _find_related_incidents(
        self,
        request: RCARequest,
        root_causes: list,
    ) -> list[dict]:
        """Find similar historical incidents."""
        # Query for similar past incidents
        query = """
        SELECT * FROM anomalies
        WHERE Timestamp >= now() - INTERVAL 30 DAY
        ORDER BY Score DESC
        LIMIT 10
        """

        try:
            result = await self.storage.execute_query(query)
            incidents = result[0] if result else []
            return [
                {
                    "id": str(i.get("AnomalyId")),
                    "timestamp": str(i.get("Timestamp")),
                    "service": i.get("ServiceName"),
                    "type": i.get("AnomalyType"),
                    "score": i.get("Score"),
                }
                for i in incidents[:5]
            ]
        except Exception:
            return []

    def _build_timeline(self, evidence: list) -> list[dict]:
        """Build timeline from evidence."""
        timeline = []
        for e in evidence:
            if isinstance(e, dict) and e.get("timestamp"):
                timeline.append({
                    "timestamp": str(e["timestamp"]),
                    "event": e.get("description", "Event"),
                    "type": e.get("type", "unknown"),
                    "source": e.get("source", "unknown"),
                })
            elif hasattr(e, "timestamp") and e.timestamp:
                timeline.append({
                    "timestamp": str(e.timestamp),
                    "event": e.description,
                    "type": e.type.value if hasattr(e.type, "value") else str(e.type),
                    "source": e.source,
                })

        # Sort by timestamp
        timeline.sort(key=lambda x: x.get("timestamp", ""))
        return timeline

    async def _generate_recommendations(
        self,
        synthesis: dict,
        related_incidents: list,
    ) -> list[Recommendation]:
        """Generate recommendations based on analysis."""
        recommendations = []

        # Add general recommendations based on root causes
        for rc in synthesis.get("root_causes", []):
            if isinstance(rc, RootCause):
                if rc.category == "database":
                    recommendations.append(Recommendation(
                        action="Review database query performance and connection pool settings",
                        priority=1,
                        category="immediate",
                        effort="medium",
                    ))
                elif rc.category == "resource":
                    recommendations.append(Recommendation(
                        action="Scale up resources or optimize resource usage",
                        priority=1,
                        category="immediate",
                        effort="low",
                    ))

        # Add recommendations from similar incidents
        if related_incidents:
            recommendations.append(Recommendation(
                action="Review similar past incidents for known fixes",
                priority=2,
                category="short-term",
                effort="low",
                details=f"Found {len(related_incidents)} similar incidents",
            ))

        # Default recommendations
        if not recommendations:
            recommendations.append(Recommendation(
                action="Review application logs for detailed error information",
                priority=2,
                category="immediate",
                effort="low",
            ))
            recommendations.append(Recommendation(
                action="Check recent deployments or configuration changes",
                priority=2,
                category="immediate",
                effort="low",
            ))

        return recommendations

    async def analyze_trace(self, trace_id: str) -> TraceAnalysisResult:
        """Analyze a specific trace."""
        query = f"""
        SELECT * FROM otel_traces
        WHERE TraceId = '{trace_id}'
        ORDER BY StartTime
        """

        result = await self.storage.execute_query(query)
        spans = result[0] if result else []

        if not spans:
            return TraceAnalysisResult(
                total_duration_ms=0,
                critical_path=[],
                bottlenecks=[],
                anomalous_spans=[],
                suggestions=[],
            )

        # Calculate total duration
        total_duration_ns = max(s.get("Duration", 0) for s in spans)
        total_duration_ms = total_duration_ns / 1_000_000

        # Find critical path (longest path through trace)
        critical_path = self._find_critical_path(spans)

        # Find bottlenecks
        bottlenecks = [
            {
                "service": s.get("ServiceName"),
                "operation": s.get("SpanName"),
                "duration_ms": s.get("Duration", 0) / 1_000_000,
                "percentage": (s.get("Duration", 0) / total_duration_ns * 100) if total_duration_ns > 0 else 0,
            }
            for s in sorted(spans, key=lambda x: x.get("Duration", 0), reverse=True)[:5]
        ]

        # Find anomalous spans
        anomalous_spans = [
            {
                "span_id": s.get("SpanId"),
                "service": s.get("ServiceName"),
                "operation": s.get("SpanName"),
                "anomaly_score": s.get("AnomalyScore", 0),
            }
            for s in spans
            if s.get("AnomalyScore", 0) > 0.7
        ]

        return TraceAnalysisResult(
            total_duration_ms=total_duration_ms,
            critical_path=critical_path,
            bottlenecks=bottlenecks,
            anomalous_spans=anomalous_spans,
            suggestions=["Review bottleneck operations for optimization opportunities"],
        )

    def _find_critical_path(self, spans: list) -> list[dict]:
        """Find critical path through trace."""
        # Build span tree
        span_map = {s.get("SpanId"): s for s in spans}

        # Find root span
        root_spans = [s for s in spans if not s.get("ParentSpanId") or s.get("ParentSpanId") not in span_map]

        if not root_spans:
            return []

        # Simple critical path: follow longest duration at each level
        path = []
        current = root_spans[0]

        while current:
            path.append({
                "span_id": current.get("SpanId"),
                "service": current.get("ServiceName"),
                "operation": current.get("SpanName"),
                "duration_ms": current.get("Duration", 0) / 1_000_000,
            })

            # Find children
            children = [s for s in spans if s.get("ParentSpanId") == current.get("SpanId")]
            if children:
                current = max(children, key=lambda x: x.get("Duration", 0))
            else:
                current = None

        return path

    async def analyze_error_pattern(
        self,
        service: str,
        time_range: str,
    ) -> ErrorPatternResult:
        """Analyze error patterns for a service."""
        query = f"""
        SELECT
            StatusMessage,
            count() as error_count,
            groupArray(TraceId)[1:5] as sample_traces
        FROM otel_traces
        WHERE ServiceName = '{service}'
          AND StatusCode = 'ERROR'
          AND Timestamp >= now() - INTERVAL {time_range}
        GROUP BY StatusMessage
        ORDER BY error_count DESC
        LIMIT 20
        """

        result = await self.storage.execute_query(query)
        errors = result[0] if result else []

        clusters = [
            {
                "message": e.get("StatusMessage"),
                "count": e.get("error_count"),
                "sample_traces": e.get("sample_traces", []),
            }
            for e in errors
        ]

        return ErrorPatternResult(
            summary=f"Found {len(clusters)} distinct error patterns",
            clusters=clusters,
            common_causes=["Check logs for detailed stack traces"],
            affected_endpoints=[],
        )

    async def analyze_impact(self, service: str) -> ImpactAnalysisResult:
        """Analyze impact of issues in a service."""
        # Get upstream dependencies
        upstream_query = f"""
        SELECT SourceService, RequestCount, ErrorRate, LatencyP99
        FROM service_topology
        WHERE TargetService = '{service}'
          AND Timestamp >= now() - INTERVAL 1 HOUR
        """

        upstream_result = await self.storage.execute_query(upstream_query)
        upstream = [
            {
                "service": r.get("SourceService"),
                "request_count": r.get("RequestCount"),
                "error_rate": r.get("ErrorRate"),
                "latency_p99": r.get("LatencyP99"),
            }
            for r in (upstream_result[0] if upstream_result else [])
        ]

        # Get downstream dependencies
        downstream_query = f"""
        SELECT TargetService, RequestCount, ErrorRate, LatencyP99
        FROM service_topology
        WHERE SourceService = '{service}'
          AND Timestamp >= now() - INTERVAL 1 HOUR
        """

        downstream_result = await self.storage.execute_query(downstream_query)
        downstream = [
            {
                "service": r.get("TargetService"),
                "request_count": r.get("RequestCount"),
                "error_rate": r.get("ErrorRate"),
                "latency_p99": r.get("LatencyP99"),
            }
            for r in (downstream_result[0] if downstream_result else [])
        ]

        return ImpactAnalysisResult(
            upstream=upstream,
            downstream=downstream,
            total_affected=len(upstream) + len(downstream),
            user_impact="Estimated based on upstream services",
        )

    async def find_similar_incidents(
        self,
        anomaly_id: Optional[str] = None,
        service: Optional[str] = None,
        symptom: Optional[str] = None,
        limit: int = 5,
    ) -> list[dict]:
        """Find similar historical incidents."""
        return await self._find_related_incidents(
            RCARequest(anomaly_id=anomaly_id, service=service, symptom=symptom),
            [],
        )

    def extract_common_patterns(self, incidents: list[dict]) -> list[str]:
        """Extract common patterns from incidents."""
        if not incidents:
            return []

        # Simple pattern extraction
        services = [i.get("service") for i in incidents if i.get("service")]
        types = [i.get("type") for i in incidents if i.get("type")]

        patterns = []
        if services:
            from collections import Counter
            common_services = Counter(services).most_common(3)
            for service, count in common_services:
                patterns.append(f"Service '{service}' appears in {count} incidents")

        return patterns
