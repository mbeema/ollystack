"""
Tests for LLM integration.

Tests the LLM client, prompts, and analyzers.
"""

import pytest
from datetime import datetime
from unittest.mock import AsyncMock, MagicMock, patch
import numpy as np

from ollystack_ai.llm.client import (
    LLMClient,
    LLMProvider,
    LLMResponse,
    Message,
    get_best_available_client,
)
from ollystack_ai.llm.prompts import (
    RootCausePrompt,
    IncidentSummaryPrompt,
    NaturalLanguageQueryPrompt,
    AnomalyExplanationPrompt,
    RunbookSuggestionPrompt,
)
from ollystack_ai.llm.analyzer import (
    RootCauseAnalyzer,
    RootCauseResult,
    IncidentSummarizer,
    NaturalLanguageQuerier,
    AnomalyExplainer,
    RunbookMatcher,
)


class TestLLMClient:
    """Tests for LLMClient."""

    def test_init_default(self):
        """Test default initialization."""
        client = LLMClient()
        assert client.provider == LLMProvider.OPENAI
        assert client.model == "gpt-4-turbo-preview"
        assert client.timeout == 60.0
        assert client.max_retries == 3

    def test_init_custom_provider(self):
        """Test initialization with custom provider."""
        client = LLMClient(provider=LLMProvider.ANTHROPIC)
        assert client.provider == LLMProvider.ANTHROPIC
        assert client.model == "claude-3-sonnet-20240229"

    def test_init_ollama(self):
        """Test Ollama initialization."""
        client = LLMClient(provider=LLMProvider.OLLAMA)
        assert client.provider == LLMProvider.OLLAMA
        assert client.model == "llama2"
        assert "localhost:11434" in client.base_url

    def test_init_custom_model(self):
        """Test custom model specification."""
        client = LLMClient(model="gpt-3.5-turbo")
        assert client.model == "gpt-3.5-turbo"

    @pytest.mark.asyncio
    async def test_mock_chat(self):
        """Test mock provider."""
        client = LLMClient(provider=LLMProvider.MOCK)
        messages = [Message(role="user", content="Hello")]

        response = await client.chat(messages)

        assert response.content.startswith("Mock response to:")
        assert response.provider == LLMProvider.MOCK
        assert response.model == "mock-model"

    @pytest.mark.asyncio
    async def test_client_close(self):
        """Test client cleanup."""
        client = LLMClient(provider=LLMProvider.MOCK)

        # Create HTTP client
        await client._get_client()
        assert client._client is not None

        # Close
        await client.close()
        assert client._client is None

    @pytest.mark.asyncio
    async def test_context_manager(self):
        """Test async context manager."""
        async with LLMClient(provider=LLMProvider.MOCK) as client:
            response = await client.chat([Message(role="user", content="Test")])
            assert response is not None


class TestPromptTemplates:
    """Tests for prompt templates."""

    def test_root_cause_prompt_basic(self):
        """Test basic root cause prompt."""
        prompt = RootCausePrompt(
            anomaly_description="High latency detected"
        )

        rendered = prompt.render()

        assert "High latency detected" in rendered
        assert "Root Cause Analysis" in rendered

    def test_root_cause_prompt_full(self):
        """Test full root cause prompt with all fields."""
        prompt = RootCausePrompt()

        rendered = prompt.render(
            anomaly_description="API errors spiked",
            affected_services=["api-gateway", "auth-service"],
            error_logs=["Connection refused", "Timeout error"],
            metrics_summary={
                "error_rate": {"value": 5.0, "baseline": 0.1},
                "latency_p99": {"value": 2000, "baseline": 200},
            },
            recent_changes=["Deployed v2.0 at 14:00"],
        )

        assert "API errors spiked" in rendered
        assert "api-gateway" in rendered
        assert "Connection refused" in rendered
        assert "error_rate" in rendered
        assert "Deployed v2.0" in rendered

    def test_root_cause_system_message(self):
        """Test system message."""
        prompt = RootCausePrompt()
        system = prompt.get_system_message()

        assert "SRE" in system or "reliability" in system.lower()
        assert "root cause" in system.lower()

    def test_incident_summary_prompt(self):
        """Test incident summary prompt."""
        prompt = IncidentSummaryPrompt()

        rendered = prompt.render(
            incident_title="Database Outage",
            start_time=datetime(2024, 1, 15, 14, 30),
            end_time=datetime(2024, 1, 15, 15, 45),
            severity="critical",
            affected_systems=["database", "api", "web"],
            timeline=[
                {"time": "14:30", "description": "Alert fired"},
                {"time": "14:45", "description": "Issue identified"},
            ],
            root_cause="Disk full",
            resolution="Expanded disk",
        )

        assert "Database Outage" in rendered
        assert "critical" in rendered.lower()
        assert "Alert fired" in rendered
        assert "Disk full" in rendered

    def test_nl_query_prompt(self):
        """Test natural language query prompt."""
        prompt = NaturalLanguageQueryPrompt()

        rendered = prompt.render(
            user_question="Show me errors in the payment service",
            available_metrics=["http_errors_total", "latency_seconds"],
            available_services=["payment", "checkout", "inventory"],
            time_context="last hour",
        )

        assert "Show me errors in the payment service" in rendered
        assert "http_errors_total" in rendered
        assert "payment" in rendered

    def test_anomaly_explanation_prompt(self):
        """Test anomaly explanation prompt."""
        prompt = AnomalyExplanationPrompt()

        rendered = prompt.render(
            anomaly_type="spike",
            metric_name="cpu_usage",
            current_value=95.0,
            expected_value=45.0,
            deviation=4.5,
            correlated_events=["Deployment started"],
        )

        assert "spike" in rendered.lower()
        assert "cpu_usage" in rendered
        assert "95" in rendered
        assert "Deployment" in rendered

    def test_runbook_suggestion_prompt(self):
        """Test runbook suggestion prompt."""
        prompt = RunbookSuggestionPrompt()

        rendered = prompt.render(
            incident_description="Database connection errors",
            error_messages=["Connection refused"],
            affected_service="api-service",
            available_runbooks=[
                {"title": "Database Troubleshooting", "tags": ["database", "connection"]},
            ],
        )

        assert "Database connection errors" in rendered
        assert "Connection refused" in rendered
        assert "Database Troubleshooting" in rendered


class TestRootCauseAnalyzer:
    """Tests for RootCauseAnalyzer."""

    @pytest.fixture
    def mock_client(self):
        """Create mock LLM client."""
        client = MagicMock(spec=LLMClient)
        client.chat = AsyncMock(return_value=LLMResponse(
            content="""
## Root Cause Summary
Database connection pool exhaustion due to connection leak.

## Evidence
- Connection count increased from 10 to 100
- Timeout errors in logs
- Recent deployment introduced new query pattern

## Confidence Level
High - clear correlation between deployment and issue

## Remediation Steps
- Increase connection pool size
- Fix connection leak in new code
- Add connection monitoring

## Prevention
- Add connection pool metrics alerting
- Code review for resource management
""",
            model="gpt-4",
            provider=LLMProvider.OPENAI,
            latency_ms=1500,
        ))
        return client

    @pytest.mark.asyncio
    async def test_analyze_basic(self, mock_client):
        """Test basic analysis."""
        analyzer = RootCauseAnalyzer(client=mock_client)

        result = await analyzer.analyze(
            anomaly_description="API latency increased",
            affected_services=["api-gateway"],
        )

        assert isinstance(result, RootCauseResult)
        assert "connection" in result.summary.lower()
        assert result.confidence == "high"
        assert len(result.remediation_steps) > 0

    @pytest.mark.asyncio
    async def test_analyze_full_context(self, mock_client):
        """Test analysis with full context."""
        analyzer = RootCauseAnalyzer(client=mock_client)

        result = await analyzer.analyze(
            anomaly_description="Database errors",
            affected_services=["api", "worker"],
            error_logs=["Connection timeout", "Pool exhausted"],
            metrics_summary={"connections": {"value": 100, "baseline": 10}},
            recent_changes=["Deployed new version"],
            trace_data={"duration_ms": 5000, "error": "timeout"},
        )

        assert result.summary
        assert result.model == "gpt-4"
        assert result.latency_ms == 1500

    def test_parse_confidence(self, mock_client):
        """Test confidence extraction."""
        analyzer = RootCauseAnalyzer(client=mock_client)

        # Test explicit confidence
        assert analyzer._extract_confidence("Confidence: High") == "high"
        assert analyzer._extract_confidence("confidence: low") == "low"

        # Test inferred confidence
        assert analyzer._extract_confidence("This is definitely the cause") == "high"
        assert analyzer._extract_confidence("This likely caused it") == "medium"


class TestIncidentSummarizer:
    """Tests for IncidentSummarizer."""

    @pytest.fixture
    def mock_client(self):
        """Create mock LLM client."""
        client = MagicMock(spec=LLMClient)
        client.chat = AsyncMock(return_value=LLMResponse(
            content="""
## Executive Summary
A database outage caused 30 minutes of service degradation affecting checkout.

## Technical Summary
The primary database ran out of disk space at 14:30, causing connection failures.
This cascaded to the API layer, resulting in 500 errors for checkout requests.

## Timeline
- 14:30: Alert triggered for disk usage
- 14:35: On-call acknowledged
- 14:45: Root cause identified
- 15:00: Disk expanded
- 15:05: Service recovered

## Impact Assessment
- 500 checkout failures
- Estimated revenue loss: $10,000
- No data loss

## Lessons Learned
- Implement disk usage alerting at 70%
- Add auto-scaling for database storage
- Create runbook for disk issues
""",
            model="gpt-4",
            provider=LLMProvider.OPENAI,
            latency_ms=2000,
        ))
        return client

    @pytest.mark.asyncio
    async def test_summarize(self, mock_client):
        """Test incident summarization."""
        summarizer = IncidentSummarizer(client=mock_client)

        result = await summarizer.summarize(
            incident_title="Database Outage",
            start_time=datetime(2024, 1, 15, 14, 30),
            end_time=datetime(2024, 1, 15, 15, 5),
            severity="critical",
            affected_systems=["database", "api"],
            root_cause="Disk full",
        )

        assert "database" in result.executive_summary.lower()
        assert result.technical_summary
        assert len(result.timeline) > 0
        assert len(result.lessons_learned) > 0


class TestNaturalLanguageQuerier:
    """Tests for NaturalLanguageQuerier."""

    @pytest.fixture
    def mock_client(self):
        """Create mock LLM client."""
        client = MagicMock(spec=LLMClient)
        client.chat = AsyncMock(return_value=LLMResponse(
            content="""
## Query Type
metrics

## Generated Query
```promql
sum(rate(http_errors_total{service="payment"}[5m])) by (endpoint)
```

## Explanation
This query calculates the rate of HTTP errors for the payment service,
summed by endpoint, over 5-minute windows.

## Suggested Visualizations
- Time series graph
- Heatmap by endpoint

## Alternative Queries
1. For error rate percentage:
```promql
sum(rate(http_errors_total{service="payment"}[5m])) / sum(rate(http_requests_total{service="payment"}[5m]))
```
""",
            model="gpt-4",
            provider=LLMProvider.OPENAI,
            latency_ms=1000,
        ))
        return client

    @pytest.mark.asyncio
    async def test_translate(self, mock_client):
        """Test query translation."""
        querier = NaturalLanguageQuerier(client=mock_client)

        result = await querier.translate(
            question="Show me payment service errors",
            available_metrics=["http_errors_total", "http_requests_total"],
            available_services=["payment", "checkout"],
        )

        assert result.query_type == "metrics"
        assert "http_errors_total" in result.query
        assert result.explanation
        assert len(result.visualizations) > 0


class TestAnomalyExplainer:
    """Tests for AnomalyExplainer."""

    @pytest.fixture
    def mock_client(self):
        """Create mock LLM client."""
        client = MagicMock(spec=LLMClient)
        client.chat = AsyncMock(return_value=LLMResponse(
            content="""
Your CPU usage has spiked to 95%, which is about 4 standard deviations above normal.

This is like your computer suddenly working much harder than usual. Normally it cruises
at around 45% capacity, but now it's nearly maxed out.

Possible causes:
1. A recent deployment may have introduced inefficient code
2. A traffic spike is causing more requests than usual
3. A runaway process might be consuming resources

Recommended actions:
1. Check recent deployments for changes
2. Review current traffic levels
3. Use top/htop to identify high-CPU processes
4. Scale horizontally if traffic is the cause
""",
            model="gpt-4",
            provider=LLMProvider.OPENAI,
            latency_ms=800,
        ))
        return client

    @pytest.mark.asyncio
    async def test_explain(self, mock_client):
        """Test anomaly explanation."""
        explainer = AnomalyExplainer(client=mock_client)

        result = await explainer.explain(
            anomaly_type="spike",
            metric_name="cpu_usage",
            current_value=95.0,
            expected_value=45.0,
            deviation=4.2,
        )

        assert "explanation" in result
        assert "CPU" in result["explanation"] or "cpu" in result["explanation"]


class TestRunbookMatcher:
    """Tests for RunbookMatcher."""

    @pytest.fixture
    def mock_client(self):
        """Create mock LLM client."""
        client = MagicMock(spec=LLMClient)
        client.chat = AsyncMock(return_value=LLMResponse(
            content="""
## Best Matching Runbook
Database Connection Troubleshooting

## Relevance
85% - Strong match based on connection errors and database symptoms

## Key Steps
1. Check database connectivity from affected services
2. Verify connection pool settings
3. Check database server logs for errors
4. Verify credentials haven't expired
5. Restart connection pools if needed

## Modifications Needed
- Add step to check recent deployments
- Include verification of new code paths

## Gaps
- No runbook for connection leak debugging
- Missing escalation for extended outages
""",
            model="gpt-4",
            provider=LLMProvider.OPENAI,
            latency_ms=900,
        ))
        return client

    @pytest.mark.asyncio
    async def test_match(self, mock_client):
        """Test runbook matching."""
        matcher = RunbookMatcher(client=mock_client)

        result = await matcher.match(
            incident_description="Database connection errors",
            error_messages=["Connection refused"],
            affected_service="api-service",
            available_runbooks=[
                {"title": "Database Connection Troubleshooting", "tags": ["database"]},
            ],
        )

        assert "matched_runbook" in result
        assert "Database" in result["matched_runbook"]
        assert "relevance" in result
        assert len(result["key_steps"]) > 0


class TestGetBestAvailableClient:
    """Tests for get_best_available_client."""

    def test_no_providers(self):
        """Test fallback to mock when no providers available."""
        with patch.dict('os.environ', {}, clear=True):
            with patch('ollystack_ai.llm.client._check_ollama_available', return_value=False):
                client = get_best_available_client()
                assert client.provider == LLMProvider.MOCK

    def test_openai_available(self):
        """Test OpenAI selection when available."""
        with patch.dict('os.environ', {'OPENAI_API_KEY': 'test-key'}):
            client = get_best_available_client()
            assert client.provider == LLMProvider.OPENAI

    def test_anthropic_available(self):
        """Test Anthropic selection when OpenAI not available."""
        with patch.dict('os.environ', {'ANTHROPIC_API_KEY': 'test-key'}, clear=True):
            client = get_best_available_client()
            assert client.provider == LLMProvider.ANTHROPIC


class TestLLMResponse:
    """Tests for LLMResponse."""

    def test_to_dict(self):
        """Test response serialization."""
        response = LLMResponse(
            content="Test content",
            model="gpt-4",
            provider=LLMProvider.OPENAI,
            usage={"prompt_tokens": 100, "completion_tokens": 50},
            latency_ms=1500,
        )

        result = response.to_dict()

        assert result["content"] == "Test content"
        assert result["model"] == "gpt-4"
        assert result["provider"] == "openai"
        assert result["latency_ms"] == 1500


class TestMessage:
    """Tests for Message dataclass."""

    def test_user_message(self):
        """Test user message creation."""
        msg = Message(role="user", content="Hello")
        assert msg.role == "user"
        assert msg.content == "Hello"

    def test_system_message(self):
        """Test system message creation."""
        msg = Message(role="system", content="You are an assistant")
        assert msg.role == "system"

    def test_assistant_message(self):
        """Test assistant message creation."""
        msg = Message(role="assistant", content="Hello, how can I help?")
        assert msg.role == "assistant"
