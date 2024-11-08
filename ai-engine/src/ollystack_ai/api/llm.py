"""
LLM API Endpoints

REST API for AI-powered analysis:
- Root cause analysis
- Incident summarization
- Natural language queries
- Anomaly explanations
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field
from typing import Optional
from datetime import datetime

from ollystack_ai.llm.client import LLMClient, LLMProvider, get_best_available_client
from ollystack_ai.llm.analyzer import (
    RootCauseAnalyzer,
    IncidentSummarizer,
    NaturalLanguageQuerier,
    AnomalyExplainer,
    RunbookMatcher,
)

router = APIRouter(prefix="/llm", tags=["LLM Analysis"])


# Request/Response Models

class RootCauseRequest(BaseModel):
    """Request for root cause analysis."""

    anomaly_description: str = Field(..., description="Description of the anomaly")
    affected_services: Optional[list[str]] = Field(default=None, description="Affected services")
    error_logs: Optional[list[str]] = Field(default=None, description="Recent error logs")
    metrics_summary: Optional[dict] = Field(default=None, description="Metrics summary")
    recent_changes: Optional[list[str]] = Field(default=None, description="Recent changes")
    trace_data: Optional[dict] = Field(default=None, description="Trace data")
    provider: Optional[str] = Field(default=None, description="LLM provider (openai, anthropic, ollama)")

    class Config:
        json_schema_extra = {
            "example": {
                "anomaly_description": "API response times increased by 300% starting at 14:30",
                "affected_services": ["api-gateway", "order-service"],
                "error_logs": [
                    "Connection timeout to database",
                    "Pool exhausted, waiting for connection",
                ],
                "metrics_summary": {
                    "response_time_p99": {"value": 2500, "baseline": 500},
                    "error_rate": {"value": 5.2, "baseline": 0.1},
                },
                "recent_changes": [
                    "14:00 - Deployed order-service v2.3.1",
                ],
            }
        }


class RootCauseResponse(BaseModel):
    """Response from root cause analysis."""

    summary: str
    confidence: str
    evidence: list[str]
    remediation_steps: list[str]
    prevention_steps: list[str]
    model: str
    latency_ms: float


class IncidentSummaryRequest(BaseModel):
    """Request for incident summarization."""

    incident_title: str = Field(..., description="Incident title")
    start_time: Optional[datetime] = Field(default=None, description="Incident start time")
    end_time: Optional[datetime] = Field(default=None, description="Incident end time")
    severity: str = Field(default="medium", description="Severity level")
    affected_systems: Optional[list[str]] = Field(default=None, description="Affected systems")
    timeline: Optional[list[dict]] = Field(default=None, description="Event timeline")
    root_cause: Optional[str] = Field(default=None, description="Root cause")
    resolution: Optional[str] = Field(default=None, description="Resolution")
    metrics_impact: Optional[dict] = Field(default=None, description="Impact metrics")
    provider: Optional[str] = Field(default=None, description="LLM provider")

    class Config:
        json_schema_extra = {
            "example": {
                "incident_title": "Database Connection Pool Exhaustion",
                "start_time": "2024-01-15T14:30:00Z",
                "end_time": "2024-01-15T15:45:00Z",
                "severity": "high",
                "affected_systems": ["api-gateway", "order-service", "payment-service"],
                "timeline": [
                    {"time": "14:30", "description": "Alert triggered for high latency"},
                    {"time": "14:35", "description": "On-call engineer acknowledged"},
                    {"time": "14:45", "description": "Root cause identified"},
                    {"time": "15:00", "description": "Connection pool size increased"},
                    {"time": "15:45", "description": "Metrics normalized"},
                ],
                "root_cause": "Connection pool sized too small for traffic spike",
                "resolution": "Increased pool size and added auto-scaling",
            }
        }


class IncidentSummaryResponse(BaseModel):
    """Response from incident summarization."""

    executive_summary: str
    technical_summary: str
    timeline: list[dict]
    impact_assessment: str
    lessons_learned: list[str]
    model: str
    latency_ms: float


class NLQueryRequest(BaseModel):
    """Request for natural language query translation."""

    question: str = Field(..., description="Natural language question")
    available_metrics: Optional[list[str]] = Field(default=None, description="Available metrics")
    available_services: Optional[list[str]] = Field(default=None, description="Available services")
    time_context: Optional[str] = Field(default=None, description="Time context")
    query_type: str = Field(default="auto", description="Query type constraint")
    provider: Optional[str] = Field(default=None, description="LLM provider")

    class Config:
        json_schema_extra = {
            "example": {
                "question": "Show me the error rate for the checkout service over the last hour",
                "available_metrics": [
                    "http_requests_total",
                    "http_request_duration_seconds",
                    "error_rate",
                ],
                "available_services": ["checkout", "cart", "inventory", "payment"],
                "time_context": "last 1 hour",
            }
        }


class NLQueryResponse(BaseModel):
    """Response from natural language query translation."""

    query_type: str
    query: str
    explanation: str
    visualizations: list[str]
    alternatives: list[dict]
    model: str
    latency_ms: float


class AnomalyExplainRequest(BaseModel):
    """Request for anomaly explanation."""

    anomaly_type: str = Field(..., description="Type of anomaly")
    metric_name: str = Field(..., description="Metric name")
    current_value: float = Field(..., description="Current value")
    expected_value: float = Field(..., description="Expected value")
    deviation: float = Field(default=0.0, description="Standard deviation")
    historical_context: Optional[list[dict]] = Field(default=None, description="Historical values")
    correlated_events: Optional[list[str]] = Field(default=None, description="Correlated events")
    provider: Optional[str] = Field(default=None, description="LLM provider")

    class Config:
        json_schema_extra = {
            "example": {
                "anomaly_type": "spike",
                "metric_name": "cpu_usage_percent",
                "current_value": 95.5,
                "expected_value": 45.0,
                "deviation": 4.2,
                "correlated_events": [
                    "Deployment of service v2.0",
                    "Traffic increased 200%",
                ],
            }
        }


class RunbookMatchRequest(BaseModel):
    """Request for runbook matching."""

    incident_description: str = Field(..., description="Incident description")
    error_messages: Optional[list[str]] = Field(default=None, description="Error messages")
    affected_service: str = Field(default="", description="Affected service")
    available_runbooks: Optional[list[dict]] = Field(default=None, description="Available runbooks")
    provider: Optional[str] = Field(default=None, description="LLM provider")


class LLMStatusResponse(BaseModel):
    """LLM service status."""

    available: bool
    provider: str
    model: str
    features: list[str]


# Helper functions

def get_client(provider: Optional[str] = None) -> LLMClient:
    """Get LLM client based on provider."""
    if provider:
        provider_map = {
            "openai": LLMProvider.OPENAI,
            "anthropic": LLMProvider.ANTHROPIC,
            "ollama": LLMProvider.OLLAMA,
        }
        if provider.lower() not in provider_map:
            raise HTTPException(
                status_code=400,
                detail=f"Invalid provider: {provider}. Choose from: openai, anthropic, ollama",
            )
        return LLMClient(provider=provider_map[provider.lower()])
    return get_best_available_client()


# Endpoints

@router.get("/status", response_model=LLMStatusResponse)
async def get_llm_status():
    """
    Get LLM service status.

    Returns available provider and supported features.
    """
    try:
        client = get_best_available_client()
        return LLMStatusResponse(
            available=True,
            provider=client.provider.value,
            model=client.model,
            features=[
                "root_cause_analysis",
                "incident_summarization",
                "natural_language_queries",
                "anomaly_explanation",
                "runbook_matching",
            ],
        )
    except Exception as e:
        return LLMStatusResponse(
            available=False,
            provider="none",
            model="none",
            features=[],
        )


@router.post("/analyze/root-cause", response_model=RootCauseResponse)
async def analyze_root_cause(request: RootCauseRequest):
    """
    Analyze anomaly to identify root cause.

    Uses LLM to correlate logs, metrics, and traces
    to identify the most likely root cause.
    """
    try:
        client = get_client(request.provider)
        analyzer = RootCauseAnalyzer(client=client)

        result = await analyzer.analyze(
            anomaly_description=request.anomaly_description,
            affected_services=request.affected_services,
            error_logs=request.error_logs,
            metrics_summary=request.metrics_summary,
            recent_changes=request.recent_changes,
            trace_data=request.trace_data,
        )

        return RootCauseResponse(
            summary=result.summary,
            confidence=result.confidence,
            evidence=result.evidence,
            remediation_steps=result.remediation_steps,
            prevention_steps=result.prevention_steps,
            model=result.model,
            latency_ms=result.latency_ms,
        )

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Analysis failed: {str(e)}")
    finally:
        await client.close()


@router.post("/summarize/incident", response_model=IncidentSummaryResponse)
async def summarize_incident(request: IncidentSummaryRequest):
    """
    Generate incident summary.

    Creates executive and technical summaries
    suitable for post-mortems and stakeholder communication.
    """
    try:
        client = get_client(request.provider)
        summarizer = IncidentSummarizer(client=client)

        result = await summarizer.summarize(
            incident_title=request.incident_title,
            start_time=request.start_time,
            end_time=request.end_time,
            severity=request.severity,
            affected_systems=request.affected_systems,
            timeline=request.timeline,
            root_cause=request.root_cause,
            resolution=request.resolution,
            metrics_impact=request.metrics_impact,
        )

        return IncidentSummaryResponse(
            executive_summary=result.executive_summary,
            technical_summary=result.technical_summary,
            timeline=result.timeline,
            impact_assessment=result.impact_assessment,
            lessons_learned=result.lessons_learned,
            model=result.model,
            latency_ms=result.latency_ms,
        )

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Summarization failed: {str(e)}")
    finally:
        await client.close()


@router.post("/query/translate", response_model=NLQueryResponse)
async def translate_query(request: NLQueryRequest):
    """
    Translate natural language to query.

    Converts plain English questions into
    PromQL, log queries, and trace queries.
    """
    try:
        client = get_client(request.provider)
        querier = NaturalLanguageQuerier(client=client)

        result = await querier.translate(
            question=request.question,
            available_metrics=request.available_metrics,
            available_services=request.available_services,
            time_context=request.time_context,
            query_type=request.query_type,
        )

        return NLQueryResponse(
            query_type=result.query_type,
            query=result.query,
            explanation=result.explanation,
            visualizations=result.visualizations,
            alternatives=result.alternatives,
            model=result.model,
            latency_ms=result.latency_ms,
        )

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Query translation failed: {str(e)}")
    finally:
        await client.close()


@router.post("/explain/anomaly")
async def explain_anomaly(request: AnomalyExplainRequest):
    """
    Explain detected anomaly.

    Generates human-readable explanation
    of what the anomaly means and recommended actions.
    """
    try:
        client = get_client(request.provider)
        explainer = AnomalyExplainer(client=client)

        result = await explainer.explain(
            anomaly_type=request.anomaly_type,
            metric_name=request.metric_name,
            current_value=request.current_value,
            expected_value=request.expected_value,
            deviation=request.deviation,
            historical_context=request.historical_context,
            correlated_events=request.correlated_events,
        )

        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Explanation failed: {str(e)}")
    finally:
        await client.close()


@router.post("/match/runbook")
async def match_runbook(request: RunbookMatchRequest):
    """
    Find matching runbook for incident.

    Uses LLM to find the most relevant runbook
    and adapt steps to the current situation.
    """
    try:
        client = get_client(request.provider)
        matcher = RunbookMatcher(client=client)

        result = await matcher.match(
            incident_description=request.incident_description,
            error_messages=request.error_messages,
            affected_service=request.affected_service,
            available_runbooks=request.available_runbooks,
        )

        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Runbook matching failed: {str(e)}")
    finally:
        await client.close()


@router.get("/providers")
async def list_providers():
    """
    List available LLM providers.

    Shows which providers are configured and available.
    """
    import os

    providers = []

    # Check OpenAI
    if os.environ.get("OPENAI_API_KEY"):
        providers.append({
            "name": "openai",
            "available": True,
            "models": ["gpt-4-turbo-preview", "gpt-4", "gpt-3.5-turbo"],
        })
    else:
        providers.append({
            "name": "openai",
            "available": False,
            "reason": "OPENAI_API_KEY not set",
        })

    # Check Anthropic
    if os.environ.get("ANTHROPIC_API_KEY"):
        providers.append({
            "name": "anthropic",
            "available": True,
            "models": ["claude-3-opus-20240229", "claude-3-sonnet-20240229"],
        })
    else:
        providers.append({
            "name": "anthropic",
            "available": False,
            "reason": "ANTHROPIC_API_KEY not set",
        })

    # Check Ollama
    try:
        import httpx
        response = httpx.get("http://localhost:11434/api/tags", timeout=2)
        if response.status_code == 200:
            models = [m["name"] for m in response.json().get("models", [])]
            providers.append({
                "name": "ollama",
                "available": True,
                "models": models or ["llama2", "mistral", "codellama"],
            })
        else:
            providers.append({
                "name": "ollama",
                "available": False,
                "reason": "Ollama not responding",
            })
    except Exception:
        providers.append({
            "name": "ollama",
            "available": False,
            "reason": "Ollama not running",
        })

    return {"providers": providers}
