"""
LLM Call Trackers

Context managers and classes for tracking LLM operations.
"""

import time
import hashlib
import logging
from contextlib import contextmanager
from datetime import datetime
from typing import Optional, Any, Generator
import uuid

from ollystack_ai.llm_observability.models import (
    LLMRequest,
    LLMCompletion,
    LLMPrompt,
    ToolCall,
    Embedding,
    RAGRetrieval,
    Chain,
    ChainStep,
    TokenUsage,
    CostInfo,
    Provider,
    RequestType,
    RequestStatus,
    FinishReason,
    ChainType,
)

logger = logging.getLogger(__name__)


# Cost per 1M tokens (in microdollars)
MODEL_COSTS = {
    # OpenAI
    "gpt-4": {"prompt": 30_000, "completion": 60_000},
    "gpt-4-turbo": {"prompt": 10_000, "completion": 30_000},
    "gpt-4o": {"prompt": 5_000, "completion": 15_000},
    "gpt-4o-mini": {"prompt": 150, "completion": 600},
    "gpt-3.5-turbo": {"prompt": 500, "completion": 1_500},
    # Anthropic
    "claude-3-opus": {"prompt": 15_000, "completion": 75_000},
    "claude-3-sonnet": {"prompt": 3_000, "completion": 15_000},
    "claude-3-haiku": {"prompt": 250, "completion": 1_250},
    "claude-3.5-sonnet": {"prompt": 3_000, "completion": 15_000},
    # Embeddings
    "text-embedding-3-small": {"prompt": 20, "completion": 0},
    "text-embedding-3-large": {"prompt": 130, "completion": 0},
    "text-embedding-ada-002": {"prompt": 100, "completion": 0},
}


def calculate_cost(model: str, prompt_tokens: int, completion_tokens: int) -> CostInfo:
    """Calculate cost based on model and token usage."""
    # Find matching model (handle versioned names)
    model_key = model.lower()
    costs = None
    for key, value in MODEL_COSTS.items():
        if key in model_key:
            costs = value
            break

    if not costs:
        # Default costs if model not found
        costs = {"prompt": 1_000, "completion": 2_000}

    prompt_cost = (prompt_tokens * costs["prompt"]) // 1_000_000
    completion_cost = (completion_tokens * costs["completion"]) // 1_000_000

    return CostInfo(
        prompt_cost_micros=prompt_cost,
        completion_cost_micros=completion_cost,
        total_cost_micros=prompt_cost + completion_cost,
    )


def content_hash(content: str) -> str:
    """Generate a hash for content deduplication."""
    return hashlib.sha256(content.encode()).hexdigest()[:16]


class LLMCallTracker:
    """
    Tracks a single LLM API call.

    Usage:
        with tracker.track_llm_call(model="gpt-4") as call:
            response = client.chat.completions.create(...)
            call.record_response(response)
    """

    def __init__(
        self,
        model: str,
        provider: Provider = Provider.OPENAI,
        service_name: str = "",
        environment: str = "production",
        trace_id: Optional[str] = None,
        span_id: Optional[str] = None,
        parent_span_id: Optional[str] = None,
        user_id: Optional[str] = None,
        session_id: Optional[str] = None,
        conversation_id: Optional[str] = None,
        labels: Optional[dict[str, str]] = None,
    ):
        self.request = LLMRequest(
            id=str(uuid.uuid4()),
            timestamp=datetime.utcnow(),
            trace_id=trace_id or str(uuid.uuid4()),
            span_id=span_id or str(uuid.uuid4())[:16],
            parent_span_id=parent_span_id or "",
            service_name=service_name,
            environment=environment,
            provider=provider,
            model=model,
            user_id=user_id or "",
            session_id=session_id or "",
            conversation_id=conversation_id or "",
            labels=labels or {},
        )
        self._start_time: float = 0
        self._first_token_time: Optional[float] = None
        self._on_complete: Optional[callable] = None

    def start(self) -> "LLMCallTracker":
        """Start timing the request."""
        self._start_time = time.perf_counter()
        return self

    def record_prompt(
        self,
        role: str,
        content: str,
        template_id: Optional[str] = None,
        template_name: Optional[str] = None,
        template_variables: Optional[dict[str, str]] = None,
    ) -> None:
        """Record a prompt message."""
        prompt = LLMPrompt(
            request_id=self.request.id,
            timestamp=datetime.utcnow(),
            role=role,
            content=content,
            content_hash=content_hash(content),
            template_id=template_id,
            template_name=template_name,
            template_variables=template_variables or {},
            message_index=len(self.request.prompts),
        )
        self.request.prompts.append(prompt)

    def record_messages(self, messages: list[dict]) -> None:
        """Record a list of messages (OpenAI format)."""
        for i, msg in enumerate(messages):
            role = msg.get("role", "user")
            content = msg.get("content", "")
            if isinstance(content, list):
                # Handle multimodal content
                content = str(content)
            self.record_prompt(role=role, content=content)

    def record_parameters(
        self,
        temperature: Optional[float] = None,
        max_tokens: Optional[int] = None,
        top_p: Optional[float] = None,
        frequency_penalty: Optional[float] = None,
        presence_penalty: Optional[float] = None,
        stop: Optional[list[str]] = None,
        stream: Optional[bool] = None,
    ) -> None:
        """Record request parameters."""
        if temperature is not None:
            self.request.temperature = temperature
        if max_tokens is not None:
            self.request.max_tokens = max_tokens
        if top_p is not None:
            self.request.top_p = top_p
        if frequency_penalty is not None:
            self.request.frequency_penalty = frequency_penalty
        if presence_penalty is not None:
            self.request.presence_penalty = presence_penalty
        if stop is not None:
            self.request.stop_sequences = stop
        if stream is not None:
            self.request.is_streaming = stream

    def record_first_token(self) -> None:
        """Record when the first token was received (for streaming)."""
        if self._first_token_time is None:
            self._first_token_time = time.perf_counter()
            self.request.time_to_first_token_ms = (
                self._first_token_time - self._start_time
            ) * 1000

    def record_stream_chunk(self) -> None:
        """Record a streaming chunk."""
        self.request.stream_chunks += 1
        if self._first_token_time is None:
            self.record_first_token()

    def record_response(
        self,
        response: Any,
        finish_reason: Optional[str] = None,
    ) -> None:
        """
        Record the response from the LLM.

        Works with OpenAI and Anthropic response objects.
        """
        # Calculate duration
        end_time = time.perf_counter()
        self.request.duration_ms = (end_time - self._start_time) * 1000

        # Handle OpenAI response format
        if hasattr(response, "choices"):
            self._record_openai_response(response)
        # Handle Anthropic response format
        elif hasattr(response, "content") and hasattr(response, "stop_reason"):
            self._record_anthropic_response(response)
        # Handle dict format
        elif isinstance(response, dict):
            self._record_dict_response(response)

        # Calculate cost
        self.request.cost = calculate_cost(
            self.request.model,
            self.request.tokens.prompt_tokens,
            self.request.tokens.completion_tokens,
        )

        # Calculate tokens per second
        if self.request.tokens.completion_tokens > 0 and self.request.duration_ms > 0:
            self.request.tokens_per_second = (
                self.request.tokens.completion_tokens / (self.request.duration_ms / 1000)
            )

        self.request.status = RequestStatus.SUCCESS
        self.request.status_code = 200

    def _record_openai_response(self, response: Any) -> None:
        """Record OpenAI-format response."""
        # Usage
        if hasattr(response, "usage") and response.usage:
            self.request.tokens = TokenUsage(
                prompt_tokens=response.usage.prompt_tokens or 0,
                completion_tokens=response.usage.completion_tokens or 0,
            )

        # Choices
        for i, choice in enumerate(response.choices):
            message = choice.message if hasattr(choice, "message") else choice

            content = ""
            if hasattr(message, "content") and message.content:
                content = message.content

            finish = FinishReason.STOP
            if hasattr(choice, "finish_reason"):
                finish = self._map_finish_reason(choice.finish_reason)

            completion = LLMCompletion(
                request_id=self.request.id,
                timestamp=datetime.utcnow(),
                content=content,
                content_hash=content_hash(content),
                finish_reason=finish,
                choice_index=i,
            )
            self.request.completions.append(completion)

            # Tool calls
            if hasattr(message, "tool_calls") and message.tool_calls:
                self.request.has_tool_calls = True
                for j, tc in enumerate(message.tool_calls):
                    tool_call = ToolCall(
                        request_id=self.request.id,
                        timestamp=datetime.utcnow(),
                        tool_name=tc.function.name if hasattr(tc, "function") else tc.get("name", ""),
                        tool_type="function",
                        input_arguments=tc.function.arguments if hasattr(tc, "function") else "",
                        call_index=j,
                    )
                    self.request.tool_calls.append(tool_call)
                    self.request.tool_names.append(tool_call.tool_name)

                self.request.tool_call_count = len(self.request.tool_calls)

    def _record_anthropic_response(self, response: Any) -> None:
        """Record Anthropic-format response."""
        # Usage
        if hasattr(response, "usage"):
            self.request.tokens = TokenUsage(
                prompt_tokens=response.usage.input_tokens or 0,
                completion_tokens=response.usage.output_tokens or 0,
            )

        # Content
        content = ""
        if hasattr(response, "content"):
            for block in response.content:
                if hasattr(block, "text"):
                    content += block.text
                elif hasattr(block, "type") and block.type == "tool_use":
                    self.request.has_tool_calls = True
                    tool_call = ToolCall(
                        request_id=self.request.id,
                        timestamp=datetime.utcnow(),
                        tool_name=block.name,
                        tool_type="function",
                        input_arguments=str(block.input),
                        call_index=len(self.request.tool_calls),
                    )
                    self.request.tool_calls.append(tool_call)
                    self.request.tool_names.append(tool_call.tool_name)

            self.request.tool_call_count = len(self.request.tool_calls)

        finish = self._map_finish_reason(response.stop_reason)

        completion = LLMCompletion(
            request_id=self.request.id,
            timestamp=datetime.utcnow(),
            content=content,
            content_hash=content_hash(content),
            finish_reason=finish,
            choice_index=0,
        )
        self.request.completions.append(completion)

    def _record_dict_response(self, response: dict) -> None:
        """Record dict-format response."""
        # Usage
        usage = response.get("usage", {})
        self.request.tokens = TokenUsage(
            prompt_tokens=usage.get("prompt_tokens", 0),
            completion_tokens=usage.get("completion_tokens", 0),
        )

        # Choices
        for i, choice in enumerate(response.get("choices", [])):
            message = choice.get("message", {})
            content = message.get("content", "")

            completion = LLMCompletion(
                request_id=self.request.id,
                timestamp=datetime.utcnow(),
                content=content,
                content_hash=content_hash(content),
                finish_reason=self._map_finish_reason(choice.get("finish_reason")),
                choice_index=i,
            )
            self.request.completions.append(completion)

    def _map_finish_reason(self, reason: Optional[str]) -> FinishReason:
        """Map provider finish reason to enum."""
        if not reason:
            return FinishReason.STOP

        reason = reason.lower()
        if reason in ("stop", "end_turn"):
            return FinishReason.STOP
        elif reason in ("length", "max_tokens"):
            return FinishReason.LENGTH
        elif reason in ("tool_calls", "tool_use"):
            return FinishReason.TOOL_CALLS
        elif reason in ("content_filter",):
            return FinishReason.CONTENT_FILTER
        else:
            return FinishReason.STOP

    def record_error(
        self,
        error: Exception,
        status_code: int = 500,
    ) -> None:
        """Record an error."""
        end_time = time.perf_counter()
        self.request.duration_ms = (end_time - self._start_time) * 1000
        self.request.status = RequestStatus.ERROR
        self.request.status_code = status_code
        self.request.error_message = str(error)
        self.request.error_type = type(error).__name__

        # Detect specific error types
        error_str = str(error).lower()
        if "rate" in error_str and "limit" in error_str:
            self.request.status = RequestStatus.RATE_LIMITED
        elif "timeout" in error_str:
            self.request.status = RequestStatus.TIMEOUT

    def record_rag_context(
        self,
        chunks: list[dict],
        retrieval_time_ms: float = 0.0,
    ) -> None:
        """Record RAG context."""
        self.request.has_rag_context = True
        self.request.rag_chunk_count = len(chunks)
        self.request.rag_retrieval_time_ms = retrieval_time_ms

    def set_quality_scores(
        self,
        quality: float = 0.0,
        relevance: float = 0.0,
        coherence: float = 0.0,
        factuality: float = 0.0,
    ) -> None:
        """Set quality evaluation scores."""
        self.request.quality_score = quality
        self.request.relevance_score = relevance
        self.request.coherence_score = coherence
        self.request.factuality_score = factuality

    def set_safety_flags(
        self,
        contains_pii: bool = False,
        safety_flagged: bool = False,
        categories: Optional[list[str]] = None,
    ) -> None:
        """Set safety flags."""
        self.request.contains_pii = contains_pii
        self.request.safety_flagged = safety_flagged
        self.request.safety_categories = categories or []

    def finish(self) -> LLMRequest:
        """Finish tracking and return the request."""
        if self._on_complete:
            self._on_complete(self.request)
        return self.request

    def __enter__(self) -> "LLMCallTracker":
        return self.start()

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        if exc_type is not None:
            self.record_error(exc_val)
        self.finish()


class EmbeddingTracker:
    """Tracks embedding operations."""

    def __init__(
        self,
        model: str,
        provider: Provider = Provider.OPENAI,
        service_name: str = "",
        environment: str = "production",
        use_case: str = "",
        collection_name: str = "",
        trace_id: Optional[str] = None,
        span_id: Optional[str] = None,
    ):
        self.embedding = Embedding(
            id=str(uuid.uuid4()),
            timestamp=datetime.utcnow(),
            trace_id=trace_id or str(uuid.uuid4()),
            span_id=span_id or str(uuid.uuid4())[:16],
            service_name=service_name,
            environment=environment,
            provider=provider,
            model=model,
            use_case=use_case,
            collection_name=collection_name,
        )
        self._start_time: float = 0
        self._on_complete: Optional[callable] = None

    def start(self) -> "EmbeddingTracker":
        self._start_time = time.perf_counter()
        return self

    def record_input(self, inputs: list[str]) -> None:
        """Record input texts."""
        self.embedding.input_count = len(inputs)
        # Estimate tokens (rough: 1 token per 4 chars)
        self.embedding.total_tokens = sum(len(t) // 4 for t in inputs)

    def record_response(self, response: Any) -> None:
        """Record response."""
        end_time = time.perf_counter()
        self.embedding.duration_ms = (end_time - self._start_time) * 1000

        # OpenAI format
        if hasattr(response, "usage"):
            self.embedding.total_tokens = response.usage.total_tokens or self.embedding.total_tokens

        if hasattr(response, "data") and len(response.data) > 0:
            self.embedding.dimensions = len(response.data[0].embedding)

        # Calculate cost
        costs = MODEL_COSTS.get(self.embedding.model.lower(), {"prompt": 100, "completion": 0})
        self.embedding.cost_micros = (self.embedding.total_tokens * costs["prompt"]) // 1_000_000

        self.embedding.status = RequestStatus.SUCCESS

    def record_error(self, error: Exception) -> None:
        """Record error."""
        end_time = time.perf_counter()
        self.embedding.duration_ms = (end_time - self._start_time) * 1000
        self.embedding.status = RequestStatus.ERROR
        self.embedding.error_message = str(error)

    def finish(self) -> Embedding:
        if self._on_complete:
            self._on_complete(self.embedding)
        return self.embedding

    def __enter__(self) -> "EmbeddingTracker":
        return self.start()

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        if exc_type is not None:
            self.record_error(exc_val)
        self.finish()


class RAGTracker:
    """Tracks RAG retrieval operations."""

    def __init__(
        self,
        vector_store: str,
        service_name: str = "",
        collection_name: str = "",
        request_id: Optional[str] = None,
    ):
        self.retrieval = RAGRetrieval(
            id=str(uuid.uuid4()),
            request_id=request_id or "",
            timestamp=datetime.utcnow(),
            service_name=service_name,
            vector_store=vector_store,
            collection_name=collection_name,
        )
        self._start_time: float = 0
        self._on_complete: Optional[callable] = None

    def start(self) -> "RAGTracker":
        self._start_time = time.perf_counter()
        return self

    def record_query(
        self,
        query: str,
        top_k: int = 10,
        search_type: str = "similarity",
        filters: Optional[dict] = None,
    ) -> None:
        """Record the query."""
        self.retrieval.query = query
        self.retrieval.query_tokens = len(query) // 4
        self.retrieval.top_k = top_k
        self.retrieval.search_type = search_type
        if filters:
            import json
            self.retrieval.metadata_filters = json.dumps(filters)

    def record_embedding_time(self, duration_ms: float) -> None:
        """Record query embedding time."""
        self.retrieval.query_embedding_time_ms = duration_ms

    def record_results(
        self,
        results: list[dict],
        scores: Optional[list[float]] = None,
    ) -> None:
        """Record retrieval results."""
        end_time = time.perf_counter()
        self.retrieval.search_time_ms = (end_time - self._start_time) * 1000

        self.retrieval.result_count = len(results)
        self.retrieval.chunks = results[:10]  # Store first 10 for debugging

        if scores:
            self.retrieval.avg_similarity_score = sum(scores) / len(scores) if scores else 0
            self.retrieval.min_similarity_score = min(scores) if scores else 0
            self.retrieval.max_similarity_score = max(scores) if scores else 0

        # Estimate total tokens
        total_tokens = 0
        for r in results:
            content = r.get("content", r.get("text", ""))
            total_tokens += len(content) // 4
        self.retrieval.total_chunk_tokens = total_tokens

    def record_reranking(self, model: str, duration_ms: float) -> None:
        """Record reranking step."""
        self.retrieval.reranker_used = True
        self.retrieval.reranker_model = model
        self.retrieval.rerank_time_ms = duration_ms

    def finish(self) -> RAGRetrieval:
        if self._on_complete:
            self._on_complete(self.retrieval)
        return self.retrieval

    def __enter__(self) -> "RAGTracker":
        return self.start()

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.finish()


class ChainTracker:
    """Tracks LLM chain/workflow executions."""

    def __init__(
        self,
        chain_name: str,
        chain_type: ChainType = ChainType.SEQUENTIAL,
        service_name: str = "",
        environment: str = "production",
        trace_id: Optional[str] = None,
        user_id: Optional[str] = None,
        session_id: Optional[str] = None,
        conversation_id: Optional[str] = None,
        labels: Optional[dict[str, str]] = None,
    ):
        self.chain = Chain(
            id=str(uuid.uuid4()),
            timestamp=datetime.utcnow(),
            trace_id=trace_id or str(uuid.uuid4()),
            root_span_id=str(uuid.uuid4())[:16],
            service_name=service_name,
            environment=environment,
            chain_type=chain_type,
            chain_name=chain_name,
            user_id=user_id or "",
            session_id=session_id or "",
            conversation_id=conversation_id or "",
            labels=labels or {},
        )
        self._start_time: float = 0
        self._on_complete: Optional[callable] = None

    def start(self) -> "ChainTracker":
        self._start_time = time.perf_counter()
        return self

    def record_input(self, input: str) -> None:
        """Record chain input."""
        self.chain.input = input

    def record_output(self, output: str) -> None:
        """Record chain output."""
        self.chain.output = output

    def add_llm_call(self, request: LLMRequest) -> None:
        """Add an LLM call to the chain."""
        self.chain.llm_call_count += 1
        self.chain.total_prompt_tokens += request.tokens.prompt_tokens
        self.chain.total_completion_tokens += request.tokens.completion_tokens
        self.chain.total_cost_micros += request.cost.total_cost_micros

        step = ChainStep(
            chain_id=self.chain.id,
            step_index=self.chain.step_count,
            step_name=f"llm_{request.model}",
            step_type="llm",
            duration_ms=request.duration_ms,
            status=request.status,
            llm_request_id=request.id,
        )
        self.chain.steps.append(step)
        self.chain.step_count += 1

    def add_tool_call(self, tool_call: ToolCall) -> None:
        """Add a tool call to the chain."""
        self.chain.tool_call_count += 1

        step = ChainStep(
            chain_id=self.chain.id,
            step_index=self.chain.step_count,
            step_name=f"tool_{tool_call.tool_name}",
            step_type="tool",
            duration_ms=tool_call.duration_ms,
            status=tool_call.status,
            tool_call_id=tool_call.id,
        )
        self.chain.steps.append(step)
        self.chain.step_count += 1

    def add_retrieval(self, retrieval: RAGRetrieval) -> None:
        """Add a retrieval to the chain."""
        self.chain.retrieval_count += 1

        step = ChainStep(
            chain_id=self.chain.id,
            step_index=self.chain.step_count,
            step_name=f"retrieval_{retrieval.vector_store}",
            step_type="retrieval",
            duration_ms=retrieval.search_time_ms,
            status=RequestStatus.SUCCESS,
            retrieval_id=retrieval.id,
        )
        self.chain.steps.append(step)
        self.chain.step_count += 1

    def record_error(self, error: Exception, step: int = 0) -> None:
        """Record an error."""
        self.chain.status = RequestStatus.ERROR
        self.chain.error_message = str(error)
        self.chain.error_step = step

    def finish(self) -> Chain:
        end_time = time.perf_counter()
        self.chain.total_duration_ms = (end_time - self._start_time) * 1000

        if self.chain.status != RequestStatus.ERROR:
            self.chain.status = RequestStatus.SUCCESS

        if self._on_complete:
            self._on_complete(self.chain)
        return self.chain

    def __enter__(self) -> "ChainTracker":
        return self.start()

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        if exc_type is not None:
            self.record_error(exc_val, self.chain.step_count)
        self.finish()
