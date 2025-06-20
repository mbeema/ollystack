# OllyStack AI/ML Architecture
## Intelligence Layer for Automated Issue Resolution

**Version:** 1.0
**Last Updated:** 2026-02-03

---

## Vision

> **"From alert to root cause in seconds, not hours"**

Traditional observability requires engineers to:
1. See an alert
2. Open dashboards
3. Correlate traces, logs, metrics manually
4. Form hypotheses
5. Test hypotheses
6. Find root cause
7. Fix the issue

**OllyStack Vision:**
1. See an alert → **AI already identified root cause**
2. Click → **Full context + explanation + fix suggestion**

---

## The Intelligence Stack

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         USER INTERACTION LAYER                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  "Why is checkout slow?"  ───►  Natural Language Query (NLQ)            │
│                                        │                                 │
│  Alert: High Latency      ───►  Auto-RCA with Explanation               │
│                                        │                                 │
│  View Correlation         ───►  AI-Enriched Context                     │
│                                        │                                 │
└────────────────────────────────────────┼────────────────────────────────┘
                                         │
                                         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         AI ORCHESTRATION LAYER                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐  │
│  │  Query Router   │  │  Context        │  │  Response Generator     │  │
│  │                 │  │  Assembler      │  │                         │  │
│  │  • Intent       │  │                 │  │  • LLM Explanation      │  │
│  │    detection    │  │  • Correlation  │  │  • Recommendations      │  │
│  │  • Query type   │  │    fetch        │  │  • Fix suggestions      │  │
│  │  • Routing      │  │  • Enrichment   │  │  • Confidence scores    │  │
│  └────────┬────────┘  └────────┬────────┘  └────────────┬────────────┘  │
│           │                    │                        │                │
│           └────────────────────┼────────────────────────┘                │
│                                │                                         │
└────────────────────────────────┼─────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         ML ANALYSIS LAYER                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    ROOT CAUSE ANALYSIS                           │    │
│  ├─────────────────┬─────────────────┬─────────────────────────────┤    │
│  │ Causal Graph    │ Critical Path   │ Error Propagation           │    │
│  │ Analysis        │ Analysis        │ Tracker                     │    │
│  │                 │                 │                             │    │
│  │ • Service deps  │ • Longest path  │ • First error origin        │    │
│  │ • Metric        │ • Bottleneck    │ • Cascade detection         │    │
│  │   causation     │   detection     │ • Blast radius              │    │
│  │ • Counterfactual│ • Parallelism   │                             │    │
│  │   reasoning     │   opportunities │                             │    │
│  └─────────────────┴─────────────────┴─────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    ANOMALY DETECTION                             │    │
│  ├─────────────────┬─────────────────┬─────────────────────────────┤    │
│  │ Statistical     │ Seasonal        │ ML-Based                    │    │
│  │ Detection       │ Decomposition   │ Detection                   │    │
│  │                 │                 │                             │    │
│  │ • Z-score       │ • Hourly        │ • Isolation Forest          │    │
│  │ • IQR           │ • Daily         │ • LSTM forecasting          │    │
│  │ • Moving avg    │ • Weekly        │ • Clustering                │    │
│  │ • Percentiles   │ • Holiday adj   │ • Autoencoders              │    │
│  └─────────────────┴─────────────────┴─────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    PATTERN RECOGNITION                           │    │
│  ├─────────────────┬─────────────────┬─────────────────────────────┤    │
│  │ Error           │ Performance     │ Behavioral                  │    │
│  │ Fingerprinting  │ Patterns        │ Analysis                    │    │
│  │                 │                 │                             │    │
│  │ • Stack trace   │ • Slow query    │ • User journey              │    │
│  │   clustering    │   patterns      │   analysis                  │    │
│  │ • Error         │ • Resource      │ • Traffic pattern           │    │
│  │   deduplication │   contention    │   detection                 │    │
│  │ • Similar       │ • N+1 query     │ • Deployment                │    │
│  │   issue linking │   detection     │   correlation               │    │
│  └─────────────────┴─────────────────┴─────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         DATA LAYER                                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐  │
│  │    ClickHouse   │  │     Redis       │  │   Vector Embeddings     │  │
│  │                 │  │                 │  │                         │  │
│  │  • Traces       │  │  • Hot cache    │  │  • Error embeddings     │  │
│  │  • Logs         │  │  • Baselines    │  │  • Log embeddings       │  │
│  │  • Metrics      │  │  • ML models    │  │  • Similar issue        │  │
│  │  • Correlations │  │  • Sessions     │  │    retrieval            │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────────────┘  │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Core AI/ML Components

### 1. Intelligent Root Cause Analysis (RCA)

The RCA system combines multiple approaches for reliable root cause identification:

```python
class IntelligentRCA:
    """
    Multi-strategy RCA that combines:
    1. Deterministic analysis (fast, reliable)
    2. Statistical analysis (anomaly-based)
    3. Causal inference (counterfactual reasoning)
    4. LLM reasoning (explanation generation)
    """

    async def analyze(self, correlation_id: str) -> RCAResult:
        context = await self.fetch_context(correlation_id)

        # Strategy 1: Deterministic (< 10ms)
        # - Find first error in trace
        # - Find slowest span
        # - Check for known error patterns
        deterministic = self.deterministic_analysis(context)

        # Strategy 2: Statistical (< 100ms)
        # - Compare metrics to baselines
        # - Detect anomalies in latency/error rate
        # - Identify deviations from normal
        statistical = await self.statistical_analysis(context)

        # Strategy 3: Causal (< 500ms)
        # - Build service dependency graph
        # - Apply causal inference
        # - Generate counterfactuals
        causal = await self.causal_analysis(context)

        # Combine evidence from all strategies
        combined = self.combine_evidence(deterministic, statistical, causal)

        # Strategy 4: LLM Explanation (< 2s)
        # - Generate human-readable explanation
        # - Suggest remediation steps
        # - Provide confidence reasoning
        explanation = await self.llm_explain(combined, context)

        return RCAResult(
            root_cause=combined.primary_cause,
            confidence=combined.confidence,
            evidence=combined.evidence,
            explanation=explanation.summary,
            remediation=explanation.suggestions,
            similar_incidents=await self.find_similar(combined)
        )
```

#### RCA Decision Tree

```
                    ┌─────────────────┐
                    │ Correlation ID  │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │ Fetch Context   │
                    │ (traces, logs,  │
                    │  metrics)       │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
     ┌────────────┐  ┌────────────┐  ┌────────────┐
     │ Any Errors │  │ Any Slow   │  │ Any Metric │
     │ in Traces? │  │ Spans?     │  │ Anomalies? │
     └─────┬──────┘  └─────┬──────┘  └─────┬──────┘
           │               │               │
     ┌─────┴─────┐   ┌─────┴─────┐   ┌─────┴─────┐
     │ YES       │   │ YES       │   │ YES       │
     ▼           │   ▼           │   ▼           │
┌─────────┐     │ ┌─────────┐   │ ┌─────────┐   │
│ Error   │     │ │ Latency │   │ │ Capacity│   │
│ Analysis│     │ │ Analysis│   │ │ Analysis│   │
└────┬────┘     │ └────┬────┘   │ └────┬────┘   │
     │          │      │        │      │        │
     └──────────┴──────┴────────┴──────┴────────┘
                       │
                       ▼
              ┌─────────────────┐
              │ Causal Graph    │
              │ Analysis        │
              │                 │
              │ Which component │
              │ CAUSED the      │
              │ issue?          │
              └────────┬────────┘
                       │
                       ▼
              ┌─────────────────┐
              │ LLM Explanation │
              │                 │
              │ "The root cause │
              │ is X because... │
              │ To fix, try..." │
              └─────────────────┘
```

---

### 2. Anomaly Detection System

Multi-layer anomaly detection that learns what's "normal" for each service:

```python
class AnomalyDetectionSystem:
    """
    Learns baselines per service/operation and detects deviations.
    """

    def __init__(self):
        self.baselines = BaselineStore()  # Redis-backed
        self.models = ModelStore()         # Trained ML models

    async def detect(self, service: str, metric: str, value: float) -> AnomalyResult:
        # Layer 1: Statistical (always runs, fast)
        baseline = await self.baselines.get(service, metric)
        z_score = (value - baseline.mean) / baseline.std
        statistical_anomaly = abs(z_score) > 3

        # Layer 2: Seasonal (if enough history)
        if baseline.has_seasonal_data:
            expected = baseline.seasonal_expected(datetime.now())
            seasonal_deviation = abs(value - expected) / expected
            seasonal_anomaly = seasonal_deviation > 0.5  # 50% deviation

        # Layer 3: ML-based (for complex patterns)
        if self.models.has_model(service, metric):
            model = self.models.get(service, metric)
            ml_score = model.predict_anomaly_score(value)
            ml_anomaly = ml_score > 0.8

        # Combine signals
        return AnomalyResult(
            is_anomaly=statistical_anomaly or seasonal_anomaly or ml_anomaly,
            severity=self.calculate_severity(z_score, seasonal_deviation, ml_score),
            expected_value=baseline.mean,
            actual_value=value,
            deviation_percent=((value - baseline.mean) / baseline.mean) * 100,
            explanation=self.explain_anomaly(...)
        )

    async def learn_baseline(self, service: str, metric: str, lookback_days: int = 14):
        """Continuously learn what's normal for each service."""
        data = await self.fetch_historical(service, metric, lookback_days)

        baseline = Baseline(
            mean=np.mean(data),
            std=np.std(data),
            p50=np.percentile(data, 50),
            p95=np.percentile(data, 95),
            p99=np.percentile(data, 99),
            hourly_pattern=self.extract_hourly_pattern(data),
            daily_pattern=self.extract_daily_pattern(data),
            weekly_pattern=self.extract_weekly_pattern(data),
        )

        await self.baselines.store(service, metric, baseline)
```

#### Baseline Learning Pipeline

```
┌──────────────────────────────────────────────────────────────────────┐
│                     BASELINE LEARNING PIPELINE                        │
├──────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  Historical Data (14 days)                                           │
│         │                                                            │
│         ▼                                                            │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    DECOMPOSITION                             │    │
│  ├─────────────────┬─────────────────┬─────────────────────────┤    │
│  │    Trend        │    Seasonal     │    Residual             │    │
│  │                 │                 │                         │    │
│  │  Long-term      │  • Hourly       │  Random noise           │    │
│  │  direction      │  • Daily        │  after removing         │    │
│  │                 │  • Weekly       │  trend + seasonal       │    │
│  └────────┬────────┴────────┬────────┴────────┬────────────────┘    │
│           │                 │                 │                      │
│           ▼                 ▼                 ▼                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    BASELINE MODEL                            │    │
│  │                                                              │    │
│  │  expected(t) = trend(t) + seasonal(t)                       │    │
│  │  anomaly_threshold = 3 * std(residual)                      │    │
│  │                                                              │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                       │
│  Example:                                                            │
│  ─────────────────────────────────────────────────────────────────  │
│  Payment Service Latency:                                            │
│  • Normal weekday 9am: 150ms ± 20ms                                 │
│  • Normal weekend 9am: 80ms ± 10ms                                  │
│  • Anomaly: > 210ms on weekday 9am                                  │
│                                                                       │
└──────────────────────────────────────────────────────────────────────┘
```

---

### 3. Natural Language Query (NLQ)

Allow users to ask questions in plain English:

```python
class NaturalLanguageQuery:
    """
    Convert natural language questions to insights.

    Examples:
    - "Why is checkout slow today?"
    - "Show me errors in payment service last hour"
    - "Compare latency this week vs last week"
    - "What changed before the outage?"
    """

    QUERY_TYPES = {
        "why": "root_cause_analysis",
        "show": "data_retrieval",
        "compare": "comparison_analysis",
        "what changed": "change_detection",
        "how many": "aggregation",
        "which": "filtering",
    }

    async def query(self, question: str) -> NLQResponse:
        # 1. Classify intent
        intent = await self.classify_intent(question)

        # 2. Extract entities
        entities = await self.extract_entities(question)
        # e.g., service="payment", time="last hour", metric="errors"

        # 3. Route to appropriate handler
        if intent == "root_cause_analysis":
            return await self.handle_rca_query(question, entities)
        elif intent == "data_retrieval":
            return await self.handle_data_query(question, entities)
        elif intent == "comparison_analysis":
            return await self.handle_comparison_query(question, entities)
        elif intent == "change_detection":
            return await self.handle_change_query(question, entities)

    async def handle_rca_query(self, question: str, entities: dict) -> NLQResponse:
        """Handle 'why' questions."""
        # Find relevant correlation IDs
        correlations = await self.find_relevant_correlations(entities)

        # Run RCA on most relevant
        rca_results = []
        for corr_id in correlations[:5]:
            rca = await self.rca_engine.analyze(corr_id)
            rca_results.append(rca)

        # Generate natural language response
        response = await self.generate_response(question, rca_results)

        return NLQResponse(
            question=question,
            answer=response.summary,
            evidence=response.evidence,
            visualizations=response.suggested_charts,
            follow_up_questions=response.suggested_questions
        )
```

#### NLQ Examples

| User Question | System Action | Response |
|---------------|---------------|----------|
| "Why is checkout slow?" | Find slow checkout traces → RCA | "Checkout is slow due to database connection pool exhaustion on orders-db. The pool hit 100% utilization at 14:23, causing 2.3s avg latency (normally 150ms)." |
| "Show errors in payments" | Query errors → Group by type | "Found 47 errors in payment-service (last hour): 32 timeout errors, 12 validation errors, 3 connection refused." |
| "What changed before the outage?" | Detect changes in window | "3 changes detected before outage: (1) Deploy of user-service v2.3.1 at 14:15, (2) Config change in redis at 14:18, (3) Traffic spike +40% at 14:20." |

---

### 4. Similar Incident Detection

Learn from past incidents to speed up future debugging:

```python
class SimilarIncidentDetector:
    """
    Find similar past incidents using embeddings.
    """

    def __init__(self):
        self.embedding_model = SentenceTransformer('all-MiniLM-L6-v2')
        self.vector_store = VectorStore()  # Could be Qdrant, Pinecone, pgvector

    async def index_incident(self, incident: Incident):
        """Index a resolved incident for future retrieval."""
        # Create embedding from incident summary
        text = f"""
        Service: {incident.service}
        Error: {incident.error_message}
        Root Cause: {incident.root_cause}
        Symptoms: {', '.join(incident.symptoms)}
        Resolution: {incident.resolution}
        """
        embedding = self.embedding_model.encode(text)

        await self.vector_store.upsert(
            id=incident.id,
            embedding=embedding,
            metadata={
                "service": incident.service,
                "error_type": incident.error_type,
                "root_cause": incident.root_cause,
                "resolution": incident.resolution,
                "timestamp": incident.timestamp,
            }
        )

    async def find_similar(self, current_context: CorrelatedContext) -> List[SimilarIncident]:
        """Find similar past incidents."""
        # Create embedding from current context
        text = f"""
        Services: {', '.join(current_context.services)}
        Errors: {self.summarize_errors(current_context.errors)}
        Symptoms: {self.extract_symptoms(current_context)}
        """
        embedding = self.embedding_model.encode(text)

        # Search vector store
        results = await self.vector_store.search(
            embedding=embedding,
            top_k=5,
            filter={"service": {"$in": current_context.services}}
        )

        return [
            SimilarIncident(
                similarity_score=r.score,
                incident_id=r.id,
                root_cause=r.metadata["root_cause"],
                resolution=r.metadata["resolution"],
                how_it_was_fixed=r.metadata.get("fix_steps", [])
            )
            for r in results
            if r.score > 0.7  # Only high-confidence matches
        ]
```

---

### 5. Predictive Alerting

Alert BEFORE problems occur:

```python
class PredictiveAlerting:
    """
    Predict issues before they impact users.
    """

    async def predict(self, service: str) -> List[PredictiveAlert]:
        alerts = []

        # 1. Trend Extrapolation
        # "At current rate, disk will be full in 4 hours"
        capacity_predictions = await self.predict_capacity(service)
        for pred in capacity_predictions:
            if pred.time_to_threshold < timedelta(hours=4):
                alerts.append(PredictiveAlert(
                    type="capacity",
                    severity="warning",
                    message=f"{pred.resource} will reach {pred.threshold} in {pred.time_to_threshold}",
                    recommended_action=pred.recommendation
                ))

        # 2. Anomaly Trajectory
        # "Error rate increasing, will breach SLO in 30 minutes"
        trajectory = await self.analyze_trajectory(service)
        if trajectory.will_breach_slo:
            alerts.append(PredictiveAlert(
                type="slo_breach",
                severity="critical",
                message=f"Error rate trending to breach SLO in {trajectory.time_to_breach}",
                current_value=trajectory.current,
                projected_value=trajectory.projected,
                recommended_action="Investigate recent deployments"
            ))

        # 3. Dependency Risk
        # "Upstream service degrading, expect impact in 2 minutes"
        dependency_risks = await self.check_dependencies(service)
        for risk in dependency_risks:
            if risk.impact_probability > 0.8:
                alerts.append(PredictiveAlert(
                    type="dependency_risk",
                    severity="warning",
                    message=f"Upstream {risk.dependency} degrading, expect impact soon",
                    recommended_action=f"Check {risk.dependency} health"
                ))

        return alerts
```

---

## ML Model Training Pipeline

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      ML MODEL TRAINING PIPELINE                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────┐                                                    │
│  │ Historical Data │                                                    │
│  │ (ClickHouse)    │                                                    │
│  └────────┬────────┘                                                    │
│           │                                                              │
│           ▼                                                              │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    FEATURE ENGINEERING                           │   │
│  ├─────────────────┬─────────────────┬─────────────────────────────┤   │
│  │ Time Features   │ Service Features│ Interaction Features        │   │
│  │                 │                 │                             │   │
│  │ • Hour of day   │ • Error rate    │ • Call patterns             │   │
│  │ • Day of week   │ • Latency p50   │ • Dependency health         │   │
│  │ • Is holiday    │ • Throughput    │ • Resource correlation      │   │
│  │ • Minutes since │ • Saturation    │ • Concurrent requests       │   │
│  │   deploy        │                 │                             │   │
│  └─────────────────┴─────────────────┴─────────────────────────────┘   │
│           │                                                              │
│           ▼                                                              │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    MODEL TRAINING                                │   │
│  ├─────────────────┬─────────────────┬─────────────────────────────┤   │
│  │ Anomaly         │ Forecasting     │ Classification              │   │
│  │ Detection       │                 │                             │   │
│  │                 │                 │                             │   │
│  │ Isolation       │ Prophet/LSTM    │ Error Type                  │   │
│  │ Forest          │ for time series │ Classifier                  │   │
│  │                 │ prediction      │                             │   │
│  └─────────────────┴─────────────────┴─────────────────────────────┘   │
│           │                                                              │
│           ▼                                                              │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    MODEL SERVING                                 │   │
│  │                                                                  │   │
│  │  • Models serialized to Redis                                   │   │
│  │  • Hot-reload on new training                                   │   │
│  │  • A/B testing for model versions                               │   │
│  │  • Fallback to statistical methods if ML fails                  │   │
│  │                                                                  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  Training Schedule:                                                      │
│  • Baselines: Every hour (incremental)                                  │
│  • Anomaly models: Daily (full retrain)                                 │
│  • Forecasting models: Weekly                                            │
│  • Incident embeddings: On each resolved incident                       │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## LLM Integration Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      LLM INTEGRATION ARCHITECTURE                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    LLM PROVIDER ABSTRACTION                      │   │
│  ├─────────────────┬─────────────────┬─────────────────────────────┤   │
│  │    OpenAI       │    Anthropic    │    Local (Ollama)           │   │
│  │    GPT-4o       │    Claude 3     │    Llama 3, Mistral         │   │
│  │                 │                 │                             │   │
│  │  Best for:      │  Best for:      │  Best for:                  │   │
│  │  • Complex      │  • Long context │  • Privacy-sensitive        │   │
│  │    reasoning    │  • Detailed     │  • Air-gapped               │   │
│  │  • Code         │    explanation  │  • Cost-sensitive           │   │
│  │    generation   │                 │                             │   │
│  └─────────────────┴─────────────────┴─────────────────────────────┘   │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    PROMPT ENGINEERING                            │   │
│  │                                                                  │   │
│  │  System Prompt:                                                  │   │
│  │  "You are an expert SRE analyzing distributed system issues.    │   │
│  │   You have access to traces, logs, and metrics. Be concise      │   │
│  │   and actionable. Always provide evidence for conclusions."     │   │
│  │                                                                  │   │
│  │  Context Injection:                                              │   │
│  │  • Service topology (which services call which)                 │   │
│  │  • Recent changes (deploys, config changes)                     │   │
│  │  • Historical baselines (what's normal)                         │   │
│  │  • Error patterns (what errors mean)                            │   │
│  │                                                                  │   │
│  │  Response Format:                                                │   │
│  │  {                                                               │   │
│  │    "root_cause": "string",                                      │   │
│  │    "confidence": 0.0-1.0,                                       │   │
│  │    "evidence": ["list", "of", "evidence"],                      │   │
│  │    "impact": "who/what is affected",                            │   │
│  │    "remediation": ["step1", "step2"],                           │   │
│  │    "prevention": "how to prevent in future"                     │   │
│  │  }                                                               │   │
│  │                                                                  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    GUARDRAILS                                    │   │
│  │                                                                  │   │
│  │  • Never expose raw credentials/secrets to LLM                  │   │
│  │  • Sanitize PII from logs before sending                        │   │
│  │  • Rate limit LLM calls (cost control)                          │   │
│  │  • Fallback to non-LLM response if API fails                    │   │
│  │  • Cache similar queries (reduce API calls)                     │   │
│  │  • Validate LLM output format (JSON schema)                     │   │
│  │                                                                  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## API Endpoints (AI/ML)

```yaml
# AI-Powered Endpoints

POST /api/v1/ai/rca:
  description: Automated root cause analysis
  request:
    correlation_id: string
    # OR
    service: string
    time_range: {start: datetime, end: datetime}
  response:
    root_cause: string
    confidence: float
    evidence: [string]
    similar_incidents: [{id, resolution}]
    remediation: [string]

POST /api/v1/ai/nlq:
  description: Natural language query
  request:
    question: string
    context: {service?: string, time_range?: object}
  response:
    answer: string
    sql_query: string  # Generated query
    data: object       # Query results
    visualizations: [{type, config}]
    follow_up_questions: [string]

GET /api/v1/ai/anomalies:
  description: Current anomalies across all services
  response:
    anomalies: [{
      service: string,
      metric: string,
      severity: string,
      current_value: float,
      expected_value: float,
      deviation: float,
      started_at: datetime
    }]

POST /api/v1/ai/predict:
  description: Predictive alerts
  request:
    service: string
    horizon: duration  # How far to predict
  response:
    predictions: [{
      type: string,  # capacity, slo_breach, dependency_risk
      probability: float,
      time_to_event: duration,
      recommended_action: string
    }]

GET /api/v1/ai/insights/{correlation_id}:
  description: AI-enriched correlation context
  response:
    # Standard correlation response PLUS:
    ai_insights:
      root_cause: string
      confidence: float
      anomalies_detected: [object]
      similar_incidents: [object]
      suggested_actions: [string]
```

---

## Success Metrics for AI/ML

| Metric | Target | Description |
|--------|--------|-------------|
| RCA Accuracy | >80% | % of times AI correctly identifies root cause |
| MTTR Reduction | -50% | Time from alert to resolution |
| False Positive Rate | <5% | Anomaly alerts that aren't real issues |
| NLQ Success Rate | >90% | % of NLQ queries that return useful results |
| Prediction Accuracy | >70% | % of predictive alerts that would have occurred |
| User Trust Score | >4/5 | User rating of AI suggestions |

---

## Data Requirements for ML

| Model | Data Needed | Minimum History |
|-------|-------------|-----------------|
| Baselines | Metrics per service | 7 days |
| Seasonal patterns | Metrics per service | 4 weeks |
| Anomaly detection | Metrics + labels | 2 weeks |
| Similar incidents | Resolved incidents | 50 incidents |
| Error fingerprinting | Error traces | 1000 errors |

---

*AI/ML Architecture v1.0 - Building intelligence into observability*
