"""
LLM Library Integrations

Auto-instrumentation for popular LLM libraries:
- OpenAI
- Anthropic
- LangChain
- LlamaIndex
"""

import functools
import logging
from typing import Any, Optional, TYPE_CHECKING

if TYPE_CHECKING:
    from ollystack_ai.llm_observability.observer import LLMObserver

from ollystack_ai.llm_observability.models import Provider

logger = logging.getLogger(__name__)


def instrument_openai(observer: "LLMObserver") -> None:
    """
    Auto-instrument OpenAI library.

    Patches:
    - client.chat.completions.create
    - client.completions.create
    - client.embeddings.create
    """
    try:
        import openai
    except ImportError:
        raise ImportError("openai package not installed")

    # Store original methods
    original_chat_create = openai.resources.chat.completions.Completions.create
    original_async_chat_create = openai.resources.chat.completions.AsyncCompletions.create
    original_embeddings_create = openai.resources.embeddings.Embeddings.create
    original_async_embeddings_create = openai.resources.embeddings.AsyncEmbeddings.create

    @functools.wraps(original_chat_create)
    def patched_chat_create(self, *args, **kwargs):
        model = kwargs.get("model", "gpt-3.5-turbo")
        messages = kwargs.get("messages", [])
        stream = kwargs.get("stream", False)

        with observer.track_llm_call(model=model, provider=Provider.OPENAI) as tracker:
            tracker.record_messages(messages)
            tracker.record_parameters(
                temperature=kwargs.get("temperature"),
                max_tokens=kwargs.get("max_tokens"),
                top_p=kwargs.get("top_p"),
                frequency_penalty=kwargs.get("frequency_penalty"),
                presence_penalty=kwargs.get("presence_penalty"),
                stop=kwargs.get("stop"),
                stream=stream,
            )

            response = original_chat_create(self, *args, **kwargs)

            if stream:
                # Return wrapped streaming response
                return _wrap_stream_response(response, tracker)
            else:
                tracker.record_response(response)
                return response

    @functools.wraps(original_async_chat_create)
    async def patched_async_chat_create(self, *args, **kwargs):
        model = kwargs.get("model", "gpt-3.5-turbo")
        messages = kwargs.get("messages", [])
        stream = kwargs.get("stream", False)

        with observer.track_llm_call(model=model, provider=Provider.OPENAI) as tracker:
            tracker.record_messages(messages)
            tracker.record_parameters(
                temperature=kwargs.get("temperature"),
                max_tokens=kwargs.get("max_tokens"),
                top_p=kwargs.get("top_p"),
                frequency_penalty=kwargs.get("frequency_penalty"),
                presence_penalty=kwargs.get("presence_penalty"),
                stop=kwargs.get("stop"),
                stream=stream,
            )

            response = await original_async_chat_create(self, *args, **kwargs)

            if stream:
                return _wrap_async_stream_response(response, tracker)
            else:
                tracker.record_response(response)
                return response

    @functools.wraps(original_embeddings_create)
    def patched_embeddings_create(self, *args, **kwargs):
        model = kwargs.get("model", "text-embedding-ada-002")
        input_texts = kwargs.get("input", [])
        if isinstance(input_texts, str):
            input_texts = [input_texts]

        with observer.track_embedding(model=model, provider=Provider.OPENAI) as tracker:
            tracker.record_input(input_texts)
            response = original_embeddings_create(self, *args, **kwargs)
            tracker.record_response(response)
            return response

    @functools.wraps(original_async_embeddings_create)
    async def patched_async_embeddings_create(self, *args, **kwargs):
        model = kwargs.get("model", "text-embedding-ada-002")
        input_texts = kwargs.get("input", [])
        if isinstance(input_texts, str):
            input_texts = [input_texts]

        with observer.track_embedding(model=model, provider=Provider.OPENAI) as tracker:
            tracker.record_input(input_texts)
            response = await original_async_embeddings_create(self, *args, **kwargs)
            tracker.record_response(response)
            return response

    # Apply patches
    openai.resources.chat.completions.Completions.create = patched_chat_create
    openai.resources.chat.completions.AsyncCompletions.create = patched_async_chat_create
    openai.resources.embeddings.Embeddings.create = patched_embeddings_create
    openai.resources.embeddings.AsyncEmbeddings.create = patched_async_embeddings_create


def _wrap_stream_response(stream, tracker):
    """Wrap a streaming response to track chunks."""
    collected_content = []

    for chunk in stream:
        tracker.record_stream_chunk()

        if hasattr(chunk, "choices") and chunk.choices:
            delta = chunk.choices[0].delta
            if hasattr(delta, "content") and delta.content:
                collected_content.append(delta.content)

        yield chunk

    # Create a synthetic response for tracking
    class SyntheticResponse:
        def __init__(self):
            self.choices = [type("Choice", (), {
                "message": type("Message", (), {
                    "content": "".join(collected_content),
                    "tool_calls": None,
                })(),
                "finish_reason": "stop",
            })()]
            self.usage = type("Usage", (), {
                "prompt_tokens": 0,
                "completion_tokens": len("".join(collected_content)) // 4,
                "total_tokens": 0,
            })()

    tracker.record_response(SyntheticResponse())


async def _wrap_async_stream_response(stream, tracker):
    """Wrap an async streaming response."""
    collected_content = []

    async for chunk in stream:
        tracker.record_stream_chunk()

        if hasattr(chunk, "choices") and chunk.choices:
            delta = chunk.choices[0].delta
            if hasattr(delta, "content") and delta.content:
                collected_content.append(delta.content)

        yield chunk

    class SyntheticResponse:
        def __init__(self):
            self.choices = [type("Choice", (), {
                "message": type("Message", (), {
                    "content": "".join(collected_content),
                    "tool_calls": None,
                })(),
                "finish_reason": "stop",
            })()]
            self.usage = type("Usage", (), {
                "prompt_tokens": 0,
                "completion_tokens": len("".join(collected_content)) // 4,
                "total_tokens": 0,
            })()

    tracker.record_response(SyntheticResponse())


def instrument_anthropic(observer: "LLMObserver") -> None:
    """
    Auto-instrument Anthropic library.

    Patches:
    - client.messages.create
    """
    try:
        import anthropic
    except ImportError:
        raise ImportError("anthropic package not installed")

    original_create = anthropic.resources.messages.Messages.create
    original_async_create = anthropic.resources.messages.AsyncMessages.create

    @functools.wraps(original_create)
    def patched_create(self, *args, **kwargs):
        model = kwargs.get("model", "claude-3-sonnet-20240229")
        messages = kwargs.get("messages", [])
        system = kwargs.get("system", "")

        with observer.track_llm_call(model=model, provider=Provider.ANTHROPIC) as tracker:
            # Record system prompt
            if system:
                tracker.record_prompt(role="system", content=system)

            # Record messages
            for msg in messages:
                role = msg.get("role", "user")
                content = msg.get("content", "")
                if isinstance(content, list):
                    content = str(content)
                tracker.record_prompt(role=role, content=content)

            tracker.record_parameters(
                temperature=kwargs.get("temperature"),
                max_tokens=kwargs.get("max_tokens"),
                top_p=kwargs.get("top_p"),
                stream=kwargs.get("stream", False),
            )

            response = original_create(self, *args, **kwargs)
            tracker.record_response(response)
            return response

    @functools.wraps(original_async_create)
    async def patched_async_create(self, *args, **kwargs):
        model = kwargs.get("model", "claude-3-sonnet-20240229")
        messages = kwargs.get("messages", [])
        system = kwargs.get("system", "")

        with observer.track_llm_call(model=model, provider=Provider.ANTHROPIC) as tracker:
            if system:
                tracker.record_prompt(role="system", content=system)

            for msg in messages:
                role = msg.get("role", "user")
                content = msg.get("content", "")
                if isinstance(content, list):
                    content = str(content)
                tracker.record_prompt(role=role, content=content)

            tracker.record_parameters(
                temperature=kwargs.get("temperature"),
                max_tokens=kwargs.get("max_tokens"),
                top_p=kwargs.get("top_p"),
                stream=kwargs.get("stream", False),
            )

            response = await original_async_create(self, *args, **kwargs)
            tracker.record_response(response)
            return response

    anthropic.resources.messages.Messages.create = patched_create
    anthropic.resources.messages.AsyncMessages.create = patched_async_create


def instrument_langchain(observer: "LLMObserver") -> None:
    """
    Auto-instrument LangChain.

    Uses LangChain's callback system for non-invasive instrumentation.
    """
    try:
        from langchain_core.callbacks import BaseCallbackHandler
        from langchain_core.outputs import LLMResult
    except ImportError:
        raise ImportError("langchain-core package not installed")

    class OllyStackCallbackHandler(BaseCallbackHandler):
        """LangChain callback handler for OllyStack."""

        def __init__(self):
            self._trackers: dict[str, Any] = {}
            self._chain_trackers: dict[str, Any] = {}

        def on_llm_start(
            self,
            serialized: dict,
            prompts: list[str],
            *,
            run_id,
            parent_run_id=None,
            **kwargs,
        ):
            model = serialized.get("kwargs", {}).get("model_name", "unknown")
            provider_name = serialized.get("id", ["unknown"])[-1].lower()

            provider = Provider.OPENAI
            if "anthropic" in provider_name:
                provider = Provider.ANTHROPIC
            elif "cohere" in provider_name:
                provider = Provider.COHERE

            tracker = observer.track_llm_call(
                model=model,
                provider=provider,
                trace_id=str(run_id),
                parent_span_id=str(parent_run_id) if parent_run_id else None,
            ).__enter__()

            for prompt in prompts:
                tracker.record_prompt(role="user", content=prompt)

            self._trackers[str(run_id)] = tracker

        def on_llm_end(
            self,
            response: LLMResult,
            *,
            run_id,
            **kwargs,
        ):
            tracker = self._trackers.pop(str(run_id), None)
            if tracker:
                # Extract response
                if response.generations:
                    for gen in response.generations[0]:
                        tracker.request.completions.append(
                            type("Completion", (), {
                                "content": gen.text,
                                "finish_reason": "stop",
                            })()
                        )

                # Token usage
                if response.llm_output:
                    usage = response.llm_output.get("token_usage", {})
                    tracker.request.tokens.prompt_tokens = usage.get("prompt_tokens", 0)
                    tracker.request.tokens.completion_tokens = usage.get("completion_tokens", 0)

                tracker.__exit__(None, None, None)

        def on_llm_error(
            self,
            error: Exception,
            *,
            run_id,
            **kwargs,
        ):
            tracker = self._trackers.pop(str(run_id), None)
            if tracker:
                tracker.record_error(error)
                tracker.__exit__(type(error), error, None)

        def on_chain_start(
            self,
            serialized: dict,
            inputs: dict,
            *,
            run_id,
            parent_run_id=None,
            **kwargs,
        ):
            chain_name = serialized.get("name", "unknown_chain")
            chain_tracker = observer.track_chain(
                chain_name=chain_name,
                trace_id=str(run_id),
            ).__enter__()

            chain_tracker.record_input(str(inputs))
            self._chain_trackers[str(run_id)] = chain_tracker

        def on_chain_end(
            self,
            outputs: dict,
            *,
            run_id,
            **kwargs,
        ):
            chain_tracker = self._chain_trackers.pop(str(run_id), None)
            if chain_tracker:
                chain_tracker.record_output(str(outputs))
                chain_tracker.__exit__(None, None, None)

        def on_chain_error(
            self,
            error: Exception,
            *,
            run_id,
            **kwargs,
        ):
            chain_tracker = self._chain_trackers.pop(str(run_id), None)
            if chain_tracker:
                chain_tracker.record_error(error)
                chain_tracker.__exit__(type(error), error, None)

        def on_tool_start(
            self,
            serialized: dict,
            input_str: str,
            *,
            run_id,
            parent_run_id=None,
            **kwargs,
        ):
            pass  # Tool tracking handled in chain

        def on_tool_end(
            self,
            output: str,
            *,
            run_id,
            **kwargs,
        ):
            pass

        def on_retriever_start(
            self,
            serialized: dict,
            query: str,
            *,
            run_id,
            parent_run_id=None,
            **kwargs,
        ):
            tracker = observer.track_retrieval(
                vector_store="langchain",
                request_id=str(parent_run_id) if parent_run_id else None,
            ).__enter__()

            tracker.record_query(query)
            self._trackers[f"retrieval_{run_id}"] = tracker

        def on_retriever_end(
            self,
            documents: list,
            *,
            run_id,
            **kwargs,
        ):
            tracker = self._trackers.pop(f"retrieval_{run_id}", None)
            if tracker:
                results = [{"content": doc.page_content} for doc in documents]
                tracker.record_results(results)
                tracker.__exit__(None, None, None)

    # Register the callback handler globally
    from langchain_core.callbacks.manager import set_handler
    handler = OllyStackCallbackHandler()
    set_handler(handler)

    # Store reference
    observer._langchain_handler = handler


def instrument_llamaindex(observer: "LLMObserver") -> None:
    """
    Auto-instrument LlamaIndex.

    Uses LlamaIndex's callback system.
    """
    try:
        from llama_index.core.callbacks import CallbackManager, CBEventType
        from llama_index.core.callbacks.base_handler import BaseCallbackHandler
        from llama_index.core import Settings
    except ImportError:
        raise ImportError("llama-index package not installed")

    class OllyStackLlamaIndexHandler(BaseCallbackHandler):
        """LlamaIndex callback handler for OllyStack."""

        def __init__(self):
            super().__init__(event_starts_to_ignore=[], event_ends_to_ignore=[])
            self._trackers: dict[str, Any] = {}

        def on_event_start(
            self,
            event_type: CBEventType,
            payload: Optional[dict] = None,
            event_id: str = "",
            parent_id: str = "",
            **kwargs,
        ) -> str:
            if event_type == CBEventType.LLM:
                model = payload.get("model_name", "unknown") if payload else "unknown"
                tracker = observer.track_llm_call(
                    model=model,
                    provider=Provider.OPENAI,
                    trace_id=event_id,
                    parent_span_id=parent_id if parent_id else None,
                ).__enter__()

                if payload and "messages" in payload:
                    tracker.record_messages(payload["messages"])

                self._trackers[event_id] = tracker

            elif event_type == CBEventType.RETRIEVE:
                tracker = observer.track_retrieval(
                    vector_store="llamaindex",
                    request_id=parent_id if parent_id else None,
                ).__enter__()

                if payload and "query_str" in payload:
                    tracker.record_query(payload["query_str"])

                self._trackers[event_id] = tracker

            elif event_type == CBEventType.EMBEDDING:
                model = payload.get("model_name", "text-embedding-ada-002") if payload else "text-embedding-ada-002"
                tracker = observer.track_embedding(
                    model=model,
                    provider=Provider.OPENAI,
                    trace_id=event_id,
                ).__enter__()

                self._trackers[event_id] = tracker

            return event_id

        def on_event_end(
            self,
            event_type: CBEventType,
            payload: Optional[dict] = None,
            event_id: str = "",
            **kwargs,
        ) -> None:
            tracker = self._trackers.pop(event_id, None)
            if not tracker:
                return

            if event_type == CBEventType.LLM:
                if payload and "response" in payload:
                    tracker.record_response(payload["response"])
                tracker.__exit__(None, None, None)

            elif event_type == CBEventType.RETRIEVE:
                if payload and "nodes" in payload:
                    results = [{"content": node.get_content()} for node in payload["nodes"]]
                    scores = [node.score for node in payload["nodes"] if hasattr(node, "score")]
                    tracker.record_results(results, scores)
                tracker.__exit__(None, None, None)

            elif event_type == CBEventType.EMBEDDING:
                tracker.__exit__(None, None, None)

        def start_trace(self, trace_id: Optional[str] = None) -> None:
            pass

        def end_trace(
            self,
            trace_id: Optional[str] = None,
            trace_map: Optional[dict] = None,
        ) -> None:
            pass

    # Register handler
    handler = OllyStackLlamaIndexHandler()
    callback_manager = CallbackManager([handler])
    Settings.callback_manager = callback_manager

    observer._llamaindex_handler = handler
