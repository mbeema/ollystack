"""
Investigation Engine

Core engine for proactive AI investigations.
Automatically analyzes anomalies, correlates data, and generates hypotheses.
"""

import logging
import asyncio
from datetime import datetime, timedelta
from typing import Optional
import uuid

from ollystack_ai.services.llm import LLMService
from ollystack_ai.services.storage import StorageService
from ollystack_ai.services.cache import CacheService
from ollystack_ai.investigations.models import (
    Investigation,
    InvestigationStatus,
    InvestigationPhase,
    TriggerType,
    Severity,
    Hypothesis,
    HypothesisCategory,
    TimelineEvent,
    Evidence,
    EvidenceType,
)

logger = logging.getLogger(__name__)


class InvestigationEngine:
    """
    Proactive Investigation Engine.

    Automatically investigates anomalies and incidents by:
    1. Gathering relevant data (traces, metrics, logs, deployments)
    2. Correlating signals across different data sources
    3. Building an incident timeline
    4. Generating root cause hypotheses using AI
    5. Suggesting remediation actions
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
        self._running_investigations: dict[str, Investigation] = {}

    async def start_investigation(
        self,
        trigger_type: TriggerType,
        trigger_id: Optional[str] = None,
        service_name: Optional[str] = None,
        operation_name: Optional[str] = None,
        trigger_timestamp: Optional[datetime] = None,
        investigation_window: str = "1h",
        created_by: str = "system",
    ) -> Investigation:
        """
        Start a new investigation.

        Args:
            trigger_type: What triggered the investigation
            trigger_id: ID of the triggering entity (anomaly, alert, etc.)
            service_name: Service to investigate
            operation_name: Specific operation if known
            trigger_timestamp: When the trigger occurred
            investigation_window: How far back to investigate
            created_by: Who/what started the investigation

        Returns:
            The created Investigation object
        """
        now = datetime.utcnow()
        trigger_ts = trigger_timestamp or now

        # Parse investigation window
        window_delta = self._parse_duration(investigation_window)
        investigation_start = trigger_ts - window_delta
        investigation_end = trigger_ts + timedelta(minutes=5)  # Include some time after

        investigation = Investigation(
            id=str(uuid.uuid4()),
            created_at=now,
            updated_at=now,
            trigger_type=trigger_type,
            trigger_id=trigger_id,
            trigger_timestamp=trigger_ts,
            service_name=service_name,
            operation_name=operation_name,
            investigation_start=investigation_start,
            investigation_end=investigation_end,
            status=InvestigationStatus.RUNNING,
            phase=InvestigationPhase.INITIALIZING,
            created_by=created_by,
        )

        # Store in running investigations
        self._running_investigations[investigation.id] = investigation

        # Start the investigation asynchronously
        asyncio.create_task(self._run_investigation(investigation))

        return investigation

    async def _run_investigation(self, investigation: Investigation) -> None:
        """
        Run the full investigation workflow.

        Phases:
        1. Gather data from all sources
        2. Analyze traces for errors and latency
        3. Analyze metrics for anomalies
        4. Analyze logs for error patterns
        5. Check for recent deployments
        6. Correlate all signals
        7. Generate hypotheses
        """
        try:
            logger.info(f"Starting investigation {investigation.id}")

            # Phase 1: Gather initial data
            await self._update_phase(investigation, InvestigationPhase.GATHERING_DATA, 5)
            context = await self._gather_context(investigation)

            # Phase 2: Analyze traces
            await self._update_phase(investigation, InvestigationPhase.ANALYZING_TRACES, 15)
            trace_findings = await self._analyze_traces(investigation, context)

            # Phase 3: Analyze metrics
            await self._update_phase(investigation, InvestigationPhase.ANALYZING_METRICS, 30)
            metric_findings = await self._analyze_metrics(investigation, context)

            # Phase 4: Analyze logs
            await self._update_phase(investigation, InvestigationPhase.ANALYZING_LOGS, 45)
            log_findings = await self._analyze_logs(investigation, context)

            # Phase 5: Check deployments
            await self._update_phase(investigation, InvestigationPhase.CHECKING_DEPLOYMENTS, 55)
            deployment_findings = await self._check_deployments(investigation, context)

            # Phase 6: Correlate all signals
            await self._update_phase(investigation, InvestigationPhase.CORRELATING, 70)
            correlations = await self._correlate_signals(
                investigation,
                trace_findings,
                metric_findings,
                log_findings,
                deployment_findings,
            )

            # Phase 7: Generate hypotheses using AI
            await self._update_phase(investigation, InvestigationPhase.GENERATING_HYPOTHESES, 85)
            await self._generate_hypotheses(investigation, correlations)

            # Complete
            await self._complete_investigation(investigation)

        except Exception as e:
            logger.exception(f"Investigation {investigation.id} failed: {e}")
            investigation.status = InvestigationStatus.FAILED
            investigation.summary = f"Investigation failed: {str(e)}"
            investigation.updated_at = datetime.utcnow()

    async def _update_phase(
        self,
        investigation: Investigation,
        phase: InvestigationPhase,
        progress: int,
    ) -> None:
        """Update investigation phase and progress."""
        investigation.phase = phase
        investigation.progress_percent = progress
        investigation.updated_at = datetime.utcnow()
        logger.info(f"Investigation {investigation.id}: {phase.value} ({progress}%)")

    async def _gather_context(self, investigation: Investigation) -> dict:
        """Gather context data for investigation."""
        context = {
            "traces": [],
            "metrics": [],
            "logs": [],
            "topology": [],
            "deployments": [],
            "alerts": [],
        }

        time_filter = self._build_time_filter(
            investigation.investigation_start,
            investigation.investigation_end,
        )
        service_filter = f"AND ServiceName = '{investigation.service_name}'" if investigation.service_name else ""

        # Get error traces
        trace_query = f"""
        SELECT *
        FROM otel_traces
        WHERE {time_filter}
          {service_filter}
          AND (StatusCode = 'ERROR' OR Duration > 1000000000)
        ORDER BY Timestamp DESC
        LIMIT 500
        """
        traces_result = await self.storage.execute_query(trace_query)
        context["traces"] = traces_result[0] if traces_result else []

        # Count affected traces
        count_query = f"""
        SELECT count() as cnt
        FROM otel_traces
        WHERE {time_filter}
          {service_filter}
          AND StatusCode = 'ERROR'
        """
        count_result = await self.storage.execute_query(count_query)
        if count_result and count_result[0]:
            investigation.error_count = count_result[0][0].get("cnt", 0)

        # Get metrics anomalies
        metrics_query = f"""
        SELECT *
        FROM otel_metrics
        WHERE {time_filter}
          {service_filter}
          AND AnomalyScore > 0.5
        ORDER BY AnomalyScore DESC
        LIMIT 200
        """
        metrics_result = await self.storage.execute_query(metrics_query)
        context["metrics"] = metrics_result[0] if metrics_result else []

        # Get error logs
        logs_query = f"""
        SELECT *
        FROM otel_logs
        WHERE {time_filter}
          {service_filter}
          AND SeverityText IN ('ERROR', 'FATAL')
        ORDER BY Timestamp DESC
        LIMIT 200
        """
        logs_result = await self.storage.execute_query(logs_query)
        context["logs"] = logs_result[0] if logs_result else []

        # Get topology
        topology_query = f"""
        SELECT *
        FROM service_topology
        WHERE {time_filter}
        """
        topology_result = await self.storage.execute_query(topology_query)
        context["topology"] = topology_result[0] if topology_result else []

        # Get deployments
        deploy_query = f"""
        SELECT *
        FROM deployments
        WHERE Timestamp BETWEEN '{investigation.investigation_start.isoformat()}'
          AND '{investigation.investigation_end.isoformat()}'
        ORDER BY Timestamp DESC
        LIMIT 50
        """
        try:
            deploy_result = await self.storage.execute_query(deploy_query)
            context["deployments"] = deploy_result[0] if deploy_result else []
        except Exception:
            context["deployments"] = []

        return context

    async def _analyze_traces(
        self,
        investigation: Investigation,
        context: dict,
    ) -> dict:
        """Analyze traces for patterns and issues."""
        traces = context.get("traces", [])
        findings = {
            "error_traces": [],
            "slow_traces": [],
            "affected_services": set(),
            "affected_endpoints": set(),
            "error_patterns": {},
            "evidence": [],
        }

        for trace in traces:
            service = trace.get("ServiceName", "unknown")
            operation = trace.get("SpanName", "unknown")
            status = trace.get("StatusCode")
            duration = trace.get("Duration", 0)
            timestamp = trace.get("Timestamp")

            findings["affected_services"].add(service)
            findings["affected_endpoints"].add(f"{service}:{operation}")

            if status == "ERROR":
                findings["error_traces"].append(trace)

                # Track error patterns
                error_msg = trace.get("StatusMessage", "Unknown error")
                pattern_key = f"{service}:{error_msg[:50]}"
                if pattern_key not in findings["error_patterns"]:
                    findings["error_patterns"][pattern_key] = {
                        "service": service,
                        "message": error_msg,
                        "count": 0,
                        "trace_ids": [],
                    }
                findings["error_patterns"][pattern_key]["count"] += 1
                findings["error_patterns"][pattern_key]["trace_ids"].append(
                    trace.get("TraceId")
                )

                # Add timeline event
                investigation.timeline.append(TimelineEvent(
                    investigation_id=investigation.id,
                    timestamp=timestamp if isinstance(timestamp, datetime) else datetime.utcnow(),
                    event_type="error",
                    event_source="traces",
                    title=f"Error in {service}",
                    description=error_msg[:200],
                    severity=Severity.HIGH,
                    impact_score=0.8,
                    service_name=service,
                    trace_id=trace.get("TraceId"),
                ))

                # Add evidence
                findings["evidence"].append(Evidence(
                    investigation_id=investigation.id,
                    evidence_type=EvidenceType.TRACE,
                    title=f"Error trace in {service}",
                    description=error_msg[:200],
                    relevance_score=0.9,
                    source_service=service,
                    trace_id=trace.get("TraceId"),
                ))

            elif duration > 1000000000:  # > 1 second
                findings["slow_traces"].append(trace)

                investigation.timeline.append(TimelineEvent(
                    investigation_id=investigation.id,
                    timestamp=timestamp if isinstance(timestamp, datetime) else datetime.utcnow(),
                    event_type="latency_spike",
                    event_source="traces",
                    title=f"Slow request in {service}",
                    description=f"{operation} took {duration/1000000:.0f}ms",
                    severity=Severity.MEDIUM,
                    impact_score=0.6,
                    service_name=service,
                    trace_id=trace.get("TraceId"),
                ))

        # Update investigation
        investigation.affected_services = list(findings["affected_services"])
        investigation.affected_endpoints = list(findings["affected_endpoints"])[:20]
        investigation.affected_trace_count = len(findings["error_traces"])
        investigation.evidence.extend(findings["evidence"][:50])

        return findings

    async def _analyze_metrics(
        self,
        investigation: Investigation,
        context: dict,
    ) -> dict:
        """Analyze metrics for anomalies."""
        metrics = context.get("metrics", [])
        findings = {
            "anomalies": [],
            "by_service": {},
            "by_metric": {},
            "evidence": [],
        }

        for metric in metrics:
            service = metric.get("ServiceName", "unknown")
            metric_name = metric.get("MetricName", "unknown")
            value = metric.get("Value", 0)
            anomaly_score = metric.get("AnomalyScore", 0)
            timestamp = metric.get("Timestamp")

            if anomaly_score > 0.7:
                findings["anomalies"].append(metric)

                # Track by service
                if service not in findings["by_service"]:
                    findings["by_service"][service] = []
                findings["by_service"][service].append(metric)

                # Track by metric name
                if metric_name not in findings["by_metric"]:
                    findings["by_metric"][metric_name] = []
                findings["by_metric"][metric_name].append(metric)

                # Add timeline event
                investigation.timeline.append(TimelineEvent(
                    investigation_id=investigation.id,
                    timestamp=timestamp if isinstance(timestamp, datetime) else datetime.utcnow(),
                    event_type="metric_anomaly",
                    event_source="metrics",
                    title=f"Anomaly in {metric_name}",
                    description=f"Value: {value:.2f}, Anomaly score: {anomaly_score:.2f}",
                    severity=Severity.MEDIUM if anomaly_score < 0.9 else Severity.HIGH,
                    impact_score=anomaly_score,
                    service_name=service,
                    metric_name=metric_name,
                    metric_value=value,
                ))

                # Add evidence
                findings["evidence"].append(Evidence(
                    investigation_id=investigation.id,
                    evidence_type=EvidenceType.METRIC,
                    title=f"Metric anomaly: {metric_name}",
                    description=f"Anomaly score {anomaly_score:.2f} for {service}",
                    relevance_score=anomaly_score,
                    source_service=service,
                    metric_name=metric_name,
                    metric_value=value,
                ))

        investigation.evidence.extend(findings["evidence"][:30])
        return findings

    async def _analyze_logs(
        self,
        investigation: Investigation,
        context: dict,
    ) -> dict:
        """Analyze logs for error patterns."""
        logs = context.get("logs", [])
        findings = {
            "error_logs": [],
            "patterns": {},
            "by_service": {},
            "evidence": [],
        }

        for log in logs:
            service = log.get("ServiceName", "unknown")
            body = log.get("Body", "")
            severity = log.get("SeverityText", "INFO")
            timestamp = log.get("Timestamp")
            trace_id = log.get("TraceId")

            if severity in ("ERROR", "FATAL"):
                findings["error_logs"].append(log)

                # Track by service
                if service not in findings["by_service"]:
                    findings["by_service"][service] = []
                findings["by_service"][service].append(log)

                # Simple pattern extraction (first 100 chars)
                pattern = body[:100] if body else "Unknown"
                if pattern not in findings["patterns"]:
                    findings["patterns"][pattern] = {
                        "count": 0,
                        "services": set(),
                        "sample_log": body[:500],
                    }
                findings["patterns"][pattern]["count"] += 1
                findings["patterns"][pattern]["services"].add(service)

                # Add timeline event for first occurrence of each pattern
                if findings["patterns"][pattern]["count"] == 1:
                    investigation.timeline.append(TimelineEvent(
                        investigation_id=investigation.id,
                        timestamp=timestamp if isinstance(timestamp, datetime) else datetime.utcnow(),
                        event_type="error_log",
                        event_source="logs",
                        title=f"Error log in {service}",
                        description=body[:200] if body else "No message",
                        severity=Severity.HIGH if severity == "FATAL" else Severity.MEDIUM,
                        impact_score=0.7,
                        service_name=service,
                        trace_id=trace_id,
                        log_message=body[:500] if body else None,
                    ))

                # Add evidence
                findings["evidence"].append(Evidence(
                    investigation_id=investigation.id,
                    evidence_type=EvidenceType.LOG,
                    title=f"Error log in {service}",
                    description=body[:200] if body else "No message",
                    relevance_score=0.8,
                    source_service=service,
                    trace_id=trace_id,
                    log_body=body[:500] if body else None,
                ))

        investigation.evidence.extend(findings["evidence"][:30])
        return findings

    async def _check_deployments(
        self,
        investigation: Investigation,
        context: dict,
    ) -> dict:
        """Check for recent deployments that might correlate with the issue."""
        deployments = context.get("deployments", [])
        findings = {
            "recent_deployments": [],
            "suspicious_deployments": [],
            "by_service": {},
            "evidence": [],
        }

        for deploy in deployments:
            service = deploy.get("ServiceName", "unknown")
            version = deploy.get("Version", "unknown")
            timestamp = deploy.get("Timestamp")
            status = deploy.get("Status", "unknown")
            commit_msg = deploy.get("CommitMessage", "")

            findings["recent_deployments"].append(deploy)

            # Track by service
            if service not in findings["by_service"]:
                findings["by_service"][service] = []
            findings["by_service"][service].append(deploy)

            # Check if deployment is for an affected service
            if service in investigation.affected_services:
                findings["suspicious_deployments"].append(deploy)

                # Add timeline event
                investigation.timeline.append(TimelineEvent(
                    investigation_id=investigation.id,
                    timestamp=timestamp if isinstance(timestamp, datetime) else datetime.utcnow(),
                    event_type="deployment",
                    event_source="deployments",
                    title=f"Deployment: {service} v{version}",
                    description=commit_msg[:200] if commit_msg else "No commit message",
                    severity=Severity.INFO,
                    impact_score=0.9 if service in investigation.affected_services else 0.3,
                    service_name=service,
                ))

                # Add evidence
                findings["evidence"].append(Evidence(
                    investigation_id=investigation.id,
                    evidence_type=EvidenceType.DEPLOYMENT,
                    title=f"Recent deployment: {service} v{version}",
                    description=f"Status: {status}. {commit_msg[:100] if commit_msg else ''}",
                    relevance_score=0.9,
                    source_service=service,
                    raw_data={
                        "version": version,
                        "previous_version": deploy.get("PreviousVersion"),
                        "commit_hash": deploy.get("CommitHash"),
                        "deployed_by": deploy.get("DeployedBy"),
                    },
                ))

        investigation.evidence.extend(findings["evidence"][:20])
        return findings

    async def _correlate_signals(
        self,
        investigation: Investigation,
        trace_findings: dict,
        metric_findings: dict,
        log_findings: dict,
        deployment_findings: dict,
    ) -> dict:
        """Correlate signals from all sources."""
        correlations = {
            "error_metric_correlation": [],
            "deployment_error_correlation": [],
            "service_impact_chain": [],
            "timeline_clusters": [],
        }

        # Check if deployments correlate with errors
        suspicious_deploys = deployment_findings.get("suspicious_deployments", [])
        error_patterns = trace_findings.get("error_patterns", {})

        if suspicious_deploys and error_patterns:
            for deploy in suspicious_deploys:
                service = deploy.get("ServiceName")
                for pattern_key, pattern_data in error_patterns.items():
                    if pattern_data["service"] == service:
                        correlations["deployment_error_correlation"].append({
                            "deployment": deploy,
                            "error_pattern": pattern_data,
                            "correlation_strength": 0.85,
                        })

        # Check if metric anomalies correlate with errors
        metric_anomalies = metric_findings.get("by_service", {})
        for service, errors in trace_findings.get("error_patterns", {}).items():
            if service.split(":")[0] in metric_anomalies:
                correlations["error_metric_correlation"].append({
                    "service": service.split(":")[0],
                    "has_metric_anomaly": True,
                    "has_errors": True,
                })

        # Sort timeline by timestamp
        investigation.timeline.sort(key=lambda x: x.timestamp)

        return correlations

    async def _generate_hypotheses(
        self,
        investigation: Investigation,
        correlations: dict,
    ) -> None:
        """Generate root cause hypotheses using AI."""
        # Build context for LLM
        context = self._build_hypothesis_context(investigation, correlations)

        # Call LLM to generate hypotheses
        prompt = f"""Based on the following investigation data, generate root cause hypotheses.

Investigation Context:
{context}

Generate 3-5 hypotheses for the root cause, ranked by likelihood.
For each hypothesis provide:
1. A clear title
2. Detailed description
3. Category (infrastructure, code, dependency, configuration, capacity, database, network, external)
4. Confidence score (0.0-1.0)
5. Reasoning
6. Suggested actions to verify or fix

Format each hypothesis as:
HYPOTHESIS #N:
Title: <title>
Category: <category>
Confidence: <score>
Description: <description>
Reasoning: <reasoning>
Actions:
- <action 1>
- <action 2>
"""

        response = await self.llm.complete(
            system_prompt="You are an expert SRE performing root cause analysis. Be specific and actionable.",
            user_prompt=prompt,
            temperature=0.3,
        )

        # Parse hypotheses from response
        hypotheses = self._parse_hypotheses(response, investigation.id)

        # Add correlation-based hypotheses
        deployment_correlations = correlations.get("deployment_error_correlation", [])
        if deployment_correlations:
            for i, corr in enumerate(deployment_correlations[:2]):
                deploy = corr["deployment"]
                hypothesis = Hypothesis(
                    investigation_id=investigation.id,
                    rank=len(hypotheses) + 1,
                    title=f"Recent deployment of {deploy.get('ServiceName')} caused the issue",
                    description=f"A deployment of {deploy.get('ServiceName')} version {deploy.get('Version')} "
                                f"occurred shortly before errors started. This is a strong correlation.",
                    category=HypothesisCategory.CODE,
                    confidence=corr["correlation_strength"],
                    reasoning="Timeline correlation between deployment and error spike",
                    related_services=[deploy.get("ServiceName")],
                    related_deployments=[str(deploy.get("DeploymentId", ""))],
                    suggested_actions=[
                        f"Review changes in {deploy.get('Version')}",
                        f"Consider rolling back to {deploy.get('PreviousVersion', 'previous version')}",
                        "Check deployment logs for any issues",
                    ],
                )
                hypotheses.append(hypothesis)

        # Sort by confidence and assign ranks
        hypotheses.sort(key=lambda h: h.confidence, reverse=True)
        for i, h in enumerate(hypotheses):
            h.rank = i + 1

        investigation.hypotheses = hypotheses[:5]  # Top 5

        # Set investigation title and summary
        if hypotheses:
            top = hypotheses[0]
            investigation.title = f"Investigation: {top.title}"
            investigation.overall_confidence = top.confidence
        else:
            investigation.title = f"Investigation: Issues in {investigation.service_name or 'multiple services'}"

    def _build_hypothesis_context(
        self,
        investigation: Investigation,
        correlations: dict,
    ) -> str:
        """Build context string for hypothesis generation."""
        parts = []

        # Service info
        parts.append(f"Primary Service: {investigation.service_name or 'Multiple'}")
        parts.append(f"Affected Services: {', '.join(investigation.affected_services[:10])}")
        parts.append(f"Error Count: {investigation.error_count}")
        parts.append(f"Time Range: {investigation.investigation_start} to {investigation.investigation_end}")
        parts.append("")

        # Timeline summary
        parts.append("Recent Events (chronological):")
        for event in investigation.timeline[:20]:
            parts.append(f"  - [{event.event_type}] {event.title}: {event.description[:100]}")
        parts.append("")

        # Evidence summary
        parts.append("Key Evidence:")
        for evidence in investigation.evidence[:15]:
            parts.append(f"  - [{evidence.evidence_type.value}] {evidence.title}")
        parts.append("")

        # Correlations
        if correlations.get("deployment_error_correlation"):
            parts.append("Deployment Correlations Found:")
            for corr in correlations["deployment_error_correlation"]:
                deploy = corr["deployment"]
                parts.append(f"  - {deploy.get('ServiceName')} deployed v{deploy.get('Version')}")

        return "\n".join(parts)

    def _parse_hypotheses(self, response: str, investigation_id: str) -> list[Hypothesis]:
        """Parse hypotheses from LLM response."""
        hypotheses = []
        current = None

        for line in response.split("\n"):
            line = line.strip()

            if line.startswith("HYPOTHESIS #"):
                if current:
                    hypotheses.append(current)
                current = Hypothesis(investigation_id=investigation_id)

            elif current:
                if line.startswith("Title:"):
                    current.title = line[6:].strip()
                elif line.startswith("Category:"):
                    cat = line[9:].strip().lower()
                    try:
                        current.category = HypothesisCategory(cat)
                    except ValueError:
                        current.category = HypothesisCategory.CODE
                elif line.startswith("Confidence:"):
                    try:
                        current.confidence = float(line[11:].strip())
                    except ValueError:
                        current.confidence = 0.5
                elif line.startswith("Description:"):
                    current.description = line[12:].strip()
                elif line.startswith("Reasoning:"):
                    current.reasoning = line[10:].strip()
                elif line.startswith("- "):
                    current.suggested_actions.append(line[2:])

        if current and current.title:
            hypotheses.append(current)

        return hypotheses

    async def _complete_investigation(self, investigation: Investigation) -> None:
        """Complete the investigation."""
        investigation.status = InvestigationStatus.COMPLETED
        investigation.phase = InvestigationPhase.COMPLETE
        investigation.progress_percent = 100
        investigation.updated_at = datetime.utcnow()

        # Generate summary
        summary_parts = []

        if investigation.hypotheses:
            top = investigation.hypotheses[0]
            summary_parts.append(f"Most likely cause: {top.title} (confidence: {top.confidence:.0%})")

        summary_parts.append(f"Analyzed {investigation.error_count} errors across {len(investigation.affected_services)} services.")

        if investigation.hypotheses:
            summary_parts.append(f"Generated {len(investigation.hypotheses)} hypotheses.")

        investigation.summary = " ".join(summary_parts)

        # Determine severity
        if investigation.error_count > 100:
            investigation.severity = Severity.CRITICAL
        elif investigation.error_count > 20:
            investigation.severity = Severity.HIGH
        elif investigation.error_count > 5:
            investigation.severity = Severity.MEDIUM
        else:
            investigation.severity = Severity.LOW

        logger.info(f"Investigation {investigation.id} completed: {investigation.summary}")

    async def get_investigation(self, investigation_id: str) -> Optional[Investigation]:
        """Get an investigation by ID."""
        # Check in-memory first
        if investigation_id in self._running_investigations:
            return self._running_investigations[investigation_id]

        # Try to load from storage
        try:
            query = f"""
            SELECT *
            FROM investigations
            WHERE InvestigationId = '{investigation_id}'
            LIMIT 1
            """
            result = await self.storage.execute_query(query)
            if result and result[0]:
                return self._row_to_investigation(result[0][0])
        except Exception:
            pass

        return None

    async def list_investigations(
        self,
        status: Optional[str] = None,
        service: Optional[str] = None,
        trigger_type: Optional[str] = None,
        limit: int = 20,
        offset: int = 0,
    ) -> tuple[list[Investigation], int]:
        """List recent investigations with filters."""
        investigations = list(self._running_investigations.values())

        # Apply filters
        if status:
            investigations = [i for i in investigations if i.status.value == status]
        if service:
            investigations = [i for i in investigations if i.service_name == service]
        if trigger_type:
            investigations = [i for i in investigations if i.trigger_type.value == trigger_type]

        # Sort by creation time
        investigations.sort(key=lambda i: i.created_at, reverse=True)

        total = len(investigations)
        return investigations[offset:offset + limit], total

    async def cancel_investigation(self, investigation_id: str) -> bool:
        """Cancel a running investigation."""
        investigation = self._running_investigations.get(investigation_id)
        if investigation and investigation.status == InvestigationStatus.RUNNING:
            investigation.status = InvestigationStatus.CANCELLED
            investigation.updated_at = datetime.utcnow()
            return True
        return False

    async def resolve_investigation(
        self,
        investigation_id: str,
        resolution: str,
        resolved_by: str = "user",
    ) -> bool:
        """Mark an investigation as resolved."""
        investigation = self._running_investigations.get(investigation_id)
        if not investigation:
            return False

        investigation.resolved_at = datetime.utcnow()
        investigation.resolved_by = resolved_by
        investigation.resolution = resolution
        investigation.updated_at = datetime.utcnow()

        # Persist to storage
        try:
            await self._save_investigation(investigation)
        except Exception as e:
            logger.error(f"Failed to persist resolution: {e}")

        return True

    async def verify_hypothesis(
        self,
        investigation_id: str,
        hypothesis_id: str,
        verified: bool,
        verified_by: str = "user",
        notes: Optional[str] = None,
    ) -> bool:
        """Verify or reject a hypothesis."""
        investigation = self._running_investigations.get(investigation_id)
        if not investigation:
            return False

        for hypothesis in investigation.hypotheses:
            if hypothesis.id == hypothesis_id:
                hypothesis.verified = verified
                hypothesis.verified_by = verified_by
                hypothesis.verified_at = datetime.utcnow()
                hypothesis.verification_notes = notes
                investigation.updated_at = datetime.utcnow()
                return True

        return False

    async def get_investigation_stats(self, time_range: str = "24h") -> dict:
        """Get investigation statistics."""
        delta = self._parse_duration(time_range)
        cutoff = datetime.utcnow() - delta

        investigations = [
            i for i in self._running_investigations.values()
            if i.created_at >= cutoff
        ]

        # Count by status
        status_counts = {}
        for status in InvestigationStatus:
            status_counts[status.value] = len([
                i for i in investigations if i.status == status
            ])

        # Count by trigger type
        trigger_counts = {}
        for trigger in TriggerType:
            trigger_counts[trigger.value] = len([
                i for i in investigations if i.trigger_type == trigger
            ])

        # Count by severity
        severity_counts = {}
        for severity in Severity:
            severity_counts[severity.value] = len([
                i for i in investigations if i.severity == severity
            ])

        # Calculate average duration for completed investigations
        completed = [
            i for i in investigations
            if i.status == InvestigationStatus.COMPLETED
        ]
        avg_duration = 0
        if completed:
            durations = [
                (i.updated_at - i.created_at).total_seconds()
                for i in completed
            ]
            avg_duration = sum(durations) / len(durations)

        # Top affected services
        service_counts = {}
        for inv in investigations:
            for svc in inv.affected_services:
                service_counts[svc] = service_counts.get(svc, 0) + 1

        top_services = sorted(
            service_counts.items(),
            key=lambda x: x[1],
            reverse=True
        )[:10]

        return {
            "time_range": time_range,
            "total_investigations": len(investigations),
            "by_status": status_counts,
            "by_trigger_type": trigger_counts,
            "by_severity": severity_counts,
            "average_duration_seconds": avg_duration,
            "top_affected_services": dict(top_services),
            "total_hypotheses_generated": sum(
                len(i.hypotheses) for i in investigations
            ),
            "verified_hypotheses": sum(
                len([h for h in i.hypotheses if h.verified])
                for i in investigations
            ),
        }

    async def get_trigger_stats(self, time_range: str = "7d") -> dict:
        """Get trigger performance statistics."""
        delta = self._parse_duration(time_range)
        cutoff = datetime.utcnow() - delta

        investigations = [
            i for i in self._running_investigations.values()
            if i.created_at >= cutoff
        ]

        # Stats per trigger type
        trigger_stats = {}
        for trigger in TriggerType:
            trigger_invs = [i for i in investigations if i.trigger_type == trigger]

            if not trigger_invs:
                continue

            completed = [i for i in trigger_invs if i.status == InvestigationStatus.COMPLETED]
            verified = [
                i for i in completed
                if any(h.verified for h in i.hypotheses)
            ]

            trigger_stats[trigger.value] = {
                "total_triggered": len(trigger_invs),
                "completed": len(completed),
                "failed": len([i for i in trigger_invs if i.status == InvestigationStatus.FAILED]),
                "accuracy_rate": len(verified) / len(completed) if completed else 0,
                "avg_confidence": sum(
                    i.overall_confidence for i in completed
                ) / len(completed) if completed else 0,
            }

        return {
            "time_range": time_range,
            "trigger_stats": trigger_stats,
        }

    async def _save_investigation(self, investigation: Investigation) -> None:
        """Save investigation to storage."""
        # This would persist to ClickHouse
        # For now, just keep in memory
        pass

    def _row_to_investigation(self, row: dict) -> Investigation:
        """Convert database row to Investigation object."""
        return Investigation(
            id=str(row.get("InvestigationId", "")),
            created_at=row.get("CreatedAt", datetime.utcnow()),
            updated_at=row.get("UpdatedAt", datetime.utcnow()),
            trigger_type=TriggerType(row.get("TriggerType", "manual")),
            trigger_id=row.get("TriggerId"),
            service_name=row.get("ServiceName"),
            status=InvestigationStatus(row.get("Status", "pending")),
            phase=InvestigationPhase(row.get("Phase", "initializing")),
            progress_percent=row.get("ProgressPercent", 0),
            title=row.get("Title", ""),
            summary=row.get("Summary", ""),
            severity=Severity(row.get("Severity", "medium")),
            affected_services=row.get("AffectedServices", []),
            overall_confidence=row.get("OverallConfidence", 0),
        )

    def _build_time_filter(
        self,
        start: Optional[datetime],
        end: Optional[datetime],
    ) -> str:
        """Build SQL time filter."""
        if start and end:
            return f"Timestamp BETWEEN '{start.isoformat()}' AND '{end.isoformat()}'"
        elif start:
            return f"Timestamp >= '{start.isoformat()}'"
        else:
            return "Timestamp >= now() - INTERVAL 1 HOUR"

    def _parse_duration(self, duration: str) -> timedelta:
        """Parse duration string to timedelta."""
        duration = duration.strip().lower()
        if duration.endswith("m"):
            return timedelta(minutes=int(duration[:-1]))
        elif duration.endswith("h"):
            return timedelta(hours=int(duration[:-1]))
        elif duration.endswith("d"):
            return timedelta(days=int(duration[:-1]))
        else:
            return timedelta(hours=1)
