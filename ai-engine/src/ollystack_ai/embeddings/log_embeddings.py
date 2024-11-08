"""
Log Embeddings

Generates semantic embeddings for log messages using sentence-transformers.
Enables similarity search, clustering, and semantic analysis.
"""

import logging
from dataclasses import dataclass, field
from typing import Optional, Literal
from datetime import datetime
import hashlib

import numpy as np

logger = logging.getLogger(__name__)

# Try to import sentence-transformers, fall back to lightweight alternative
try:
    from sentence_transformers import SentenceTransformer
    SENTENCE_TRANSFORMERS_AVAILABLE = True
except ImportError:
    SENTENCE_TRANSFORMERS_AVAILABLE = False
    logger.warning(
        "sentence-transformers not installed. "
        "Using lightweight fallback embeddings."
    )


@dataclass
class EmbeddingResult:
    """Result of embedding generation."""

    text: str
    embedding: np.ndarray
    model_name: str
    dimension: int

    def to_dict(self) -> dict:
        return {
            "text": self.text[:200],
            "embedding": self.embedding.tolist(),
            "model_name": self.model_name,
            "dimension": self.dimension,
        }


@dataclass
class SimilarityResult:
    """Result of similarity comparison."""

    text1: str
    text2: str
    similarity: float  # 0-1, cosine similarity
    is_similar: bool  # Above threshold


class LogEmbedder:
    """
    Generates embeddings for log messages.

    Uses sentence-transformers for high-quality embeddings.
    Falls back to TF-IDF based embeddings if not available.
    """

    # Recommended models for different use cases
    MODELS = {
        "fast": "all-MiniLM-L6-v2",  # 384d, fast, good quality
        "balanced": "all-mpnet-base-v2",  # 768d, balanced
        "accurate": "all-MiniLM-L12-v2",  # 384d, more accurate
        "multilingual": "paraphrase-multilingual-MiniLM-L12-v2",  # 384d
    }

    def __init__(
        self,
        model_name: str = "all-MiniLM-L6-v2",
        device: Optional[str] = None,
        cache_embeddings: bool = True,
        max_seq_length: int = 256,
    ):
        """
        Initialize the embedder.

        Args:
            model_name: Name of the sentence-transformers model
            device: Device to use ('cpu', 'cuda', or None for auto)
            cache_embeddings: Whether to cache computed embeddings
            max_seq_length: Maximum sequence length
        """
        self.model_name = model_name
        self.device = device
        self.cache_embeddings = cache_embeddings
        self.max_seq_length = max_seq_length

        self._model: Optional["SentenceTransformer"] = None
        self._cache: dict[str, np.ndarray] = {}
        self._dimension: int = 384  # Default for MiniLM

        # Fallback TF-IDF components
        self._tfidf_vocab: dict[str, int] = {}
        self._tfidf_idf: Optional[np.ndarray] = None

    def load_model(self) -> None:
        """Load the embedding model."""
        if SENTENCE_TRANSFORMERS_AVAILABLE:
            logger.info(f"Loading sentence-transformers model: {self.model_name}")
            self._model = SentenceTransformer(self.model_name, device=self.device)
            self._model.max_seq_length = self.max_seq_length
            self._dimension = self._model.get_sentence_embedding_dimension()
            logger.info(f"Model loaded. Embedding dimension: {self._dimension}")
        else:
            logger.info("Using TF-IDF fallback embeddings")
            self._dimension = 512  # TF-IDF dimension

    def embed(self, text: str) -> EmbeddingResult:
        """
        Generate embedding for a single text.

        Args:
            text: Text to embed

        Returns:
            EmbeddingResult with embedding vector
        """
        # Check cache
        cache_key = self._cache_key(text)
        if self.cache_embeddings and cache_key in self._cache:
            return EmbeddingResult(
                text=text,
                embedding=self._cache[cache_key],
                model_name=self.model_name,
                dimension=self._dimension,
            )

        # Load model if needed
        if self._model is None and SENTENCE_TRANSFORMERS_AVAILABLE:
            self.load_model()

        # Generate embedding
        if SENTENCE_TRANSFORMERS_AVAILABLE and self._model is not None:
            embedding = self._model.encode(
                text,
                convert_to_numpy=True,
                normalize_embeddings=True,
            )
        else:
            embedding = self._tfidf_embed(text)

        # Cache
        if self.cache_embeddings:
            self._cache[cache_key] = embedding

        return EmbeddingResult(
            text=text,
            embedding=embedding,
            model_name=self.model_name if SENTENCE_TRANSFORMERS_AVAILABLE else "tfidf-fallback",
            dimension=len(embedding),
        )

    def embed_batch(
        self,
        texts: list[str],
        batch_size: int = 32,
        show_progress: bool = False,
    ) -> list[EmbeddingResult]:
        """
        Generate embeddings for multiple texts.

        Args:
            texts: List of texts to embed
            batch_size: Batch size for encoding
            show_progress: Show progress bar

        Returns:
            List of EmbeddingResult
        """
        if not texts:
            return []

        # Load model if needed
        if self._model is None and SENTENCE_TRANSFORMERS_AVAILABLE:
            self.load_model()

        # Check cache for all texts
        results = []
        texts_to_embed = []
        text_indices = []

        for i, text in enumerate(texts):
            cache_key = self._cache_key(text)
            if self.cache_embeddings and cache_key in self._cache:
                results.append((i, self._cache[cache_key]))
            else:
                texts_to_embed.append(text)
                text_indices.append(i)

        # Embed uncached texts
        if texts_to_embed:
            if SENTENCE_TRANSFORMERS_AVAILABLE and self._model is not None:
                embeddings = self._model.encode(
                    texts_to_embed,
                    batch_size=batch_size,
                    convert_to_numpy=True,
                    normalize_embeddings=True,
                    show_progress_bar=show_progress,
                )
            else:
                embeddings = np.array([self._tfidf_embed(t) for t in texts_to_embed])

            # Cache and add to results
            for idx, embedding, text in zip(text_indices, embeddings, texts_to_embed):
                if self.cache_embeddings:
                    self._cache[self._cache_key(text)] = embedding
                results.append((idx, embedding))

        # Sort by original index
        results.sort(key=lambda x: x[0])

        return [
            EmbeddingResult(
                text=texts[idx],
                embedding=emb,
                model_name=self.model_name if SENTENCE_TRANSFORMERS_AVAILABLE else "tfidf-fallback",
                dimension=len(emb),
            )
            for idx, emb in results
        ]

    def similarity(self, text1: str, text2: str) -> SimilarityResult:
        """
        Calculate similarity between two texts.

        Args:
            text1: First text
            text2: Second text

        Returns:
            SimilarityResult with cosine similarity
        """
        emb1 = self.embed(text1).embedding
        emb2 = self.embed(text2).embedding

        similarity = float(np.dot(emb1, emb2))  # Already normalized

        return SimilarityResult(
            text1=text1,
            text2=text2,
            similarity=similarity,
            is_similar=similarity > 0.7,
        )

    def find_similar(
        self,
        query: str,
        candidates: list[str],
        top_k: int = 10,
        threshold: float = 0.5,
    ) -> list[tuple[str, float]]:
        """
        Find similar texts from candidates.

        Args:
            query: Query text
            candidates: List of candidate texts
            top_k: Maximum results to return
            threshold: Minimum similarity threshold

        Returns:
            List of (text, similarity) tuples, sorted by similarity
        """
        query_emb = self.embed(query).embedding
        candidate_embs = self.embed_batch(candidates)

        # Calculate similarities
        similarities = []
        for result in candidate_embs:
            sim = float(np.dot(query_emb, result.embedding))
            if sim >= threshold:
                similarities.append((result.text, sim))

        # Sort by similarity
        similarities.sort(key=lambda x: x[1], reverse=True)

        return similarities[:top_k]

    def clear_cache(self) -> None:
        """Clear the embedding cache."""
        self._cache.clear()

    def get_cache_stats(self) -> dict:
        """Get cache statistics."""
        return {
            "cached_embeddings": len(self._cache),
            "cache_size_mb": sum(
                emb.nbytes for emb in self._cache.values()
            ) / (1024 * 1024),
        }

    def _cache_key(self, text: str) -> str:
        """Generate cache key for text."""
        return hashlib.md5(text.encode()).hexdigest()

    def _tfidf_embed(self, text: str) -> np.ndarray:
        """
        Fallback TF-IDF based embedding.

        Simple but effective for log similarity.
        """
        # Tokenize
        tokens = self._tokenize(text)

        # Build vocabulary if needed
        for token in tokens:
            if token not in self._tfidf_vocab:
                self._tfidf_vocab[token] = len(self._tfidf_vocab)

        # Create sparse vector
        tf = np.zeros(self._dimension)
        for token in tokens:
            idx = self._tfidf_vocab.get(token, hash(token) % self._dimension)
            tf[idx % self._dimension] += 1

        # Normalize
        norm = np.linalg.norm(tf)
        if norm > 0:
            tf /= norm

        return tf

    def _tokenize(self, text: str) -> list[str]:
        """Simple tokenization for TF-IDF."""
        import re
        # Lowercase and split on non-alphanumeric
        tokens = re.findall(r'\b\w+\b', text.lower())
        return tokens


class LogPreprocessor:
    """
    Preprocesses log messages for better embedding quality.
    """

    # Patterns to normalize
    NORMALIZATIONS = [
        (r'\b\d+\.\d+\.\d+\.\d+\b', '<IP>'),
        (r'\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b', '<UUID>'),
        (r'\b[0-9a-f]{32,}\b', '<HASH>'),
        (r'\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}\b', '<TIMESTAMP>'),
        (r'https?://[^\s]+', '<URL>'),
        (r'/[a-zA-Z0-9_/\-\.]+', '<PATH>'),
        (r'\b\d+\b', '<NUM>'),
    ]

    def __init__(
        self,
        normalize_variables: bool = True,
        lowercase: bool = True,
        remove_timestamps: bool = True,
    ):
        self.normalize_variables = normalize_variables
        self.lowercase = lowercase
        self.remove_timestamps = remove_timestamps

        import re
        self._patterns = [
            (re.compile(pattern, re.IGNORECASE), replacement)
            for pattern, replacement in self.NORMALIZATIONS
        ]

    def preprocess(self, text: str) -> str:
        """Preprocess a log message."""
        result = text

        if self.normalize_variables:
            for pattern, replacement in self._patterns:
                result = pattern.sub(replacement, result)

        if self.lowercase:
            result = result.lower()

        return result

    def preprocess_batch(self, texts: list[str]) -> list[str]:
        """Preprocess multiple log messages."""
        return [self.preprocess(text) for text in texts]


class EmbeddingIndex:
    """
    Simple in-memory embedding index for fast similarity search.

    Uses brute-force search, suitable for up to ~100k embeddings.
    For larger scales, use FAISS or similar.
    """

    def __init__(self, dimension: int = 384):
        self.dimension = dimension
        self._embeddings: list[np.ndarray] = []
        self._metadata: list[dict] = []

    def add(
        self,
        embedding: np.ndarray,
        metadata: Optional[dict] = None,
    ) -> int:
        """Add embedding to index. Returns index."""
        idx = len(self._embeddings)
        self._embeddings.append(embedding)
        self._metadata.append(metadata or {})
        return idx

    def add_batch(
        self,
        embeddings: list[np.ndarray],
        metadata: Optional[list[dict]] = None,
    ) -> list[int]:
        """Add multiple embeddings. Returns indices."""
        start_idx = len(self._embeddings)
        indices = []

        for i, emb in enumerate(embeddings):
            meta = metadata[i] if metadata else {}
            idx = self.add(emb, meta)
            indices.append(idx)

        return indices

    def search(
        self,
        query: np.ndarray,
        top_k: int = 10,
        threshold: float = 0.0,
    ) -> list[tuple[int, float, dict]]:
        """
        Search for similar embeddings.

        Returns list of (index, similarity, metadata) tuples.
        """
        if not self._embeddings:
            return []

        # Stack all embeddings
        all_embeddings = np.stack(self._embeddings)

        # Calculate cosine similarities
        similarities = all_embeddings @ query

        # Get top k
        top_indices = np.argsort(similarities)[-top_k:][::-1]

        results = []
        for idx in top_indices:
            sim = float(similarities[idx])
            if sim >= threshold:
                results.append((int(idx), sim, self._metadata[idx]))

        return results

    def __len__(self) -> int:
        return len(self._embeddings)

    def save(self, path: str) -> None:
        """Save index to disk."""
        import pickle
        with open(path, 'wb') as f:
            pickle.dump({
                'embeddings': self._embeddings,
                'metadata': self._metadata,
                'dimension': self.dimension,
            }, f)

    def load(self, path: str) -> "EmbeddingIndex":
        """Load index from disk."""
        import pickle
        with open(path, 'rb') as f:
            data = pickle.load(f)
        self._embeddings = data['embeddings']
        self._metadata = data['metadata']
        self.dimension = data['dimension']
        return self
