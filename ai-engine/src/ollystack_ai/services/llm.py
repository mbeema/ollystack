"""
LLM Service

Abstraction layer for LLM providers (OpenAI, Anthropic, local models).
"""

import logging
import os
from typing import Optional
from enum import Enum

logger = logging.getLogger(__name__)


class LLMProvider(str, Enum):
    """Supported LLM providers."""

    OPENAI = "openai"
    ANTHROPIC = "anthropic"
    LOCAL = "local"


class LLMService:
    """
    LLM service for natural language processing.

    Supports multiple providers with fallback capability.
    """

    def __init__(
        self,
        provider: Optional[LLMProvider] = None,
        model: Optional[str] = None,
        api_key: Optional[str] = None,
    ):
        self.provider = provider or self._detect_provider()
        self.model = model or self._default_model()
        self.api_key = api_key or self._get_api_key()
        self._client = None

    def _detect_provider(self) -> LLMProvider:
        """Detect available LLM provider."""
        if os.getenv("OPENAI_API_KEY"):
            return LLMProvider.OPENAI
        elif os.getenv("ANTHROPIC_API_KEY"):
            return LLMProvider.ANTHROPIC
        else:
            return LLMProvider.LOCAL

    def _default_model(self) -> str:
        """Get default model for provider."""
        if self.provider == LLMProvider.OPENAI:
            return os.getenv("OPENAI_MODEL", "gpt-4-turbo-preview")
        elif self.provider == LLMProvider.ANTHROPIC:
            return os.getenv("ANTHROPIC_MODEL", "claude-3-sonnet-20240229")
        else:
            return os.getenv("LOCAL_MODEL", "llama2")

    def _get_api_key(self) -> Optional[str]:
        """Get API key for provider."""
        if self.provider == LLMProvider.OPENAI:
            return os.getenv("OPENAI_API_KEY")
        elif self.provider == LLMProvider.ANTHROPIC:
            return os.getenv("ANTHROPIC_API_KEY")
        return None

    async def complete(
        self,
        system_prompt: str,
        user_prompt: str,
        temperature: float = 0.7,
        max_tokens: int = 2000,
    ) -> str:
        """
        Generate a completion from the LLM.

        Args:
            system_prompt: System instruction
            user_prompt: User message
            temperature: Sampling temperature
            max_tokens: Maximum tokens in response

        Returns:
            Generated text
        """
        try:
            if self.provider == LLMProvider.OPENAI:
                return await self._openai_complete(
                    system_prompt, user_prompt, temperature, max_tokens
                )
            elif self.provider == LLMProvider.ANTHROPIC:
                return await self._anthropic_complete(
                    system_prompt, user_prompt, temperature, max_tokens
                )
            else:
                return await self._local_complete(
                    system_prompt, user_prompt, temperature, max_tokens
                )
        except Exception as e:
            logger.exception(f"LLM completion failed: {e}")
            # Return a fallback response
            return self._fallback_response(user_prompt)

    async def _openai_complete(
        self,
        system_prompt: str,
        user_prompt: str,
        temperature: float,
        max_tokens: int,
    ) -> str:
        """Complete using OpenAI API."""
        try:
            from openai import AsyncOpenAI

            if self._client is None:
                self._client = AsyncOpenAI(api_key=self.api_key)

            response = await self._client.chat.completions.create(
                model=self.model,
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_prompt},
                ],
                temperature=temperature,
                max_tokens=max_tokens,
            )

            return response.choices[0].message.content or ""

        except ImportError:
            logger.warning("OpenAI package not installed, using fallback")
            return self._fallback_response(user_prompt)

    async def _anthropic_complete(
        self,
        system_prompt: str,
        user_prompt: str,
        temperature: float,
        max_tokens: int,
    ) -> str:
        """Complete using Anthropic API."""
        try:
            from anthropic import AsyncAnthropic

            if self._client is None:
                self._client = AsyncAnthropic(api_key=self.api_key)

            response = await self._client.messages.create(
                model=self.model,
                max_tokens=max_tokens,
                system=system_prompt,
                messages=[
                    {"role": "user", "content": user_prompt},
                ],
            )

            return response.content[0].text

        except ImportError:
            logger.warning("Anthropic package not installed, using fallback")
            return self._fallback_response(user_prompt)

    async def _local_complete(
        self,
        system_prompt: str,
        user_prompt: str,
        temperature: float,
        max_tokens: int,
    ) -> str:
        """Complete using local model (Ollama)."""
        try:
            import httpx

            async with httpx.AsyncClient() as client:
                response = await client.post(
                    os.getenv("OLLAMA_URL", "http://localhost:11434") + "/api/generate",
                    json={
                        "model": self.model,
                        "prompt": f"{system_prompt}\n\nUser: {user_prompt}\n\nAssistant:",
                        "stream": False,
                        "options": {
                            "temperature": temperature,
                            "num_predict": max_tokens,
                        },
                    },
                    timeout=60.0,
                )
                response.raise_for_status()
                return response.json().get("response", "")

        except Exception as e:
            logger.warning(f"Local model failed: {e}, using fallback")
            return self._fallback_response(user_prompt)

    def _fallback_response(self, prompt: str) -> str:
        """
        Generate a fallback response when LLM is unavailable.

        Uses simple keyword matching for basic functionality.
        """
        prompt_lower = prompt.lower()

        # Extract basic query patterns
        if "error" in prompt_lower:
            return """OBSERVQL: SELECT * FROM otel_traces WHERE StatusCode = 'ERROR' ORDER BY Timestamp DESC LIMIT 100
SQL: SELECT Timestamp, TraceId, ServiceName, SpanName, StatusMessage FROM otel_traces WHERE StatusCode = 'ERROR' ORDER BY Timestamp DESC LIMIT 100
EXPLANATION: Query for recent errors across all services
CONFIDENCE: 0.5
SUGGESTIONS:
- Filter by specific service
- Check error logs for more details"""

        elif "slow" in prompt_lower or "latency" in prompt_lower:
            return """OBSERVQL: SELECT * FROM otel_traces WHERE Duration > 1000000000 ORDER BY Duration DESC LIMIT 100
SQL: SELECT Timestamp, TraceId, ServiceName, SpanName, Duration/1000000 as duration_ms FROM otel_traces WHERE Duration > 1000000000 ORDER BY Duration DESC LIMIT 100
EXPLANATION: Query for slow requests (>1 second)
CONFIDENCE: 0.5
SUGGESTIONS:
- Check specific endpoint latency
- Analyze database query times"""

        elif "service" in prompt_lower and "depend" in prompt_lower:
            return """OBSERVQL: SELECT * FROM service_topology ORDER BY RequestCount DESC
SQL: SELECT SourceService, TargetService, RequestCount, ErrorRate, LatencyP99 FROM service_topology ORDER BY RequestCount DESC
EXPLANATION: Query for service dependencies
CONFIDENCE: 0.5
SUGGESTIONS:
- Filter by specific service
- Check error rates between services"""

        else:
            return """OBSERVQL: SELECT * FROM otel_traces ORDER BY Timestamp DESC LIMIT 100
SQL: SELECT Timestamp, TraceId, ServiceName, SpanName, Duration/1000000 as duration_ms, StatusCode FROM otel_traces ORDER BY Timestamp DESC LIMIT 100
EXPLANATION: Query for recent traces
CONFIDENCE: 0.4
SUGGESTIONS:
- Specify what you're looking for
- Filter by service or time range"""

    async def embed(self, text: str) -> list[float]:
        """
        Generate embeddings for text.

        Useful for semantic search and similarity.
        """
        try:
            if self.provider == LLMProvider.OPENAI:
                from openai import AsyncOpenAI

                if self._client is None:
                    self._client = AsyncOpenAI(api_key=self.api_key)

                response = await self._client.embeddings.create(
                    model="text-embedding-3-small",
                    input=text,
                )
                return response.data[0].embedding

        except Exception as e:
            logger.warning(f"Embedding failed: {e}")

        # Return zero vector as fallback
        return [0.0] * 1536
