"""
Log Clustering

Clusters similar log messages for:
- Error grouping (deduplicate similar errors)
- Pattern discovery
- Anomaly grouping
"""

import logging
from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional
from collections import defaultdict

import numpy as np
from sklearn.cluster import HDBSCAN, KMeans, DBSCAN
from sklearn.metrics import silhouette_score

from ollystack_ai.embeddings.log_embeddings import LogEmbedder, LogPreprocessor

logger = logging.getLogger(__name__)


@dataclass
class ClusterResult:
    """Result of clustering."""

    cluster_id: int
    size: int
    centroid: Optional[np.ndarray]
    representative_log: str
    sample_logs: list[str]
    keywords: list[str]
    severity_distribution: dict[str, int]
    first_seen: datetime
    last_seen: datetime


@dataclass
class ClusteringOutput:
    """Output of clustering operation."""

    clusters: list[ClusterResult]
    labels: list[int]  # Cluster label for each input
    n_clusters: int
    n_noise: int  # Points not in any cluster (-1 labels)
    silhouette_score: Optional[float]
    summary: str


class LogClusterer:
    """
    Clusters log messages using embeddings.

    Supports multiple algorithms:
    - HDBSCAN: Best for unknown cluster count, handles noise
    - DBSCAN: Good for density-based clustering
    - KMeans: When cluster count is known
    """

    def __init__(
        self,
        embedder: Optional[LogEmbedder] = None,
        algorithm: str = "hdbscan",
        min_cluster_size: int = 5,
        min_samples: int = 3,
        preprocess: bool = True,
    ):
        """
        Initialize the clusterer.

        Args:
            embedder: LogEmbedder instance (creates one if None)
            algorithm: Clustering algorithm ('hdbscan', 'dbscan', 'kmeans')
            min_cluster_size: Minimum cluster size for HDBSCAN
            min_samples: Minimum samples for core points
            preprocess: Preprocess logs before embedding
        """
        self.embedder = embedder or LogEmbedder()
        self.algorithm = algorithm
        self.min_cluster_size = min_cluster_size
        self.min_samples = min_samples
        self.preprocess = preprocess

        self._preprocessor = LogPreprocessor() if preprocess else None
        self._last_labels: Optional[np.ndarray] = None
        self._last_embeddings: Optional[np.ndarray] = None

    def cluster(
        self,
        logs: list[str],
        severities: Optional[list[str]] = None,
        timestamps: Optional[list[datetime]] = None,
        n_clusters: Optional[int] = None,  # For KMeans
    ) -> ClusteringOutput:
        """
        Cluster log messages.

        Args:
            logs: List of log messages
            severities: Optional severity level for each log
            timestamps: Optional timestamp for each log
            n_clusters: Number of clusters (only for KMeans)

        Returns:
            ClusteringOutput with cluster assignments and stats
        """
        if len(logs) < self.min_cluster_size:
            logger.warning(f"Too few logs ({len(logs)}) for clustering")
            return ClusteringOutput(
                clusters=[],
                labels=[-1] * len(logs),
                n_clusters=0,
                n_noise=len(logs),
                silhouette_score=None,
                summary="Too few logs for clustering",
            )

        # Preprocess
        processed = self._preprocessor.preprocess_batch(logs) if self._preprocessor else logs

        # Generate embeddings
        logger.info(f"Generating embeddings for {len(logs)} logs...")
        embeddings = self.embedder.embed_batch(processed)
        embedding_matrix = np.array([e.embedding for e in embeddings])
        self._last_embeddings = embedding_matrix

        # Cluster
        logger.info(f"Clustering with {self.algorithm}...")
        labels = self._cluster_embeddings(embedding_matrix, n_clusters)
        self._last_labels = labels

        # Build cluster results
        clusters = self._build_clusters(
            logs=logs,
            labels=labels,
            embeddings=embedding_matrix,
            severities=severities,
            timestamps=timestamps,
        )

        # Calculate silhouette score if we have multiple clusters
        unique_labels = set(labels)
        n_clusters = len([l for l in unique_labels if l >= 0])
        n_noise = sum(1 for l in labels if l == -1)

        sil_score = None
        if n_clusters > 1 and n_noise < len(logs) - n_clusters:
            try:
                valid_mask = labels >= 0
                if sum(valid_mask) > n_clusters:
                    sil_score = silhouette_score(
                        embedding_matrix[valid_mask],
                        labels[valid_mask]
                    )
            except Exception as e:
                logger.warning(f"Could not compute silhouette score: {e}")

        return ClusteringOutput(
            clusters=clusters,
            labels=labels.tolist(),
            n_clusters=n_clusters,
            n_noise=n_noise,
            silhouette_score=sil_score,
            summary=self._generate_summary(n_clusters, n_noise, len(logs)),
        )

    def cluster_incremental(
        self,
        new_logs: list[str],
        existing_centroids: list[np.ndarray],
        threshold: float = 0.7,
    ) -> list[int]:
        """
        Assign new logs to existing clusters.

        Args:
            new_logs: New log messages
            existing_centroids: Centroids of existing clusters
            threshold: Similarity threshold for assignment

        Returns:
            Cluster assignments (-1 for no match)
        """
        if not existing_centroids:
            return [-1] * len(new_logs)

        processed = self._preprocessor.preprocess_batch(new_logs) if self._preprocessor else new_logs
        embeddings = self.embedder.embed_batch(processed)

        centroid_matrix = np.array(existing_centroids)
        assignments = []

        for emb_result in embeddings:
            emb = emb_result.embedding
            similarities = centroid_matrix @ emb

            best_idx = np.argmax(similarities)
            best_sim = similarities[best_idx]

            if best_sim >= threshold:
                assignments.append(int(best_idx))
            else:
                assignments.append(-1)

        return assignments

    def _cluster_embeddings(
        self,
        embeddings: np.ndarray,
        n_clusters: Optional[int] = None,
    ) -> np.ndarray:
        """Run clustering algorithm on embeddings."""
        if self.algorithm == "hdbscan":
            clusterer = HDBSCAN(
                min_cluster_size=self.min_cluster_size,
                min_samples=self.min_samples,
                metric="euclidean",
                cluster_selection_method="eom",
            )
            labels = clusterer.fit_predict(embeddings)

        elif self.algorithm == "dbscan":
            clusterer = DBSCAN(
                eps=0.3,
                min_samples=self.min_samples,
                metric="cosine",
            )
            labels = clusterer.fit_predict(embeddings)

        elif self.algorithm == "kmeans":
            if n_clusters is None:
                # Estimate cluster count
                n_clusters = min(max(len(embeddings) // 20, 2), 50)

            clusterer = KMeans(
                n_clusters=n_clusters,
                random_state=42,
                n_init=10,
            )
            labels = clusterer.fit_predict(embeddings)

        else:
            raise ValueError(f"Unknown algorithm: {self.algorithm}")

        return labels

    def _build_clusters(
        self,
        logs: list[str],
        labels: np.ndarray,
        embeddings: np.ndarray,
        severities: Optional[list[str]],
        timestamps: Optional[list[datetime]],
    ) -> list[ClusterResult]:
        """Build ClusterResult objects for each cluster."""
        clusters = []

        # Group by cluster
        cluster_members: dict[int, list[int]] = defaultdict(list)
        for i, label in enumerate(labels):
            if label >= 0:  # Skip noise points
                cluster_members[label].append(i)

        for cluster_id, member_indices in sorted(cluster_members.items()):
            # Get member data
            member_logs = [logs[i] for i in member_indices]
            member_embeddings = embeddings[member_indices]

            # Calculate centroid
            centroid = np.mean(member_embeddings, axis=0)
            centroid /= np.linalg.norm(centroid)  # Normalize

            # Find representative (closest to centroid)
            distances = np.linalg.norm(member_embeddings - centroid, axis=1)
            representative_idx = member_indices[np.argmin(distances)]
            representative_log = logs[representative_idx]

            # Sample logs (up to 5)
            sample_indices = member_indices[:5]
            sample_logs = [logs[i] for i in sample_indices]

            # Extract keywords
            keywords = self._extract_keywords(member_logs)

            # Severity distribution
            severity_dist = {}
            if severities:
                for i in member_indices:
                    sev = severities[i]
                    severity_dist[sev] = severity_dist.get(sev, 0) + 1

            # Timestamps
            first_seen = datetime.utcnow()
            last_seen = datetime.utcnow()
            if timestamps:
                cluster_times = [timestamps[i] for i in member_indices]
                first_seen = min(cluster_times)
                last_seen = max(cluster_times)

            clusters.append(ClusterResult(
                cluster_id=cluster_id,
                size=len(member_indices),
                centroid=centroid,
                representative_log=representative_log,
                sample_logs=sample_logs,
                keywords=keywords,
                severity_distribution=severity_dist,
                first_seen=first_seen,
                last_seen=last_seen,
            ))

        # Sort by size
        clusters.sort(key=lambda c: c.size, reverse=True)

        return clusters

    def _extract_keywords(self, logs: list[str], top_k: int = 5) -> list[str]:
        """Extract common keywords from logs."""
        from collections import Counter
        import re

        # Tokenize all logs
        all_tokens = []
        for log in logs:
            tokens = re.findall(r'\b[a-zA-Z_][a-zA-Z0-9_]*\b', log.lower())
            all_tokens.extend(tokens)

        # Count and filter
        counter = Counter(all_tokens)

        # Remove very common words
        stopwords = {
            'the', 'a', 'an', 'is', 'are', 'was', 'were', 'be', 'been',
            'being', 'have', 'has', 'had', 'do', 'does', 'did', 'will',
            'would', 'could', 'should', 'may', 'might', 'must', 'shall',
            'at', 'by', 'for', 'from', 'in', 'into', 'of', 'on', 'to',
            'with', 'and', 'or', 'but', 'if', 'then', 'else', 'when',
            'up', 'down', 'out', 'off', 'over', 'under', 'again',
            'true', 'false', 'null', 'none', 'info', 'debug', 'warn',
        }

        keywords = [
            word for word, count in counter.most_common(top_k + len(stopwords))
            if word not in stopwords and len(word) > 2
        ][:top_k]

        return keywords

    def _generate_summary(self, n_clusters: int, n_noise: int, total: int) -> str:
        """Generate clustering summary."""
        if n_clusters == 0:
            return "No clusters found"

        coverage = (total - n_noise) / total * 100
        return (
            f"Found {n_clusters} clusters covering {coverage:.1f}% of logs. "
            f"{n_noise} logs not clustered."
        )


class ErrorGrouper:
    """
    Groups similar errors for deduplication and aggregation.

    Specifically designed for error/exception messages.
    """

    def __init__(
        self,
        similarity_threshold: float = 0.8,
        max_groups: int = 100,
    ):
        self.similarity_threshold = similarity_threshold
        self.max_groups = max_groups
        self.embedder = LogEmbedder()
        self.preprocessor = LogPreprocessor()

        # Groups: {group_id: {'centroid': np.ndarray, 'representative': str, 'count': int, ...}}
        self._groups: dict[int, dict] = {}
        self._next_group_id: int = 0

    def add_error(
        self,
        error_message: str,
        timestamp: Optional[datetime] = None,
        metadata: Optional[dict] = None,
    ) -> dict:
        """
        Add an error and get its group assignment.

        Returns group info including whether this is a new group.
        """
        if timestamp is None:
            timestamp = datetime.utcnow()

        # Preprocess and embed
        processed = self.preprocessor.preprocess(error_message)
        embedding = self.embedder.embed(processed).embedding

        # Find matching group
        best_match = None
        best_similarity = 0

        for group_id, group in self._groups.items():
            similarity = float(np.dot(embedding, group['centroid']))
            if similarity > best_similarity:
                best_similarity = similarity
                best_match = group_id

        if best_match is not None and best_similarity >= self.similarity_threshold:
            # Update existing group
            group = self._groups[best_match]
            group['count'] += 1
            group['last_seen'] = timestamp

            # Update centroid (running average)
            n = group['count']
            group['centroid'] = (
                group['centroid'] * (n - 1) + embedding
            ) / n
            group['centroid'] /= np.linalg.norm(group['centroid'])

            return {
                'group_id': best_match,
                'is_new': False,
                'similarity': best_similarity,
                'count': group['count'],
                'representative': group['representative'],
                'first_seen': group['first_seen'].isoformat(),
            }

        else:
            # Create new group
            if len(self._groups) >= self.max_groups:
                # Evict least recent group
                oldest_id = min(
                    self._groups.keys(),
                    key=lambda k: self._groups[k]['last_seen']
                )
                del self._groups[oldest_id]

            group_id = self._next_group_id
            self._next_group_id += 1

            self._groups[group_id] = {
                'centroid': embedding,
                'representative': error_message,
                'count': 1,
                'first_seen': timestamp,
                'last_seen': timestamp,
                'metadata': metadata or {},
            }

            return {
                'group_id': group_id,
                'is_new': True,
                'similarity': 1.0,
                'count': 1,
                'representative': error_message,
                'first_seen': timestamp.isoformat(),
            }

    def get_groups(self) -> list[dict]:
        """Get all current error groups."""
        return [
            {
                'group_id': gid,
                'representative': g['representative'][:200],
                'count': g['count'],
                'first_seen': g['first_seen'].isoformat(),
                'last_seen': g['last_seen'].isoformat(),
            }
            for gid, g in sorted(
                self._groups.items(),
                key=lambda x: x[1]['count'],
                reverse=True
            )
        ]

    def get_top_groups(self, n: int = 10) -> list[dict]:
        """Get top N error groups by count."""
        return self.get_groups()[:n]

    def clear(self) -> None:
        """Clear all groups."""
        self._groups.clear()
        self._next_group_id = 0
