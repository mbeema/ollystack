"""
LLM-Powered Analyzers

High-level analyzers that use LLMs for:
- Root cause analysis
- Incident summarization
- Natural language querying
"""

import logging
import json
import re
from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, Any

from ollystack_ai.llm.client import LLMClient, LLMProvider, Message, LLMResponse
from ollystack_ai.llm.prompts import (
    RootCausePrompt,
    IncidentSummaryPrompt,
    NaturalLanguageQueryPrompt,
    AnomalyExplanationPrompt,
    RunbookSuggestionPrompt,
)

logger = logging.getLogger(__name__)


@dataclass
class RootCauseResult:
    """Result of root cause analysis."""

    summary: str
    confidence: str  # high, medium, low
    evidence: list[str]
    remediation_steps: list[str]
    prevention_steps: list[str]
    raw_response: str
    model: str
    latency_ms: float

    def to_dict(self) -> dict:
        return {
            "summary": self.summary,
            "confidence": self.confidence,
            "evidence": self.evidence,
            "remediation_steps": self.remediation_steps,
            "prevention_steps": self.prevention_steps,
            "model": self.model,
            "latency_ms": self.latency_ms,
        }


@dataclass
class IncidentSummary:
    """Result of incident summarization."""

    executive_summary: str
    technical_summary: str
    timeline: list[dict]
    impact_assessment: str
    lessons_learned: list[str]
    raw_response: str
    model: str
    latency_ms: float

    def to_dict(self) -> dict:
        return {
            "executive_summary": self.executive_summary,
            "technical_summary": self.technical_summary,
            "timeline": self.timeline,
            "impact_assessment": self.impact_assessment,
            "lessons_learned": self.lessons_learned,
            "model": self.model,
            "latency_ms": self.latency_ms,
        }


@dataclass
class QueryTranslation:
    """Result of natural language query translation."""

    query_type: str  # metrics, logs, traces, combined
    query: str
    explanation: str
    visualizations: list[str]
    alternatives: list[dict]
    raw_response: str
    model: str
    latency_ms: float

    def to_dict(self) -> dict:
        return {
            "query_type": self.query_type,
            "query": self.query,
            "explanation": self.explanation,
            "visualizations": self.visualizations,
            "alternatives": self.alternatives,
            "model": self.model,
            "latency_ms": self.latency_ms,
        }


class RootCauseAnalyzer:
    """
    Analyzes anomalies and incidents to identify root causes.

    Uses LLM to correlate logs, metrics, and traces
    and identify the most likely root cause.
    """

    def __init__(
        self,
        client: Optional[LLMClient] = None,
        provider: LLMProvider = LLMProvider.OPENAI,
        model: Optional[str] = None,
    ):
        """
        Initialize the analyzer.

        Args:
            client: LLM client (creates one if None)
            provider: LLM provider if creating client
            model: Model name if creating client
        """
        self.client = client or LLMClient(provider=provider, model=model)
        self.prompt_template = RootCausePrompt()

    async def analyze(
        self,
        anomaly_description: str,
        affected_services: Optional[list[str]] = None,
        error_logs: Optional[list[str]] = None,
        metrics_summary: Optional[dict] = None,
        recent_changes: Optional[list[str]] = None,
        trace_data: Optional[dict] = None,
        temperature: float = 0.3,
    ) -> RootCauseResult:
        """
        Analyze an incident to identify root cause.

        Args:
            anomaly_description: Description of the anomaly/incident
            affected_services: List of affected service names
            error_logs: Recent error log messages
            metrics_summary: Summary of relevant metrics
            recent_changes: Recent deployments/changes
            trace_data: Relevant trace information
            temperature: LLM temperature (lower = more focused)

        Returns:
            RootCauseResult with analysis
        """
        # Build prompt
        user_prompt = self.prompt_template.render(
            anomaly_description=anomaly_description,
            affected_services=affected_services or [],
            error_logs=error_logs or [],
            metrics_summary=metrics_summary or {},
            recent_changes=recent_changes or [],
            trace_data=trace_data,
        )

        messages = [
            Message(role="system", content=self.prompt_template.get_system_message()),
            Message(role="user", content=user_prompt),
        ]

        # Get LLM response
        response = await self.client.chat(messages, temperature=temperature)

        # Parse response
        return self._parse_response(response)

    def _parse_response(self, response: LLMResponse) -> RootCauseResult:
        """Parse LLM response into structured result."""
        content = response.content

        # Extract sections using patterns
        summary = self._extract_section(content, "Root Cause Summary", "Evidence")
        evidence = self._extract_list(content, "Evidence", "Confidence")
        confidence = self._extract_confidence(content)
        remediation = self._extract_list(content, "Remediation", "Prevention")
        prevention = self._extract_list(content, "Prevention", None)

        # Fallbacks if parsing fails
        if not summary:
            summary = content.split("\n")[0] if content else "Unable to determine root cause"

        return RootCauseResult(
            summary=summary.strip(),
            confidence=confidence,
            evidence=evidence or ["No specific evidence extracted"],
            remediation_steps=remediation or ["Investigate further"],
            prevention_steps=prevention or [],
            raw_response=content,
            model=response.model,
            latency_ms=response.latency_ms,
        )

    def _extract_section(
        self,
        content: str,
        start_marker: str,
        end_marker: Optional[str],
    ) -> str:
        """Extract text between section markers."""
        patterns = [
            rf"(?:#{1,3}\s*)?{start_marker}[:\s]*\n?(.*?)(?=(?:#{1,3}\s*)?{end_marker}|\Z)",
            rf"\*\*{start_marker}\*\*[:\s]*(.*?)(?=\*\*{end_marker}\*\*|\Z)" if end_marker else rf"\*\*{start_marker}\*\*[:\s]*(.*)",
        ]

        for pattern in patterns:
            match = re.search(pattern, content, re.IGNORECASE | re.DOTALL)
            if match:
                return match.group(1).strip()

        return ""

    def _extract_list(
        self,
        content: str,
        start_marker: str,
        end_marker: Optional[str],
    ) -> list[str]:
        """Extract a bulleted list from a section."""
        section = self._extract_section(content, start_marker, end_marker)
        if not section:
            return []

        # Extract bullet points
        items = re.findall(r"[-•*]\s*(.+?)(?=\n[-•*]|\n\n|\Z)", section, re.DOTALL)
        # Also try numbered items
        if not items:
            items = re.findall(r"\d+[.)]\s*(.+?)(?=\n\d+[.)]|\n\n|\Z)", section, re.DOTALL)

        return [item.strip() for item in items if item.strip()]

    def _extract_confidence(self, content: str) -> str:
        """Extract confidence level from response."""
        content_lower = content.lower()

        # Look for explicit confidence
        match = re.search(r"confidence[:\s]*(high|medium|low)", content_lower)
        if match:
            return match.group(1)

        # Infer from language
        if any(word in content_lower for word in ["definitely", "clearly", "certain", "strong evidence"]):
            return "high"
        elif any(word in content_lower for word in ["likely", "probably", "suggests"]):
            return "medium"
        else:
            return "low"


class IncidentSummarizer:
    """
    Generates incident summaries for different audiences.

    Creates executive and technical summaries
    suitable for post-mortems and stakeholder communication.
    """

    def __init__(
        self,
        client: Optional[LLMClient] = None,
        provider: LLMProvider = LLMProvider.OPENAI,
        model: Optional[str] = None,
    ):
        self.client = client or LLMClient(provider=provider, model=model)
        self.prompt_template = IncidentSummaryPrompt()

    async def summarize(
        self,
        incident_title: str,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        severity: str = "medium",
        affected_systems: Optional[list[str]] = None,
        timeline: Optional[list[dict]] = None,
        root_cause: str = "",
        resolution: str = "",
        metrics_impact: Optional[dict] = None,
        temperature: float = 0.5,
    ) -> IncidentSummary:
        """
        Generate incident summary.

        Args:
            incident_title: Title of the incident
            start_time: When incident started
            end_time: When incident was resolved
            severity: Severity level
            affected_systems: List of affected systems
            timeline: List of timeline events
            root_cause: Root cause description
            resolution: How it was resolved
            metrics_impact: Impact metrics
            temperature: LLM temperature

        Returns:
            IncidentSummary with formatted summaries
        """
        user_prompt = self.prompt_template.render(
            incident_title=incident_title,
            start_time=start_time,
            end_time=end_time,
            severity=severity,
            affected_systems=affected_systems or [],
            timeline=timeline or [],
            root_cause=root_cause,
            resolution=resolution,
            metrics_impact=metrics_impact or {},
        )

        messages = [
            Message(role="system", content=self.prompt_template.get_system_message()),
            Message(role="user", content=user_prompt),
        ]

        response = await self.client.chat(messages, temperature=temperature)

        return self._parse_response(response)

    def _parse_response(self, response: LLMResponse) -> IncidentSummary:
        """Parse LLM response into structured summary."""
        content = response.content

        # Extract sections
        exec_summary = self._extract_section(content, "Executive Summary", "Technical Summary")
        tech_summary = self._extract_section(content, "Technical Summary", "Timeline")
        timeline_text = self._extract_section(content, "Timeline", "Impact")
        impact = self._extract_section(content, "Impact", "Lessons")
        lessons = self._extract_list(content, "Lessons Learned")

        # Parse timeline
        timeline = self._parse_timeline(timeline_text)

        # Fallbacks
        if not exec_summary:
            exec_summary = content.split("\n\n")[0] if content else "Incident occurred"

        return IncidentSummary(
            executive_summary=exec_summary.strip(),
            technical_summary=tech_summary.strip() or exec_summary.strip(),
            timeline=timeline,
            impact_assessment=impact.strip() or "Impact under assessment",
            lessons_learned=lessons or [],
            raw_response=content,
            model=response.model,
            latency_ms=response.latency_ms,
        )

    def _extract_section(
        self,
        content: str,
        start_marker: str,
        end_marker: Optional[str],
    ) -> str:
        """Extract section text."""
        pattern = rf"(?:#{1,3}\s*)?{start_marker}[:\s]*\n?(.*?)(?=(?:#{1,3}\s*)?{end_marker}|\Z)" if end_marker else rf"(?:#{1,3}\s*)?{start_marker}[:\s]*\n?(.*)"
        match = re.search(pattern, content, re.IGNORECASE | re.DOTALL)
        return match.group(1).strip() if match else ""

    def _extract_list(self, content: str, section: str) -> list[str]:
        """Extract list items from section."""
        section_text = self._extract_section(content, section, None)
        if not section_text:
            return []

        items = re.findall(r"[-•*]\s*(.+?)(?=\n[-•*]|\n\n|\Z)", section_text, re.DOTALL)
        if not items:
            items = re.findall(r"\d+[.)]\s*(.+?)(?=\n\d+[.)]|\n\n|\Z)", section_text, re.DOTALL)

        return [item.strip() for item in items if item.strip()]

    def _parse_timeline(self, text: str) -> list[dict]:
        """Parse timeline text into structured events."""
        events = []
        # Match patterns like "10:30 - Event happened" or "- 10:30: Event"
        patterns = [
            r"[-•*]?\s*(\d{1,2}:\d{2}(?::\d{2})?(?:\s*[AP]M)?)\s*[-:]\s*(.+?)(?=\n|$)",
            r"[-•*]?\s*(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2})\s*[-:]\s*(.+?)(?=\n|$)",
        ]

        for pattern in patterns:
            matches = re.findall(pattern, text, re.IGNORECASE)
            for time, event in matches:
                events.append({"time": time.strip(), "event": event.strip()})

        return events


class NaturalLanguageQuerier:
    """
    Translates natural language questions into queries.

    Converts plain English questions into
    PromQL, log queries, and trace queries.
    """

    def __init__(
        self,
        client: Optional[LLMClient] = None,
        provider: LLMProvider = LLMProvider.OPENAI,
        model: Optional[str] = None,
    ):
        self.client = client or LLMClient(provider=provider, model=model)
        self.prompt_template = NaturalLanguageQueryPrompt()

    async def translate(
        self,
        question: str,
        available_metrics: Optional[list[str]] = None,
        available_services: Optional[list[str]] = None,
        time_context: Optional[str] = None,
        query_type: str = "auto",
        temperature: float = 0.2,
    ) -> QueryTranslation:
        """
        Translate natural language to query.

        Args:
            question: User's natural language question
            available_metrics: List of available metric names
            available_services: List of available services
            time_context: Time context (e.g., "last 1 hour")
            query_type: Constraint on query type
            temperature: LLM temperature (lower = more precise)

        Returns:
            QueryTranslation with generated query
        """
        user_prompt = self.prompt_template.render(
            user_question=question,
            available_metrics=available_metrics or [],
            available_services=available_services or [],
            time_context=time_context,
            query_type=query_type,
        )

        messages = [
            Message(role="system", content=self.prompt_template.get_system_message()),
            Message(role="user", content=user_prompt),
        ]

        response = await self.client.chat(messages, temperature=temperature)

        return self._parse_response(response)

    def _parse_response(self, response: LLMResponse) -> QueryTranslation:
        """Parse LLM response into query translation."""
        content = response.content

        # Extract query type
        query_type = "metrics"  # default
        type_match = re.search(r"Query Type[:\s]*(metrics|logs|traces|combined)", content, re.IGNORECASE)
        if type_match:
            query_type = type_match.group(1).lower()

        # Extract query (look for code blocks)
        query = ""
        code_match = re.search(r"```(?:\w+)?\n?(.*?)\n?```", content, re.DOTALL)
        if code_match:
            query = code_match.group(1).strip()
        else:
            # Try to find query after "Query:" or "Generated Query:"
            query_match = re.search(r"(?:Generated )?Query[:\s]*\n?(.+?)(?=\n\n|Explanation|$)", content, re.DOTALL)
            if query_match:
                query = query_match.group(1).strip()

        # Extract explanation
        explanation = ""
        expl_match = re.search(r"Explanation[:\s]*\n?(.*?)(?=\n\n(?:Suggested|Alternative)|$)", content, re.DOTALL | re.IGNORECASE)
        if expl_match:
            explanation = expl_match.group(1).strip()

        # Extract visualizations
        viz_section = self._extract_section(content, "Visualization", "Alternative")
        visualizations = self._extract_list(viz_section) if viz_section else []

        # Extract alternatives
        alt_section = self._extract_section(content, "Alternative", None)
        alternatives = self._parse_alternatives(alt_section) if alt_section else []

        return QueryTranslation(
            query_type=query_type,
            query=query or "Unable to generate query",
            explanation=explanation or "Query translated from natural language",
            visualizations=visualizations,
            alternatives=alternatives,
            raw_response=content,
            model=response.model,
            latency_ms=response.latency_ms,
        )

    def _extract_section(self, content: str, start: str, end: Optional[str]) -> str:
        """Extract section text."""
        pattern = rf"{start}[s]?[:\s]*\n?(.*?)(?={end}|\Z)" if end else rf"{start}[s]?[:\s]*\n?(.*)"
        match = re.search(pattern, content, re.IGNORECASE | re.DOTALL)
        return match.group(1).strip() if match else ""

    def _extract_list(self, text: str) -> list[str]:
        """Extract list items."""
        items = re.findall(r"[-•*]\s*(.+?)(?=\n[-•*]|\n\n|\Z)", text, re.DOTALL)
        return [item.strip() for item in items if item.strip()]

    def _parse_alternatives(self, text: str) -> list[dict]:
        """Parse alternative queries."""
        alternatives = []
        # Look for numbered alternatives or bullet points with queries
        items = re.findall(r"[-•*\d.]+\s*(.+?)(?:\n```(?:\w+)?\n?(.*?)\n?```)?(?=\n[-•*\d.]|\n\n|\Z)", text, re.DOTALL)

        for desc, query in items:
            if desc.strip():
                alternatives.append({
                    "description": desc.strip(),
                    "query": query.strip() if query else "",
                })

        return alternatives


class AnomalyExplainer:
    """
    Explains detected anomalies in plain language.

    Makes ML anomaly detection results understandable.
    """

    def __init__(
        self,
        client: Optional[LLMClient] = None,
        provider: LLMProvider = LLMProvider.OPENAI,
        model: Optional[str] = None,
    ):
        self.client = client or LLMClient(provider=provider, model=model)
        self.prompt_template = AnomalyExplanationPrompt()

    async def explain(
        self,
        anomaly_type: str,
        metric_name: str,
        current_value: float,
        expected_value: float,
        deviation: float = 0.0,
        historical_context: Optional[list[dict]] = None,
        correlated_events: Optional[list[str]] = None,
        temperature: float = 0.5,
    ) -> dict:
        """
        Generate human-readable anomaly explanation.

        Args:
            anomaly_type: Type of anomaly (spike, drop, etc.)
            metric_name: Name of the affected metric
            current_value: Current value
            expected_value: Expected/baseline value
            deviation: Standard deviations from normal
            historical_context: Recent historical values
            correlated_events: Events that correlate with anomaly
            temperature: LLM temperature

        Returns:
            Dictionary with explanation and recommendations
        """
        user_prompt = self.prompt_template.render(
            anomaly_type=anomaly_type,
            metric_name=metric_name,
            current_value=current_value,
            expected_value=expected_value,
            deviation=deviation,
            historical_context=historical_context or [],
            correlated_events=correlated_events or [],
        )

        messages = [
            Message(role="system", content=self.prompt_template.get_system_message()),
            Message(role="user", content=user_prompt),
        ]

        response = await self.client.chat(messages, temperature=temperature)

        return {
            "explanation": response.content,
            "model": response.model,
            "latency_ms": response.latency_ms,
        }


class RunbookMatcher:
    """
    Matches incidents to relevant runbooks.

    Uses LLM to find best matching runbooks
    and adapt steps to the current situation.
    """

    def __init__(
        self,
        client: Optional[LLMClient] = None,
        provider: LLMProvider = LLMProvider.OPENAI,
        model: Optional[str] = None,
    ):
        self.client = client or LLMClient(provider=provider, model=model)
        self.prompt_template = RunbookSuggestionPrompt()

    async def match(
        self,
        incident_description: str,
        error_messages: Optional[list[str]] = None,
        affected_service: str = "",
        available_runbooks: Optional[list[dict]] = None,
        temperature: float = 0.3,
    ) -> dict:
        """
        Find matching runbooks for an incident.

        Args:
            incident_description: Description of the incident
            error_messages: Relevant error messages
            affected_service: Primary affected service
            available_runbooks: List of available runbooks
            temperature: LLM temperature

        Returns:
            Dictionary with matched runbook and steps
        """
        user_prompt = self.prompt_template.render(
            incident_description=incident_description,
            error_messages=error_messages or [],
            affected_service=affected_service,
            available_runbooks=available_runbooks or [],
        )

        messages = [
            Message(role="system", content=self.prompt_template.get_system_message()),
            Message(role="user", content=user_prompt),
        ]

        response = await self.client.chat(messages, temperature=temperature)

        return self._parse_response(response)

    def _parse_response(self, response: LLMResponse) -> dict:
        """Parse runbook matching response."""
        content = response.content

        # Extract matched runbook
        match_section = re.search(r"(?:Best )?Match(?:ing Runbook)?[:\s]*\n?(.*?)(?=Relevance|Steps|$)", content, re.IGNORECASE | re.DOTALL)
        matched_runbook = match_section.group(1).strip() if match_section else "No exact match found"

        # Extract relevance
        relevance_match = re.search(r"Relevance[:\s]*(\d+(?:\.\d+)?%?|high|medium|low)", content, re.IGNORECASE)
        relevance = relevance_match.group(1) if relevance_match else "unknown"

        # Extract steps
        steps_section = re.search(r"(?:Key )?Steps[:\s]*\n?(.*?)(?=Modifications|Gaps|$)", content, re.IGNORECASE | re.DOTALL)
        steps = []
        if steps_section:
            steps = re.findall(r"[-•*\d.]+\s*(.+?)(?=\n[-•*\d.]|\n\n|\Z)", steps_section.group(1), re.DOTALL)
            steps = [s.strip() for s in steps if s.strip()]

        # Extract modifications needed
        mods_section = re.search(r"Modifications[:\s]*\n?(.*?)(?=Gaps|$)", content, re.IGNORECASE | re.DOTALL)
        modifications = []
        if mods_section:
            modifications = re.findall(r"[-•*]\s*(.+?)(?=\n[-•*]|\n\n|\Z)", mods_section.group(1), re.DOTALL)
            modifications = [m.strip() for m in modifications if m.strip()]

        return {
            "matched_runbook": matched_runbook,
            "relevance": relevance,
            "key_steps": steps,
            "modifications_needed": modifications,
            "raw_response": content,
            "model": response.model,
            "latency_ms": response.latency_ms,
        }
