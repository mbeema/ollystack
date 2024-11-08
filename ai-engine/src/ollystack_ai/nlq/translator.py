"""
NLQ Translator

Converts natural language questions into ObservQL and SQL queries
using LLM-powered translation with observability domain knowledge.
"""

import logging
import hashlib
from dataclasses import dataclass, field
from typing import Optional

from ollystack_ai.services.llm import LLMService
from ollystack_ai.services.cache import CacheService
from ollystack_ai.nlq.context import QueryContext
from ollystack_ai.nlq.prompts import (
    SYSTEM_PROMPT,
    TRANSLATION_PROMPT,
    SUGGESTION_PROMPT,
    EXPLANATION_PROMPT,
)

logger = logging.getLogger(__name__)


@dataclass
class TranslationResult:
    """Result of translating natural language to query."""

    observql: str
    sql: Optional[str] = None
    explanation: str = ""
    confidence: float = 0.0
    suggestions: list[str] = field(default_factory=list)


@dataclass
class SuggestionResult:
    """Suggested queries result."""

    top_suggestions: list[str]
    by_category: dict[str, list[str]]


@dataclass
class ExplanationResult:
    """Query explanation result."""

    text: str
    components: dict[str, str]


class NLQTranslator:
    """
    Translates natural language questions to ObservQL/SQL.

    Uses LLM with domain-specific prompting and caching for efficiency.
    """

    def __init__(
        self,
        llm_service: LLMService,
        cache_service: Optional[CacheService] = None,
        cache_ttl: int = 3600,
    ):
        self.llm = llm_service
        self.cache = cache_service
        self.cache_ttl = cache_ttl

    async def translate(
        self,
        question: str,
        context: Optional[QueryContext] = None,
    ) -> TranslationResult:
        """
        Translate a natural language question to ObservQL.

        Args:
            question: Natural language question
            context: Optional query context (time range, filters)

        Returns:
            TranslationResult with query and metadata
        """
        # Check cache
        cache_key = self._cache_key(question, context)
        if self.cache:
            cached = await self.cache.get(cache_key)
            if cached:
                logger.debug(f"Cache hit for query: {question[:50]}...")
                return TranslationResult(**cached)

        # Build prompt
        prompt = self._build_translation_prompt(question, context)

        # Call LLM
        response = await self.llm.complete(
            system_prompt=SYSTEM_PROMPT,
            user_prompt=prompt,
            temperature=0.1,  # Low temperature for consistent output
        )

        # Parse response
        result = self._parse_translation_response(response, question)

        # Cache result
        if self.cache and result.confidence > 0.7:
            await self.cache.set(
                cache_key,
                {
                    "observql": result.observql,
                    "sql": result.sql,
                    "explanation": result.explanation,
                    "confidence": result.confidence,
                    "suggestions": result.suggestions,
                },
                ttl=self.cache_ttl,
            )

        return result

    async def get_suggestions(
        self,
        context: Optional[str] = None,
        service: Optional[str] = None,
        limit: int = 5,
    ) -> SuggestionResult:
        """
        Get suggested questions based on context.

        Args:
            context: Current context or partial query
            service: Service name for targeted suggestions
            limit: Maximum number of suggestions

        Returns:
            SuggestionResult with categorized suggestions
        """
        prompt = SUGGESTION_PROMPT.format(
            context=context or "general observability",
            service=service or "any service",
            limit=limit,
        )

        response = await self.llm.complete(
            system_prompt=SYSTEM_PROMPT,
            user_prompt=prompt,
            temperature=0.7,  # Higher temperature for variety
        )

        return self._parse_suggestions_response(response)

    async def explain(self, query: str) -> ExplanationResult:
        """
        Explain an ObservQL query in natural language.

        Args:
            query: ObservQL or SQL query to explain

        Returns:
            ExplanationResult with natural language explanation
        """
        prompt = EXPLANATION_PROMPT.format(query=query)

        response = await self.llm.complete(
            system_prompt=SYSTEM_PROMPT,
            user_prompt=prompt,
            temperature=0.3,
        )

        return self._parse_explanation_response(response)

    def _build_translation_prompt(
        self,
        question: str,
        context: Optional[QueryContext],
    ) -> str:
        """Build the translation prompt with context."""
        context_str = ""
        if context:
            context_str = f"""
Context:
- Time range: {context.time_range}
- Service filter: {context.service_filter or 'none'}
- Available services: {', '.join(context.available_services) if context.available_services else 'unknown'}
"""

        return TRANSLATION_PROMPT.format(
            question=question,
            context=context_str,
        )

    def _parse_translation_response(
        self,
        response: str,
        original_question: str,
    ) -> TranslationResult:
        """Parse LLM response into TranslationResult."""
        lines = response.strip().split("\n")

        observql = ""
        sql = None
        explanation = ""
        confidence = 0.8
        suggestions = []

        current_section = None

        for line in lines:
            line = line.strip()

            if line.startswith("OBSERVQL:"):
                current_section = "observql"
                observql = line[9:].strip()
            elif line.startswith("SQL:"):
                current_section = "sql"
                sql = line[4:].strip()
            elif line.startswith("EXPLANATION:"):
                current_section = "explanation"
                explanation = line[12:].strip()
            elif line.startswith("CONFIDENCE:"):
                try:
                    confidence = float(line[11:].strip())
                except ValueError:
                    confidence = 0.8
            elif line.startswith("SUGGESTIONS:"):
                current_section = "suggestions"
            elif line.startswith("- ") and current_section == "suggestions":
                suggestions.append(line[2:])
            elif current_section == "observql" and line and not line.startswith(("SQL:", "EXPLANATION:")):
                observql += " " + line
            elif current_section == "sql" and line and not line.startswith(("EXPLANATION:", "CONFIDENCE:")):
                sql = (sql or "") + " " + line
            elif current_section == "explanation" and line and not line.startswith(("CONFIDENCE:", "SUGGESTIONS:")):
                explanation += " " + line

        # Clean up
        observql = observql.strip()
        if sql:
            sql = sql.strip()
        explanation = explanation.strip()

        # Generate default if parsing failed
        if not observql:
            observql = self._generate_default_query(original_question)
            confidence = 0.5
            explanation = "Generated a basic query based on keywords."

        return TranslationResult(
            observql=observql,
            sql=sql,
            explanation=explanation,
            confidence=confidence,
            suggestions=suggestions,
        )

    def _parse_suggestions_response(self, response: str) -> SuggestionResult:
        """Parse suggestions response."""
        suggestions = []
        categories: dict[str, list[str]] = {
            "latency": [],
            "errors": [],
            "traces": [],
            "general": [],
        }

        current_category = "general"

        for line in response.strip().split("\n"):
            line = line.strip()
            if not line:
                continue

            # Check for category headers
            lower = line.lower()
            if "latency" in lower and ":" in line:
                current_category = "latency"
            elif "error" in lower and ":" in line:
                current_category = "errors"
            elif "trace" in lower and ":" in line:
                current_category = "traces"
            elif line.startswith("- ") or line.startswith("* "):
                suggestion = line[2:].strip()
                suggestions.append(suggestion)
                categories[current_category].append(suggestion)

        return SuggestionResult(
            top_suggestions=suggestions[:10],
            by_category=categories,
        )

    def _parse_explanation_response(self, response: str) -> ExplanationResult:
        """Parse explanation response."""
        components = {}
        text = response.strip()

        # Try to extract component explanations
        for line in response.split("\n"):
            if ":" in line and not line.startswith(" "):
                parts = line.split(":", 1)
                if len(parts) == 2:
                    key = parts[0].strip().lower().replace(" ", "_")
                    components[key] = parts[1].strip()

        return ExplanationResult(text=text, components=components)

    def _generate_default_query(self, question: str) -> str:
        """Generate a default query based on keywords."""
        lower = question.lower()

        if "error" in lower:
            return "SELECT * FROM otel_traces WHERE StatusCode = 'ERROR' ORDER BY Timestamp DESC LIMIT 100"
        elif "slow" in lower or "latency" in lower:
            return "SELECT * FROM otel_traces WHERE Duration > 1000000000 ORDER BY Duration DESC LIMIT 100"
        elif "trace" in lower:
            return "SELECT * FROM otel_traces ORDER BY Timestamp DESC LIMIT 100"
        elif "log" in lower:
            return "SELECT * FROM otel_logs ORDER BY Timestamp DESC LIMIT 100"
        elif "metric" in lower:
            return "SELECT * FROM otel_metrics ORDER BY Timestamp DESC LIMIT 100"
        else:
            return "SELECT * FROM otel_traces ORDER BY Timestamp DESC LIMIT 100"

    def _cache_key(self, question: str, context: Optional[QueryContext]) -> str:
        """Generate cache key for a query."""
        key_data = f"{question}:{context.time_range if context else 'default'}"
        return f"nlq:{hashlib.sha256(key_data.encode()).hexdigest()[:16]}"
