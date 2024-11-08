"""
Investigation Triggers

Automatic triggers that start investigations when conditions are met.
Runs as a background task checking for anomalies, alerts, and SLO breaches.
"""

import logging
import asyncio
from datetime import datetime, timedelta
from typing import Optional, Callable, Awaitable
import re

from ollystack_ai.services.storage import StorageService
from ollystack_ai.services.cache import CacheService
from ollystack_ai.investigations.models import (
    TriggerType,
    InvestigationTriggerConfig,
)
from ollystack_ai.investigations.engine import InvestigationEngine

logger = logging.getLogger(__name__)


class InvestigationTrigger:
    """
    Monitors for conditions that should trigger automatic investigations.

    Checks for:
    - Anomaly score spikes
    - Error rate increases
    - Latency spikes
    - SLO breaches
    - Alert firing
    """

    def __init__(
        self,
        engine: InvestigationEngine,
        storage: StorageService,
        cache: Optional[CacheService] = None,
        check_interval: int = 60,  # Check every 60 seconds
    ):
        self.engine = engine
        self.storage = storage
        self.cache = cache
        self.check_interval = check_interval
        self._running = False
        self._task: Optional[asyncio.Task] = None
        self._triggers: dict[str, InvestigationTriggerConfig] = {}
        self._recent_investigations: dict[str, datetime] = {}  # Prevent duplicate investigations
        self._cooldown_minutes = 15  # Don't re-trigger for same service within this window

        # Initialize default triggers
        self._init_default_triggers()

    def _init_default_triggers(self) -> None:
        """Initialize default trigger configurations."""
        default_triggers = [
            InvestigationTriggerConfig(
                id="default-anomaly-high",
                name="High Anomaly Score",
                description="Triggers when anomaly score exceeds 0.9",
                trigger_type=TriggerType.ANOMALY,
                threshold=0.9,
                operator="gt",
                duration="2m",
                auto_start=True,
                investigation_window="30m",
                priority="high",
            ),
            InvestigationTriggerConfig(
                id="default-error-spike",
                name="Error Rate Spike",
                description="Triggers when error rate exceeds 10%",
                trigger_type=TriggerType.ERROR_SPIKE,
                threshold=0.1,
                operator="gt",
                duration="5m",
                auto_start=True,
                investigation_window="1h",
                priority="high",
            ),
            InvestigationTriggerConfig(
                id="default-latency-spike",
                name="Latency Spike",
                description="Triggers when p99 latency exceeds 5 seconds",
                trigger_type=TriggerType.LATENCY_SPIKE,
                threshold=5000,  # milliseconds
                operator="gt",
                duration="5m",
                auto_start=True,
                investigation_window="1h",
                priority="normal",
            ),
            InvestigationTriggerConfig(
                id="default-slo-breach",
                name="SLO Breach",
                description="Triggers when SLO error budget is exhausted",
                trigger_type=TriggerType.SLO_BREACH,
                threshold=0,  # Error budget remaining
                operator="lte",
                duration="1m",
                auto_start=True,
                investigation_window="2h",
                priority="high",
            ),
        ]

        for trigger in default_triggers:
            self._triggers[trigger.id] = trigger

    async def start(self) -> None:
        """Start the trigger monitoring loop."""
        if self._running:
            return

        self._running = True
        self._task = asyncio.create_task(self._monitoring_loop())
        logger.info("Investigation trigger monitoring started")

    async def stop(self) -> None:
        """Stop the trigger monitoring loop."""
        self._running = False
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
        logger.info("Investigation trigger monitoring stopped")

    async def _monitoring_loop(self) -> None:
        """Main monitoring loop."""
        while self._running:
            try:
                await self._check_all_triggers()
            except Exception as e:
                logger.exception(f"Error in trigger monitoring loop: {e}")

            await asyncio.sleep(self.check_interval)

    async def _check_all_triggers(self) -> None:
        """Check all enabled triggers."""
        for trigger_id, trigger in self._triggers.items():
            if not trigger.enabled:
                continue

            try:
                await self._check_trigger(trigger)
            except Exception as e:
                logger.error(f"Error checking trigger {trigger_id}: {e}")

    async def _check_trigger(self, trigger: InvestigationTriggerConfig) -> None:
        """Check a single trigger and start investigation if conditions are met."""
        if trigger.trigger_type == TriggerType.ANOMALY:
            await self._check_anomaly_trigger(trigger)
        elif trigger.trigger_type == TriggerType.ERROR_SPIKE:
            await self._check_error_spike_trigger(trigger)
        elif trigger.trigger_type == TriggerType.LATENCY_SPIKE:
            await self._check_latency_spike_trigger(trigger)
        elif trigger.trigger_type == TriggerType.SLO_BREACH:
            await self._check_slo_breach_trigger(trigger)
        elif trigger.trigger_type == TriggerType.ALERT:
            await self._check_alert_trigger(trigger)

    async def _check_anomaly_trigger(self, trigger: InvestigationTriggerConfig) -> None:
        """Check for high anomaly scores."""
        service_filter = ""
        if trigger.service_filter:
            service_filter = f"AND match(ServiceName, '{trigger.service_filter}')"

        query = f"""
        SELECT
            ServiceName,
            max(AnomalyScore) as max_score,
            count() as anomaly_count
        FROM otel_metrics
        WHERE Timestamp >= now() - INTERVAL 5 MINUTE
          AND AnomalyScore > {trigger.threshold}
          {service_filter}
        GROUP BY ServiceName
        HAVING anomaly_count >= 3
        ORDER BY max_score DESC
        LIMIT 10
        """

        result = await self.storage.execute_query(query)
        rows = result[0] if result else []

        for row in rows:
            service = row.get("ServiceName")
            score = row.get("max_score", 0)

            if self._should_trigger(trigger, service, score):
                await self._start_investigation(
                    trigger=trigger,
                    service_name=service,
                    trigger_id=f"anomaly-{service}-{datetime.utcnow().timestamp()}",
                )

    async def _check_error_spike_trigger(self, trigger: InvestigationTriggerConfig) -> None:
        """Check for error rate spikes."""
        service_filter = ""
        if trigger.service_filter:
            service_filter = f"AND match(ServiceName, '{trigger.service_filter}')"

        query = f"""
        SELECT
            ServiceName,
            count() as total,
            countIf(StatusCode = 'ERROR') as errors,
            countIf(StatusCode = 'ERROR') / count() as error_rate
        FROM otel_traces
        WHERE Timestamp >= now() - INTERVAL 5 MINUTE
          AND IsRootSpan = 1
          {service_filter}
        GROUP BY ServiceName
        HAVING total >= 10 AND error_rate > {trigger.threshold}
        ORDER BY error_rate DESC
        LIMIT 10
        """

        result = await self.storage.execute_query(query)
        rows = result[0] if result else []

        for row in rows:
            service = row.get("ServiceName")
            error_rate = row.get("error_rate", 0)

            if self._should_trigger(trigger, service, error_rate):
                await self._start_investigation(
                    trigger=trigger,
                    service_name=service,
                    trigger_id=f"error-spike-{service}-{datetime.utcnow().timestamp()}",
                )

    async def _check_latency_spike_trigger(self, trigger: InvestigationTriggerConfig) -> None:
        """Check for latency spikes."""
        service_filter = ""
        if trigger.service_filter:
            service_filter = f"AND match(ServiceName, '{trigger.service_filter}')"

        # Threshold is in milliseconds, Duration is in nanoseconds
        threshold_ns = trigger.threshold * 1000000

        query = f"""
        SELECT
            ServiceName,
            quantile(0.99)(Duration) / 1000000 as p99_ms,
            count() as request_count
        FROM otel_traces
        WHERE Timestamp >= now() - INTERVAL 5 MINUTE
          AND IsRootSpan = 1
          {service_filter}
        GROUP BY ServiceName
        HAVING request_count >= 10 AND p99_ms > {trigger.threshold}
        ORDER BY p99_ms DESC
        LIMIT 10
        """

        result = await self.storage.execute_query(query)
        rows = result[0] if result else []

        for row in rows:
            service = row.get("ServiceName")
            p99 = row.get("p99_ms", 0)

            if self._should_trigger(trigger, service, p99):
                await self._start_investigation(
                    trigger=trigger,
                    service_name=service,
                    trigger_id=f"latency-spike-{service}-{datetime.utcnow().timestamp()}",
                )

    async def _check_slo_breach_trigger(self, trigger: InvestigationTriggerConfig) -> None:
        """Check for SLO breaches."""
        query = """
        SELECT
            SLOId,
            ServiceName,
            ErrorBudgetRemainingPercent
        FROM slo_status
        WHERE ErrorBudgetRemainingPercent <= 0
        """

        try:
            result = await self.storage.execute_query(query)
            rows = result[0] if result else []

            for row in rows:
                slo_id = row.get("SLOId")
                service = row.get("ServiceName")

                if self._should_trigger(trigger, service, 0):
                    await self._start_investigation(
                        trigger=trigger,
                        service_name=service,
                        trigger_id=f"slo-breach-{slo_id}",
                    )
        except Exception:
            # SLO tables might not exist yet
            pass

    async def _check_alert_trigger(self, trigger: InvestigationTriggerConfig) -> None:
        """Check for firing alerts."""
        query = """
        SELECT
            AlertId,
            AlertName,
            ServiceName,
            Severity
        FROM alerts
        WHERE Status = 'firing'
          AND Timestamp >= now() - INTERVAL 5 MINUTE
        ORDER BY Timestamp DESC
        LIMIT 10
        """

        try:
            result = await self.storage.execute_query(query)
            rows = result[0] if result else []

            for row in rows:
                alert_id = row.get("AlertId")
                service = row.get("ServiceName")

                if self._should_trigger(trigger, service, 1):
                    await self._start_investigation(
                        trigger=trigger,
                        service_name=service,
                        trigger_id=f"alert-{alert_id}",
                    )
        except Exception:
            pass

    def _should_trigger(
        self,
        trigger: InvestigationTriggerConfig,
        service: str,
        value: float,
    ) -> bool:
        """Check if we should trigger an investigation."""
        # Check operator condition
        if trigger.operator == "gt" and not (value > trigger.threshold):
            return False
        elif trigger.operator == "lt" and not (value < trigger.threshold):
            return False
        elif trigger.operator == "gte" and not (value >= trigger.threshold):
            return False
        elif trigger.operator == "lte" and not (value <= trigger.threshold):
            return False
        elif trigger.operator == "eq" and not (value == trigger.threshold):
            return False

        # Check service filter
        if trigger.service_filter:
            if not re.match(trigger.service_filter, service):
                return False

        # Check cooldown
        key = f"{trigger.id}:{service}"
        last_triggered = self._recent_investigations.get(key)
        if last_triggered:
            elapsed = datetime.utcnow() - last_triggered
            if elapsed < timedelta(minutes=self._cooldown_minutes):
                logger.debug(f"Skipping trigger {trigger.id} for {service} - in cooldown")
                return False

        return True

    async def _start_investigation(
        self,
        trigger: InvestigationTriggerConfig,
        service_name: str,
        trigger_id: str,
    ) -> None:
        """Start an investigation based on trigger."""
        if not trigger.auto_start:
            logger.info(f"Trigger {trigger.id} fired for {service_name} but auto_start is disabled")
            return

        logger.info(f"Starting investigation for {service_name} (trigger: {trigger.name})")

        # Record this trigger to prevent duplicates
        key = f"{trigger.id}:{service_name}"
        self._recent_investigations[key] = datetime.utcnow()

        # Start the investigation
        try:
            investigation = await self.engine.start_investigation(
                trigger_type=trigger.trigger_type,
                trigger_id=trigger_id,
                service_name=service_name,
                investigation_window=trigger.investigation_window,
                created_by="auto-trigger",
            )

            logger.info(f"Investigation {investigation.id} started for {service_name}")

            # TODO: Send notification if configured
            if trigger.notify_on_start and trigger.notify_channels:
                await self._send_notification(trigger, investigation)

        except Exception as e:
            logger.error(f"Failed to start investigation: {e}")

    async def _send_notification(
        self,
        trigger: InvestigationTriggerConfig,
        investigation,
    ) -> None:
        """Send notification about started investigation."""
        # TODO: Implement notification sending (Slack, PagerDuty, etc.)
        logger.info(f"Would notify channels {trigger.notify_channels} about investigation {investigation.id}")

    # Trigger management methods

    def add_trigger(self, trigger: InvestigationTriggerConfig) -> None:
        """Add a new trigger configuration."""
        self._triggers[trigger.id] = trigger
        logger.info(f"Added trigger: {trigger.name}")

    def remove_trigger(self, trigger_id: str) -> bool:
        """Remove a trigger configuration."""
        if trigger_id in self._triggers:
            del self._triggers[trigger_id]
            logger.info(f"Removed trigger: {trigger_id}")
            return True
        return False

    def get_trigger(self, trigger_id: str) -> Optional[InvestigationTriggerConfig]:
        """Get a trigger by ID."""
        return self._triggers.get(trigger_id)

    def list_triggers(self) -> list[InvestigationTriggerConfig]:
        """List all triggers."""
        return list(self._triggers.values())

    def enable_trigger(self, trigger_id: str) -> bool:
        """Enable a trigger."""
        trigger = self._triggers.get(trigger_id)
        if trigger:
            trigger.enabled = True
            return True
        return False

    def disable_trigger(self, trigger_id: str) -> bool:
        """Disable a trigger."""
        trigger = self._triggers.get(trigger_id)
        if trigger:
            trigger.enabled = False
            return True
        return False
