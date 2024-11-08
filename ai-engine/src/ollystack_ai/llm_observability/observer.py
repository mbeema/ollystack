"""
LLM Observer

Main class for LLM observability. Provides auto-instrumentation
and manual tracking APIs.
"""

import logging
import asyncio
import json
from datetime import datetime
from typing import Optional, Any, Callable
from contextlib import contextmanager
import uuid
import httpx

from ollystack_ai.llm_observability.models import (
    LLMRequest,
    LLMCompletion,
    Embedding,
    RAGRetrieval,
    Chain,
    Evaluation,
    Provider,
    RequestType,
    ChainType,
)
from ollystack_ai.llm_observability.tracker import (
    LLMCallTracker,
    EmbeddingTracker,
    RAGTracker,
    ChainTracker,
)

logger = logging.getLogger(__name__)


class LLMObserver:
    """
    Main LLM Observability class.

    Provides auto-instrumentation for popular LLM libraries and manual
    tracking APIs.

    Example:
        observer = LLMObserver(
            service_name="my-ai-app",
            endpoint="https://ollystack.example.com",
            api_key="your-api-key"
        )

        # Auto-instrument OpenAI
        observer.instrument_openai()

        # Or track manually
        with observer.track_llm_call(model="gpt-4") as tracker:
            response = client.chat.completions.create(...)
            tracker.record_response(response)
    """

    def __init__(
        self,
        service_name: str,
        endpoint: str,
        api_key: Optional[str] = None,
        environment: str = "production",
        version: str = "",
        debug: bool = False,
        batch_size: int = 10,
        flush_interval: float = 5.0,
        sample_rate: float = 1.0,
        capture_prompts: bool = True,
        capture_completions: bool = True,
        labels: Optional[dict[str, str]] = None,
    ):
        self.service_name = service_name
        self.endpoint = endpoint.rstrip("/")
        self.api_key = api_key
        self.environment = environment
        self.version = version
        self.debug = debug
        self.batch_size = batch_size
        self.flush_interval = flush_interval
        self.sample_rate = sample_rate
        self.capture_prompts = capture_prompts
        self.capture_completions = capture_completions
        self.labels = labels or {}

        # Event queue
        self._queue: list[dict] = []
        self._flush_task: Optional[asyncio.Task] = None
        self._client: Optional[httpx.AsyncClient] = None

        # Callbacks
        self._on_request: Optional[Callable[[LLMRequest], None]] = None
        self._on_error: Optional[Callable[[Exception], None]] = None

        # Instrumented libraries
        self._instrumented: set[str] = set()

        self._log("debug", f"LLMObserver initialized for {service_name}")

    # -------------------------------------------------------------------------
    # Manual Tracking API
    # -------------------------------------------------------------------------

    @contextmanager
    def track_llm_call(
        self,
        model: str,
        provider: Provider = Provider.OPENAI,
        trace_id: Optional[str] = None,
        span_id: Optional[str] = None,
        parent_span_id: Optional[str] = None,
        user_id: Optional[str] = None,
        session_id: Optional[str] = None,
        conversation_id: Optional[str] = None,
        labels: Optional[dict[str, str]] = None,
    ) -> LLMCallTracker:
        """
        Context manager for tracking an LLM call.

        Example:
            with observer.track_llm_call(model="gpt-4") as tracker:
                tracker.record_messages(messages)
                tracker.record_parameters(temperature=0.7)
                response = client.chat.completions.create(...)
                tracker.record_response(response)
        """
        combined_labels = {**self.labels, **(labels or {})}

        tracker = LLMCallTracker(
            model=model,
            provider=provider,
            service_name=self.service_name,
            environment=self.environment,
            trace_id=trace_id,
            span_id=span_id,
            parent_span_id=parent_span_id,
            user_id=user_id,
            session_id=session_id,
            conversation_id=conversation_id,
            labels=combined_labels,
        )
        tracker._on_complete = self._handle_request

        tracker.start()
        try:
            yield tracker
        except Exception as e:
            tracker.record_error(e)
            raise
        finally:
            tracker.finish()

    @contextmanager
    def track_embedding(
        self,
        model: str,
        provider: Provider = Provider.OPENAI,
        use_case: str = "",
        collection_name: str = "",
        trace_id: Optional[str] = None,
        span_id: Optional[str] = None,
    ) -> EmbeddingTracker:
        """Track an embedding operation."""
        tracker = EmbeddingTracker(
            model=model,
            provider=provider,
            service_name=self.service_name,
            environment=self.environment,
            use_case=use_case,
            collection_name=collection_name,
            trace_id=trace_id,
            span_id=span_id,
        )
        tracker._on_complete = self._handle_embedding

        tracker.start()
        try:
            yield tracker
        except Exception as e:
            tracker.record_error(e)
            raise
        finally:
            tracker.finish()

    @contextmanager
    def track_retrieval(
        self,
        vector_store: str,
        collection_name: str = "",
        request_id: Optional[str] = None,
    ) -> RAGTracker:
        """Track a RAG retrieval operation."""
        tracker = RAGTracker(
            vector_store=vector_store,
            service_name=self.service_name,
            collection_name=collection_name,
            request_id=request_id,
        )
        tracker._on_complete = self._handle_retrieval

        tracker.start()
        try:
            yield tracker
        finally:
            tracker.finish()

    @contextmanager
    def track_chain(
        self,
        chain_name: str,
        chain_type: ChainType = ChainType.SEQUENTIAL,
        trace_id: Optional[str] = None,
        user_id: Optional[str] = None,
        session_id: Optional[str] = None,
        conversation_id: Optional[str] = None,
        labels: Optional[dict[str, str]] = None,
    ) -> ChainTracker:
        """Track an LLM chain/workflow."""
        combined_labels = {**self.labels, **(labels or {})}

        tracker = ChainTracker(
            chain_name=chain_name,
            chain_type=chain_type,
            service_name=self.service_name,
            environment=self.environment,
            trace_id=trace_id,
            user_id=user_id,
            session_id=session_id,
            conversation_id=conversation_id,
            labels=combined_labels,
        )
        tracker._on_complete = self._handle_chain

        tracker.start()
        try:
            yield tracker
        except Exception as e:
            tracker.record_error(e)
            raise
        finally:
            tracker.finish()

    def record_evaluation(
        self,
        request_id: str,
        evaluation_type: str = "auto",
        overall_score: float = 0.0,
        relevance_score: float = 0.0,
        faithfulness_score: float = 0.0,
        safety_score: float = 0.0,
        feedback: str = "",
        failure_reasons: Optional[list[str]] = None,
        evaluator_model: str = "",
        custom_scores: Optional[dict[str, float]] = None,
    ) -> Evaluation:
        """Record an evaluation result."""
        evaluation = Evaluation(
            request_id=request_id,
            service_name=self.service_name,
            evaluation_type=evaluation_type,
            overall_score=overall_score,
            relevance_score=relevance_score,
            faithfulness_score=faithfulness_score,
            safety_score=safety_score,
            feedback=feedback,
            failure_reasons=failure_reasons or [],
            evaluator_model=evaluator_model,
            custom_scores=custom_scores or {},
        )

        self._handle_evaluation(evaluation)
        return evaluation

    def record_user_feedback(
        self,
        request_id: str,
        rating: int,  # -1, 0, or 1
        feedback: str = "",
    ) -> None:
        """Record user feedback for a request."""
        self._enqueue({
            "type": "user_feedback",
            "request_id": request_id,
            "timestamp": datetime.utcnow().isoformat(),
            "service_name": self.service_name,
            "rating": rating,
            "feedback": feedback,
        })

    # -------------------------------------------------------------------------
    # Auto-Instrumentation
    # -------------------------------------------------------------------------

    def instrument_openai(self) -> None:
        """Auto-instrument OpenAI library."""
        if "openai" in self._instrumented:
            return

        from ollystack_ai.llm_observability.integrations import instrument_openai
        instrument_openai(self)
        self._instrumented.add("openai")
        self._log("info", "OpenAI instrumented")

    def instrument_anthropic(self) -> None:
        """Auto-instrument Anthropic library."""
        if "anthropic" in self._instrumented:
            return

        from ollystack_ai.llm_observability.integrations import instrument_anthropic
        instrument_anthropic(self)
        self._instrumented.add("anthropic")
        self._log("info", "Anthropic instrumented")

    def instrument_langchain(self) -> None:
        """Auto-instrument LangChain."""
        if "langchain" in self._instrumented:
            return

        from ollystack_ai.llm_observability.integrations import instrument_langchain
        instrument_langchain(self)
        self._instrumented.add("langchain")
        self._log("info", "LangChain instrumented")

    def instrument_llamaindex(self) -> None:
        """Auto-instrument LlamaIndex."""
        if "llamaindex" in self._instrumented:
            return

        from ollystack_ai.llm_observability.integrations import instrument_llamaindex
        instrument_llamaindex(self)
        self._instrumented.add("llamaindex")
        self._log("info", "LlamaIndex instrumented")

    def instrument_all(self) -> None:
        """Instrument all available libraries."""
        try:
            self.instrument_openai()
        except ImportError:
            pass

        try:
            self.instrument_anthropic()
        except ImportError:
            pass

        try:
            self.instrument_langchain()
        except ImportError:
            pass

        try:
            self.instrument_llamaindex()
        except ImportError:
            pass

    # -------------------------------------------------------------------------
    # Callbacks
    # -------------------------------------------------------------------------

    def on_request(self, callback: Callable[[LLMRequest], None]) -> None:
        """Set callback for when a request is tracked."""
        self._on_request = callback

    def on_error(self, callback: Callable[[Exception], None]) -> None:
        """Set callback for errors."""
        self._on_error = callback

    # -------------------------------------------------------------------------
    # Data Export
    # -------------------------------------------------------------------------

    async def flush(self) -> None:
        """Flush pending events."""
        if not self._queue:
            return

        events = self._queue[:]
        self._queue.clear()

        try:
            await self._send_events(events)
        except Exception as e:
            self._log("error", f"Failed to flush events: {e}")
            if self._on_error:
                self._on_error(e)
            # Re-queue events
            self._queue.extend(events)

    def flush_sync(self) -> None:
        """Synchronous flush."""
        import asyncio
        try:
            loop = asyncio.get_event_loop()
            if loop.is_running():
                asyncio.create_task(self.flush())
            else:
                loop.run_until_complete(self.flush())
        except RuntimeError:
            asyncio.run(self.flush())

    async def close(self) -> None:
        """Close the observer and flush pending events."""
        await self.flush()
        if self._client:
            await self._client.aclose()

    # -------------------------------------------------------------------------
    # Internal Methods
    # -------------------------------------------------------------------------

    def _handle_request(self, request: LLMRequest) -> None:
        """Handle a completed LLM request."""
        # Sample rate
        import random
        if random.random() > self.sample_rate:
            return

        if self._on_request:
            self._on_request(request)

        # Prepare event
        event = {
            "type": "llm_request",
            **request.to_dict(),
        }

        # Add prompts if enabled
        if self.capture_prompts:
            event["prompts"] = [p.to_dict() for p in request.prompts]

        # Add completions if enabled
        if self.capture_completions:
            event["completions"] = [c.to_dict() for c in request.completions]

        # Add tool calls
        if request.tool_calls:
            event["tool_calls"] = [t.to_dict() for t in request.tool_calls]

        self._enqueue(event)

    def _handle_embedding(self, embedding: Embedding) -> None:
        """Handle a completed embedding."""
        self._enqueue({
            "type": "embedding",
            **embedding.to_dict(),
        })

    def _handle_retrieval(self, retrieval: RAGRetrieval) -> None:
        """Handle a completed retrieval."""
        self._enqueue({
            "type": "rag_retrieval",
            **retrieval.to_dict(),
        })

    def _handle_chain(self, chain: Chain) -> None:
        """Handle a completed chain."""
        self._enqueue({
            "type": "chain",
            **chain.to_dict(),
        })

    def _handle_evaluation(self, evaluation: Evaluation) -> None:
        """Handle an evaluation."""
        self._enqueue({
            "type": "evaluation",
            **evaluation.to_dict(),
        })

    def _enqueue(self, event: dict) -> None:
        """Add event to queue."""
        self._queue.append(event)

        if len(self._queue) >= self.batch_size:
            self.flush_sync()

    async def _send_events(self, events: list[dict]) -> None:
        """Send events to the backend."""
        if not events:
            return

        if not self._client:
            self._client = httpx.AsyncClient(timeout=30.0)

        url = f"{self.endpoint}/api/v1/llm/events"
        headers = {
            "Content-Type": "application/json",
        }
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"

        payload = {
            "events": events,
            "metadata": {
                "service_name": self.service_name,
                "environment": self.environment,
                "version": self.version,
                "sdk_version": "0.1.0",
            },
        }

        response = await self._client.post(url, json=payload, headers=headers)
        response.raise_for_status()

        self._log("debug", f"Sent {len(events)} events")

    def _log(self, level: str, message: str) -> None:
        """Log a message."""
        if not self.debug and level == "debug":
            return

        log_fn = getattr(logger, level, logger.info)
        log_fn(f"[LLMObserver] {message}")


# Global instance
_observer: Optional[LLMObserver] = None


def init(
    service_name: str,
    endpoint: str,
    api_key: Optional[str] = None,
    **kwargs,
) -> LLMObserver:
    """Initialize the global LLM observer."""
    global _observer
    _observer = LLMObserver(
        service_name=service_name,
        endpoint=endpoint,
        api_key=api_key,
        **kwargs,
    )
    return _observer


def get_observer() -> Optional[LLMObserver]:
    """Get the global LLM observer."""
    return _observer
