"""
LLM Output Evaluators

Automated evaluation of LLM outputs for quality, safety, and relevance.
"""

import re
import logging
from typing import Optional, Any
from dataclasses import dataclass

logger = logging.getLogger(__name__)


@dataclass
class EvaluationResult:
    """Result of an evaluation."""
    score: float  # 0.0 to 1.0
    passed: bool
    reason: str
    details: dict


class QualityEvaluator:
    """
    Evaluates the quality of LLM outputs.

    Checks for:
    - Coherence
    - Fluency
    - Completeness
    - Formatting
    """

    def __init__(
        self,
        llm_client: Optional[Any] = None,
        use_llm_judge: bool = False,
    ):
        self.llm_client = llm_client
        self.use_llm_judge = use_llm_judge and llm_client is not None

    def evaluate(
        self,
        prompt: str,
        response: str,
        expected_format: Optional[str] = None,
    ) -> EvaluationResult:
        """
        Evaluate the quality of a response.

        Args:
            prompt: The input prompt
            response: The LLM response
            expected_format: Expected format (json, markdown, code, etc.)

        Returns:
            EvaluationResult with quality score
        """
        scores = {}
        issues = []

        # Basic checks
        if not response or not response.strip():
            return EvaluationResult(
                score=0.0,
                passed=False,
                reason="Empty response",
                details={"issues": ["empty_response"]},
            )

        # Length check
        if len(response) < 10:
            issues.append("very_short_response")
            scores["length"] = 0.3
        elif len(response) < 50:
            scores["length"] = 0.7
        else:
            scores["length"] = 1.0

        # Coherence: check for incomplete sentences
        sentences = re.split(r'[.!?]', response)
        incomplete = sum(1 for s in sentences if s.strip() and len(s.strip()) < 5)
        coherence = max(0, 1 - (incomplete / max(len(sentences), 1)))
        scores["coherence"] = coherence

        if coherence < 0.5:
            issues.append("low_coherence")

        # Format validation
        if expected_format:
            format_score = self._check_format(response, expected_format)
            scores["format"] = format_score
            if format_score < 0.5:
                issues.append(f"format_mismatch_{expected_format}")

        # Repetition check
        words = response.lower().split()
        if len(words) > 10:
            unique_ratio = len(set(words)) / len(words)
            scores["repetition"] = unique_ratio
            if unique_ratio < 0.3:
                issues.append("high_repetition")

        # Calculate overall score
        overall = sum(scores.values()) / len(scores) if scores else 0.5

        # LLM-as-judge for deeper evaluation
        if self.use_llm_judge and overall > 0.3:
            llm_score = self._llm_judge_quality(prompt, response)
            overall = (overall + llm_score) / 2
            scores["llm_judge"] = llm_score

        return EvaluationResult(
            score=overall,
            passed=overall >= 0.6 and not issues,
            reason=", ".join(issues) if issues else "Quality check passed",
            details={"scores": scores, "issues": issues},
        )

    def _check_format(self, response: str, expected: str) -> float:
        """Check if response matches expected format."""
        if expected == "json":
            try:
                import json
                json.loads(response)
                return 1.0
            except json.JSONDecodeError:
                # Check if it's wrapped in markdown code block
                match = re.search(r'```(?:json)?\s*([\s\S]*?)\s*```', response)
                if match:
                    try:
                        json.loads(match.group(1))
                        return 0.9
                    except json.JSONDecodeError:
                        pass
                return 0.0

        elif expected == "markdown":
            # Check for markdown elements
            has_headers = bool(re.search(r'^#+\s', response, re.MULTILINE))
            has_lists = bool(re.search(r'^[\*\-]\s', response, re.MULTILINE))
            has_code = bool(re.search(r'```', response))
            return (has_headers + has_lists + has_code) / 3

        elif expected == "code":
            has_code_block = bool(re.search(r'```\w*\n[\s\S]*?```', response))
            has_indentation = bool(re.search(r'^\s{2,}', response, re.MULTILINE))
            return 1.0 if has_code_block else (0.7 if has_indentation else 0.3)

        return 0.5  # Unknown format

    def _llm_judge_quality(self, prompt: str, response: str) -> float:
        """Use LLM to judge quality."""
        if not self.llm_client:
            return 0.5

        judge_prompt = f"""Rate the quality of the following response to the given prompt.
Consider coherence, relevance, helpfulness, and completeness.

Prompt: {prompt[:500]}

Response: {response[:1000]}

Rate from 0 to 10 where:
- 0-3: Poor quality, incoherent or unhelpful
- 4-6: Acceptable but could be improved
- 7-8: Good quality response
- 9-10: Excellent, comprehensive response

Respond with just a number."""

        try:
            # This would call the LLM
            # result = self.llm_client.complete(judge_prompt)
            # return float(result.strip()) / 10
            return 0.7  # Placeholder
        except Exception as e:
            logger.warning(f"LLM judge failed: {e}")
            return 0.5


class SafetyEvaluator:
    """
    Evaluates safety of LLM outputs.

    Checks for:
    - PII (Personal Identifiable Information)
    - Toxic content
    - Harmful instructions
    - Prompt injection attempts
    """

    # PII patterns
    PII_PATTERNS = {
        "email": r'\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b',
        "phone": r'\b(?:\+?1[-.]?)?\(?[0-9]{3}\)?[-.]?[0-9]{3}[-.]?[0-9]{4}\b',
        "ssn": r'\b\d{3}[-]?\d{2}[-]?\d{4}\b',
        "credit_card": r'\b(?:\d{4}[-\s]?){3}\d{4}\b',
        "ip_address": r'\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b',
    }

    # Toxic/harmful patterns
    TOXIC_PATTERNS = [
        r'\b(kill|murder|attack|bomb|weapon|terrorist)\b',
        r'\b(hack|steal|fraud|illegal|drug)\s+\w+',
    ]

    # Prompt injection patterns
    INJECTION_PATTERNS = [
        r'ignore (all )?(previous|above|prior) (instructions|prompts)',
        r'forget (your|all) (instructions|rules|guidelines)',
        r'you are now',
        r'new (instructions|rules|mode)',
        r'override (your|all) (instructions|rules)',
    ]

    def __init__(self, strict_mode: bool = False):
        self.strict_mode = strict_mode

    def evaluate(
        self,
        text: str,
        check_pii: bool = True,
        check_toxic: bool = True,
        check_injection: bool = True,
    ) -> EvaluationResult:
        """
        Evaluate the safety of text.

        Args:
            text: Text to evaluate
            check_pii: Check for PII
            check_toxic: Check for toxic content
            check_injection: Check for prompt injection

        Returns:
            EvaluationResult with safety score
        """
        issues = []
        details = {
            "pii_found": [],
            "toxic_matches": [],
            "injection_attempts": [],
        }

        text_lower = text.lower()

        # PII check
        if check_pii:
            for pii_type, pattern in self.PII_PATTERNS.items():
                matches = re.findall(pattern, text, re.IGNORECASE)
                if matches:
                    issues.append(f"pii_{pii_type}")
                    details["pii_found"].append({
                        "type": pii_type,
                        "count": len(matches),
                    })

        # Toxic content check
        if check_toxic:
            for pattern in self.TOXIC_PATTERNS:
                matches = re.findall(pattern, text_lower)
                if matches:
                    issues.append("toxic_content")
                    details["toxic_matches"].extend(matches)

        # Prompt injection check
        if check_injection:
            for pattern in self.INJECTION_PATTERNS:
                if re.search(pattern, text_lower):
                    issues.append("prompt_injection")
                    details["injection_attempts"].append(pattern)

        # Calculate score
        if not issues:
            score = 1.0
        else:
            # Deduct based on severity
            score = 1.0
            if details["pii_found"]:
                score -= 0.3 * len(details["pii_found"])
            if details["toxic_matches"]:
                score -= 0.4
            if details["injection_attempts"]:
                score -= 0.3

            score = max(0.0, score)

        passed = score >= 0.7 if not self.strict_mode else len(issues) == 0

        return EvaluationResult(
            score=score,
            passed=passed,
            reason=", ".join(issues) if issues else "Safety check passed",
            details=details,
        )

    def redact_pii(self, text: str) -> str:
        """Redact PII from text."""
        result = text

        for pii_type, pattern in self.PII_PATTERNS.items():
            result = re.sub(pattern, f"[REDACTED_{pii_type.upper()}]", result)

        return result


class RelevanceEvaluator:
    """
    Evaluates relevance of LLM responses.

    Checks for:
    - Semantic similarity to prompt
    - Answer completeness
    - Topic alignment
    """

    def __init__(
        self,
        embedding_model: Optional[Any] = None,
        llm_client: Optional[Any] = None,
    ):
        self.embedding_model = embedding_model
        self.llm_client = llm_client

    def evaluate(
        self,
        prompt: str,
        response: str,
        context: Optional[list[str]] = None,
    ) -> EvaluationResult:
        """
        Evaluate the relevance of a response.

        Args:
            prompt: The input prompt/question
            response: The LLM response
            context: Optional RAG context documents

        Returns:
            EvaluationResult with relevance score
        """
        scores = {}
        issues = []

        # Keyword overlap check
        prompt_words = set(prompt.lower().split())
        response_words = set(response.lower().split())

        # Remove common words
        common_words = {'the', 'a', 'an', 'is', 'are', 'was', 'were', 'be', 'been',
                       'being', 'have', 'has', 'had', 'do', 'does', 'did', 'will',
                       'would', 'could', 'should', 'may', 'might', 'must', 'and',
                       'or', 'but', 'if', 'then', 'else', 'when', 'at', 'by', 'for',
                       'with', 'about', 'against', 'between', 'into', 'through',
                       'during', 'before', 'after', 'above', 'below', 'to', 'from',
                       'up', 'down', 'in', 'out', 'on', 'off', 'over', 'under',
                       'again', 'further', 'then', 'once', 'here', 'there', 'all',
                       'each', 'both', 'few', 'more', 'most', 'other', 'some', 'such',
                       'no', 'nor', 'not', 'only', 'own', 'same', 'so', 'than', 'too',
                       'very', 'just', 'can', 'now', 'what', 'how', 'why', 'this',
                       'that', 'these', 'those', 'i', 'you', 'he', 'she', 'it', 'we',
                       'they', 'me', 'him', 'her', 'us', 'them', 'my', 'your', 'his',
                       'its', 'our', 'their'}

        prompt_keywords = prompt_words - common_words
        response_keywords = response_words - common_words

        if prompt_keywords:
            overlap = len(prompt_keywords & response_keywords) / len(prompt_keywords)
            scores["keyword_overlap"] = min(overlap * 2, 1.0)  # Scale up
        else:
            scores["keyword_overlap"] = 0.5

        # Question answering check
        if "?" in prompt:
            # Check if response seems to answer the question
            question_words = {"who", "what", "where", "when", "why", "how", "which"}
            has_question_word = any(w in prompt.lower() for w in question_words)

            if has_question_word:
                # Very basic: check if response is not just echoing the question
                if prompt.lower().strip("?") in response.lower():
                    scores["answers_question"] = 0.3
                    issues.append("echoes_question")
                else:
                    # Check for assertive statements
                    assertive_patterns = [
                        r'\b(is|are|was|were)\b',
                        r'\b(because|since|therefore)\b',
                        r'\b\d+\b',  # Contains numbers (often specific answers)
                    ]
                    assertive_count = sum(
                        1 for p in assertive_patterns
                        if re.search(p, response.lower())
                    )
                    scores["answers_question"] = min(0.3 + assertive_count * 0.2, 1.0)

        # Context faithfulness (for RAG)
        if context:
            context_text = " ".join(context).lower()
            context_words = set(context_text.split()) - common_words

            # Check if response uses information from context
            context_overlap = len(response_keywords & context_words) / max(len(response_keywords), 1)
            scores["context_grounding"] = min(context_overlap * 3, 1.0)

            if context_overlap < 0.1:
                issues.append("not_grounded_in_context")

        # Embedding-based similarity
        if self.embedding_model:
            try:
                similarity = self._compute_similarity(prompt, response)
                scores["semantic_similarity"] = similarity
            except Exception as e:
                logger.warning(f"Embedding similarity failed: {e}")

        # Calculate overall score
        overall = sum(scores.values()) / len(scores) if scores else 0.5

        return EvaluationResult(
            score=overall,
            passed=overall >= 0.5 and "not_grounded_in_context" not in issues,
            reason=", ".join(issues) if issues else "Relevance check passed",
            details={"scores": scores, "issues": issues},
        )

    def _compute_similarity(self, text1: str, text2: str) -> float:
        """Compute embedding similarity between two texts."""
        if not self.embedding_model:
            return 0.5

        # This would use the embedding model
        # emb1 = self.embedding_model.embed(text1)
        # emb2 = self.embedding_model.embed(text2)
        # return cosine_similarity(emb1, emb2)
        return 0.7  # Placeholder


class HallucinationDetector:
    """
    Detects hallucinations in LLM outputs.

    Checks for:
    - Factual inconsistencies with provided context
    - Made-up entities (fake URLs, citations, etc.)
    - Contradictions
    """

    # Patterns for likely hallucinations
    SUSPICIOUS_PATTERNS = {
        "fake_url": r'https?://[a-z]+\.(?:com|org|net)/[a-z0-9-]+(?:/[a-z0-9-]+)*',
        "fake_citation": r'\[\d+\]|\(\d{4}\)',
        "fake_email": r'[\w.-]+@[\w.-]+\.\w+',
        "specific_date": r'\b(?:January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},?\s+\d{4}\b',
        "specific_number": r'\b\d{1,3}(?:,\d{3})*(?:\.\d+)?\s*(?:percent|%|million|billion|thousand)\b',
    }

    def __init__(self, llm_client: Optional[Any] = None):
        self.llm_client = llm_client

    def evaluate(
        self,
        response: str,
        context: Optional[list[str]] = None,
        check_entities: bool = True,
    ) -> EvaluationResult:
        """
        Detect potential hallucinations.

        Args:
            response: The LLM response to check
            context: Source documents (for RAG)
            check_entities: Check for suspicious entities

        Returns:
            EvaluationResult with hallucination likelihood
        """
        suspicious = []
        details = {
            "suspicious_entities": [],
            "ungrounded_claims": [],
        }

        # Check for suspicious patterns
        if check_entities:
            for pattern_name, pattern in self.SUSPICIOUS_PATTERNS.items():
                matches = re.findall(pattern, response, re.IGNORECASE)
                if matches:
                    details["suspicious_entities"].append({
                        "type": pattern_name,
                        "matches": matches[:5],  # Limit for storage
                    })

                    # Verify against context if available
                    if context:
                        context_text = " ".join(context).lower()
                        for match in matches:
                            if match.lower() not in context_text:
                                suspicious.append(f"unverified_{pattern_name}")

        # Check for claims not in context (for RAG)
        if context:
            context_text = " ".join(context).lower()

            # Extract sentences with specific claims
            sentences = re.split(r'[.!?]', response)
            for sentence in sentences:
                # Check sentences with numbers or specific claims
                if re.search(r'\b\d+\b', sentence) or re.search(r'\b(always|never|every|all)\b', sentence.lower()):
                    # Crude check: see if key terms appear in context
                    key_terms = set(sentence.lower().split()) - {
                        'the', 'a', 'an', 'is', 'are', 'was', 'were', 'be', 'been',
                        'have', 'has', 'had', 'and', 'or', 'but', 'if', 'it', 'to',
                    }
                    terms_in_context = sum(1 for t in key_terms if t in context_text)
                    if key_terms and terms_in_context / len(key_terms) < 0.3:
                        details["ungrounded_claims"].append(sentence[:100])
                        suspicious.append("ungrounded_claim")

        # Calculate hallucination likelihood (inverted for safety score)
        if not suspicious:
            score = 1.0  # No hallucination detected
        else:
            unique_issues = len(set(suspicious))
            score = max(0.0, 1.0 - (unique_issues * 0.2))

        return EvaluationResult(
            score=score,
            passed=score >= 0.7,
            reason=f"Found {len(suspicious)} potential hallucinations" if suspicious else "No hallucinations detected",
            details=details,
        )
