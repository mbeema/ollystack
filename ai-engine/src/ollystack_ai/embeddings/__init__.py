"""
Embeddings for Semantic Analysis

Local embedding models for:
- Log similarity and clustering
- Semantic search
- Error grouping
- Anomaly explanation matching
"""

from ollystack_ai.embeddings.log_embeddings import (
    LogEmbedder,
    EmbeddingResult,
    SimilarityResult,
)
from ollystack_ai.embeddings.clustering import (
    LogClusterer,
    ClusterResult,
    ErrorGrouper,
)
from ollystack_ai.embeddings.semantic_search import (
    SemanticSearchIndex,
    SearchResult,
)

__all__ = [
    "LogEmbedder",
    "EmbeddingResult",
    "SimilarityResult",
    "LogClusterer",
    "ClusterResult",
    "ErrorGrouper",
    "SemanticSearchIndex",
    "SearchResult",
]
