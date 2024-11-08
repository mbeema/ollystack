"""
Log Sequence Anomaly Detection

Detects anomalies in sequences of log events:
- Unusual event orderings
- Missing expected follow-up events
- Unexpected event combinations
- State machine violations
"""

import logging
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Optional
from collections import defaultdict
from enum import Enum

import numpy as np

logger = logging.getLogger(__name__)


class SequenceAnomalyType(Enum):
    """Types of sequence anomalies."""

    UNEXPECTED_TRANSITION = "unexpected_transition"  # Unusual A -> B
    MISSING_FOLLOWUP = "missing_followup"  # Expected B after A, but missing
    OUT_OF_ORDER = "out_of_order"  # Events in wrong order
    UNUSUAL_GAP = "unusual_gap"  # Unusual time between events
    LOOP_DETECTED = "loop_detected"  # Same sequence repeating unusually
    STATE_VIOLATION = "state_violation"  # Invalid state transition


@dataclass
class SequenceAnomaly:
    """Represents a detected sequence anomaly."""

    anomaly_type: SequenceAnomalyType
    timestamp: datetime
    sequence: list[str]  # Pattern IDs in sequence
    expected_next: Optional[str]
    actual_next: Optional[str]
    probability: float  # How likely this sequence is
    score: float  # 0-1 anomaly score
    description: str
    context: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "anomaly_type": self.anomaly_type.value,
            "timestamp": self.timestamp.isoformat(),
            "sequence": self.sequence,
            "expected_next": self.expected_next,
            "actual_next": self.actual_next,
            "probability": self.probability,
            "score": self.score,
            "description": self.description,
            "context": self.context,
        }


@dataclass
class TransitionStats:
    """Statistics for a pattern transition."""

    from_pattern: str
    to_pattern: str
    count: int = 0
    probability: float = 0.0
    mean_gap_seconds: float = 0.0
    std_gap_seconds: float = 0.0
    min_gap_seconds: float = float("inf")
    max_gap_seconds: float = 0.0


@dataclass
class PatternState:
    """State tracking for a pattern in a session."""

    pattern_id: str
    timestamp: datetime
    count: int = 1


class SequenceAnalyzer:
    """
    Analyzes sequences of log patterns for anomalies.

    Builds a Markov-like model of pattern transitions and detects:
    - Unusual transitions (low probability)
    - Missing expected follow-up events
    - Out-of-order events
    - Unusual time gaps
    """

    def __init__(
        self,
        sequence_window_seconds: int = 60,
        ngram_size: int = 3,
        min_transition_count: int = 10,
        low_probability_threshold: float = 0.01,
        gap_anomaly_sigma: float = 3.0,
    ):
        """
        Initialize the sequence analyzer.

        Args:
            sequence_window_seconds: Max time between events in a sequence
            ngram_size: Size of n-grams for sequence analysis
            min_transition_count: Minimum transitions to establish baseline
            low_probability_threshold: Threshold for unusual transitions
            gap_anomaly_sigma: Sigma threshold for time gap anomalies
        """
        self.sequence_window_seconds = sequence_window_seconds
        self.ngram_size = ngram_size
        self.min_transition_count = min_transition_count
        self.low_probability_threshold = low_probability_threshold
        self.gap_anomaly_sigma = gap_anomaly_sigma

        # Transition matrix: from_pattern -> {to_pattern: TransitionStats}
        self._transitions: dict[str, dict[str, TransitionStats]] = defaultdict(dict)

        # N-gram counts: tuple of pattern_ids -> count
        self._ngram_counts: dict[tuple, int] = defaultdict(int)

        # Session tracking: session_id -> list of (timestamp, pattern_id)
        self._sessions: dict[str, list[tuple[datetime, str]]] = defaultdict(list)

        # Expected follow-ups: pattern_id -> list of expected next patterns
        self._expected_followups: dict[str, list[str]] = {}

        # State machine rules (optional)
        self._state_rules: dict[str, set[str]] = {}

    def record_event(
        self,
        pattern_id: str,
        timestamp: Optional[datetime] = None,
        session_id: str = "default",
    ) -> list[SequenceAnomaly]:
        """
        Record a pattern occurrence and check for sequence anomalies.

        Args:
            pattern_id: The pattern identifier
            timestamp: When the event occurred
            session_id: Session/request identifier for grouping

        Returns:
            List of detected anomalies
        """
        if timestamp is None:
            timestamp = datetime.utcnow()

        anomalies = []

        # Get session history
        session = self._sessions[session_id]

        # Clean old events from session
        cutoff = timestamp - timedelta(seconds=self.sequence_window_seconds * 10)
        session[:] = [(ts, pid) for ts, pid in session if ts >= cutoff]

        # Check for sequence anomalies if we have history
        if session:
            last_ts, last_pattern = session[-1]
            gap_seconds = (timestamp - last_ts).total_seconds()

            # Only analyze if within sequence window
            if gap_seconds <= self.sequence_window_seconds:
                # Check transition anomaly
                transition_anomaly = self._check_transition(
                    last_pattern, pattern_id, gap_seconds, timestamp
                )
                if transition_anomaly:
                    anomalies.append(transition_anomaly)

                # Check n-gram anomaly
                ngram_anomaly = self._check_ngram(session, pattern_id, timestamp)
                if ngram_anomaly:
                    anomalies.append(ngram_anomaly)

                # Check state machine rules
                state_anomaly = self._check_state_rules(
                    last_pattern, pattern_id, timestamp
                )
                if state_anomaly:
                    anomalies.append(state_anomaly)

                # Update transition stats
                self._update_transition(last_pattern, pattern_id, gap_seconds)

            # Check for missing expected follow-up
            missing_anomaly = self._check_missing_followup(
                last_pattern, pattern_id, gap_seconds, timestamp
            )
            if missing_anomaly:
                anomalies.append(missing_anomaly)

        # Add event to session
        session.append((timestamp, pattern_id))

        # Update n-gram counts
        self._update_ngrams(session)

        return anomalies

    def add_expected_followup(
        self,
        from_pattern: str,
        to_patterns: list[str],
        max_gap_seconds: float = 30.0,
    ) -> None:
        """
        Define expected follow-up patterns.

        Args:
            from_pattern: The triggering pattern
            to_patterns: Expected subsequent patterns
            max_gap_seconds: Maximum time for follow-up
        """
        self._expected_followups[from_pattern] = to_patterns

    def add_state_rule(
        self,
        from_pattern: str,
        valid_next_patterns: set[str],
    ) -> None:
        """
        Add a state machine rule.

        Args:
            from_pattern: The current state pattern
            valid_next_patterns: Set of valid next patterns
        """
        self._state_rules[from_pattern] = valid_next_patterns

    def get_transition_matrix(self) -> dict[str, dict[str, float]]:
        """Get the transition probability matrix."""
        matrix = {}
        for from_pattern, transitions in self._transitions.items():
            total = sum(t.count for t in transitions.values())
            if total > 0:
                matrix[from_pattern] = {
                    to_pattern: stats.count / total
                    for to_pattern, stats in transitions.items()
                }
        return matrix

    def get_likely_next(
        self, current_pattern: str, top_k: int = 5
    ) -> list[tuple[str, float]]:
        """Get the most likely next patterns."""
        transitions = self._transitions.get(current_pattern, {})
        total = sum(t.count for t in transitions.values())

        if total == 0:
            return []

        probs = [
            (to_pattern, stats.count / total)
            for to_pattern, stats in transitions.items()
        ]
        probs.sort(key=lambda x: x[1], reverse=True)
        return probs[:top_k]

    def get_sequence_probability(self, sequence: list[str]) -> float:
        """Calculate the probability of a sequence."""
        if len(sequence) < 2:
            return 1.0

        prob = 1.0
        for i in range(len(sequence) - 1):
            from_pattern = sequence[i]
            to_pattern = sequence[i + 1]

            transitions = self._transitions.get(from_pattern, {})
            total = sum(t.count for t in transitions.values())

            if total == 0:
                # Unknown transition
                prob *= 0.01
            else:
                trans_stats = transitions.get(to_pattern)
                if trans_stats:
                    prob *= trans_stats.count / total
                else:
                    # Unseen transition
                    prob *= 0.001

        return prob

    def analyze_session(self, session_id: str) -> dict:
        """Analyze a session's sequence patterns."""
        session = self._sessions.get(session_id, [])

        if not session:
            return {"event_count": 0}

        patterns = [pid for _, pid in session]
        prob = self.get_sequence_probability(patterns)

        # Calculate gaps
        gaps = []
        for i in range(1, len(session)):
            gap = (session[i][0] - session[i - 1][0]).total_seconds()
            gaps.append(gap)

        return {
            "event_count": len(session),
            "unique_patterns": len(set(patterns)),
            "sequence_probability": prob,
            "duration_seconds": (session[-1][0] - session[0][0]).total_seconds(),
            "mean_gap_seconds": np.mean(gaps) if gaps else 0,
            "patterns": patterns,
        }

    def _check_transition(
        self,
        from_pattern: str,
        to_pattern: str,
        gap_seconds: float,
        timestamp: datetime,
    ) -> Optional[SequenceAnomaly]:
        """Check if a transition is anomalous."""
        transitions = self._transitions.get(from_pattern, {})
        total = sum(t.count for t in transitions.values())

        if total < self.min_transition_count:
            # Not enough data for baseline
            return None

        trans_stats = transitions.get(to_pattern)

        if trans_stats:
            probability = trans_stats.count / total

            # Check for low probability transition
            if probability < self.low_probability_threshold:
                return SequenceAnomaly(
                    anomaly_type=SequenceAnomalyType.UNEXPECTED_TRANSITION,
                    timestamp=timestamp,
                    sequence=[from_pattern, to_pattern],
                    expected_next=self._get_most_likely_next(from_pattern),
                    actual_next=to_pattern,
                    probability=probability,
                    score=min((self.low_probability_threshold / probability) * 0.5, 1.0),
                    description=f"Unusual transition: {from_pattern} -> {to_pattern} (prob: {probability:.4f})",
                )

            # Check for unusual time gap
            if trans_stats.count >= 10 and trans_stats.std_gap_seconds > 0:
                z_score = abs(gap_seconds - trans_stats.mean_gap_seconds) / trans_stats.std_gap_seconds
                if z_score > self.gap_anomaly_sigma:
                    return SequenceAnomaly(
                        anomaly_type=SequenceAnomalyType.UNUSUAL_GAP,
                        timestamp=timestamp,
                        sequence=[from_pattern, to_pattern],
                        expected_next=None,
                        actual_next=to_pattern,
                        probability=probability,
                        score=min(z_score / (self.gap_anomaly_sigma * 2), 1.0),
                        description=f"Unusual gap: {gap_seconds:.1f}s (expected ~{trans_stats.mean_gap_seconds:.1f}s)",
                        context={
                            "gap_seconds": gap_seconds,
                            "expected_gap": trans_stats.mean_gap_seconds,
                            "z_score": z_score,
                        },
                    )
        else:
            # Never seen this transition
            return SequenceAnomaly(
                anomaly_type=SequenceAnomalyType.UNEXPECTED_TRANSITION,
                timestamp=timestamp,
                sequence=[from_pattern, to_pattern],
                expected_next=self._get_most_likely_next(from_pattern),
                actual_next=to_pattern,
                probability=0.0,
                score=0.9,
                description=f"New transition: {from_pattern} -> {to_pattern} (never seen before)",
            )

        return None

    def _check_ngram(
        self,
        session: list[tuple[datetime, str]],
        new_pattern: str,
        timestamp: datetime,
    ) -> Optional[SequenceAnomaly]:
        """Check if the n-gram ending with new_pattern is anomalous."""
        if len(session) < self.ngram_size - 1:
            return None

        # Build n-gram
        recent = [pid for _, pid in session[-(self.ngram_size - 1):]]
        ngram = tuple(recent + [new_pattern])

        # Check if this n-gram has been seen
        count = self._ngram_counts.get(ngram, 0)
        total_ngrams = sum(self._ngram_counts.values())

        if total_ngrams > 1000 and count == 0:
            # Never seen n-gram with significant baseline
            return SequenceAnomaly(
                anomaly_type=SequenceAnomalyType.OUT_OF_ORDER,
                timestamp=timestamp,
                sequence=list(ngram),
                expected_next=None,
                actual_next=new_pattern,
                probability=0.0,
                score=0.8,
                description=f"Unusual sequence: {' -> '.join(ngram)}",
            )

        return None

    def _check_state_rules(
        self,
        from_pattern: str,
        to_pattern: str,
        timestamp: datetime,
    ) -> Optional[SequenceAnomaly]:
        """Check state machine rules."""
        valid_next = self._state_rules.get(from_pattern)

        if valid_next is not None and to_pattern not in valid_next:
            return SequenceAnomaly(
                anomaly_type=SequenceAnomalyType.STATE_VIOLATION,
                timestamp=timestamp,
                sequence=[from_pattern, to_pattern],
                expected_next=list(valid_next)[0] if valid_next else None,
                actual_next=to_pattern,
                probability=0.0,
                score=1.0,
                description=f"State violation: {to_pattern} not valid after {from_pattern}",
                context={"valid_transitions": list(valid_next)},
            )

        return None

    def _check_missing_followup(
        self,
        last_pattern: str,
        current_pattern: str,
        gap_seconds: float,
        timestamp: datetime,
    ) -> Optional[SequenceAnomaly]:
        """Check for missing expected follow-up patterns."""
        expected = self._expected_followups.get(last_pattern)

        if expected and current_pattern not in expected and gap_seconds > 30:
            return SequenceAnomaly(
                anomaly_type=SequenceAnomalyType.MISSING_FOLLOWUP,
                timestamp=timestamp,
                sequence=[last_pattern, current_pattern],
                expected_next=expected[0],
                actual_next=current_pattern,
                probability=0.0,
                score=0.7,
                description=f"Missing follow-up: expected {expected} after {last_pattern}",
                context={"expected_patterns": expected},
            )

        return None

    def _update_transition(
        self,
        from_pattern: str,
        to_pattern: str,
        gap_seconds: float,
    ) -> None:
        """Update transition statistics."""
        if to_pattern not in self._transitions[from_pattern]:
            self._transitions[from_pattern][to_pattern] = TransitionStats(
                from_pattern=from_pattern,
                to_pattern=to_pattern,
            )

        stats = self._transitions[from_pattern][to_pattern]
        stats.count += 1

        # Update gap statistics (online algorithm)
        if stats.count == 1:
            stats.mean_gap_seconds = gap_seconds
            stats.min_gap_seconds = gap_seconds
            stats.max_gap_seconds = gap_seconds
        else:
            # Welford's online algorithm for mean and variance
            delta = gap_seconds - stats.mean_gap_seconds
            stats.mean_gap_seconds += delta / stats.count
            delta2 = gap_seconds - stats.mean_gap_seconds
            stats.std_gap_seconds = np.sqrt(
                (stats.std_gap_seconds ** 2 * (stats.count - 1) + delta * delta2)
                / stats.count
            )
            stats.min_gap_seconds = min(stats.min_gap_seconds, gap_seconds)
            stats.max_gap_seconds = max(stats.max_gap_seconds, gap_seconds)

    def _update_ngrams(self, session: list[tuple[datetime, str]]) -> None:
        """Update n-gram counts."""
        if len(session) < self.ngram_size:
            return

        patterns = [pid for _, pid in session[-self.ngram_size:]]
        ngram = tuple(patterns)
        self._ngram_counts[ngram] += 1

    def _get_most_likely_next(self, from_pattern: str) -> Optional[str]:
        """Get the most likely next pattern."""
        transitions = self._transitions.get(from_pattern, {})
        if not transitions:
            return None

        best = max(transitions.items(), key=lambda x: x[1].count)
        return best[0]
