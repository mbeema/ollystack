import { useState, useEffect } from 'react';
import {
  GitCompare,
  ArrowRight,
  ArrowLeft,
  Clock,
  AlertCircle,
  CheckCircle,
  Loader2,
  X,
  ChevronDown,
  ChevronRight,
  TrendingUp,
  TrendingDown,
  Minus,
} from 'lucide-react';
import { clsx } from 'clsx';
import { getApiUrl } from '../../lib/config';

interface Span {
  spanId: string;
  serviceName: string;
  operationName: string;
  duration: number;
  status: string;
  parentSpanId?: string;
}

interface TraceSummary {
  traceId: string;
  serviceName: string;
  operationName: string;
  duration: number;
  spanCount: number;
  errorCount: number;
  timestamp: string;
  spans: Span[];
}

interface SpanDiff {
  operation: string;
  service: string;
  baseSpan?: Span;
  compareSpan?: Span;
  durationDiff?: number;
  durationDiffPercent?: number;
  statusChanged: boolean;
}

interface TraceComparisonProps {
  baseTraceId: string;
  compareTraceId?: string;
  onClose: () => void;
  onSelectCompareTrace: (traceId: string) => void;
}

export function TraceComparison({
  baseTraceId,
  compareTraceId,
  onClose,
  onSelectCompareTrace,
}: TraceComparisonProps) {
  const [baseTrace, setBaseTrace] = useState<TraceSummary | null>(null);
  const [compareTrace, setCompareTrace] = useState<TraceSummary | null>(null);
  const [recentTraces, setRecentTraces] = useState<TraceSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedOps, setExpandedOps] = useState<Set<string>>(new Set());

  const API_URL = getApiUrl();

  useEffect(() => {
    fetchTraces();
  }, [baseTraceId, compareTraceId]);

  const fetchTraces = async () => {
    setLoading(true);
    try {
      // Fetch base trace
      const baseRes = await fetch(`${API_URL}/api/v1/traces/${baseTraceId}`);
      if (baseRes.ok) {
        const baseData = await baseRes.json();
        setBaseTrace(transformTrace(baseData, baseTraceId));
      }

      // Fetch compare trace if specified
      if (compareTraceId) {
        const compareRes = await fetch(`${API_URL}/api/v1/traces/${compareTraceId}`);
        if (compareRes.ok) {
          const compareData = await compareRes.json();
          setCompareTrace(transformTrace(compareData, compareTraceId));
        }
      }

      // Fetch recent traces for selection
      const recentRes = await fetch(`${API_URL}/api/v1/traces?limit=20`);
      if (recentRes.ok) {
        const recentData = await recentRes.json();
        setRecentTraces(
          (recentData.traces || [])
            .filter((t: any) => t.traceId !== baseTraceId)
            .slice(0, 10)
        );
      }
    } catch (error) {
      console.error('Failed to fetch traces:', error);
    } finally {
      setLoading(false);
    }
  };

  const transformTrace = (data: any, traceId: string): TraceSummary => {
    const spans = data.spans || [];
    const rootSpan = spans.find((s: any) => !s.parentSpanId) || spans[0] || {};
    return {
      traceId,
      serviceName: rootSpan.serviceName || 'unknown',
      operationName: rootSpan.operationName || 'unknown',
      duration: spans.reduce((max: number, s: any) => Math.max(max, s.duration || 0), 0),
      spanCount: spans.length,
      errorCount: spans.filter((s: any) => s.status === 'ERROR').length,
      timestamp: rootSpan.startTime ? new Date(rootSpan.startTime / 1000000).toISOString() : '',
      spans: spans.map((s: any) => ({
        spanId: s.spanId,
        serviceName: s.serviceName,
        operationName: s.operationName,
        duration: s.duration,
        status: s.status,
        parentSpanId: s.parentSpanId,
      })),
    };
  };

  const computeDiffs = (): SpanDiff[] => {
    if (!baseTrace || !compareTrace) return [];

    const diffs: SpanDiff[] = [];
    const baseOps = new Map<string, Span[]>();
    const compareOps = new Map<string, Span[]>();

    // Group spans by operation
    baseTrace.spans.forEach((span) => {
      const key = `${span.serviceName}::${span.operationName}`;
      if (!baseOps.has(key)) baseOps.set(key, []);
      baseOps.get(key)!.push(span);
    });

    compareTrace.spans.forEach((span) => {
      const key = `${span.serviceName}::${span.operationName}`;
      if (!compareOps.has(key)) compareOps.set(key, []);
      compareOps.get(key)!.push(span);
    });

    // Find all unique operations
    const allOps = new Set([...baseOps.keys(), ...compareOps.keys()]);

    allOps.forEach((key) => {
      const [service, operation] = key.split('::');
      const baseSpans = baseOps.get(key) || [];
      const compareSpans = compareOps.get(key) || [];

      // Compare first span of each operation
      const baseSpan = baseSpans[0];
      const compareSpan = compareSpans[0];

      let durationDiff: number | undefined;
      let durationDiffPercent: number | undefined;
      let statusChanged = false;

      if (baseSpan && compareSpan) {
        durationDiff = compareSpan.duration - baseSpan.duration;
        durationDiffPercent = baseSpan.duration > 0
          ? ((compareSpan.duration - baseSpan.duration) / baseSpan.duration) * 100
          : 0;
        statusChanged = baseSpan.status !== compareSpan.status;
      }

      diffs.push({
        operation,
        service,
        baseSpan,
        compareSpan,
        durationDiff,
        durationDiffPercent,
        statusChanged,
      });
    });

    // Sort by absolute duration diff
    return diffs.sort((a, b) => {
      const aDiff = Math.abs(a.durationDiff || 0);
      const bDiff = Math.abs(b.durationDiff || 0);
      return bDiff - aDiff;
    });
  };

  const formatDuration = (ns: number) => {
    const ms = ns / 1000000;
    if (ms < 1) return `${Math.round(ns / 1000)}μs`;
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const formatDiff = (diff: number, percent: number) => {
    const sign = diff > 0 ? '+' : '';
    const ms = diff / 1000000;
    return `${sign}${ms.toFixed(1)}ms (${sign}${percent.toFixed(1)}%)`;
  };

  const toggleOp = (op: string) => {
    setExpandedOps((prev) => {
      const next = new Set(prev);
      if (next.has(op)) {
        next.delete(op);
      } else {
        next.add(op);
      }
      return next;
    });
  };

  const diffs = computeDiffs();

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-50 p-4">
      <div className="bg-gray-900 rounded-xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-gray-700">
          <div className="flex items-center gap-3">
            <GitCompare className="w-5 h-5 text-blue-400" />
            <h2 className="text-lg font-semibold text-white">Trace Comparison</h2>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-gray-800 rounded-lg text-gray-400 hover:text-white"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {loading ? (
          <div className="flex-1 flex items-center justify-center">
            <Loader2 className="w-8 h-8 text-blue-500 animate-spin" />
          </div>
        ) : (
          <div className="flex-1 overflow-hidden flex flex-col">
            {/* Trace Selectors */}
            <div className="p-4 border-b border-gray-700">
              <div className="grid grid-cols-2 gap-8">
                {/* Base Trace */}
                <div>
                  <div className="text-sm text-gray-400 mb-2">Base Trace</div>
                  {baseTrace && (
                    <div className="p-3 bg-blue-900/20 border border-blue-800 rounded-lg">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-blue-300 font-medium">{baseTrace.serviceName}</span>
                        <span className="text-gray-400 text-sm">{formatDuration(baseTrace.duration)}</span>
                      </div>
                      <div className="text-sm text-gray-300">{baseTrace.operationName}</div>
                      <div className="flex items-center gap-3 mt-2 text-xs text-gray-500">
                        <span>{baseTrace.spanCount} spans</span>
                        {baseTrace.errorCount > 0 && (
                          <span className="text-red-400">{baseTrace.errorCount} errors</span>
                        )}
                      </div>
                    </div>
                  )}
                </div>

                {/* Compare Trace */}
                <div>
                  <div className="text-sm text-gray-400 mb-2">Compare Trace</div>
                  {compareTrace ? (
                    <div className="p-3 bg-purple-900/20 border border-purple-800 rounded-lg">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-purple-300 font-medium">{compareTrace.serviceName}</span>
                        <span className="text-gray-400 text-sm">{formatDuration(compareTrace.duration)}</span>
                      </div>
                      <div className="text-sm text-gray-300">{compareTrace.operationName}</div>
                      <div className="flex items-center gap-3 mt-2 text-xs text-gray-500">
                        <span>{compareTrace.spanCount} spans</span>
                        {compareTrace.errorCount > 0 && (
                          <span className="text-red-400">{compareTrace.errorCount} errors</span>
                        )}
                      </div>
                    </div>
                  ) : (
                    <div className="p-3 bg-gray-800 border border-gray-700 rounded-lg">
                      <div className="text-sm text-gray-400 mb-3">Select a trace to compare:</div>
                      <div className="space-y-2 max-h-48 overflow-auto">
                        {recentTraces.map((trace) => (
                          <button
                            key={trace.traceId}
                            onClick={() => onSelectCompareTrace(trace.traceId)}
                            className="w-full text-left p-2 rounded hover:bg-gray-700 transition-colors"
                          >
                            <div className="flex items-center justify-between">
                              <span className="text-sm text-white">{trace.serviceName}</span>
                              <span className="text-xs text-gray-500">{formatDuration(trace.duration)}</span>
                            </div>
                            <div className="text-xs text-gray-400 truncate">{trace.operationName}</div>
                          </button>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>

            {/* Comparison Results */}
            {compareTrace && (
              <div className="flex-1 overflow-auto p-4">
                {/* Summary Stats */}
                <div className="grid grid-cols-3 gap-4 mb-6">
                  <StatCard
                    label="Duration Change"
                    value={formatDiff(
                      compareTrace.duration - baseTrace!.duration,
                      ((compareTrace.duration - baseTrace!.duration) / baseTrace!.duration) * 100
                    )}
                    isPositive={compareTrace.duration <= baseTrace!.duration}
                  />
                  <StatCard
                    label="Span Count Change"
                    value={`${compareTrace.spanCount - baseTrace!.spanCount >= 0 ? '+' : ''}${compareTrace.spanCount - baseTrace!.spanCount}`}
                    isPositive={compareTrace.spanCount <= baseTrace!.spanCount}
                  />
                  <StatCard
                    label="Error Count Change"
                    value={`${compareTrace.errorCount - baseTrace!.errorCount >= 0 ? '+' : ''}${compareTrace.errorCount - baseTrace!.errorCount}`}
                    isPositive={compareTrace.errorCount <= baseTrace!.errorCount}
                  />
                </div>

                {/* Operation Diffs */}
                <div className="space-y-2">
                  <h3 className="text-sm font-medium text-gray-400 mb-3">Operation Differences</h3>
                  {diffs.map((diff, idx) => (
                    <div
                      key={idx}
                      className={clsx(
                        'rounded-lg border overflow-hidden',
                        diff.statusChanged
                          ? 'border-yellow-700 bg-yellow-900/10'
                          : !diff.baseSpan
                          ? 'border-green-700 bg-green-900/10'
                          : !diff.compareSpan
                          ? 'border-red-700 bg-red-900/10'
                          : 'border-gray-700 bg-gray-800/50'
                      )}
                    >
                      <button
                        onClick={() => toggleOp(`${diff.service}::${diff.operation}`)}
                        className="w-full flex items-center justify-between p-3 hover:bg-gray-800/50"
                      >
                        <div className="flex items-center gap-3">
                          {expandedOps.has(`${diff.service}::${diff.operation}`) ? (
                            <ChevronDown className="w-4 h-4 text-gray-500" />
                          ) : (
                            <ChevronRight className="w-4 h-4 text-gray-500" />
                          )}
                          <div>
                            <div className="text-sm text-white">{diff.operation}</div>
                            <div className="text-xs text-gray-500">{diff.service}</div>
                          </div>
                        </div>
                        <div className="flex items-center gap-4">
                          {!diff.baseSpan && (
                            <span className="px-2 py-0.5 bg-green-900/50 text-green-300 text-xs rounded">
                              New
                            </span>
                          )}
                          {!diff.compareSpan && (
                            <span className="px-2 py-0.5 bg-red-900/50 text-red-300 text-xs rounded">
                              Removed
                            </span>
                          )}
                          {diff.statusChanged && (
                            <span className="px-2 py-0.5 bg-yellow-900/50 text-yellow-300 text-xs rounded">
                              Status Changed
                            </span>
                          )}
                          {diff.durationDiff !== undefined && diff.durationDiff !== 0 && (
                            <div className="flex items-center gap-1">
                              {diff.durationDiff > 0 ? (
                                <TrendingUp className="w-4 h-4 text-red-400" />
                              ) : (
                                <TrendingDown className="w-4 h-4 text-green-400" />
                              )}
                              <span
                                className={clsx(
                                  'text-sm font-mono',
                                  diff.durationDiff > 0 ? 'text-red-400' : 'text-green-400'
                                )}
                              >
                                {formatDiff(diff.durationDiff, diff.durationDiffPercent || 0)}
                              </span>
                            </div>
                          )}
                        </div>
                      </button>

                      {expandedOps.has(`${diff.service}::${diff.operation}`) && (
                        <div className="px-3 pb-3 grid grid-cols-2 gap-4 border-t border-gray-700/50 pt-3">
                          <div>
                            <div className="text-xs text-blue-400 mb-2">Base</div>
                            {diff.baseSpan ? (
                              <div className="text-sm">
                                <div className="flex items-center gap-2 mb-1">
                                  <Clock className="w-3 h-3 text-gray-500" />
                                  <span className="text-gray-300">{formatDuration(diff.baseSpan.duration)}</span>
                                </div>
                                <div className="flex items-center gap-2">
                                  {diff.baseSpan.status === 'ERROR' ? (
                                    <AlertCircle className="w-3 h-3 text-red-400" />
                                  ) : (
                                    <CheckCircle className="w-3 h-3 text-green-400" />
                                  )}
                                  <span className={diff.baseSpan.status === 'ERROR' ? 'text-red-400' : 'text-green-400'}>
                                    {diff.baseSpan.status}
                                  </span>
                                </div>
                              </div>
                            ) : (
                              <div className="text-sm text-gray-500 italic">Not present</div>
                            )}
                          </div>
                          <div>
                            <div className="text-xs text-purple-400 mb-2">Compare</div>
                            {diff.compareSpan ? (
                              <div className="text-sm">
                                <div className="flex items-center gap-2 mb-1">
                                  <Clock className="w-3 h-3 text-gray-500" />
                                  <span className="text-gray-300">{formatDuration(diff.compareSpan.duration)}</span>
                                </div>
                                <div className="flex items-center gap-2">
                                  {diff.compareSpan.status === 'ERROR' ? (
                                    <AlertCircle className="w-3 h-3 text-red-400" />
                                  ) : (
                                    <CheckCircle className="w-3 h-3 text-green-400" />
                                  )}
                                  <span className={diff.compareSpan.status === 'ERROR' ? 'text-red-400' : 'text-green-400'}>
                                    {diff.compareSpan.status}
                                  </span>
                                </div>
                              </div>
                            ) : (
                              <div className="text-sm text-gray-500 italic">Not present</div>
                            )}
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function StatCard({
  label,
  value,
  isPositive,
}: {
  label: string;
  value: string;
  isPositive: boolean;
}) {
  return (
    <div className="p-3 bg-gray-800 rounded-lg">
      <div className="text-xs text-gray-400 mb-1">{label}</div>
      <div className={clsx('text-lg font-mono', isPositive ? 'text-green-400' : 'text-red-400')}>
        {value}
      </div>
    </div>
  );
}

export default TraceComparison;
