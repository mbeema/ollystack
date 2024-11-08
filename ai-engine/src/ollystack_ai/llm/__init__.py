"""
LLM Integration

Provides unified interface to LLM providers for:
- Root cause analysis
- Natural language queries
- Incident summarization
- Runbook suggestions
"""

from ollystack_ai.llm.client import (
    LLMClient,
    LLMProvider,
    LLMResponse,
    Message,
)
from ollystack_ai.llm.prompts import (
    PromptTemplate,
    RootCausePrompt,
    IncidentSummaryPrompt,
    NaturalLanguageQueryPrompt,
)
from ollystack_ai.llm.analyzer import (
    RootCauseAnalyzer,
    RootCauseResult,
    IncidentSummarizer,
    NaturalLanguageQuerier,
)

__all__ = [
    # Client
    "LLMClient",
    "LLMProvider",
    "LLMResponse",
    "Message",
    # Prompts
    "PromptTemplate",
    "RootCausePrompt",
    "IncidentSummaryPrompt",
    "NaturalLanguageQueryPrompt",
    # Analyzers
    "RootCauseAnalyzer",
    "RootCauseResult",
    "IncidentSummarizer",
    "NaturalLanguageQuerier",
]
