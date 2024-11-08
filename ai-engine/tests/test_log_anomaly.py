"""
Tests for Log Pattern Anomaly Detection
"""

import pytest
from datetime import datetime, timedelta

from ollystack_ai.log_anomaly.pattern_extractor import LogPatternExtractor, LogPattern
from ollystack_ai.log_anomaly.frequency import FrequencyAnalyzer, FrequencyAnomalyType
from ollystack_ai.log_anomaly.sequence import SequenceAnalyzer, SequenceAnomalyType
from ollystack_ai.log_anomaly.detector import LogAnomalyDetector, AnomalyType


class TestLogPatternExtractor:
    """Tests for the Drain-based log pattern extractor."""

    def test_basic_pattern_extraction(self):
        """Test basic pattern extraction from similar logs."""
        extractor = LogPatternExtractor()

        logs = [
            "User 123 logged in from 192.168.1.1",
            "User 456 logged in from 10.0.0.1",
            "User 789 logged in from 172.16.0.1",
        ]

        patterns = []
        for log in logs:
            pattern, is_new = extractor.parse(log)
            patterns.append((pattern, is_new))

        # First log creates new pattern
        assert patterns[0][1] is True

        # Subsequent logs should match same pattern
        assert patterns[1][1] is False
        assert patterns[2][1] is False

        # All should have same pattern ID
        assert patterns[0][0].pattern_id == patterns[1][0].pattern_id
        assert patterns[1][0].pattern_id == patterns[2][0].pattern_id

        # Pattern should have count 3
        assert patterns[2][0].count == 3

    def test_different_patterns(self):
        """Test that different log types create different patterns."""
        extractor = LogPatternExtractor()

        logs = [
            "User logged in successfully",
            "Database connection established",
            "HTTP request received at /api/users",
        ]

        pattern_ids = set()
        for log in logs:
            pattern, _ = extractor.parse(log)
            pattern_ids.add(pattern.pattern_id)

        # Should have 3 different patterns
        assert len(pattern_ids) == 3

    def test_variable_masking(self):
        """Test that variables like IPs and UUIDs are properly masked."""
        extractor = LogPatternExtractor()

        log1 = "Request abc123-def456-789012 from 192.168.1.1 took 150ms"
        log2 = "Request xyz789-abc123-456789 from 10.0.0.5 took 200ms"

        pattern1, _ = extractor.parse(log1)
        pattern2, _ = extractor.parse(log2)

        # Should match same pattern despite different IPs/UUIDs
        assert pattern1.pattern_id == pattern2.pattern_id

    def test_pattern_statistics(self):
        """Test pattern statistics tracking."""
        extractor = LogPatternExtractor()

        # Generate logs with different severities
        for _ in range(5):
            extractor.parse("Info message", severity="INFO")
        for _ in range(3):
            extractor.parse("Info message", severity="ERROR")

        patterns = extractor.get_all_patterns()
        assert len(patterns) == 1

        pattern = patterns[0]
        assert pattern.count == 8
        assert pattern.severity_distribution.get("INFO") == 5
        assert pattern.severity_distribution.get("ERROR") == 3

    def test_get_top_patterns(self):
        """Test retrieving top patterns by frequency."""
        extractor = LogPatternExtractor()

        # Create patterns with different frequencies
        for _ in range(100):
            extractor.parse("Frequent log message")
        for _ in range(10):
            extractor.parse("Less frequent message")
        extractor.parse("Rare message")

        top = extractor.get_top_patterns(2)
        assert len(top) == 2
        assert top[0].count == 100
        assert top[1].count == 10

    def test_get_rare_patterns(self):
        """Test retrieving rare patterns."""
        extractor = LogPatternExtractor()

        for _ in range(100):
            extractor.parse("Common message")
        for _ in range(3):
            extractor.parse("Rare message type A")
        extractor.parse("Very rare message")

        rare = extractor.get_rare_patterns(threshold=5)
        assert len(rare) == 2

    def test_compression_ratio(self):
        """Test that pattern extraction provides good compression."""
        extractor = LogPatternExtractor()

        # Simulate realistic logs
        for i in range(1000):
            extractor.parse(f"User {i} performed action from IP 10.0.0.{i % 256}")
            extractor.parse(f"Request {i} completed in {50 + i % 100}ms")
            extractor.parse(f"Database query {i} returned {i % 50} rows")

        stats = extractor.get_statistics()

        # Should achieve significant compression (many logs -> few patterns)
        assert stats["compression_ratio"] > 100


class TestFrequencyAnalyzer:
    """Tests for log frequency anomaly detection."""

    def test_burst_detection(self):
        """Test detection of burst patterns."""
        analyzer = FrequencyAnalyzer(burst_window_seconds=5, burst_threshold=5)

        # Create a burst
        base_time = datetime.utcnow()
        anomaly = None

        for i in range(10):
            ts = base_time + timedelta(milliseconds=i * 100)
            result = analyzer.record_occurrence(
                pattern_id="test_pattern",
                timestamp=ts,
                pattern_template="Test message",
            )
            if result:
                anomaly = result

        assert anomaly is not None
        assert anomaly.anomaly_type == FrequencyAnomalyType.BURST

    def test_frequency_spike(self):
        """Test detection of frequency spikes."""
        analyzer = FrequencyAnalyzer(
            window_minutes=1,
            spike_threshold=3.0,
            min_baseline_samples=10,
        )

        # Build baseline (10 occurrences per minute)
        base_time = datetime.utcnow() - timedelta(hours=1)
        for i in range(60):
            for j in range(10):
                ts = base_time + timedelta(minutes=i, seconds=j * 6)
                analyzer.record_occurrence("test_pattern", ts, "Test")

        # Update baseline
        analyzer.update_baseline("test_pattern")

        # Create spike (50 occurrences in one minute)
        current = datetime.utcnow()
        anomaly = None
        for i in range(50):
            ts = current + timedelta(seconds=i)
            result = analyzer.record_occurrence("test_pattern", ts, "Test")
            if result and result.anomaly_type == FrequencyAnomalyType.SPIKE:
                anomaly = result

        assert anomaly is not None
        assert anomaly.observed_count > anomaly.expected_count

    def test_missing_pattern_detection(self):
        """Test detection of missing expected patterns."""
        analyzer = FrequencyAnalyzer(min_baseline_samples=10)

        # Build baseline
        base_time = datetime.utcnow() - timedelta(hours=1)
        for i in range(100):
            ts = base_time + timedelta(minutes=i % 60)
            analyzer.record_occurrence("heartbeat_pattern", ts, "Heartbeat")

        analyzer.update_baseline("heartbeat_pattern")

        # Check for missing pattern (no recent occurrences)
        missing = analyzer.check_missing_patterns(
            ["heartbeat_pattern"],
            window_minutes=5,
        )

        assert len(missing) > 0
        assert missing[0].anomaly_type == FrequencyAnomalyType.MISSING

    def test_frequency_stats(self):
        """Test frequency statistics calculation."""
        analyzer = FrequencyAnalyzer()

        # Record occurrences
        base_time = datetime.utcnow()
        for i in range(30):
            ts = base_time + timedelta(seconds=i * 2)
            analyzer.record_occurrence("test", ts, "Test")

        stats = analyzer.get_frequency_stats("test", window_minutes=1)

        assert stats["count"] == 30
        assert stats["rate_per_minute"] == 30


class TestSequenceAnalyzer:
    """Tests for log sequence anomaly detection."""

    def test_transition_learning(self):
        """Test that transitions are learned correctly."""
        analyzer = SequenceAnalyzer()

        # Record typical sequence
        base_time = datetime.utcnow()
        for i in range(100):
            ts = base_time + timedelta(seconds=i * 3)
            analyzer.record_event("login", ts, session_id=f"session_{i}")
            analyzer.record_event("auth_check", ts + timedelta(seconds=1), f"session_{i}")
            analyzer.record_event("dashboard", ts + timedelta(seconds=2), f"session_{i}")

        # Check transition matrix
        matrix = analyzer.get_transition_matrix()
        assert "login" in matrix
        assert matrix["login"]["auth_check"] > 0.9

    def test_unexpected_transition(self):
        """Test detection of unexpected transitions."""
        analyzer = SequenceAnalyzer(min_transition_count=5)

        # Build normal sequence pattern
        base_time = datetime.utcnow()
        for i in range(20):
            session = f"session_{i}"
            ts = base_time + timedelta(seconds=i * 5)
            analyzer.record_event("start", ts, session)
            analyzer.record_event("process", ts + timedelta(seconds=1), session)
            analyzer.record_event("complete", ts + timedelta(seconds=2), session)

        # Introduce unexpected transition
        session = "anomaly_session"
        ts = datetime.utcnow()
        analyzer.record_event("start", ts, session)
        anomalies = analyzer.record_event("error", ts + timedelta(seconds=1), session)

        # Should detect unexpected transition
        assert len(anomalies) > 0
        assert any(a.anomaly_type == SequenceAnomalyType.UNEXPECTED_TRANSITION for a in anomalies)

    def test_state_violation(self):
        """Test detection of state machine violations."""
        analyzer = SequenceAnalyzer()

        # Define state rules
        analyzer.add_state_rule("checkout_start", {"payment", "cart_update"})
        analyzer.add_state_rule("payment", {"payment_success", "payment_failed"})

        # Valid sequence
        ts = datetime.utcnow()
        anomalies1 = analyzer.record_event("checkout_start", ts, "valid_session")
        anomalies2 = analyzer.record_event("payment", ts + timedelta(seconds=1), "valid_session")
        assert len(anomalies1) == 0
        assert len(anomalies2) == 0

        # Invalid sequence (violates state rule)
        ts2 = datetime.utcnow()
        analyzer.record_event("checkout_start", ts2, "invalid_session")
        anomalies3 = analyzer.record_event("dashboard", ts2 + timedelta(seconds=1), "invalid_session")

        assert len(anomalies3) > 0
        assert any(a.anomaly_type == SequenceAnomalyType.STATE_VIOLATION for a in anomalies3)

    def test_likely_next_prediction(self):
        """Test prediction of likely next patterns."""
        analyzer = SequenceAnalyzer()

        # Build sequence pattern
        base_time = datetime.utcnow()
        for i in range(50):
            session = f"session_{i}"
            ts = base_time + timedelta(seconds=i * 5)
            analyzer.record_event("A", ts, session)
            analyzer.record_event("B", ts + timedelta(seconds=1), session)

        likely = analyzer.get_likely_next("A", top_k=3)
        assert len(likely) > 0
        assert likely[0][0] == "B"
        assert likely[0][1] > 0.9

    def test_sequence_probability(self):
        """Test sequence probability calculation."""
        analyzer = SequenceAnalyzer()

        # Build patterns
        base_time = datetime.utcnow()
        for i in range(100):
            session = f"session_{i}"
            ts = base_time + timedelta(seconds=i * 5)
            analyzer.record_event("A", ts, session)
            analyzer.record_event("B", ts + timedelta(seconds=1), session)
            analyzer.record_event("C", ts + timedelta(seconds=2), session)

        # Common sequence should have high probability
        common_prob = analyzer.get_sequence_probability(["A", "B", "C"])
        assert common_prob > 0.5

        # Rare sequence should have low probability
        rare_prob = analyzer.get_sequence_probability(["A", "X", "Y"])
        assert rare_prob < 0.1


class TestLogAnomalyDetector:
    """Tests for the main log anomaly detector."""

    def test_new_pattern_detection(self):
        """Test detection of new log patterns."""
        detector = LogAnomalyDetector(service_name="test-service")

        # First occurrence should be flagged as new
        anomalies = detector.analyze(
            "New error: Connection refused to database server",
            severity="ERROR",
        )

        assert len(anomalies) > 0
        assert any(a.anomaly_type == AnomalyType.NEW_PATTERN for a in anomalies)

    def test_sensitive_data_detection(self):
        """Test detection of sensitive data in logs."""
        detector = LogAnomalyDetector(service_name="test-service")

        # Log with password
        anomalies = detector.analyze(
            "Login attempt with password=secret123 failed",
            severity="WARN",
        )

        assert any(a.anomaly_type == AnomalyType.SENSITIVE_DATA for a in anomalies)

        # Log with API key
        anomalies2 = detector.analyze(
            "API call with api_key=sk-abc123xyz789",
            severity="INFO",
        )

        assert any(a.anomaly_type == AnomalyType.SENSITIVE_DATA for a in anomalies2)

    def test_batch_analysis(self):
        """Test batch log analysis."""
        detector = LogAnomalyDetector(service_name="test-service")

        logs = [
            {"message": "User login successful", "severity": "INFO"},
            {"message": "Database query executed", "severity": "INFO"},
            {"message": "Critical error: Out of memory", "severity": "FATAL"},
            {"message": "User login successful", "severity": "INFO"},
            {"message": "User login successful", "severity": "INFO"},
        ]

        result = detector.analyze_batch(logs)

        assert result.patterns_analyzed == 5
        assert result.new_patterns_count >= 2  # At least login and error
        assert len(result.anomalies) > 0

    def test_error_pattern_tracking(self):
        """Test tracking of error-prone patterns."""
        detector = LogAnomalyDetector(service_name="test-service")

        # Generate logs with mixed severities
        for _ in range(10):
            detector.analyze("Request processed successfully", severity="INFO")

        for _ in range(15):
            detector.analyze("Request failed with error", severity="ERROR")

        error_patterns = detector.get_error_patterns()

        assert len(error_patterns) > 0
        assert error_patterns[0]["error_count"] > 0

    def test_statistics(self):
        """Test statistics collection."""
        detector = LogAnomalyDetector(service_name="test-service")

        for i in range(100):
            detector.analyze(f"Log message variant {i % 5}")

        stats = detector.get_statistics()

        assert stats["total_logs_analyzed"] == 100
        assert stats["pattern_stats"]["unique_patterns"] == 5
        assert stats["pattern_stats"]["compression_ratio"] == 20

    def test_rare_pattern_detection(self):
        """Test detection of rare patterns."""
        detector = LogAnomalyDetector(
            service_name="test-service",
            rare_pattern_threshold=5,
        )

        # Create a common pattern
        for _ in range(50):
            detector.analyze("Common log message")

        # Create a rare pattern
        anomalies = detector.analyze("Extremely rare event occurred")

        # The rare pattern should be flagged
        assert any(a.anomaly_type == AnomalyType.NEW_PATTERN for a in anomalies)


class TestIntegration:
    """Integration tests for the complete log anomaly detection system."""

    def test_full_pipeline(self):
        """Test the complete detection pipeline."""
        detector = LogAnomalyDetector(
            service_name="api-server",
            enable_pattern_detection=True,
            enable_frequency_detection=True,
            enable_sequence_detection=True,
            enable_content_detection=True,
        )

        # Simulate realistic log stream
        logs = []
        base_time = datetime.utcnow()

        # Normal operation logs
        for i in range(100):
            logs.append({
                "message": f"Request {i} processed in {50 + i % 30}ms",
                "timestamp": base_time + timedelta(seconds=i),
                "severity": "INFO",
            })

        # Inject some anomalies
        logs.append({
            "message": "CRITICAL: Database connection lost",
            "timestamp": base_time + timedelta(seconds=101),
            "severity": "CRITICAL",
        })

        logs.append({
            "message": "Error: password=admin123 authentication failed",
            "timestamp": base_time + timedelta(seconds=102),
            "severity": "ERROR",
        })

        # Analyze
        result = detector.analyze_batch(logs, session_id="test-session")

        # Should detect anomalies
        assert result.total_anomalies > 0

        # Should detect sensitive data
        sensitive = [a for a in result.anomalies if a.anomaly_type == AnomalyType.SENSITIVE_DATA]
        assert len(sensitive) > 0

    def test_session_tracking(self):
        """Test sequence tracking across sessions."""
        detector = LogAnomalyDetector(service_name="web-app")

        # Simulate user session
        session_id = "user-session-123"
        base_time = datetime.utcnow()

        events = [
            "User navigated to homepage",
            "User clicked login button",
            "Authentication request received",
            "User authenticated successfully",
            "User accessed dashboard",
        ]

        for i, event in enumerate(events):
            detector.analyze(
                event,
                timestamp=base_time + timedelta(seconds=i),
                session_id=session_id,
            )

        # Check session analysis
        analysis = detector.sequence_analyzer.analyze_session(session_id)
        assert analysis["event_count"] == 5

    def test_pattern_evolution(self):
        """Test that the detector adapts to evolving patterns."""
        detector = LogAnomalyDetector(service_name="evolving-app")

        # Phase 1: Establish baseline
        for _ in range(50):
            detector.analyze("Standard operation log v1")

        # Phase 2: New pattern emerges
        anomalies = []
        for _ in range(20):
            result = detector.analyze("Standard operation log v2")
            anomalies.extend(result)

        # First occurrence of v2 should be new
        new_pattern_count = sum(1 for a in anomalies if a.anomaly_type == AnomalyType.NEW_PATTERN)
        assert new_pattern_count >= 1

        # Get patterns
        patterns = detector.get_top_patterns(10)
        assert len(patterns) >= 2


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
