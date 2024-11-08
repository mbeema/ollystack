"""
Prompt Templates

Structured prompts for observability AI tasks:
- Root cause analysis
- Incident summarization
- Natural language queries
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, Any


@dataclass
class PromptTemplate(ABC):
    """Base class for prompt templates."""

    @abstractmethod
    def render(self, **kwargs) -> str:
        """Render the prompt with given parameters."""
        pass

    @abstractmethod
    def get_system_message(self) -> str:
        """Get the system message for this prompt type."""
        pass


@dataclass
class RootCausePrompt(PromptTemplate):
    """
    Prompt template for root cause analysis.

    Analyzes anomalies, errors, and performance issues
    to identify probable root causes.
    """

    # Context sections
    anomaly_description: str = ""
    affected_services: list[str] = field(default_factory=list)
    error_logs: list[str] = field(default_factory=list)
    metrics_summary: dict = field(default_factory=dict)
    recent_changes: list[str] = field(default_factory=list)
    trace_data: Optional[dict] = None

    def get_system_message(self) -> str:
        return """You are an expert Site Reliability Engineer (SRE) and observability specialist.
Your task is to analyze system anomalies and identify root causes based on logs, metrics, and traces.

Guidelines:
1. Be systematic - check each layer (infrastructure, network, application, external)
2. Look for correlations between metrics, logs, and traces
3. Consider recent changes as potential causes
4. Prioritize causes by likelihood and evidence
5. Provide actionable remediation steps
6. Be concise but thorough

Always structure your response with:
- Root Cause Summary (1-2 sentences)
- Evidence (supporting data points)
- Confidence Level (high/medium/low with reasoning)
- Remediation Steps (actionable items)
- Prevention (how to avoid in future)"""

    def render(self, **kwargs) -> str:
        # Allow overrides
        anomaly = kwargs.get("anomaly_description", self.anomaly_description)
        services = kwargs.get("affected_services", self.affected_services)
        logs = kwargs.get("error_logs", self.error_logs)
        metrics = kwargs.get("metrics_summary", self.metrics_summary)
        changes = kwargs.get("recent_changes", self.recent_changes)
        traces = kwargs.get("trace_data", self.trace_data)

        prompt_parts = [
            "# Root Cause Analysis Request",
            "",
            "## Anomaly Description",
            anomaly or "No description provided",
            "",
        ]

        if services:
            prompt_parts.extend([
                "## Affected Services",
                "\n".join(f"- {s}" for s in services),
                "",
            ])

        if metrics:
            prompt_parts.extend([
                "## Metrics Summary",
                self._format_metrics(metrics),
                "",
            ])

        if logs:
            prompt_parts.extend([
                "## Recent Error Logs",
                "```",
                "\n".join(logs[:20]),  # Limit to 20 logs
                "```",
                "",
            ])

        if traces:
            prompt_parts.extend([
                "## Trace Analysis",
                self._format_trace(traces),
                "",
            ])

        if changes:
            prompt_parts.extend([
                "## Recent Changes",
                "\n".join(f"- {c}" for c in changes),
                "",
            ])

        prompt_parts.extend([
            "## Task",
            "Analyze the above information and identify the most likely root cause.",
            "Provide your analysis in the structured format specified.",
        ])

        return "\n".join(prompt_parts)

    def _format_metrics(self, metrics: dict) -> str:
        lines = []
        for name, data in metrics.items():
            if isinstance(data, dict):
                value = data.get("value", data.get("current", "N/A"))
                baseline = data.get("baseline", data.get("normal", "N/A"))
                lines.append(f"- {name}: {value} (baseline: {baseline})")
            else:
                lines.append(f"- {name}: {data}")
        return "\n".join(lines) if lines else "No metrics available"

    def _format_trace(self, trace: dict) -> str:
        lines = []
        if "duration_ms" in trace:
            lines.append(f"- Total duration: {trace['duration_ms']}ms")
        if "error" in trace:
            lines.append(f"- Error: {trace['error']}")
        if "spans" in trace:
            lines.append(f"- Span count: {len(trace['spans'])}")
            slow_spans = [s for s in trace["spans"] if s.get("duration_ms", 0) > 100]
            if slow_spans:
                lines.append("- Slow spans:")
                for span in slow_spans[:5]:
                    lines.append(f"  - {span.get('name', 'unknown')}: {span.get('duration_ms', 0)}ms")
        return "\n".join(lines) if lines else "No trace data available"


@dataclass
class IncidentSummaryPrompt(PromptTemplate):
    """
    Prompt template for incident summarization.

    Creates concise summaries of incidents for
    stakeholder communication and post-mortems.
    """

    incident_title: str = ""
    start_time: Optional[datetime] = None
    end_time: Optional[datetime] = None
    severity: str = "medium"
    affected_systems: list[str] = field(default_factory=list)
    timeline: list[dict] = field(default_factory=list)
    root_cause: str = ""
    resolution: str = ""
    metrics_impact: dict = field(default_factory=dict)

    def get_system_message(self) -> str:
        return """You are an expert technical writer specializing in incident communication.
Your task is to create clear, concise incident summaries for different audiences.

Guidelines:
1. Lead with impact - what users/business experienced
2. Be factual and avoid blame
3. Use plain language, minimize jargon
4. Include timeline with key events
5. Highlight lessons learned
6. Keep executive summary under 3 sentences

Structure your response as:
- Executive Summary (for leadership)
- Technical Summary (for engineers)
- Timeline (key events)
- Impact Assessment
- Lessons Learned"""

    def render(self, **kwargs) -> str:
        title = kwargs.get("incident_title", self.incident_title)
        start = kwargs.get("start_time", self.start_time)
        end = kwargs.get("end_time", self.end_time)
        severity = kwargs.get("severity", self.severity)
        systems = kwargs.get("affected_systems", self.affected_systems)
        timeline = kwargs.get("timeline", self.timeline)
        root_cause = kwargs.get("root_cause", self.root_cause)
        resolution = kwargs.get("resolution", self.resolution)
        impact = kwargs.get("metrics_impact", self.metrics_impact)

        prompt_parts = [
            "# Incident Summary Request",
            "",
            f"## Incident: {title or 'Untitled Incident'}",
            f"- Severity: {severity.upper()}",
        ]

        if start:
            prompt_parts.append(f"- Start Time: {start.isoformat()}")
        if end:
            prompt_parts.append(f"- End Time: {end.isoformat()}")
            if start:
                duration = end - start
                prompt_parts.append(f"- Duration: {duration}")

        prompt_parts.append("")

        if systems:
            prompt_parts.extend([
                "## Affected Systems",
                "\n".join(f"- {s}" for s in systems),
                "",
            ])

        if timeline:
            prompt_parts.extend([
                "## Timeline",
                self._format_timeline(timeline),
                "",
            ])

        if root_cause:
            prompt_parts.extend([
                "## Root Cause",
                root_cause,
                "",
            ])

        if resolution:
            prompt_parts.extend([
                "## Resolution",
                resolution,
                "",
            ])

        if impact:
            prompt_parts.extend([
                "## Impact Metrics",
                self._format_impact(impact),
                "",
            ])

        prompt_parts.extend([
            "## Task",
            "Create a comprehensive incident summary following the specified structure.",
            "Make it suitable for both technical and non-technical audiences.",
        ])

        return "\n".join(prompt_parts)

    def _format_timeline(self, timeline: list[dict]) -> str:
        lines = []
        for event in timeline:
            time = event.get("time", "")
            desc = event.get("description", event.get("event", ""))
            if time and desc:
                lines.append(f"- {time}: {desc}")
            elif desc:
                lines.append(f"- {desc}")
        return "\n".join(lines) if lines else "No timeline available"

    def _format_impact(self, impact: dict) -> str:
        lines = []
        for metric, value in impact.items():
            lines.append(f"- {metric}: {value}")
        return "\n".join(lines) if lines else "No impact data available"


@dataclass
class NaturalLanguageQueryPrompt(PromptTemplate):
    """
    Prompt template for natural language to query translation.

    Converts user questions into structured queries
    for logs, metrics, and traces.
    """

    user_question: str = ""
    available_metrics: list[str] = field(default_factory=list)
    available_services: list[str] = field(default_factory=list)
    time_context: Optional[str] = None
    query_type: str = "auto"  # auto, metrics, logs, traces

    def get_system_message(self) -> str:
        return """You are an expert in observability query languages.
Your task is to translate natural language questions into structured queries.

You understand:
- PromQL for metrics queries
- Log query syntax (similar to Loki/Elasticsearch)
- Trace query syntax

Guidelines:
1. Identify the intent (metrics, logs, traces, or combined)
2. Extract time ranges from the question
3. Identify services, metrics, and filters
4. Generate valid query syntax
5. Explain the query in plain language

Always respond with:
- Query Type (metrics/logs/traces/combined)
- Generated Query (in appropriate syntax)
- Explanation (what the query does)
- Suggested Visualizations (if applicable)
- Alternative Queries (if relevant)"""

    def render(self, **kwargs) -> str:
        question = kwargs.get("user_question", self.user_question)
        metrics = kwargs.get("available_metrics", self.available_metrics)
        services = kwargs.get("available_services", self.available_services)
        time_ctx = kwargs.get("time_context", self.time_context)
        query_type = kwargs.get("query_type", self.query_type)

        prompt_parts = [
            "# Natural Language Query Translation",
            "",
            "## User Question",
            f'"{question}"',
            "",
        ]

        if time_ctx:
            prompt_parts.extend([
                "## Time Context",
                time_ctx,
                "",
            ])

        if metrics:
            prompt_parts.extend([
                "## Available Metrics",
                "\n".join(f"- {m}" for m in metrics[:50]),  # Limit to 50
                "",
            ])

        if services:
            prompt_parts.extend([
                "## Available Services",
                "\n".join(f"- {s}" for s in services[:30]),  # Limit to 30
                "",
            ])

        if query_type != "auto":
            prompt_parts.extend([
                "## Query Type Constraint",
                f"Generate a {query_type} query specifically.",
                "",
            ])

        prompt_parts.extend([
            "## Task",
            "Translate the user question into appropriate observability queries.",
            "Follow the response structure specified in your instructions.",
        ])

        return "\n".join(prompt_parts)


@dataclass
class AnomalyExplanationPrompt(PromptTemplate):
    """
    Prompt template for explaining detected anomalies.

    Makes anomaly detection results understandable
    for operators and developers.
    """

    anomaly_type: str = ""
    metric_name: str = ""
    current_value: float = 0.0
    expected_value: float = 0.0
    deviation: float = 0.0
    historical_context: list[dict] = field(default_factory=list)
    correlated_events: list[str] = field(default_factory=list)

    def get_system_message(self) -> str:
        return """You are an observability expert explaining anomalies to engineers.
Your task is to make anomaly detection results understandable and actionable.

Guidelines:
1. Explain what normal looks like for this metric
2. Describe what the anomaly means in practical terms
3. Suggest possible causes based on the pattern
4. Recommend immediate actions if needed
5. Use analogies when helpful

Keep explanations concise but informative."""

    def render(self, **kwargs) -> str:
        atype = kwargs.get("anomaly_type", self.anomaly_type)
        metric = kwargs.get("metric_name", self.metric_name)
        current = kwargs.get("current_value", self.current_value)
        expected = kwargs.get("expected_value", self.expected_value)
        deviation = kwargs.get("deviation", self.deviation)
        history = kwargs.get("historical_context", self.historical_context)
        events = kwargs.get("correlated_events", self.correlated_events)

        prompt_parts = [
            "# Anomaly Explanation Request",
            "",
            "## Anomaly Details",
            f"- Type: {atype}",
            f"- Metric: {metric}",
            f"- Current Value: {current}",
            f"- Expected Value: {expected}",
            f"- Deviation: {deviation:.2f} standard deviations",
            "",
        ]

        if history:
            prompt_parts.extend([
                "## Historical Context",
                self._format_history(history),
                "",
            ])

        if events:
            prompt_parts.extend([
                "## Correlated Events",
                "\n".join(f"- {e}" for e in events),
                "",
            ])

        prompt_parts.extend([
            "## Task",
            "Explain this anomaly in plain language.",
            "Include what it means, possible causes, and recommended actions.",
        ])

        return "\n".join(prompt_parts)

    def _format_history(self, history: list[dict]) -> str:
        lines = []
        for h in history[:10]:
            time = h.get("time", "")
            value = h.get("value", "")
            lines.append(f"- {time}: {value}")
        return "\n".join(lines) if lines else "No history available"


@dataclass
class RunbookSuggestionPrompt(PromptTemplate):
    """
    Prompt template for suggesting relevant runbooks.

    Matches incidents to existing runbooks and
    suggests remediation steps.
    """

    incident_description: str = ""
    error_messages: list[str] = field(default_factory=list)
    affected_service: str = ""
    available_runbooks: list[dict] = field(default_factory=list)

    def get_system_message(self) -> str:
        return """You are an SRE expert helping operators find relevant runbooks.
Your task is to match incidents to runbooks and suggest remediation steps.

Guidelines:
1. Match based on symptoms, not just keywords
2. Rank runbooks by relevance
3. Highlight the most important steps
4. Suggest modifications if runbook isn't exact match
5. Identify gaps if no runbook matches

Always provide:
- Best matching runbook (if any)
- Relevance score and reasoning
- Key steps to follow
- Modifications needed
- Gaps in documentation"""

    def render(self, **kwargs) -> str:
        description = kwargs.get("incident_description", self.incident_description)
        errors = kwargs.get("error_messages", self.error_messages)
        service = kwargs.get("affected_service", self.affected_service)
        runbooks = kwargs.get("available_runbooks", self.available_runbooks)

        prompt_parts = [
            "# Runbook Suggestion Request",
            "",
            "## Incident Description",
            description or "No description provided",
            "",
        ]

        if service:
            prompt_parts.extend([
                "## Affected Service",
                service,
                "",
            ])

        if errors:
            prompt_parts.extend([
                "## Error Messages",
                "```",
                "\n".join(errors[:10]),
                "```",
                "",
            ])

        if runbooks:
            prompt_parts.extend([
                "## Available Runbooks",
                self._format_runbooks(runbooks),
                "",
            ])

        prompt_parts.extend([
            "## Task",
            "Find the most relevant runbook and provide guidance.",
            "If no exact match, suggest closest alternatives and manual steps.",
        ])

        return "\n".join(prompt_parts)

    def _format_runbooks(self, runbooks: list[dict]) -> str:
        lines = []
        for rb in runbooks[:20]:
            title = rb.get("title", "Untitled")
            tags = rb.get("tags", [])
            summary = rb.get("summary", "")[:100]
            lines.append(f"### {title}")
            if tags:
                lines.append(f"Tags: {', '.join(tags)}")
            if summary:
                lines.append(f"Summary: {summary}")
            lines.append("")
        return "\n".join(lines) if lines else "No runbooks available"
