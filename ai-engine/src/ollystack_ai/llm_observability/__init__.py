"""
OllyStack LLM Observability

Monitor and trace LLM/AI application performance, costs, and quality.

Features:
- Token and cost tracking
- Latency monitoring
- Quality evaluation
- RAG pipeline monitoring
- Chain/Agent tracing
- Safety monitoring

Usage:
    from ollystack_ai.llm_observability import LLMObserver

    observer = LLMObserver(
        service_name="my-ai-app",
        endpoint="https://ollystack.example.com"
    )

    # Auto-instrument OpenAI
    observer.instrument_openai()

    # Manual tracking
    with observer.track_llm_call(model="gpt-4") as tracker:
        response = client.chat.completions.create(...)
        tracker.record_response(response)
"""

from ollystack_ai.llm_observability.observer import LLMObserver
from ollystack_ai.llm_observability.tracker import (
    LLMCallTracker,
    ChainTracker,
    EmbeddingTracker,
    RAGTracker,
)
from ollystack_ai.llm_observability.models import (
    LLMRequest,
    LLMCompletion,
    LLMPrompt,
    ToolCall,
    Embedding,
    RAGRetrieval,
    Chain,
    Evaluation,
    TokenUsage,
    CostInfo,
)
from ollystack_ai.llm_observability.evaluators import (
    QualityEvaluator,
    SafetyEvaluator,
    RelevanceEvaluator,
)
from ollystack_ai.llm_observability.integrations import (
    instrument_openai,
    instrument_anthropic,
    instrument_langchain,
    instrument_llamaindex,
)

__all__ = [
    # Core
    "LLMObserver",
    "LLMCallTracker",
    "ChainTracker",
    "EmbeddingTracker",
    "RAGTracker",
    # Models
    "LLMRequest",
    "LLMCompletion",
    "LLMPrompt",
    "ToolCall",
    "Embedding",
    "RAGRetrieval",
    "Chain",
    "Evaluation",
    "TokenUsage",
    "CostInfo",
    # Evaluators
    "QualityEvaluator",
    "SafetyEvaluator",
    "RelevanceEvaluator",
    # Integrations
    "instrument_openai",
    "instrument_anthropic",
    "instrument_langchain",
    "instrument_llamaindex",
]
