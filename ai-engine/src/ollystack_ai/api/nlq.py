"""
Natural Language Query (NLQ) API

Translates natural language questions into ObservQL queries
and executes them against the observability data.
"""

import logging
from typing import Optional
from datetime import datetime, timedelta

from fastapi import APIRouter, HTTPException, Request, Depends
from pydantic import BaseModel, Field

from ollystack_ai.nlq.translator import NLQTranslator
from ollystack_ai.nlq.executor import QueryExecutor
from ollystack_ai.nlq.context import QueryContext

logger = logging.getLogger(__name__)
router = APIRouter()


class NLQRequest(BaseModel):
    """Natural language query request."""

    question: str = Field(..., description="Natural language question", min_length=3, max_length=1000)
    time_range: Optional[str] = Field("1h", description="Time range (e.g., '1h', '24h', '7d')")
    service_filter: Optional[str] = Field(None, description="Optional service name filter")
    include_suggestions: bool = Field(True, description="Include follow-up suggestions")
    execute: bool = Field(True, description="Execute the generated query")


class NLQResponse(BaseModel):
    """Natural language query response."""

    question: str
    observql: str
    sql: Optional[str] = None
    explanation: str
    confidence: float
    results: Optional[dict] = None
    suggestions: Optional[list[str]] = None
    execution_time_ms: Optional[float] = None


class SuggestionRequest(BaseModel):
    """Request for query suggestions."""

    context: Optional[str] = Field(None, description="Current context or partial query")
    service: Optional[str] = Field(None, description="Service name for context")
    limit: int = Field(5, description="Number of suggestions to return")


class SuggestionResponse(BaseModel):
    """Query suggestions response."""

    suggestions: list[str]
    categories: dict[str, list[str]]


def get_translator(request: Request) -> NLQTranslator:
    """Dependency to get NLQ translator."""
    return NLQTranslator(
        llm_service=request.app.state.llm_service,
        cache_service=request.app.state.cache_service,
    )


def get_executor(request: Request) -> QueryExecutor:
    """Dependency to get query executor."""
    return QueryExecutor(
        storage_service=request.app.state.storage_service,
    )


@router.post("/query", response_model=NLQResponse)
async def process_natural_language_query(
    request: NLQRequest,
    translator: NLQTranslator = Depends(get_translator),
    executor: QueryExecutor = Depends(get_executor),
) -> NLQResponse:
    """
    Process a natural language query.

    Translates the question to ObservQL, optionally executes it,
    and returns results with explanations.

    Examples:
    - "Why was checkout slow yesterday?"
    - "Show me all errors in the payment service in the last hour"
    - "What's the p99 latency for the API gateway?"
    - "Which services had the most errors last week?"
    """
    try:
        # Build query context
        context = QueryContext(
            time_range=request.time_range,
            service_filter=request.service_filter,
        )

        # Translate to ObservQL
        translation = await translator.translate(request.question, context)

        response = NLQResponse(
            question=request.question,
            observql=translation.observql,
            sql=translation.sql,
            explanation=translation.explanation,
            confidence=translation.confidence,
            suggestions=translation.suggestions if request.include_suggestions else None,
        )

        # Execute if requested
        if request.execute:
            result = await executor.execute(translation.sql or translation.observql)
            response.results = result.data
            response.execution_time_ms = result.execution_time_ms

        return response

    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        logger.exception("Error processing NLQ")
        raise HTTPException(status_code=500, detail=f"Failed to process query: {str(e)}")


@router.post("/suggestions", response_model=SuggestionResponse)
async def get_query_suggestions(
    request: SuggestionRequest,
    translator: NLQTranslator = Depends(get_translator),
) -> SuggestionResponse:
    """
    Get suggested questions based on context.

    Returns categorized suggestions for common observability questions.
    """
    suggestions = await translator.get_suggestions(
        context=request.context,
        service=request.service,
        limit=request.limit,
    )

    return SuggestionResponse(
        suggestions=suggestions.top_suggestions,
        categories=suggestions.by_category,
    )


@router.get("/examples")
async def get_example_queries() -> dict:
    """
    Get example natural language queries.

    Returns examples organized by category to help users understand capabilities.
    """
    return {
        "latency": [
            "Why was the checkout service slow yesterday between 2-4pm?",
            "What's the p99 latency for the API gateway?",
            "Show me latency trends for the payment service over the last week",
            "Which endpoints have the highest latency?",
        ],
        "errors": [
            "Show me all errors in the last hour",
            "What caused the spike in 500 errors yesterday?",
            "Which services have the highest error rate?",
            "Find all timeout errors in the order service",
        ],
        "traces": [
            "Show me the slowest traces for /api/checkout",
            "Find traces where the database took more than 1 second",
            "What's the typical flow for a checkout request?",
            "Show me failed traces in the last 30 minutes",
        ],
        "dependencies": [
            "Which services depend on the user service?",
            "Show me the service map for the checkout flow",
            "What downstream services were affected by the database outage?",
            "Which services communicate with the Redis cache?",
        ],
        "capacity": [
            "What's the current request rate for each service?",
            "Show me CPU usage trends for the API servers",
            "Are there any services running low on memory?",
            "Predict traffic for the next hour based on current trends",
        ],
        "comparison": [
            "Compare error rates between production and staging",
            "How does today's latency compare to last week?",
            "Show me before/after metrics for the latest deployment",
        ],
    }


@router.post("/explain")
async def explain_query(
    observql: str,
    translator: NLQTranslator = Depends(get_translator),
) -> dict:
    """
    Explain an ObservQL query in natural language.

    Useful for understanding what a query does.
    """
    explanation = await translator.explain(observql)
    return {
        "query": observql,
        "explanation": explanation.text,
        "components": explanation.components,
    }
