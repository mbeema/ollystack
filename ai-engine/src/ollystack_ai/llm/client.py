"""
LLM Client

Unified interface to multiple LLM providers:
- OpenAI (GPT-4, GPT-3.5)
- Anthropic (Claude)
- Local (Ollama, llama.cpp)
"""

import logging
import os
from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, Literal, AsyncIterator
from enum import Enum
import json
import asyncio

import httpx

logger = logging.getLogger(__name__)


class LLMProvider(Enum):
    """Supported LLM providers."""

    OPENAI = "openai"
    ANTHROPIC = "anthropic"
    OLLAMA = "ollama"  # Local models via Ollama
    MOCK = "mock"  # For testing


@dataclass
class Message:
    """A chat message."""

    role: Literal["system", "user", "assistant"]
    content: str


@dataclass
class LLMResponse:
    """Response from LLM."""

    content: str
    model: str
    provider: LLMProvider
    usage: dict = field(default_factory=dict)
    latency_ms: float = 0
    timestamp: datetime = field(default_factory=datetime.utcnow)

    def to_dict(self) -> dict:
        return {
            "content": self.content,
            "model": self.model,
            "provider": self.provider.value,
            "usage": self.usage,
            "latency_ms": self.latency_ms,
            "timestamp": self.timestamp.isoformat(),
        }


class LLMClient:
    """
    Unified LLM client supporting multiple providers.

    Usage:
        client = LLMClient(provider=LLMProvider.OPENAI)
        response = await client.chat([
            Message(role="user", content="What caused this error?")
        ])
    """

    # Default models for each provider
    DEFAULT_MODELS = {
        LLMProvider.OPENAI: "gpt-4-turbo-preview",
        LLMProvider.ANTHROPIC: "claude-3-sonnet-20240229",
        LLMProvider.OLLAMA: "llama2",
        LLMProvider.MOCK: "mock-model",
    }

    def __init__(
        self,
        provider: LLMProvider = LLMProvider.OPENAI,
        model: Optional[str] = None,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
        timeout: float = 60.0,
        max_retries: int = 3,
        temperature: float = 0.7,
        max_tokens: int = 4096,
    ):
        """
        Initialize the LLM client.

        Args:
            provider: LLM provider to use
            model: Model name (uses default if None)
            api_key: API key (uses env var if None)
            base_url: Custom API base URL
            timeout: Request timeout in seconds
            max_retries: Maximum retry attempts
            temperature: Sampling temperature
            max_tokens: Maximum tokens in response
        """
        self.provider = provider
        self.model = model or self.DEFAULT_MODELS[provider]
        self.timeout = timeout
        self.max_retries = max_retries
        self.temperature = temperature
        self.max_tokens = max_tokens

        # Get API key from env if not provided
        if api_key is None:
            if provider == LLMProvider.OPENAI:
                api_key = os.environ.get("OPENAI_API_KEY")
            elif provider == LLMProvider.ANTHROPIC:
                api_key = os.environ.get("ANTHROPIC_API_KEY")

        self.api_key = api_key

        # Set base URLs
        if base_url:
            self.base_url = base_url
        elif provider == LLMProvider.OPENAI:
            self.base_url = "https://api.openai.com/v1"
        elif provider == LLMProvider.ANTHROPIC:
            self.base_url = "https://api.anthropic.com/v1"
        elif provider == LLMProvider.OLLAMA:
            self.base_url = os.environ.get("OLLAMA_HOST", "http://localhost:11434")
        else:
            self.base_url = ""

        self._client: Optional[httpx.AsyncClient] = None

    async def _get_client(self) -> httpx.AsyncClient:
        """Get or create HTTP client."""
        if self._client is None:
            self._client = httpx.AsyncClient(timeout=self.timeout)
        return self._client

    async def chat(
        self,
        messages: list[Message],
        temperature: Optional[float] = None,
        max_tokens: Optional[int] = None,
        json_mode: bool = False,
    ) -> LLMResponse:
        """
        Send a chat completion request.

        Args:
            messages: List of messages in the conversation
            temperature: Override default temperature
            max_tokens: Override default max tokens
            json_mode: Request JSON output (if supported)

        Returns:
            LLMResponse with the completion
        """
        import time
        start = time.time()

        temp = temperature if temperature is not None else self.temperature
        tokens = max_tokens if max_tokens is not None else self.max_tokens

        for attempt in range(self.max_retries):
            try:
                if self.provider == LLMProvider.OPENAI:
                    response = await self._openai_chat(messages, temp, tokens, json_mode)
                elif self.provider == LLMProvider.ANTHROPIC:
                    response = await self._anthropic_chat(messages, temp, tokens)
                elif self.provider == LLMProvider.OLLAMA:
                    response = await self._ollama_chat(messages, temp, tokens)
                elif self.provider == LLMProvider.MOCK:
                    response = self._mock_chat(messages)
                else:
                    raise ValueError(f"Unsupported provider: {self.provider}")

                response.latency_ms = (time.time() - start) * 1000
                return response

            except Exception as e:
                logger.warning(f"LLM request failed (attempt {attempt + 1}): {e}")
                if attempt == self.max_retries - 1:
                    raise
                await asyncio.sleep(2 ** attempt)  # Exponential backoff

        raise RuntimeError("Should not reach here")

    async def chat_stream(
        self,
        messages: list[Message],
        temperature: Optional[float] = None,
        max_tokens: Optional[int] = None,
    ) -> AsyncIterator[str]:
        """
        Stream chat completion response.

        Yields chunks of the response as they arrive.
        """
        temp = temperature if temperature is not None else self.temperature
        tokens = max_tokens if max_tokens is not None else self.max_tokens

        if self.provider == LLMProvider.OPENAI:
            async for chunk in self._openai_chat_stream(messages, temp, tokens):
                yield chunk
        elif self.provider == LLMProvider.OLLAMA:
            async for chunk in self._ollama_chat_stream(messages, temp, tokens):
                yield chunk
        else:
            # Fall back to non-streaming
            response = await self.chat(messages, temp, tokens)
            yield response.content

    async def _openai_chat(
        self,
        messages: list[Message],
        temperature: float,
        max_tokens: int,
        json_mode: bool = False,
    ) -> LLMResponse:
        """OpenAI chat completion."""
        client = await self._get_client()

        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }

        payload = {
            "model": self.model,
            "messages": [{"role": m.role, "content": m.content} for m in messages],
            "temperature": temperature,
            "max_tokens": max_tokens,
        }

        if json_mode:
            payload["response_format"] = {"type": "json_object"}

        response = await client.post(
            f"{self.base_url}/chat/completions",
            headers=headers,
            json=payload,
        )
        response.raise_for_status()
        data = response.json()

        return LLMResponse(
            content=data["choices"][0]["message"]["content"],
            model=data["model"],
            provider=self.provider,
            usage={
                "prompt_tokens": data["usage"]["prompt_tokens"],
                "completion_tokens": data["usage"]["completion_tokens"],
                "total_tokens": data["usage"]["total_tokens"],
            },
        )

    async def _openai_chat_stream(
        self,
        messages: list[Message],
        temperature: float,
        max_tokens: int,
    ) -> AsyncIterator[str]:
        """OpenAI streaming chat completion."""
        client = await self._get_client()

        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }

        payload = {
            "model": self.model,
            "messages": [{"role": m.role, "content": m.content} for m in messages],
            "temperature": temperature,
            "max_tokens": max_tokens,
            "stream": True,
        }

        async with client.stream(
            "POST",
            f"{self.base_url}/chat/completions",
            headers=headers,
            json=payload,
        ) as response:
            async for line in response.aiter_lines():
                if line.startswith("data: "):
                    data = line[6:]
                    if data == "[DONE]":
                        break
                    try:
                        chunk = json.loads(data)
                        delta = chunk["choices"][0].get("delta", {})
                        if "content" in delta:
                            yield delta["content"]
                    except json.JSONDecodeError:
                        continue

    async def _anthropic_chat(
        self,
        messages: list[Message],
        temperature: float,
        max_tokens: int,
    ) -> LLMResponse:
        """Anthropic chat completion."""
        client = await self._get_client()

        headers = {
            "x-api-key": self.api_key,
            "anthropic-version": "2023-06-01",
            "Content-Type": "application/json",
        }

        # Anthropic uses a different format - extract system message
        system = ""
        chat_messages = []
        for m in messages:
            if m.role == "system":
                system = m.content
            else:
                chat_messages.append({"role": m.role, "content": m.content})

        payload = {
            "model": self.model,
            "messages": chat_messages,
            "max_tokens": max_tokens,
            "temperature": temperature,
        }

        if system:
            payload["system"] = system

        response = await client.post(
            f"{self.base_url}/messages",
            headers=headers,
            json=payload,
        )
        response.raise_for_status()
        data = response.json()

        return LLMResponse(
            content=data["content"][0]["text"],
            model=data["model"],
            provider=self.provider,
            usage={
                "prompt_tokens": data["usage"]["input_tokens"],
                "completion_tokens": data["usage"]["output_tokens"],
                "total_tokens": data["usage"]["input_tokens"] + data["usage"]["output_tokens"],
            },
        )

    async def _ollama_chat(
        self,
        messages: list[Message],
        temperature: float,
        max_tokens: int,
    ) -> LLMResponse:
        """Ollama local chat completion."""
        client = await self._get_client()

        payload = {
            "model": self.model,
            "messages": [{"role": m.role, "content": m.content} for m in messages],
            "options": {
                "temperature": temperature,
                "num_predict": max_tokens,
            },
            "stream": False,
        }

        response = await client.post(
            f"{self.base_url}/api/chat",
            json=payload,
        )
        response.raise_for_status()
        data = response.json()

        return LLMResponse(
            content=data["message"]["content"],
            model=data["model"],
            provider=self.provider,
            usage={
                "prompt_tokens": data.get("prompt_eval_count", 0),
                "completion_tokens": data.get("eval_count", 0),
                "total_tokens": data.get("prompt_eval_count", 0) + data.get("eval_count", 0),
            },
        )

    async def _ollama_chat_stream(
        self,
        messages: list[Message],
        temperature: float,
        max_tokens: int,
    ) -> AsyncIterator[str]:
        """Ollama streaming chat completion."""
        client = await self._get_client()

        payload = {
            "model": self.model,
            "messages": [{"role": m.role, "content": m.content} for m in messages],
            "options": {
                "temperature": temperature,
                "num_predict": max_tokens,
            },
            "stream": True,
        }

        async with client.stream(
            "POST",
            f"{self.base_url}/api/chat",
            json=payload,
        ) as response:
            async for line in response.aiter_lines():
                try:
                    data = json.loads(line)
                    if "message" in data and "content" in data["message"]:
                        yield data["message"]["content"]
                except json.JSONDecodeError:
                    continue

    def _mock_chat(self, messages: list[Message]) -> LLMResponse:
        """Mock response for testing."""
        last_message = messages[-1].content if messages else ""

        return LLMResponse(
            content=f"Mock response to: {last_message[:100]}...",
            model="mock-model",
            provider=self.provider,
            usage={"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
        )

    async def close(self) -> None:
        """Close the HTTP client."""
        if self._client:
            await self._client.aclose()
            self._client = None

    async def __aenter__(self) -> "LLMClient":
        return self

    async def __aexit__(self, *args) -> None:
        await self.close()


def get_best_available_client() -> LLMClient:
    """
    Get the best available LLM client based on environment.

    Priority:
    1. OpenAI if OPENAI_API_KEY is set
    2. Anthropic if ANTHROPIC_API_KEY is set
    3. Ollama if running locally
    4. Mock for testing
    """
    if os.environ.get("OPENAI_API_KEY"):
        return LLMClient(provider=LLMProvider.OPENAI)
    elif os.environ.get("ANTHROPIC_API_KEY"):
        return LLMClient(provider=LLMProvider.ANTHROPIC)
    elif os.environ.get("OLLAMA_HOST") or _check_ollama_available():
        return LLMClient(provider=LLMProvider.OLLAMA)
    else:
        logger.warning("No LLM provider available, using mock")
        return LLMClient(provider=LLMProvider.MOCK)


def _check_ollama_available() -> bool:
    """Check if Ollama is running locally."""
    import httpx
    try:
        response = httpx.get("http://localhost:11434/api/tags", timeout=2)
        return response.status_code == 200
    except Exception:
        return False
