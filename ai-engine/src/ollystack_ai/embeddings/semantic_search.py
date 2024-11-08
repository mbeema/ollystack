"""
Semantic Search

Enables natural language search over logs, traces, and other observability data.
"""

import logging
from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, Any
import pickle

import numpy as np

from ollystack_ai.embeddings.log_embeddings import LogEmbedder, EmbeddingIndex

logger = logging.getLogger(__name__)


@dataclass
class SearchResult:
    """A single search result."""

    text: str
    score: float
    metadata: dict
    highlight: Optional[str] = None

    def to_dict(self) -> dict:
        return {
            "text": self.text[:500],
            "score": self.score,
            "metadata": self.metadata,
            "highlight": self.highlight,
        }


@dataclass
class SearchResponse:
    """Response from a search query."""

    results: list[SearchResult]
    total_hits: int
    query: str
    took_ms: float


class SemanticSearchIndex:
    """
    Semantic search index for observability data.

    Supports:
    - Natural language queries
    - Hybrid search (semantic + keyword)
    - Filtering by metadata
    - Real-time indexing
    """

    def __init__(
        self,
        embedder: Optional[LogEmbedder] = None,
        dimension: int = 384,
    ):
        """
        Initialize the search index.

        Args:
            embedder: LogEmbedder instance
            dimension: Embedding dimension
        """
        self.embedder = embedder or LogEmbedder()
        self.dimension = dimension

        self._index = EmbeddingIndex(dimension=dimension)
        self._texts: list[str] = []

    def add(
        self,
        text: str,
        metadata: Optional[dict] = None,
    ) -> int:
        """
        Add a document to the index.

        Args:
            text: Document text
            metadata: Optional metadata (service, timestamp, etc.)

        Returns:
            Document index
        """
        embedding = self.embedder.embed(text).embedding
        idx = self._index.add(embedding, metadata or {})
        self._texts.append(text)
        return idx

    def add_batch(
        self,
        texts: list[str],
        metadata: Optional[list[dict]] = None,
    ) -> list[int]:
        """Add multiple documents to the index."""
        embeddings = self.embedder.embed_batch(texts)
        embedding_arrays = [e.embedding for e in embeddings]

        indices = self._index.add_batch(
            embedding_arrays,
            metadata or [{} for _ in texts]
        )
        self._texts.extend(texts)
        return indices

    def search(
        self,
        query: str,
        top_k: int = 10,
        threshold: float = 0.3,
        filters: Optional[dict] = None,
    ) -> SearchResponse:
        """
        Search for documents matching the query.

        Args:
            query: Natural language query
            top_k: Maximum results to return
            threshold: Minimum similarity threshold
            filters: Metadata filters (e.g., {'service': 'api'})

        Returns:
            SearchResponse with results
        """
        import time
        start = time.time()

        # Embed query
        query_embedding = self.embedder.embed(query).embedding

        # Search
        raw_results = self._index.search(query_embedding, top_k * 2, threshold)

        # Apply filters and build results
        results = []
        for idx, score, metadata in raw_results:
            # Apply metadata filters
            if filters:
                match = all(
                    metadata.get(k) == v
                    for k, v in filters.items()
                )
                if not match:
                    continue

            text = self._texts[idx] if idx < len(self._texts) else ""

            results.append(SearchResult(
                text=text,
                score=score,
                metadata=metadata,
                highlight=self._generate_highlight(query, text),
            ))

            if len(results) >= top_k:
                break

        took_ms = (time.time() - start) * 1000

        return SearchResponse(
            results=results,
            total_hits=len(results),
            query=query,
            took_ms=took_ms,
        )

    def similar(
        self,
        text: str,
        top_k: int = 10,
        exclude_self: bool = True,
    ) -> list[SearchResult]:
        """
        Find documents similar to the given text.

        Args:
            text: Reference text
            top_k: Maximum results
            exclude_self: Exclude exact matches

        Returns:
            List of similar documents
        """
        embedding = self.embedder.embed(text).embedding
        raw_results = self._index.search(embedding, top_k + 1 if exclude_self else top_k)

        results = []
        for idx, score, metadata in raw_results:
            doc_text = self._texts[idx] if idx < len(self._texts) else ""

            # Skip exact matches if requested
            if exclude_self and score > 0.99:
                continue

            results.append(SearchResult(
                text=doc_text,
                score=score,
                metadata=metadata,
            ))

            if len(results) >= top_k:
                break

        return results

    def _generate_highlight(self, query: str, text: str, window: int = 100) -> str:
        """Generate a highlighted snippet from the text."""
        query_words = set(query.lower().split())
        text_lower = text.lower()

        # Find best window containing query words
        best_pos = 0
        best_score = 0

        words = text.split()
        for i, word in enumerate(words):
            if word.lower() in query_words:
                score = sum(1 for w in words[max(0, i-5):i+5] if w.lower() in query_words)
                if score > best_score:
                    best_score = score
                    best_pos = sum(len(w) + 1 for w in words[:i])

        # Extract window
        start = max(0, best_pos - window // 2)
        end = min(len(text), start + window)

        snippet = text[start:end]
        if start > 0:
            snippet = "..." + snippet
        if end < len(text):
            snippet = snippet + "..."

        return snippet

    def __len__(self) -> int:
        return len(self._index)

    def save(self, path: str) -> None:
        """Save index to disk."""
        with open(path, 'wb') as f:
            pickle.dump({
                'index': self._index,
                'texts': self._texts,
                'dimension': self.dimension,
            }, f)

    def load(self, path: str) -> "SemanticSearchIndex":
        """Load index from disk."""
        with open(path, 'rb') as f:
            data = pickle.load(f)
        self._index = data['index']
        self._texts = data['texts']
        self.dimension = data['dimension']
        return self


class KnowledgeBase:
    """
    Knowledge base for storing and retrieving operational knowledge.

    Useful for:
    - Runbook matching
    - Similar incident lookup
    - Documentation search
    """

    def __init__(self):
        self.search_index = SemanticSearchIndex()

        # Categories of knowledge
        self._runbooks: list[dict] = []
        self._incidents: list[dict] = []
        self._docs: list[dict] = []

    def add_runbook(
        self,
        title: str,
        content: str,
        tags: list[str] = None,
        metadata: dict = None,
    ) -> int:
        """Add a runbook to the knowledge base."""
        doc = {
            'type': 'runbook',
            'title': title,
            'content': content,
            'tags': tags or [],
            **(metadata or {}),
        }
        self._runbooks.append(doc)

        # Index for search
        search_text = f"{title}. {content}"
        return self.search_index.add(search_text, doc)

    def add_incident(
        self,
        title: str,
        description: str,
        resolution: str,
        timestamp: datetime,
        severity: str = "medium",
        metadata: dict = None,
    ) -> int:
        """Add a past incident for reference."""
        doc = {
            'type': 'incident',
            'title': title,
            'description': description,
            'resolution': resolution,
            'timestamp': timestamp.isoformat(),
            'severity': severity,
            **(metadata or {}),
        }
        self._incidents.append(doc)

        search_text = f"{title}. {description}. Resolution: {resolution}"
        return self.search_index.add(search_text, doc)

    def add_documentation(
        self,
        title: str,
        content: str,
        source: str = "",
        metadata: dict = None,
    ) -> int:
        """Add documentation."""
        doc = {
            'type': 'documentation',
            'title': title,
            'content': content,
            'source': source,
            **(metadata or {}),
        }
        self._docs.append(doc)

        search_text = f"{title}. {content}"
        return self.search_index.add(search_text, doc)

    def find_relevant(
        self,
        query: str,
        top_k: int = 5,
        doc_type: Optional[str] = None,
    ) -> list[dict]:
        """
        Find relevant knowledge for a query.

        Args:
            query: Natural language query or error message
            top_k: Maximum results
            doc_type: Filter by type ('runbook', 'incident', 'documentation')

        Returns:
            List of relevant documents
        """
        filters = {'type': doc_type} if doc_type else None
        response = self.search_index.search(query, top_k, filters=filters)

        return [
            {
                'score': r.score,
                **r.metadata,
            }
            for r in response.results
        ]

    def find_similar_incidents(
        self,
        description: str,
        top_k: int = 5,
    ) -> list[dict]:
        """Find incidents similar to the given description."""
        return self.find_relevant(description, top_k, doc_type='incident')

    def find_runbooks(
        self,
        problem: str,
        top_k: int = 3,
    ) -> list[dict]:
        """Find runbooks relevant to a problem."""
        return self.find_relevant(problem, top_k, doc_type='runbook')

    def get_stats(self) -> dict:
        """Get knowledge base statistics."""
        return {
            'total_documents': len(self.search_index),
            'runbooks': len(self._runbooks),
            'incidents': len(self._incidents),
            'documentation': len(self._docs),
        }
