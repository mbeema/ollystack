import { useState, useEffect } from 'react';
import {
  Brain,
  AlertTriangle,
  TrendingUp,
  Lightbulb,
  ChevronDown,
  ChevronRight,
  Target,
  Zap,
  Clock,
  GitBranch,
  Loader2,
  RefreshCw,
  AlertCircle,
  CheckCircle,
} from 'lucide-react';
import { clsx } from 'clsx';
import { getApiUrl } from '../../lib/config';

interface RootCauseAnalysis {
  spanId: string;
  serviceName: string;
  operationName: string;
  errorType: string;
  errorMessage: string;
  confidence: number;
  contributingFactors: string[];
}

interface Insight {
  type: 'performance' | 'error' | 'pattern' | 'anomaly';
  severity: 'info' | 'warning' | 'critical';
  title: string;
  description: string;
  spanId?: string;
  recommendation?: string;
}

interface CriticalPathSpan {
  spanId: string;
  serviceName: string;
  operationName: string;
  duration: number;
  percentageOfTrace: number;
}

interface ServiceMetrics {
  serviceName: string;
  spanCount: number;
  totalDuration: number;
  errorCount: number;
  avgDuration: number;
}

interface TraceAnalysis {
  traceId: string;
  summary: string;
  rootCause?: RootCauseAnalysis;
  anomalyScore: number;
  insights: Insight[];
  recommendations: string[];
  criticalPath: CriticalPathSpan[];
  serviceBreakdown: ServiceMetrics[];
}

interface AIInsightsPanelProps {
  traceId: string;
  onSpanClick?: (spanId: string) => void;
}

export function AIInsightsPanel({ traceId, onSpanClick }: AIInsightsPanelProps) {
  const [analysis, setAnalysis] = useState<TraceAnalysis | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set(['summary', 'rootCause', 'insights'])
  );

  const API_URL = getApiUrl();

  const fetchAnalysis = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch(`${API_URL}/api/v1/traces/${traceId}/analyze`);
      if (!response.ok) {
        throw new Error('Failed to fetch analysis');
      }
      const data = await response.json();
      setAnalysis(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to analyze trace');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (traceId) {
      fetchAnalysis();
    }
  }, [traceId]);

  const toggleSection = (section: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      if (next.has(section)) {
        next.delete(section);
      } else {
        next.add(section);
      }
      return next;
    });
  };

  const formatDuration = (ns: number) => {
    const ms = ns / 1000000;
    if (ms < 1) return `${Math.round(ns / 1000)}μs`;
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const getAnomalyColor = (score: number) => {
    if (score >= 0.8) return 'text-red-400';
    if (score >= 0.5) return 'text-yellow-400';
    return 'text-green-400';
  };

  const getSeverityIcon = (severity: string) => {
    switch (severity) {
      case 'critical':
        return <AlertCircle className="w-4 h-4 text-red-400" />;
      case 'warning':
        return <AlertTriangle className="w-4 h-4 text-yellow-400" />;
      default:
        return <Lightbulb className="w-4 h-4 text-blue-400" />;
    }
  };

  const getSeverityBg = (severity: string) => {
    switch (severity) {
      case 'critical':
        return 'bg-red-900/30 border-red-800';
      case 'warning':
        return 'bg-yellow-900/30 border-yellow-800';
      default:
        return 'bg-blue-900/30 border-blue-800';
    }
  };

  if (loading) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-gray-850">
        <Loader2 className="w-8 h-8 text-blue-500 animate-spin mb-4" />
        <div className="text-gray-400 text-sm">Analyzing trace with AI...</div>
        <div className="text-gray-500 text-xs mt-2">Detecting anomalies and patterns</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-gray-850">
        <AlertCircle className="w-8 h-8 text-red-400 mb-4" />
        <div className="text-gray-400 text-sm mb-4">{error}</div>
        <button
          onClick={fetchAnalysis}
          className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg text-sm"
        >
          <RefreshCw className="w-4 h-4" />
          Retry
        </button>
      </div>
    );
  }

  if (!analysis) return null;

  return (
    <div className="h-full flex flex-col bg-gray-850 overflow-hidden">
      {/* Header */}
      <div className="flex-shrink-0 p-4 border-b border-gray-700 bg-gradient-to-r from-purple-900/20 to-blue-900/20">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Brain className="w-5 h-5 text-purple-400" />
            <span className="font-semibold text-white">AI Insights</span>
          </div>
          <button
            onClick={fetchAnalysis}
            className="p-1.5 hover:bg-gray-700 rounded text-gray-400 hover:text-white"
            title="Refresh analysis"
          >
            <RefreshCw className="w-4 h-4" />
          </button>
        </div>
        {/* Anomaly Score */}
        <div className="mt-3 flex items-center gap-3">
          <span className="text-xs text-gray-400">Anomaly Score:</span>
          <div className="flex-1 bg-gray-700 rounded-full h-2">
            <div
              className={clsx(
                'h-full rounded-full transition-all',
                analysis.anomalyScore >= 0.8
                  ? 'bg-red-500'
                  : analysis.anomalyScore >= 0.5
                  ? 'bg-yellow-500'
                  : 'bg-green-500'
              )}
              style={{ width: `${analysis.anomalyScore * 100}%` }}
            />
          </div>
          <span className={clsx('text-sm font-mono', getAnomalyColor(analysis.anomalyScore))}>
            {(analysis.anomalyScore * 100).toFixed(0)}%
          </span>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto">
        {/* Summary Section */}
        <Section
          title="Summary"
          icon={<Target className="w-4 h-4 text-blue-400" />}
          isExpanded={expandedSections.has('summary')}
          onToggle={() => toggleSection('summary')}
        >
          <p className="text-sm text-gray-300 leading-relaxed">{analysis.summary}</p>
        </Section>

        {/* Root Cause Section */}
        {analysis.rootCause && (
          <Section
            title="Root Cause"
            icon={<AlertTriangle className="w-4 h-4 text-red-400" />}
            isExpanded={expandedSections.has('rootCause')}
            onToggle={() => toggleSection('rootCause')}
            badge={
              <span className="px-2 py-0.5 bg-red-900/50 text-red-300 text-xs rounded">
                {(analysis.rootCause.confidence * 100).toFixed(0)}% confidence
              </span>
            }
          >
            <div className="space-y-3">
              <div
                className="p-3 bg-red-900/20 border border-red-800 rounded-lg cursor-pointer hover:bg-red-900/30"
                onClick={() => onSpanClick?.(analysis.rootCause!.spanId)}
              >
                <div className="flex items-center gap-2 mb-2">
                  <span className="text-red-400 font-medium">
                    {analysis.rootCause.serviceName}
                  </span>
                  <span className="text-gray-500">/</span>
                  <span className="text-gray-300">{analysis.rootCause.operationName}</span>
                </div>
                <div className="text-sm text-red-300 font-mono">
                  {analysis.rootCause.errorType}: {analysis.rootCause.errorMessage}
                </div>
              </div>

              {analysis.rootCause.contributingFactors.length > 0 && (
                <div>
                  <div className="text-xs text-gray-400 mb-2">Contributing Factors:</div>
                  <ul className="space-y-1">
                    {analysis.rootCause.contributingFactors.map((factor, idx) => (
                      <li key={idx} className="text-sm text-gray-300 flex items-start gap-2">
                        <span className="text-gray-500 mt-1">•</span>
                        {factor}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          </Section>
        )}

        {/* Insights Section */}
        {analysis.insights.length > 0 && (
          <Section
            title="Insights"
            icon={<Lightbulb className="w-4 h-4 text-yellow-400" />}
            isExpanded={expandedSections.has('insights')}
            onToggle={() => toggleSection('insights')}
            badge={
              <span className="px-2 py-0.5 bg-gray-700 text-gray-300 text-xs rounded">
                {analysis.insights.length}
              </span>
            }
          >
            <div className="space-y-2">
              {analysis.insights.map((insight, idx) => (
                <div
                  key={idx}
                  className={clsx(
                    'p-3 rounded-lg border cursor-pointer',
                    getSeverityBg(insight.severity)
                  )}
                  onClick={() => insight.spanId && onSpanClick?.(insight.spanId)}
                >
                  <div className="flex items-start gap-2">
                    {getSeverityIcon(insight.severity)}
                    <div className="flex-1">
                      <div className="text-sm font-medium text-white">{insight.title}</div>
                      <div className="text-xs text-gray-400 mt-1">{insight.description}</div>
                      {insight.recommendation && (
                        <div className="text-xs text-blue-400 mt-2 flex items-center gap-1">
                          <Zap className="w-3 h-3" />
                          {insight.recommendation}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </Section>
        )}

        {/* Critical Path Section */}
        {analysis.criticalPath.length > 0 && (
          <Section
            title="Critical Path"
            icon={<GitBranch className="w-4 h-4 text-purple-400" />}
            isExpanded={expandedSections.has('criticalPath')}
            onToggle={() => toggleSection('criticalPath')}
          >
            <div className="space-y-2">
              {analysis.criticalPath.map((span, idx) => (
                <div
                  key={span.spanId}
                  className="flex items-center gap-2 p-2 bg-gray-800 rounded cursor-pointer hover:bg-gray-750"
                  onClick={() => onSpanClick?.(span.spanId)}
                >
                  <div className="w-6 h-6 rounded-full bg-purple-900/50 flex items-center justify-center text-xs text-purple-300">
                    {idx + 1}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="text-sm text-white truncate">{span.operationName}</div>
                    <div className="text-xs text-gray-500">{span.serviceName}</div>
                  </div>
                  <div className="text-right">
                    <div className="text-sm text-gray-300 font-mono">
                      {formatDuration(span.duration)}
                    </div>
                    <div className="text-xs text-gray-500">
                      {span.percentageOfTrace.toFixed(1)}%
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </Section>
        )}

        {/* Recommendations Section */}
        {analysis.recommendations.length > 0 && (
          <Section
            title="Recommendations"
            icon={<Zap className="w-4 h-4 text-green-400" />}
            isExpanded={expandedSections.has('recommendations')}
            onToggle={() => toggleSection('recommendations')}
          >
            <ul className="space-y-2">
              {analysis.recommendations.map((rec, idx) => (
                <li key={idx} className="flex items-start gap-2 text-sm">
                  <CheckCircle className="w-4 h-4 text-green-400 mt-0.5 flex-shrink-0" />
                  <span className="text-gray-300">{rec}</span>
                </li>
              ))}
            </ul>
          </Section>
        )}

        {/* Service Breakdown Section */}
        {analysis.serviceBreakdown.length > 0 && (
          <Section
            title="Service Breakdown"
            icon={<TrendingUp className="w-4 h-4 text-cyan-400" />}
            isExpanded={expandedSections.has('serviceBreakdown')}
            onToggle={() => toggleSection('serviceBreakdown')}
          >
            <div className="space-y-2">
              {analysis.serviceBreakdown.map((svc) => (
                <div key={svc.serviceName} className="p-2 bg-gray-800 rounded">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-sm text-white font-medium">{svc.serviceName}</span>
                    {svc.errorCount > 0 && (
                      <span className="px-1.5 py-0.5 bg-red-900/50 text-red-300 text-xs rounded">
                        {svc.errorCount} errors
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-4 text-xs text-gray-400">
                    <span>{svc.spanCount} spans</span>
                    <span>Total: {formatDuration(svc.totalDuration)}</span>
                    <span>Avg: {formatDuration(svc.avgDuration)}</span>
                  </div>
                </div>
              ))}
            </div>
          </Section>
        )}
      </div>
    </div>
  );
}

interface SectionProps {
  title: string;
  icon: React.ReactNode;
  isExpanded: boolean;
  onToggle: () => void;
  badge?: React.ReactNode;
  children: React.ReactNode;
}

function Section({ title, icon, isExpanded, onToggle, badge, children }: SectionProps) {
  return (
    <div className="border-b border-gray-700">
      <button
        onClick={onToggle}
        className="w-full flex items-center justify-between p-3 hover:bg-gray-800/50"
      >
        <div className="flex items-center gap-2">
          {icon}
          <span className="text-sm font-medium text-white">{title}</span>
          {badge}
        </div>
        {isExpanded ? (
          <ChevronDown className="w-4 h-4 text-gray-400" />
        ) : (
          <ChevronRight className="w-4 h-4 text-gray-400" />
        )}
      </button>
      {isExpanded && <div className="px-3 pb-3">{children}</div>}
    </div>
  );
}

export default AIInsightsPanel;
