"""
Log Anomaly Detector

Main class for detecting anomalies in log data.
Combines pattern extraction, frequency analysis, and sequence analysis.
"""

import logging
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Optional
from enum import Enum
import hashlib

from ollystack_ai.log_anomaly.pattern_extractor import LogPatternExtractor, LogPattern
from ollystack_ai.log_anomaly.frequency import (
    FrequencyAnalyzer,
    FrequencyAnomaly,
    FrequencyAnomalyType,
)
from ollystack_ai.log_anomaly.sequence import (
    SequenceAnalyzer,
    SequenceAnomaly,
    SequenceAnomalyType,
)

logger = logging.getLogger(__name__)


class AnomalyType(Enum):
    """Types of log anomalies."""

    # Pattern-based
    NEW_PATTERN = "new_pattern"
    RARE_PATTERN = "rare_pattern"
    ERROR_PATTERN = "error_pattern"

    # Frequency-based
    FREQUENCY_SPIKE = "frequency_spike"
    FREQUENCY_DROP = "frequency_drop"
    BURST = "burst"
    MISSING_PATTERN = "missing_pattern"

    # Sequence-based
    UNEXPECTED_SEQUENCE = "unexpected_sequence"
    MISSING_FOLLOWUP = "missing_followup"
    STATE_VIOLATION = "state_violation"

    # Content-based
    SENSITIVE_DATA = "sensitive_data"
    UNUSUAL_CONTENT = "unusual_content"


@dataclass
class LogAnomaly:
    """Represents a detected log anomaly."""

    anomaly_id: str
    anomaly_type: AnomalyType
    timestamp: datetime
    service_name: str
    log_message: str
    pattern_id: Optional[str]
    pattern_template: Optional[str]
    score: float  # 0-1 anomaly score
    severity: str
    description: str
    details: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "anomaly_id": self.anomaly_id,
            "anomaly_type": self.anomaly_type.value,
            "timestamp": self.timestamp.isoformat(),
            "service_name": self.service_name,
            "log_message": self.log_message[:500],  # Truncate long messages
            "pattern_id": self.pattern_id,
            "pattern_template": self.pattern_template,
            "score": self.score,
            "severity": self.severity,
            "description": self.description,
            "details": self.details,
        }


@dataclass
class DetectionResult:
    """Result of log anomaly detection."""

    anomalies: list[LogAnomaly]
    patterns_analyzed: int
    new_patterns_count: int
    summary: str


class LogAnomalyDetector:
    """
    Main log anomaly detection class.

    Integrates multiple detection methods:
    1. Pattern extraction and new pattern detection
    2. Frequency anomaly detection
    3. Sequence anomaly detection
    4. Content-based detection (sensitive data, unusual content)
    """

    # Sensitive data patterns
    SENSITIVE_PATTERNS = [
        (r"password[=:]\s*\S+", "password"),
        (r"api[_-]?key[=:]\s*\S+", "api_key"),
        (r"secret[=:]\s*\S+", "secret"),
        (r"token[=:]\s*[a-zA-Z0-9_\-]+", "token"),
        (r"authorization:\s*bearer\s+\S+", "auth_token"),
        (r"\b\d{16}\b", "credit_card"),
        (r"\b\d{3}-\d{2}-\d{4}\b", "ssn"),
        (r"private[_-]?key", "private_key"),
    ]

    # Error indicators
    ERROR_KEYWORDS = [
        "error", "exception", "fail", "fatal", "critical",
        "panic", "crash", "abort", "timeout", "refused",
        "denied", "unauthorized", "forbidden", "invalid",
    ]

    def __init__(
        self,
        service_name: str = "default",
        enable_pattern_detection: bool = True,
        enable_frequency_detection: bool = True,
        enable_sequence_detection: bool = True,
        enable_content_detection: bool = True,
        new_pattern_score: float = 0.6,
        rare_pattern_threshold: int = 5,
        error_pattern_score: float = 0.8,
    ):
        """
        Initialize the log anomaly detector.

        Args:
            service_name: Service identifier
            enable_pattern_detection: Enable pattern-based detection
            enable_frequency_detection: Enable frequency-based detection
            enable_sequence_detection: Enable sequence-based detection
            enable_content_detection: Enable content-based detection
            new_pattern_score: Base anomaly score for new patterns
            rare_pattern_threshold: Count threshold for rare patterns
            error_pattern_score: Base score for error patterns
        """
        self.service_name = service_name
        self.enable_pattern_detection = enable_pattern_detection
        self.enable_frequency_detection = enable_frequency_detection
        self.enable_sequence_detection = enable_sequence_detection
        self.enable_content_detection = enable_content_detection
        self.new_pattern_score = new_pattern_score
        self.rare_pattern_threshold = rare_pattern_threshold
        self.error_pattern_score = error_pattern_score

        # Initialize components
        self.pattern_extractor = LogPatternExtractor()
        self.frequency_analyzer = FrequencyAnalyzer()
        self.sequence_analyzer = SequenceAnalyzer()

        # Compile sensitive patterns
        import re
        self._sensitive_patterns = [
            (re.compile(pattern, re.IGNORECASE), name)
            for pattern, name in self.SENSITIVE_PATTERNS
        ]

        # Statistics
        self._total_logs = 0
        self._total_anomalies = 0

    def analyze(
        self,
        log_message: str,
        timestamp: Optional[datetime] = None,
        severity: str = "INFO",
        session_id: str = "default",
        metadata: Optional[dict] = None,
    ) -> list[LogAnomaly]:
        """
        Analyze a single log message for anomalies.

        Args:
            log_message: The log message to analyze
            timestamp: When the log was generated
            severity: Log severity level
            session_id: Session/request identifier
            metadata: Additional metadata

        Returns:
            List of detected anomalies
        """
        if timestamp is None:
            timestamp = datetime.utcnow()

        self._total_logs += 1
        anomalies = []

        # Extract pattern
        pattern, is_new = self.pattern_extractor.parse(log_message, severity)

        # Pattern-based detection
        if self.enable_pattern_detection:
            pattern_anomalies = self._detect_pattern_anomalies(
                log_message, pattern, is_new, timestamp, severity
            )
            anomalies.extend(pattern_anomalies)

        # Frequency-based detection
        if self.enable_frequency_detection:
            freq_anomaly = self.frequency_analyzer.record_occurrence(
                pattern_id=pattern.pattern_id,
                timestamp=timestamp,
                pattern_template=pattern.template,
            )
            if freq_anomaly:
                anomalies.append(self._convert_frequency_anomaly(
                    freq_anomaly, log_message, severity
                ))

        # Sequence-based detection
        if self.enable_sequence_detection:
            seq_anomalies = self.sequence_analyzer.record_event(
                pattern_id=pattern.pattern_id,
                timestamp=timestamp,
                session_id=session_id,
            )
            for seq_anomaly in seq_anomalies:
                anomalies.append(self._convert_sequence_anomaly(
                    seq_anomaly, log_message, pattern, severity
                ))

        # Content-based detection
        if self.enable_content_detection:
            content_anomalies = self._detect_content_anomalies(
                log_message, pattern, timestamp, severity
            )
            anomalies.extend(content_anomalies)

        self._total_anomalies += len(anomalies)
        return anomalies

    def analyze_batch(
        self,
        logs: list[dict],
        session_id: str = "default",
    ) -> DetectionResult:
        """
        Analyze a batch of logs.

        Args:
            logs: List of dicts with 'message', 'timestamp', 'severity'
            session_id: Session/request identifier

        Returns:
            DetectionResult with all anomalies
        """
        all_anomalies = []
        new_patterns = 0

        for log in logs:
            message = log.get("message", "")
            timestamp = log.get("timestamp")
            if isinstance(timestamp, str):
                timestamp = datetime.fromisoformat(timestamp)
            severity = log.get("severity", "INFO")

            anomalies = self.analyze(
                log_message=message,
                timestamp=timestamp,
                severity=severity,
                session_id=session_id,
            )
            all_anomalies.extend(anomalies)

            # Count new patterns
            if any(a.anomaly_type == AnomalyType.NEW_PATTERN for a in anomalies):
                new_patterns += 1

        # Check for missing expected patterns
        if self.enable_frequency_detection:
            # Get patterns that should occur regularly
            baseline_patterns = [
                p.pattern_id for p in self.pattern_extractor.get_top_patterns(10)
                if p.count > 100
            ]
            missing_anomalies = self.frequency_analyzer.check_missing_patterns(
                baseline_patterns
            )
            for freq_anomaly in missing_anomalies:
                all_anomalies.append(self._convert_frequency_anomaly(
                    freq_anomaly, "", "INFO"
                ))

        # Sort by score
        all_anomalies.sort(key=lambda a: a.score, reverse=True)

        return DetectionResult(
            anomalies=all_anomalies,
            patterns_analyzed=len(logs),
            new_patterns_count=new_patterns,
            summary=self._generate_summary(all_anomalies, new_patterns),
        )

    def get_statistics(self) -> dict:
        """Get detection statistics."""
        pattern_stats = self.pattern_extractor.get_statistics()
        return {
            "total_logs_analyzed": self._total_logs,
            "total_anomalies_detected": self._total_anomalies,
            "anomaly_rate": self._total_anomalies / max(self._total_logs, 1),
            "pattern_stats": pattern_stats,
        }

    def get_top_patterns(self, n: int = 20) -> list[dict]:
        """Get the N most frequent log patterns."""
        patterns = self.pattern_extractor.get_top_patterns(n)
        return [p.to_dict() for p in patterns]

    def get_rare_patterns(self, threshold: int = 5) -> list[dict]:
        """Get rare log patterns."""
        patterns = self.pattern_extractor.get_rare_patterns(threshold)
        return [p.to_dict() for p in patterns]

    def get_new_patterns(self, hours: int = 24) -> list[dict]:
        """Get patterns first seen in the last N hours."""
        since = datetime.utcnow() - timedelta(hours=hours)
        patterns = self.pattern_extractor.get_new_patterns(since)
        return [p.to_dict() for p in patterns]

    def get_error_patterns(self) -> list[dict]:
        """Get patterns associated with errors."""
        patterns = []
        for pattern in self.pattern_extractor.get_all_patterns():
            error_count = sum(
                pattern.severity_distribution.get(sev, 0)
                for sev in ["ERROR", "FATAL", "CRITICAL"]
            )
            if error_count > 0:
                p_dict = pattern.to_dict()
                p_dict["error_count"] = error_count
                patterns.append(p_dict)

        patterns.sort(key=lambda p: p["error_count"], reverse=True)
        return patterns

    def update_baselines(self) -> None:
        """Update frequency baselines for all known patterns."""
        for pattern in self.pattern_extractor.get_all_patterns():
            self.frequency_analyzer.update_baseline(pattern.pattern_id)

    def add_sequence_rule(
        self,
        from_pattern_template: str,
        valid_next_templates: list[str],
    ) -> None:
        """
        Add a sequence rule for state machine validation.

        Args:
            from_pattern_template: Template of the triggering pattern
            valid_next_templates: List of valid next pattern templates
        """
        # Find pattern IDs by template
        from_pattern = None
        valid_next = set()

        for pattern in self.pattern_extractor.get_all_patterns():
            if pattern.template == from_pattern_template:
                from_pattern = pattern.pattern_id
            if pattern.template in valid_next_templates:
                valid_next.add(pattern.pattern_id)

        if from_pattern:
            self.sequence_analyzer.add_state_rule(from_pattern, valid_next)

    def _detect_pattern_anomalies(
        self,
        log_message: str,
        pattern: LogPattern,
        is_new: bool,
        timestamp: datetime,
        severity: str,
    ) -> list[LogAnomaly]:
        """Detect pattern-based anomalies."""
        anomalies = []

        # New pattern detection
        if is_new:
            # Check if it's an error pattern
            is_error = self._is_error_log(log_message, severity)
            score = self.error_pattern_score if is_error else self.new_pattern_score

            anomalies.append(LogAnomaly(
                anomaly_id=self._generate_id(pattern.pattern_id, timestamp),
                anomaly_type=AnomalyType.NEW_PATTERN,
                timestamp=timestamp,
                service_name=self.service_name,
                log_message=log_message,
                pattern_id=pattern.pattern_id,
                pattern_template=pattern.template,
                score=score,
                severity=severity,
                description=f"New log pattern detected: {pattern.template[:100]}",
                details={
                    "is_error_pattern": is_error,
                },
            ))

        # Rare pattern detection (for patterns we've seen before)
        elif pattern.count <= self.rare_pattern_threshold:
            anomalies.append(LogAnomaly(
                anomaly_id=self._generate_id(pattern.pattern_id, timestamp),
                anomaly_type=AnomalyType.RARE_PATTERN,
                timestamp=timestamp,
                service_name=self.service_name,
                log_message=log_message,
                pattern_id=pattern.pattern_id,
                pattern_template=pattern.template,
                score=0.5 * (1 - pattern.count / self.rare_pattern_threshold),
                severity=severity,
                description=f"Rare pattern (seen {pattern.count} times): {pattern.template[:100]}",
                details={
                    "occurrence_count": pattern.count,
                },
            ))

        # Error pattern with increasing frequency
        if self._is_error_log(log_message, severity):
            error_ratio = self._get_error_ratio(pattern)
            if error_ratio > 0.5 and pattern.count > 10:
                anomalies.append(LogAnomaly(
                    anomaly_id=self._generate_id(pattern.pattern_id, timestamp),
                    anomaly_type=AnomalyType.ERROR_PATTERN,
                    timestamp=timestamp,
                    service_name=self.service_name,
                    log_message=log_message,
                    pattern_id=pattern.pattern_id,
                    pattern_template=pattern.template,
                    score=min(error_ratio, 1.0),
                    severity=severity,
                    description=f"Error-prone pattern ({error_ratio*100:.0f}% errors): {pattern.template[:100]}",
                    details={
                        "error_ratio": error_ratio,
                        "total_count": pattern.count,
                    },
                ))

        return anomalies

    def _detect_content_anomalies(
        self,
        log_message: str,
        pattern: LogPattern,
        timestamp: datetime,
        severity: str,
    ) -> list[LogAnomaly]:
        """Detect content-based anomalies."""
        anomalies = []

        # Check for sensitive data
        for regex, data_type in self._sensitive_patterns:
            if regex.search(log_message):
                anomalies.append(LogAnomaly(
                    anomaly_id=self._generate_id(f"sensitive_{data_type}", timestamp),
                    anomaly_type=AnomalyType.SENSITIVE_DATA,
                    timestamp=timestamp,
                    service_name=self.service_name,
                    log_message=log_message,
                    pattern_id=pattern.pattern_id,
                    pattern_template=pattern.template,
                    score=1.0,
                    severity="CRITICAL",
                    description=f"Sensitive data detected in logs: {data_type}",
                    details={
                        "data_type": data_type,
                    },
                ))
                break  # One sensitive data anomaly per log

        return anomalies

    def _convert_frequency_anomaly(
        self,
        freq_anomaly: FrequencyAnomaly,
        log_message: str,
        severity: str,
    ) -> LogAnomaly:
        """Convert a FrequencyAnomaly to LogAnomaly."""
        anomaly_type_map = {
            FrequencyAnomalyType.SPIKE: AnomalyType.FREQUENCY_SPIKE,
            FrequencyAnomalyType.DROP: AnomalyType.FREQUENCY_DROP,
            FrequencyAnomalyType.BURST: AnomalyType.BURST,
            FrequencyAnomalyType.MISSING: AnomalyType.MISSING_PATTERN,
            FrequencyAnomalyType.TREND_CHANGE: AnomalyType.FREQUENCY_SPIKE,
            FrequencyAnomalyType.UNUSUAL_TIME: AnomalyType.FREQUENCY_SPIKE,
        }

        return LogAnomaly(
            anomaly_id=self._generate_id(freq_anomaly.pattern_id, freq_anomaly.timestamp),
            anomaly_type=anomaly_type_map.get(freq_anomaly.anomaly_type, AnomalyType.FREQUENCY_SPIKE),
            timestamp=freq_anomaly.timestamp,
            service_name=self.service_name,
            log_message=log_message,
            pattern_id=freq_anomaly.pattern_id,
            pattern_template=freq_anomaly.pattern_template,
            score=freq_anomaly.score,
            severity=severity,
            description=freq_anomaly.description,
            details={
                "observed_count": freq_anomaly.observed_count,
                "expected_count": freq_anomaly.expected_count,
                "deviation_sigma": freq_anomaly.deviation_sigma,
                **freq_anomaly.context,
            },
        )

    def _convert_sequence_anomaly(
        self,
        seq_anomaly: SequenceAnomaly,
        log_message: str,
        pattern: LogPattern,
        severity: str,
    ) -> LogAnomaly:
        """Convert a SequenceAnomaly to LogAnomaly."""
        anomaly_type_map = {
            SequenceAnomalyType.UNEXPECTED_TRANSITION: AnomalyType.UNEXPECTED_SEQUENCE,
            SequenceAnomalyType.MISSING_FOLLOWUP: AnomalyType.MISSING_FOLLOWUP,
            SequenceAnomalyType.OUT_OF_ORDER: AnomalyType.UNEXPECTED_SEQUENCE,
            SequenceAnomalyType.UNUSUAL_GAP: AnomalyType.UNEXPECTED_SEQUENCE,
            SequenceAnomalyType.STATE_VIOLATION: AnomalyType.STATE_VIOLATION,
            SequenceAnomalyType.LOOP_DETECTED: AnomalyType.UNEXPECTED_SEQUENCE,
        }

        return LogAnomaly(
            anomaly_id=self._generate_id(pattern.pattern_id, seq_anomaly.timestamp),
            anomaly_type=anomaly_type_map.get(seq_anomaly.anomaly_type, AnomalyType.UNEXPECTED_SEQUENCE),
            timestamp=seq_anomaly.timestamp,
            service_name=self.service_name,
            log_message=log_message,
            pattern_id=pattern.pattern_id,
            pattern_template=pattern.template,
            score=seq_anomaly.score,
            severity=severity,
            description=seq_anomaly.description,
            details={
                "sequence": seq_anomaly.sequence,
                "expected_next": seq_anomaly.expected_next,
                "actual_next": seq_anomaly.actual_next,
                "probability": seq_anomaly.probability,
                **seq_anomaly.context,
            },
        )

    def _is_error_log(self, log_message: str, severity: str) -> bool:
        """Check if a log is an error."""
        if severity.upper() in ["ERROR", "FATAL", "CRITICAL", "SEVERE"]:
            return True

        message_lower = log_message.lower()
        return any(keyword in message_lower for keyword in self.ERROR_KEYWORDS)

    def _get_error_ratio(self, pattern: LogPattern) -> float:
        """Get the ratio of errors for a pattern."""
        total = sum(pattern.severity_distribution.values())
        if total == 0:
            return 0

        error_count = sum(
            pattern.severity_distribution.get(sev, 0)
            for sev in ["ERROR", "FATAL", "CRITICAL", "SEVERE"]
        )
        return error_count / total

    def _generate_id(self, base: str, timestamp: datetime) -> str:
        """Generate a unique anomaly ID."""
        data = f"{base}:{timestamp.isoformat()}:{self._total_anomalies}"
        return hashlib.md5(data.encode()).hexdigest()[:12]

    def _generate_summary(
        self, anomalies: list[LogAnomaly], new_patterns: int
    ) -> str:
        """Generate a summary of detection results."""
        if not anomalies:
            return "No anomalies detected"

        # Count by type
        type_counts = {}
        for a in anomalies:
            type_name = a.anomaly_type.value
            type_counts[type_name] = type_counts.get(type_name, 0) + 1

        parts = [f"{count} {atype}" for atype, count in type_counts.items()]
        summary = f"Detected {len(anomalies)} anomalies: {', '.join(parts)}"

        if new_patterns > 0:
            summary += f". {new_patterns} new patterns discovered."

        # Highlight high severity
        critical = [a for a in anomalies if a.score > 0.8]
        if critical:
            summary += f" {len(critical)} high-severity anomalies require attention."

        return summary
