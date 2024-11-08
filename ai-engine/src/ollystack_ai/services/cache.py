"""
Cache Service

Redis-based caching for AI engine.
"""

import json
import logging
import os
from typing import Any, Optional

logger = logging.getLogger(__name__)


class CacheService:
    """
    Redis cache service for caching NLQ results and baselines.
    """

    def __init__(
        self,
        url: Optional[str] = None,
        prefix: str = "ollystack:ai:",
    ):
        self.url = url or os.getenv("REDIS_URL", "redis://localhost:6379")
        self.prefix = prefix
        self._client = None

    async def connect(self) -> None:
        """Connect to Redis."""
        try:
            import redis.asyncio as redis

            self._client = redis.from_url(
                self.url,
                encoding="utf-8",
                decode_responses=True,
            )
            await self._client.ping()
            logger.info(f"Connected to Redis at {self.url}")

        except ImportError:
            logger.warning("redis package not installed, using in-memory cache")
            self._client = InMemoryCache()

        except Exception as e:
            logger.warning(f"Failed to connect to Redis: {e}, using in-memory cache")
            self._client = InMemoryCache()

    async def disconnect(self) -> None:
        """Disconnect from Redis."""
        if self._client and hasattr(self._client, "close"):
            await self._client.close()
        self._client = None

    async def get(self, key: str) -> Optional[Any]:
        """
        Get a value from cache.

        Args:
            key: Cache key

        Returns:
            Cached value or None
        """
        if self._client is None:
            await self.connect()

        try:
            full_key = f"{self.prefix}{key}"
            value = await self._client.get(full_key)

            if value is None:
                return None

            return json.loads(value)

        except Exception as e:
            logger.warning(f"Cache get failed: {e}")
            return None

    async def set(
        self,
        key: str,
        value: Any,
        ttl: int = 3600,
    ) -> None:
        """
        Set a value in cache.

        Args:
            key: Cache key
            value: Value to cache
            ttl: Time-to-live in seconds
        """
        if self._client is None:
            await self.connect()

        try:
            full_key = f"{self.prefix}{key}"
            serialized = json.dumps(value, default=str)
            await self._client.setex(full_key, ttl, serialized)

        except Exception as e:
            logger.warning(f"Cache set failed: {e}")

    async def delete(self, key: str) -> None:
        """
        Delete a value from cache.

        Args:
            key: Cache key
        """
        if self._client is None:
            return

        try:
            full_key = f"{self.prefix}{key}"
            await self._client.delete(full_key)

        except Exception as e:
            logger.warning(f"Cache delete failed: {e}")

    async def clear_prefix(self, prefix: str) -> int:
        """
        Clear all keys with a prefix.

        Args:
            prefix: Key prefix to clear

        Returns:
            Number of keys deleted
        """
        if self._client is None:
            return 0

        try:
            full_prefix = f"{self.prefix}{prefix}*"
            keys = []

            if hasattr(self._client, "scan_iter"):
                async for key in self._client.scan_iter(match=full_prefix):
                    keys.append(key)
            elif hasattr(self._client, "keys"):
                keys = await self._client.keys(full_prefix)

            if keys:
                await self._client.delete(*keys)

            return len(keys)

        except Exception as e:
            logger.warning(f"Cache clear failed: {e}")
            return 0


class InMemoryCache:
    """Simple in-memory cache for development/testing."""

    def __init__(self):
        self._cache: dict[str, tuple[Any, float]] = {}

    async def get(self, key: str) -> Optional[str]:
        """Get value from cache."""
        import time

        if key not in self._cache:
            return None

        value, expiry = self._cache[key]
        if expiry < time.time():
            del self._cache[key]
            return None

        return value

    async def setex(self, key: str, ttl: int, value: str) -> None:
        """Set value with expiry."""
        import time

        self._cache[key] = (value, time.time() + ttl)

    async def delete(self, key: str) -> None:
        """Delete value."""
        self._cache.pop(key, None)

    async def keys(self, pattern: str) -> list[str]:
        """Get keys matching pattern."""
        import fnmatch

        return [k for k in self._cache.keys() if fnmatch.fnmatch(k, pattern)]
