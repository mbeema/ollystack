"""
Query Executor

Executes ObservQL/SQL queries against ClickHouse storage.
"""

import logging
import time
from dataclasses import dataclass
from typing import Any, Optional

from ollystack_ai.services.storage import StorageService

logger = logging.getLogger(__name__)


@dataclass
class QueryResult:
    """Result of query execution."""

    data: dict[str, Any]
    row_count: int
    execution_time_ms: float
    truncated: bool = False
    error: Optional[str] = None


class QueryExecutor:
    """Executes queries against the observability storage."""

    def __init__(
        self,
        storage_service: StorageService,
        max_rows: int = 1000,
        timeout_seconds: int = 30,
    ):
        self.storage = storage_service
        self.max_rows = max_rows
        self.timeout = timeout_seconds

    async def execute(self, query: str) -> QueryResult:
        """
        Execute a query and return results.

        Args:
            query: SQL query to execute

        Returns:
            QueryResult with data and metadata
        """
        start_time = time.time()

        try:
            # Validate and sanitize query
            query = self._sanitize_query(query)

            # Add LIMIT if not present
            if "LIMIT" not in query.upper():
                query = f"{query} LIMIT {self.max_rows}"

            # Execute query
            rows, columns = await self.storage.execute_query(
                query,
                timeout=self.timeout,
            )

            execution_time_ms = (time.time() - start_time) * 1000

            # Check if truncated
            truncated = len(rows) >= self.max_rows

            # Format results
            data = self._format_results(rows, columns)

            return QueryResult(
                data=data,
                row_count=len(rows),
                execution_time_ms=execution_time_ms,
                truncated=truncated,
            )

        except Exception as e:
            logger.exception(f"Query execution failed: {query[:100]}...")
            execution_time_ms = (time.time() - start_time) * 1000
            return QueryResult(
                data={},
                row_count=0,
                execution_time_ms=execution_time_ms,
                error=str(e),
            )

    def _sanitize_query(self, query: str) -> str:
        """
        Sanitize query to prevent dangerous operations.

        Only SELECT queries are allowed.
        """
        query = query.strip()

        # Remove trailing semicolon
        if query.endswith(";"):
            query = query[:-1]

        # Check for dangerous operations
        upper_query = query.upper()
        dangerous_keywords = [
            "DROP",
            "DELETE",
            "TRUNCATE",
            "INSERT",
            "UPDATE",
            "ALTER",
            "CREATE",
            "GRANT",
            "REVOKE",
        ]

        for keyword in dangerous_keywords:
            if keyword in upper_query:
                raise ValueError(f"Query contains disallowed keyword: {keyword}")

        # Must start with SELECT or WITH
        if not (upper_query.startswith("SELECT") or upper_query.startswith("WITH")):
            raise ValueError("Only SELECT queries are allowed")

        return query

    def _format_results(
        self,
        rows: list[tuple],
        columns: list[str],
    ) -> dict[str, Any]:
        """Format query results as a dictionary."""
        # Convert to list of dicts
        results = []
        for row in rows:
            row_dict = {}
            for i, col in enumerate(columns):
                value = row[i]
                # Handle special types
                if hasattr(value, "isoformat"):
                    value = value.isoformat()
                elif isinstance(value, bytes):
                    value = value.decode("utf-8", errors="replace")
                row_dict[col] = value
            results.append(row_dict)

        return {
            "columns": columns,
            "rows": results,
        }

    async def get_available_services(self) -> list[str]:
        """Get list of available services."""
        query = """
        SELECT DISTINCT ServiceName
        FROM otel_traces
        WHERE Timestamp >= now() - INTERVAL 1 DAY
        ORDER BY ServiceName
        """
        result = await self.execute(query)
        if result.error:
            return []
        return [row["ServiceName"] for row in result.data.get("rows", [])]

    async def get_available_metrics(self) -> list[str]:
        """Get list of available metric names."""
        query = """
        SELECT DISTINCT MetricName
        FROM otel_metrics
        WHERE Timestamp >= now() - INTERVAL 1 DAY
        ORDER BY MetricName
        """
        result = await self.execute(query)
        if result.error:
            return []
        return [row["MetricName"] for row in result.data.get("rows", [])]


class QueryValidator:
    """Validates queries before execution."""

    # Allowed functions in queries
    ALLOWED_FUNCTIONS = {
        "count",
        "sum",
        "avg",
        "min",
        "max",
        "quantile",
        "quantiles",
        "uniq",
        "uniqExact",
        "groupArray",
        "arrayJoin",
        "toStartOfMinute",
        "toStartOfHour",
        "toStartOfDay",
        "toDate",
        "toDateTime",
        "now",
        "today",
        "yesterday",
        "dateDiff",
        "formatDateTime",
        "if",
        "multiIf",
        "coalesce",
        "nullIf",
        "assumeNotNull",
        "isNull",
        "isNotNull",
        "lower",
        "upper",
        "trim",
        "concat",
        "substring",
        "length",
        "position",
        "match",
        "extract",
        "replaceAll",
        "splitByString",
        "round",
        "floor",
        "ceil",
        "abs",
    }

    @classmethod
    def validate(cls, query: str) -> tuple[bool, Optional[str]]:
        """
        Validate a query.

        Returns:
            Tuple of (is_valid, error_message)
        """
        upper_query = query.upper()

        # Check for SELECT
        if not upper_query.strip().startswith(("SELECT", "WITH")):
            return False, "Query must be a SELECT statement"

        # Check for dangerous patterns
        dangerous_patterns = [
            ("INTO OUTFILE", "File output not allowed"),
            ("INTO FILE", "File output not allowed"),
            ("SYSTEM", "System commands not allowed"),
            ("; DROP", "Multiple statements not allowed"),
            ("; DELETE", "Multiple statements not allowed"),
        ]

        for pattern, message in dangerous_patterns:
            if pattern in upper_query:
                return False, message

        return True, None
