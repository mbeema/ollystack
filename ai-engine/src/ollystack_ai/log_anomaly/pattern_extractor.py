"""
Log Pattern Extractor

Implements the Drain algorithm for log parsing and pattern extraction.
Drain is a fixed-depth tree based online log parsing method that can
parse logs with high accuracy and speed.

Reference: He, P., Zhu, J., Zheng, Z., & Lyu, M. R. (2017).
"Drain: An Online Log Parsing Approach with Fixed Depth Tree"
"""

import re
import hashlib
import logging
from dataclasses import dataclass, field
from typing import Optional
from collections import defaultdict
from datetime import datetime

logger = logging.getLogger(__name__)


@dataclass
class LogPattern:
    """Represents an extracted log pattern (template)."""

    pattern_id: str
    template: str  # Log template with <*> for variables
    tokens: list[str]
    regex: str  # Compiled regex for matching
    count: int = 0
    first_seen: datetime = field(default_factory=datetime.utcnow)
    last_seen: datetime = field(default_factory=datetime.utcnow)
    sample_logs: list[str] = field(default_factory=list)
    severity_distribution: dict[str, int] = field(default_factory=dict)

    def matches(self, log_tokens: list[str]) -> bool:
        """Check if log tokens match this pattern."""
        if len(log_tokens) != len(self.tokens):
            return False

        for pattern_token, log_token in zip(self.tokens, log_tokens):
            if pattern_token != "<*>" and pattern_token != log_token:
                return False
        return True

    def update(self, log_message: str, severity: str = "INFO") -> None:
        """Update pattern statistics with a new matching log."""
        self.count += 1
        self.last_seen = datetime.utcnow()

        # Keep up to 5 sample logs
        if len(self.sample_logs) < 5:
            self.sample_logs.append(log_message)

        # Track severity distribution
        self.severity_distribution[severity] = (
            self.severity_distribution.get(severity, 0) + 1
        )

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "pattern_id": self.pattern_id,
            "template": self.template,
            "count": self.count,
            "first_seen": self.first_seen.isoformat(),
            "last_seen": self.last_seen.isoformat(),
            "sample_logs": self.sample_logs,
            "severity_distribution": self.severity_distribution,
        }


@dataclass
class PatternNode:
    """Node in the Drain parse tree."""

    children: dict = field(default_factory=dict)
    patterns: list[LogPattern] = field(default_factory=list)


class LogPatternExtractor:
    """
    Extracts log patterns using the Drain algorithm.

    The Drain algorithm builds a fixed-depth tree where:
    - Level 1: Log length (number of tokens)
    - Level 2-N: First N tokens (prefix)
    - Leaf: Pattern clusters

    This allows efficient O(1) lookup for matching patterns.
    """

    # Common variable patterns to pre-mask
    VARIABLE_PATTERNS = [
        # IP addresses
        (r"\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b", "<IP>"),
        # URLs
        (r"https?://[^\s]+", "<URL>"),
        # File paths
        (r"(?:/[a-zA-Z0-9_\-\.]+)+", "<PATH>"),
        # UUIDs
        (r"\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b", "<UUID>"),
        # Hex strings (32+ chars)
        (r"\b[0-9a-fA-F]{32,}\b", "<HEX>"),
        # Numbers
        (r"\b\d+\.?\d*\b", "<NUM>"),
        # Email addresses
        (r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b", "<EMAIL>"),
        # Timestamps
        (r"\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}", "<TIMESTAMP>"),
        # Memory addresses
        (r"0x[0-9a-fA-F]+", "<ADDR>"),
    ]

    def __init__(
        self,
        depth: int = 4,
        similarity_threshold: float = 0.5,
        max_children: int = 100,
        max_patterns_per_node: int = 100,
        preprocess: bool = True,
    ):
        """
        Initialize the pattern extractor.

        Args:
            depth: Depth of the prefix tree (excluding length layer)
            similarity_threshold: Minimum similarity for pattern matching (0-1)
            max_children: Maximum children per node
            max_patterns_per_node: Maximum patterns per leaf node
            preprocess: Whether to preprocess logs (mask common variables)
        """
        self.depth = depth
        self.similarity_threshold = similarity_threshold
        self.max_children = max_children
        self.max_patterns_per_node = max_patterns_per_node
        self.preprocess = preprocess

        # Root of the parse tree (keyed by log length)
        self.root: dict[int, PatternNode] = {}

        # Pattern registry
        self.patterns: dict[str, LogPattern] = {}

        # Compiled regex patterns for preprocessing
        self._compiled_patterns = [
            (re.compile(pattern), replacement)
            for pattern, replacement in self.VARIABLE_PATTERNS
        ]

        # Statistics
        self.total_logs = 0
        self.unique_patterns = 0

    def parse(
        self, log_message: str, severity: str = "INFO"
    ) -> tuple[LogPattern, bool]:
        """
        Parse a log message and extract/match its pattern.

        Args:
            log_message: The log message to parse
            severity: Log severity level

        Returns:
            Tuple of (matched pattern, is_new_pattern)
        """
        self.total_logs += 1

        # Preprocess log message
        processed = self._preprocess(log_message) if self.preprocess else log_message

        # Tokenize
        tokens = self._tokenize(processed)

        if not tokens:
            # Handle empty logs
            tokens = ["<EMPTY>"]

        log_length = len(tokens)

        # Traverse tree to find matching pattern
        pattern, is_new = self._tree_search(tokens, log_length)

        # Update pattern
        pattern.update(log_message, severity)

        return pattern, is_new

    def parse_batch(
        self, logs: list[dict]
    ) -> list[tuple[LogPattern, bool]]:
        """
        Parse a batch of logs.

        Args:
            logs: List of dicts with 'message' and optional 'severity'

        Returns:
            List of (pattern, is_new) tuples
        """
        results = []
        for log in logs:
            message = log.get("message", "")
            severity = log.get("severity", "INFO")
            result = self.parse(message, severity)
            results.append(result)
        return results

    def get_pattern(self, pattern_id: str) -> Optional[LogPattern]:
        """Get a pattern by ID."""
        return self.patterns.get(pattern_id)

    def get_all_patterns(self) -> list[LogPattern]:
        """Get all extracted patterns."""
        return list(self.patterns.values())

    def get_top_patterns(self, n: int = 20) -> list[LogPattern]:
        """Get the N most frequent patterns."""
        return sorted(
            self.patterns.values(), key=lambda p: p.count, reverse=True
        )[:n]

    def get_rare_patterns(self, threshold: int = 5) -> list[LogPattern]:
        """Get patterns with count below threshold."""
        return [p for p in self.patterns.values() if p.count <= threshold]

    def get_new_patterns(self, since: datetime) -> list[LogPattern]:
        """Get patterns first seen since a given time."""
        return [p for p in self.patterns.values() if p.first_seen >= since]

    def get_statistics(self) -> dict:
        """Get extraction statistics."""
        patterns = list(self.patterns.values())
        if not patterns:
            return {
                "total_logs": self.total_logs,
                "unique_patterns": 0,
                "compression_ratio": 0,
            }

        counts = [p.count for p in patterns]
        return {
            "total_logs": self.total_logs,
            "unique_patterns": len(patterns),
            "compression_ratio": self.total_logs / max(len(patterns), 1),
            "avg_pattern_frequency": sum(counts) / len(counts),
            "max_pattern_frequency": max(counts),
            "min_pattern_frequency": min(counts),
            "single_occurrence_patterns": sum(1 for c in counts if c == 1),
        }

    def _preprocess(self, log_message: str) -> str:
        """Preprocess log message by masking common variables."""
        result = log_message

        for pattern, replacement in self._compiled_patterns:
            result = pattern.sub(replacement, result)

        return result

    def _tokenize(self, log_message: str) -> list[str]:
        """Tokenize log message into words."""
        # Split by whitespace and common delimiters
        tokens = re.split(r"[\s=:,\[\]\(\)\{\}]+", log_message)
        # Filter empty tokens
        return [t for t in tokens if t]

    def _tree_search(
        self, tokens: list[str], log_length: int
    ) -> tuple[LogPattern, bool]:
        """
        Search the parse tree for a matching pattern.

        If no match is found, create a new pattern.
        """
        # Get or create length node
        if log_length not in self.root:
            self.root[log_length] = PatternNode()

        current_node = self.root[log_length]

        # Traverse prefix tree
        for i in range(min(self.depth, log_length)):
            token = tokens[i]

            # Use wildcard for variable-like tokens
            if self._is_variable(token):
                token = "<*>"

            if token not in current_node.children:
                if len(current_node.children) >= self.max_children:
                    # Too many children, use wildcard
                    token = "<*>"

                if token not in current_node.children:
                    current_node.children[token] = PatternNode()

            current_node = current_node.children[token]

        # Search for matching pattern in leaf node
        matched_pattern = self._find_matching_pattern(current_node, tokens)

        if matched_pattern:
            return matched_pattern, False

        # No match found, create new pattern
        new_pattern = self._create_pattern(tokens)
        current_node.patterns.append(new_pattern)
        self.patterns[new_pattern.pattern_id] = new_pattern
        self.unique_patterns += 1

        # Limit patterns per node
        if len(current_node.patterns) > self.max_patterns_per_node:
            # Merge least frequent patterns
            self._merge_patterns(current_node)

        return new_pattern, True

    def _find_matching_pattern(
        self, node: PatternNode, tokens: list[str]
    ) -> Optional[LogPattern]:
        """Find a matching pattern in the node."""
        best_match = None
        best_similarity = self.similarity_threshold

        for pattern in node.patterns:
            if len(pattern.tokens) != len(tokens):
                continue

            similarity = self._calculate_similarity(pattern.tokens, tokens)

            if similarity >= best_similarity:
                best_similarity = similarity
                best_match = pattern

                # Update pattern tokens if needed (generalize)
                if similarity < 1.0:
                    self._update_pattern_tokens(pattern, tokens)

        return best_match

    def _calculate_similarity(
        self, pattern_tokens: list[str], log_tokens: list[str]
    ) -> float:
        """Calculate similarity between pattern and log tokens."""
        if len(pattern_tokens) != len(log_tokens):
            return 0.0

        matches = 0
        for p_token, l_token in zip(pattern_tokens, log_tokens):
            if p_token == l_token or p_token == "<*>":
                matches += 1

        return matches / len(pattern_tokens)

    def _update_pattern_tokens(
        self, pattern: LogPattern, log_tokens: list[str]
    ) -> None:
        """Update pattern tokens to generalize (replace mismatches with <*>)."""
        for i, (p_token, l_token) in enumerate(zip(pattern.tokens, log_tokens)):
            if p_token != l_token and p_token != "<*>":
                pattern.tokens[i] = "<*>"

        # Update template
        pattern.template = " ".join(pattern.tokens)

    def _create_pattern(self, tokens: list[str]) -> LogPattern:
        """Create a new pattern from tokens."""
        # Generate pattern ID from tokens
        pattern_str = " ".join(tokens)
        pattern_id = hashlib.md5(pattern_str.encode()).hexdigest()[:12]

        # Create regex for matching
        regex_parts = []
        for token in tokens:
            if token == "<*>" or token.startswith("<") and token.endswith(">"):
                regex_parts.append(r".+?")
            else:
                regex_parts.append(re.escape(token))

        regex = r"\s*".join(regex_parts)

        return LogPattern(
            pattern_id=pattern_id,
            template=pattern_str,
            tokens=tokens.copy(),
            regex=regex,
        )

    def _is_variable(self, token: str) -> bool:
        """Check if a token looks like a variable."""
        # Already a placeholder
        if token.startswith("<") and token.endswith(">"):
            return True

        # Contains digits
        if any(c.isdigit() for c in token):
            return True

        # Very long token
        if len(token) > 30:
            return True

        return False

    def _merge_patterns(self, node: PatternNode) -> None:
        """Merge similar patterns to reduce pattern count."""
        # Sort by count (keep most frequent)
        node.patterns.sort(key=lambda p: p.count, reverse=True)

        # Keep top patterns
        kept = node.patterns[: self.max_patterns_per_node // 2]
        to_merge = node.patterns[self.max_patterns_per_node // 2 :]

        # Try to merge into kept patterns
        for pattern in to_merge:
            merged = False
            for kept_pattern in kept:
                if len(kept_pattern.tokens) == len(pattern.tokens):
                    # Merge by generalizing
                    for i in range(len(kept_pattern.tokens)):
                        if kept_pattern.tokens[i] != pattern.tokens[i]:
                            kept_pattern.tokens[i] = "<*>"

                    kept_pattern.count += pattern.count
                    kept_pattern.template = " ".join(kept_pattern.tokens)
                    merged = True
                    break

            if not merged:
                kept.append(pattern)

        node.patterns = kept

    def export_patterns(self) -> list[dict]:
        """Export all patterns as dictionaries."""
        return [p.to_dict() for p in self.patterns.values()]

    def import_patterns(self, patterns: list[dict]) -> None:
        """Import patterns from dictionaries."""
        for p_dict in patterns:
            pattern = LogPattern(
                pattern_id=p_dict["pattern_id"],
                template=p_dict["template"],
                tokens=p_dict["template"].split(),
                regex="",
                count=p_dict.get("count", 0),
                first_seen=datetime.fromisoformat(p_dict.get("first_seen", datetime.utcnow().isoformat())),
                last_seen=datetime.fromisoformat(p_dict.get("last_seen", datetime.utcnow().isoformat())),
                sample_logs=p_dict.get("sample_logs", []),
                severity_distribution=p_dict.get("severity_distribution", {}),
            )
            self.patterns[pattern.pattern_id] = pattern

            # Rebuild tree structure
            tokens = pattern.tokens
            log_length = len(tokens)

            if log_length not in self.root:
                self.root[log_length] = PatternNode()

            current_node = self.root[log_length]
            for i in range(min(self.depth, log_length)):
                token = tokens[i]
                if token not in current_node.children:
                    current_node.children[token] = PatternNode()
                current_node = current_node.children[token]

            current_node.patterns.append(pattern)
