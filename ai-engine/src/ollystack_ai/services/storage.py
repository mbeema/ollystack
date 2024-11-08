"""
Storage Service

ClickHouse client for querying observability data.
"""

import logging
import os
from typing import Any, Optional

logger = logging.getLogger(__name__)


class StorageService:
    """
    ClickHouse storage service for observability data.

    Provides async query execution with connection pooling.
    """

    def __init__(
        self,
        host: Optional[str] = None,
        port: Optional[int] = None,
        database: str = "ollystack",
        username: Optional[str] = None,
        password: Optional[str] = None,
    ):
        self.host = host or os.getenv("CLICKHOUSE_HOST", "localhost")
        self.port = port or int(os.getenv("CLICKHOUSE_PORT", "8123"))
        self.database = database or os.getenv("CLICKHOUSE_DATABASE", "ollystack")
        self.username = username or os.getenv("CLICKHOUSE_USER", "ollystack")
        self.password = password or os.getenv("CLICKHOUSE_PASSWORD", "ollystack123")
        self._client = None

    async def connect(self) -> None:
        """Connect to ClickHouse."""
        try:
            import clickhouse_connect

            self._client = await clickhouse_connect.get_async_client(
                host=self.host,
                port=self.port,
                database=self.database,
                username=self.username,
                password=self.password,
            )
            logger.info(f"Connected to ClickHouse at {self.host}:{self.port}")

        except ImportError:
            logger.warning("clickhouse-connect not installed, using mock client")
            self._client = MockClickHouseClient()

        except Exception as e:
            logger.warning(f"Failed to connect to ClickHouse: {e}, using mock client")
            self._client = MockClickHouseClient()

    async def disconnect(self) -> None:
        """Disconnect from ClickHouse."""
        if self._client and hasattr(self._client, "close"):
            await self._client.close()
        self._client = None

    async def execute_query(
        self,
        query: str,
        parameters: Optional[dict] = None,
        timeout: int = 30,
    ) -> tuple[list[tuple], list[str]]:
        """
        Execute a query and return results.

        Args:
            query: SQL query to execute
            parameters: Query parameters
            timeout: Query timeout in seconds

        Returns:
            Tuple of (rows, column_names)
        """
        if self._client is None:
            await self.connect()

        try:
            if hasattr(self._client, "query"):
                # Real ClickHouse client
                result = await self._client.query(
                    query,
                    parameters=parameters,
                    settings={"max_execution_time": timeout},
                )
                return result.result_rows, result.column_names
            else:
                # Mock client
                return await self._client.execute(query, parameters)

        except Exception as e:
            logger.error(f"Query execution failed: {e}")
            logger.debug(f"Query: {query}")
            return [], []

    async def insert(
        self,
        table: str,
        data: list[dict],
        column_names: Optional[list[str]] = None,
    ) -> None:
        """
        Insert data into a table.

        Args:
            table: Table name
            data: List of row dictionaries
            column_names: Optional column names
        """
        if not data:
            return

        if self._client is None:
            await self.connect()

        try:
            if column_names is None:
                column_names = list(data[0].keys())

            rows = [[row.get(col) for col in column_names] for row in data]

            if hasattr(self._client, "insert"):
                await self._client.insert(
                    table,
                    rows,
                    column_names=column_names,
                )
            else:
                # Mock client
                logger.debug(f"Mock insert into {table}: {len(data)} rows")

        except Exception as e:
            logger.error(f"Insert failed: {e}")


class MockClickHouseClient:
    """Mock ClickHouse client for development/testing."""

    async def execute(
        self,
        query: str,
        parameters: Optional[dict] = None,
    ) -> tuple[list[dict], list[str]]:
        """Execute a mock query."""
        logger.debug(f"Mock query: {query[:100]}...")

        # Return empty results for most queries
        query_upper = query.upper()

        if "FROM OTEL_TRACES" in query_upper:
            return self._mock_traces(), [
                "Timestamp",
                "TraceId",
                "SpanId",
                "ServiceName",
                "SpanName",
                "Duration",
                "StatusCode",
            ]
        elif "FROM OTEL_METRICS" in query_upper:
            return self._mock_metrics(), [
                "Timestamp",
                "MetricName",
                "ServiceName",
                "Value",
                "AnomalyScore",
            ]
        elif "FROM OTEL_LOGS" in query_upper:
            return self._mock_logs(), [
                "Timestamp",
                "ServiceName",
                "SeverityText",
                "Body",
            ]
        elif "FROM SERVICE_TOPOLOGY" in query_upper:
            return self._mock_topology(), [
                "SourceService",
                "TargetService",
                "RequestCount",
                "ErrorRate",
            ]

        return [], []

    def _mock_traces(self) -> list[dict]:
        """Generate mock trace data."""
        from datetime import datetime, timedelta
        import random

        services = ["api-gateway", "user-service", "order-service", "payment-service"]
        operations = ["GET /api/users", "POST /api/orders", "GET /api/products"]

        traces = []
        for i in range(10):
            traces.append({
                "Timestamp": datetime.utcnow() - timedelta(minutes=i),
                "TraceId": f"trace-{i:08d}",
                "SpanId": f"span-{i:08d}",
                "ServiceName": random.choice(services),
                "SpanName": random.choice(operations),
                "Duration": random.randint(100000000, 2000000000),
                "StatusCode": random.choice(["OK", "OK", "OK", "ERROR"]),
            })
        return traces

    def _mock_metrics(self) -> list[dict]:
        """Generate mock metric data."""
        from datetime import datetime, timedelta
        import random

        metrics = []
        for i in range(10):
            metrics.append({
                "Timestamp": datetime.utcnow() - timedelta(minutes=i),
                "MetricName": "http_request_duration_seconds",
                "ServiceName": "api-gateway",
                "Value": random.uniform(0.1, 2.0),
                "AnomalyScore": random.uniform(0, 1),
            })
        return metrics

    def _mock_logs(self) -> list[dict]:
        """Generate mock log data."""
        from datetime import datetime, timedelta

        return [
            {
                "Timestamp": datetime.utcnow() - timedelta(minutes=i),
                "ServiceName": "api-gateway",
                "SeverityText": "ERROR" if i % 3 == 0 else "INFO",
                "Body": f"Sample log message {i}",
            }
            for i in range(10)
        ]

    def _mock_topology(self) -> list[dict]:
        """Generate mock topology data."""
        return [
            {
                "SourceService": "api-gateway",
                "TargetService": "user-service",
                "RequestCount": 1000,
                "ErrorRate": 0.02,
            },
            {
                "SourceService": "api-gateway",
                "TargetService": "order-service",
                "RequestCount": 500,
                "ErrorRate": 0.05,
            },
            {
                "SourceService": "order-service",
                "TargetService": "payment-service",
                "RequestCount": 500,
                "ErrorRate": 0.01,
            },
        ]
