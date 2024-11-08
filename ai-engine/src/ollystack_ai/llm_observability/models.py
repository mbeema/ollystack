"""
LLM Observability Data Models

Data classes for tracking LLM requests, completions, and related events.
"""

from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, Any
from enum import Enum
import uuid


class Provider(str, Enum):
    """LLM Provider."""
    OPENAI = "openai"
    ANTHROPIC = "anthropic"
    COHERE = "cohere"
    AZURE = "azure"
    BEDROCK = "bedrock"
    GOOGLE = "google"
    HUGGINGFACE = "huggingface"
    LOCAL = "local"
    CUSTOM = "custom"


class RequestType(str, Enum):
    """Type of LLM request."""
    COMPLETION = "completion"
    CHAT = "chat"
    EMBEDDING = "embedding"
    FUNCTION_CALL = "function_call"
    AGENT = "agent"


class RequestStatus(str, Enum):
    """Request status."""
    SUCCESS = "success"
    ERROR = "error"
    TIMEOUT = "timeout"
    RATE_LIMITED = "rate_limited"
    CANCELLED = "cancelled"


class FinishReason(str, Enum):
    """Why the completion ended."""
    STOP = "stop"
    LENGTH = "length"
    TOOL_CALLS = "tool_calls"
    CONTENT_FILTER = "content_filter"
    ERROR = "error"


class ChainType(str, Enum):
    """Type of LLM chain/workflow."""
    SEQUENTIAL = "sequential"
    PARALLEL = "parallel"
    AGENT = "agent"
    ROUTER = "router"
    RAG = "rag"
    CUSTOM = "custom"


@dataclass
class TokenUsage:
    """Token usage information."""
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0

    def __post_init__(self):
        if self.total_tokens == 0:
            self.total_tokens = self.prompt_tokens + self.completion_tokens


@dataclass
class CostInfo:
    """Cost information in microdollars (1/1,000,000 USD)."""
    prompt_cost_micros: int = 0
    completion_cost_micros: int = 0
    total_cost_micros: int = 0

    @property
    def prompt_cost_usd(self) -> float:
        return self.prompt_cost_micros / 1_000_000

    @property
    def completion_cost_usd(self) -> float:
        return self.completion_cost_micros / 1_000_000

    @property
    def total_cost_usd(self) -> float:
        return self.total_cost_micros / 1_000_000

    def __post_init__(self):
        if self.total_cost_micros == 0:
            self.total_cost_micros = self.prompt_cost_micros + self.completion_cost_micros


@dataclass
class LLMPrompt:
    """A prompt message."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    request_id: str = ""
    timestamp: datetime = field(default_factory=datetime.utcnow)

    role: str = "user"  # system, user, assistant, function, tool
    content: str = ""
    content_hash: str = ""

    # Template info
    template_id: Optional[str] = None
    template_name: Optional[str] = None
    template_version: Optional[str] = None
    template_variables: dict[str, str] = field(default_factory=dict)

    # Metrics
    token_count: int = 0
    message_index: int = 0

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "request_id": self.request_id,
            "timestamp": self.timestamp.isoformat(),
            "role": self.role,
            "content": self.content[:500] if len(self.content) > 500 else self.content,
            "content_hash": self.content_hash,
            "template_id": self.template_id,
            "template_name": self.template_name,
            "token_count": self.token_count,
            "message_index": self.message_index,
        }


@dataclass
class LLMCompletion:
    """A completion/response."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    request_id: str = ""
    timestamp: datetime = field(default_factory=datetime.utcnow)

    content: str = ""
    content_hash: str = ""
    finish_reason: FinishReason = FinishReason.STOP
    token_count: int = 0
    choice_index: int = 0

    # Quality scores (0-1)
    quality_score: float = 0.0
    relevance_score: float = 0.0
    hallucination_score: float = 0.0
    toxicity_score: float = 0.0

    # User feedback
    user_rating: int = 0  # -1, 0, 1
    user_feedback: str = ""
    was_edited: bool = False
    edited_content: str = ""

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "request_id": self.request_id,
            "timestamp": self.timestamp.isoformat(),
            "content": self.content[:500] if len(self.content) > 500 else self.content,
            "finish_reason": self.finish_reason.value,
            "token_count": self.token_count,
            "choice_index": self.choice_index,
            "quality_score": self.quality_score,
            "relevance_score": self.relevance_score,
            "user_rating": self.user_rating,
        }


@dataclass
class ToolCall:
    """A tool/function call."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    request_id: str = ""
    timestamp: datetime = field(default_factory=datetime.utcnow)

    tool_name: str = ""
    tool_type: str = "function"  # function, retrieval, code_interpreter, custom
    tool_description: str = ""

    input_arguments: str = ""  # JSON
    output_result: str = ""  # JSON
    output_token_count: int = 0

    duration_ms: float = 0.0
    status: RequestStatus = RequestStatus.SUCCESS
    error_message: str = ""

    call_index: int = 0

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "request_id": self.request_id,
            "timestamp": self.timestamp.isoformat(),
            "tool_name": self.tool_name,
            "tool_type": self.tool_type,
            "input_arguments": self.input_arguments[:500] if len(self.input_arguments) > 500 else self.input_arguments,
            "output_result": self.output_result[:500] if len(self.output_result) > 500 else self.output_result,
            "duration_ms": self.duration_ms,
            "status": self.status.value,
            "call_index": self.call_index,
        }


@dataclass
class LLMRequest:
    """An LLM API request."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    timestamp: datetime = field(default_factory=datetime.utcnow)

    # Trace context
    trace_id: str = ""
    span_id: str = ""
    parent_span_id: str = ""

    # Service info
    service_name: str = ""
    environment: str = "production"
    version: str = ""

    # Provider/model info
    provider: Provider = Provider.OPENAI
    model: str = ""
    model_version: str = ""
    endpoint: str = ""

    # Request type
    request_type: RequestType = RequestType.CHAT

    # Token usage
    tokens: TokenUsage = field(default_factory=TokenUsage)

    # Cost
    cost: CostInfo = field(default_factory=CostInfo)

    # Timing
    duration_ms: float = 0.0
    time_to_first_token_ms: float = 0.0
    tokens_per_second: float = 0.0

    # Request parameters
    temperature: float = 1.0
    max_tokens: int = 0
    top_p: float = 1.0
    frequency_penalty: float = 0.0
    presence_penalty: float = 0.0
    stop_sequences: list[str] = field(default_factory=list)

    # Status
    status: RequestStatus = RequestStatus.SUCCESS
    status_code: int = 200
    error_message: str = ""
    error_type: str = ""

    # Streaming
    is_streaming: bool = False
    stream_chunks: int = 0

    # Tool calls
    has_tool_calls: bool = False
    tool_call_count: int = 0
    tool_names: list[str] = field(default_factory=list)

    # RAG
    has_rag_context: bool = False
    rag_chunk_count: int = 0
    rag_retrieval_time_ms: float = 0.0

    # Quality (from evaluation)
    quality_score: float = 0.0
    relevance_score: float = 0.0
    coherence_score: float = 0.0
    factuality_score: float = 0.0

    # Safety
    contains_pii: bool = False
    safety_flagged: bool = False
    safety_categories: list[str] = field(default_factory=list)

    # User context
    user_id: str = ""
    session_id: str = ""
    conversation_id: str = ""

    # Prompts and completions
    prompts: list[LLMPrompt] = field(default_factory=list)
    completions: list[LLMCompletion] = field(default_factory=list)
    tool_calls: list[ToolCall] = field(default_factory=list)

    # Metadata
    labels: dict[str, str] = field(default_factory=dict)
    organization: str = ""

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "timestamp": self.timestamp.isoformat(),
            "trace_id": self.trace_id,
            "span_id": self.span_id,
            "service_name": self.service_name,
            "environment": self.environment,
            "provider": self.provider.value,
            "model": self.model,
            "request_type": self.request_type.value,
            "tokens": {
                "prompt": self.tokens.prompt_tokens,
                "completion": self.tokens.completion_tokens,
                "total": self.tokens.total_tokens,
            },
            "cost_usd": self.cost.total_cost_usd,
            "duration_ms": self.duration_ms,
            "status": self.status.value,
            "is_streaming": self.is_streaming,
            "has_tool_calls": self.has_tool_calls,
            "has_rag_context": self.has_rag_context,
            "quality_score": self.quality_score,
            "safety_flagged": self.safety_flagged,
            "labels": self.labels,
        }


@dataclass
class Embedding:
    """An embedding request."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    timestamp: datetime = field(default_factory=datetime.utcnow)

    # Trace context
    trace_id: str = ""
    span_id: str = ""

    # Service
    service_name: str = ""
    environment: str = "production"

    # Provider
    provider: Provider = Provider.OPENAI
    model: str = ""

    # Request
    input_type: str = "text"  # text, document, query, image
    input_count: int = 0
    total_tokens: int = 0
    dimensions: int = 0

    # Timing & cost
    duration_ms: float = 0.0
    cost_micros: int = 0

    # Status
    status: RequestStatus = RequestStatus.SUCCESS
    error_message: str = ""

    # Context
    use_case: str = ""  # rag_indexing, rag_query, similarity_search, clustering
    collection_name: str = ""

    # Metadata
    labels: dict[str, str] = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "timestamp": self.timestamp.isoformat(),
            "trace_id": self.trace_id,
            "service_name": self.service_name,
            "provider": self.provider.value,
            "model": self.model,
            "input_count": self.input_count,
            "total_tokens": self.total_tokens,
            "dimensions": self.dimensions,
            "duration_ms": self.duration_ms,
            "status": self.status.value,
            "use_case": self.use_case,
        }


@dataclass
class RAGRetrieval:
    """A RAG retrieval operation."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    request_id: str = ""
    timestamp: datetime = field(default_factory=datetime.utcnow)

    # Service
    service_name: str = ""

    # Query
    query: str = ""
    query_tokens: int = 0
    query_embedding_time_ms: float = 0.0

    # Vector search
    vector_store: str = ""  # pinecone, weaviate, chroma, milvus, pgvector
    collection_name: str = ""
    search_type: str = "similarity"  # similarity, mmr, hybrid
    top_k: int = 0
    search_time_ms: float = 0.0

    # Results
    result_count: int = 0
    total_chunk_tokens: int = 0
    avg_similarity_score: float = 0.0
    min_similarity_score: float = 0.0
    max_similarity_score: float = 0.0

    # Reranking
    reranker_used: bool = False
    reranker_model: str = ""
    rerank_time_ms: float = 0.0

    # Filters
    metadata_filters: str = ""  # JSON

    # Quality
    retrieval_quality_score: float = 0.0

    # Retrieved chunks
    chunks: list[dict] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "request_id": self.request_id,
            "timestamp": self.timestamp.isoformat(),
            "query": self.query[:200] if len(self.query) > 200 else self.query,
            "vector_store": self.vector_store,
            "search_type": self.search_type,
            "top_k": self.top_k,
            "search_time_ms": self.search_time_ms,
            "result_count": self.result_count,
            "avg_similarity_score": self.avg_similarity_score,
            "reranker_used": self.reranker_used,
        }


@dataclass
class Chain:
    """An LLM chain/workflow execution."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    timestamp: datetime = field(default_factory=datetime.utcnow)

    # Trace context
    trace_id: str = ""
    root_span_id: str = ""

    # Service
    service_name: str = ""
    environment: str = "production"

    # Chain info
    chain_type: ChainType = ChainType.SEQUENTIAL
    chain_name: str = ""

    # Execution
    step_count: int = 0
    total_duration_ms: float = 0.0
    llm_call_count: int = 0
    tool_call_count: int = 0
    retrieval_count: int = 0

    # Tokens & cost
    total_prompt_tokens: int = 0
    total_completion_tokens: int = 0
    total_cost_micros: int = 0

    # Status
    status: RequestStatus = RequestStatus.SUCCESS
    error_message: str = ""
    error_step: int = 0

    # Input/Output
    input: str = ""
    output: str = ""

    # User context
    user_id: str = ""
    session_id: str = ""
    conversation_id: str = ""

    # Steps
    steps: list["ChainStep"] = field(default_factory=list)

    # Metadata
    labels: dict[str, str] = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "timestamp": self.timestamp.isoformat(),
            "trace_id": self.trace_id,
            "service_name": self.service_name,
            "chain_type": self.chain_type.value,
            "chain_name": self.chain_name,
            "step_count": self.step_count,
            "total_duration_ms": self.total_duration_ms,
            "llm_call_count": self.llm_call_count,
            "tool_call_count": self.tool_call_count,
            "total_tokens": self.total_prompt_tokens + self.total_completion_tokens,
            "cost_usd": self.total_cost_micros / 1_000_000,
            "status": self.status.value,
        }


@dataclass
class ChainStep:
    """A step in an LLM chain."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    chain_id: str = ""
    step_index: int = 0
    step_name: str = ""
    step_type: str = ""  # llm, tool, retrieval, transform, condition

    # Execution
    duration_ms: float = 0.0
    status: RequestStatus = RequestStatus.SUCCESS
    error_message: str = ""

    # Input/Output
    input: str = ""
    output: str = ""

    # References
    llm_request_id: Optional[str] = None
    tool_call_id: Optional[str] = None
    retrieval_id: Optional[str] = None


@dataclass
class Evaluation:
    """An evaluation result."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    timestamp: datetime = field(default_factory=datetime.utcnow)

    # Reference
    request_id: str = ""
    completion_id: str = ""
    chain_id: str = ""

    # Service
    service_name: str = ""

    # Evaluation type
    evaluation_type: str = "auto"  # auto, human, llm_judge
    evaluator_model: str = ""

    # Scores (0-1)
    overall_score: float = 0.0
    relevance_score: float = 0.0
    faithfulness_score: float = 0.0
    coherence_score: float = 0.0
    fluency_score: float = 0.0
    safety_score: float = 0.0
    instruction_following_score: float = 0.0

    # Custom scores
    custom_scores: dict[str, float] = field(default_factory=dict)

    # Feedback
    feedback: str = ""
    failure_reasons: list[str] = field(default_factory=list)

    # Human evaluation
    evaluator_id: str = ""
    evaluation_time_seconds: int = 0

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "timestamp": self.timestamp.isoformat(),
            "request_id": self.request_id,
            "evaluation_type": self.evaluation_type,
            "overall_score": self.overall_score,
            "relevance_score": self.relevance_score,
            "faithfulness_score": self.faithfulness_score,
            "safety_score": self.safety_score,
            "feedback": self.feedback,
            "failure_reasons": self.failure_reasons,
        }
