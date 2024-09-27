import { useState, useEffect, useMemo } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  Link2, Search, AlertTriangle, CheckCircle, GitBranch, Loader2,
  RefreshCw, ChevronRight, ChevronLeft, FileText, Activity, Layers,
  Share2, ArrowRight, Copy, ExternalLink, ChevronDown, ChevronUp, Database, Terminal
} from 'lucide-react';
import { clsx } from 'clsx';
import { getApiUrl } from '../lib/config';

const API_URL = getApiUrl();

// Types
interface CorrelationSummary {
  correlationId: string;
  firstSeen: string;
  lastSeen: string;
  services: string[];
  spanCount: number;
  errorCount: number;
  duration: number;
}

interface SpanData {
  traceId: string;
  spanId: string;
  parentSpanId: string;
  serviceName: string;
  operationName: string;
  duration: number;
  status: string;
  timestamp: string;
  attributes?: Record<string, string>;
}

interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
  service: string;
  traceId?: string;
  spanId?: string;
  attributes?: Record<string, string>;
}

interface CorrelationDetails {
  correlationId: string;
  firstSeen: string;
  lastSeen: string;
  services: string[];
  spanCount: number;
  errorCount: number;
  totalDuration: number;
  spans: SpanData[];
  logs: LogEntry[];
}

// Waterfall Timeline Component
function WaterfallTimeline({ spans, totalDuration }: { spans: SpanData[]; totalDuration: number }) {
  if (!spans || spans.length === 0) {
    return (
      <div className="flex items-center justify-center h-48 text-gray-400">
        <Activity className="w-6 h-6 mr-2 opacity-50" />
        <span>No spans available</span>
      </div>
    );
  }

  // Sort by timestamp and build tree structure
  const sortedSpans = [...spans].sort((a, b) =>
    new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
  );

  const minTime = new Date(sortedSpans[0].timestamp).getTime();
  const maxTime = minTime + (totalDuration / 1000000); // Convert ns to ms
  const timeRange = maxTime - minTime || 1;

  // Build parent-child map
  const childrenMap = new Map<string, SpanData[]>();
  const rootSpans: SpanData[] = [];

  sortedSpans.forEach(span => {
    if (!span.parentSpanId || span.parentSpanId === '') {
      rootSpans.push(span);
    } else {
      const children = childrenMap.get(span.parentSpanId) || [];
      children.push(span);
      childrenMap.set(span.parentSpanId, children);
    }
  });

  // Flatten tree with depth info
  interface FlatSpan extends SpanData {
    depth: number;
  }

  const flattenTree = (span: SpanData, depth: number): FlatSpan[] => {
    const result: FlatSpan[] = [{ ...span, depth }];
    const children = childrenMap.get(span.spanId) || [];
    children.forEach(child => {
      result.push(...flattenTree(child, depth + 1));
    });
    return result;
  };

  const flatSpans: FlatSpan[] = rootSpans.flatMap(s => flattenTree(s, 0));

  // If no clear hierarchy, just use all spans
  const displaySpans = flatSpans.length > 0 ? flatSpans : sortedSpans.map(s => ({ ...s, depth: 0 }));

  const formatDuration = (ns: number) => {
    const ms = ns / 1000000;
    if (ms < 1) return `${Math.round(ns / 1000)}µs`;
    if (ms < 1000) return `${ms.toFixed(1)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const getServiceColor = (service: string) => {
    const colors = [
      'bg-blue-500', 'bg-green-500', 'bg-purple-500', 'bg-orange-500',
      'bg-pink-500', 'bg-cyan-500', 'bg-yellow-500', 'bg-indigo-500'
    ];
    let hash = 0;
    for (let i = 0; i < service.length; i++) {
      hash = service.charCodeAt(i) + ((hash << 5) - hash);
    }
    return colors[Math.abs(hash) % colors.length];
  };

  return (
    <div className="space-y-1">
      {/* Timeline header */}
      <div className="flex items-center text-xs text-gray-500 mb-3 pl-48">
        <div className="flex-1 flex justify-between">
          <span>0ms</span>
          <span>{formatDuration(totalDuration)}</span>
        </div>
      </div>

      {/* Spans */}
      {displaySpans.map((span, idx) => {
        const spanStart = new Date(span.timestamp).getTime() - minTime;
        const spanDuration = span.duration / 1000000; // ns to ms
        const leftPercent = (spanStart / timeRange) * 100;
        const widthPercent = Math.max((spanDuration / timeRange) * 100, 0.5);
        const isError = span.status === 'STATUS_CODE_ERROR' || span.status === 'error';

        return (
          <div
            key={span.spanId || idx}
            className={clsx(
              'flex items-center h-8 hover:bg-gray-800/50 rounded transition-colors group',
              isError && 'bg-red-900/10'
            )}
          >
            {/* Service & Operation */}
            <div
              className="w-48 flex-shrink-0 flex items-center gap-1 pr-2 overflow-hidden"
              style={{ paddingLeft: `${span.depth * 12}px` }}
            >
              {span.depth > 0 && (
                <div className="w-2 h-2 border-l border-b border-gray-600 mr-1" />
              )}
              <div className={clsx('w-2 h-2 rounded-full flex-shrink-0', getServiceColor(span.serviceName))} />
              <span className="text-xs text-gray-400 truncate" title={span.serviceName}>
                {span.serviceName}
              </span>
              <span className="text-xs text-gray-600 mx-0.5">:</span>
              <span className="text-xs text-white truncate" title={span.operationName}>
                {span.operationName}
              </span>
            </div>

            {/* Timeline bar */}
            <div className="flex-1 relative h-5">
              <div
                className={clsx(
                  'absolute h-full rounded-sm transition-all',
                  isError ? 'bg-red-500/80' : getServiceColor(span.serviceName).replace('bg-', 'bg-') + '/70'
                )}
                style={{
                  left: `${leftPercent}%`,
                  width: `${widthPercent}%`,
                  minWidth: '2px'
                }}
              >
                {/* Duration label */}
                <span className="absolute right-full mr-1 text-xs text-gray-400 whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity">
                  {formatDuration(span.duration)}
                </span>
              </div>
            </div>

            {/* Duration */}
            <div className="w-20 flex-shrink-0 text-right text-xs text-gray-400 pl-2">
              {formatDuration(span.duration)}
            </div>

            {/* Status */}
            <div className="w-6 flex-shrink-0 flex justify-center">
              {isError ? (
                <AlertTriangle className="w-3 h-3 text-red-400" />
              ) : (
                <CheckCircle className="w-3 h-3 text-green-400" />
              )}
            </div>
          </div>
        );
      })}

      {/* Service Legend */}
      <div className="flex flex-wrap gap-3 mt-4 pt-3 border-t border-gray-700">
        {Array.from(new Set(displaySpans.map(s => s.serviceName))).map(service => (
          <div key={service} className="flex items-center gap-1.5 text-xs text-gray-400">
            <div className={clsx('w-2.5 h-2.5 rounded-full', getServiceColor(service))} />
            <span>{service}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

// Service Flow Component
function ServiceFlow({ spans, services }: { spans: SpanData[]; services: string[] }) {
  // Build service call graph
  const serviceMetrics = useMemo(() => {
    const metrics = new Map<string, { calls: number; errors: number; latency: number }>();

    services.forEach(svc => {
      metrics.set(svc, { calls: 0, errors: 0, latency: 0 });
    });

    spans.forEach(span => {
      const m = metrics.get(span.serviceName);
      if (m) {
        m.calls++;
        if (span.status === 'STATUS_CODE_ERROR' || span.status === 'error') {
          m.errors++;
        }
        m.latency += span.duration / 1000000;
      }
    });

    return metrics;
  }, [spans, services]);

  const getStatus = (svc: string) => {
    const m = serviceMetrics.get(svc);
    if (!m || m.calls === 0) return 'healthy';
    const errorRate = m.errors / m.calls;
    if (errorRate > 0.1) return 'error';
    if (errorRate > 0) return 'degraded';
    return 'healthy';
  };

  const statusColors = {
    healthy: 'border-green-500 bg-green-500/10',
    degraded: 'border-yellow-500 bg-yellow-500/10',
    error: 'border-red-500 bg-red-500/10',
  };

  if (services.length === 0) {
    return (
      <div className="flex items-center justify-center h-48 text-gray-400">
        <Share2 className="w-6 h-6 mr-2 opacity-50" />
        <span>No service flow data</span>
      </div>
    );
  }

  return (
    <div className="p-4">
      <div className="flex items-center gap-2 mb-4 text-sm text-gray-400">
        <span>Request Flow</span>
        <ArrowRight className="w-4 h-4" />
      </div>

      <div className="flex flex-wrap items-center gap-3">
        {services.map((service, idx) => {
          const metrics = serviceMetrics.get(service);
          const status = getStatus(service);

          return (
            <div key={service} className="flex items-center">
              <div
                className={clsx(
                  'px-4 py-3 rounded-lg border-2 min-w-[140px]',
                  statusColors[status]
                )}
              >
                <div className="font-medium text-sm text-white mb-2 truncate">
                  {service}
                </div>
                <div className="grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
                  <span className="text-gray-400">Spans:</span>
                  <span className="text-white text-right">{metrics?.calls || 0}</span>
                  <span className="text-gray-400">Latency:</span>
                  <span className="text-white text-right">
                    {metrics && metrics.calls > 0 ? `${(metrics.latency / metrics.calls).toFixed(0)}ms` : '0ms'}
                  </span>
                  {(metrics?.errors || 0) > 0 && (
                    <>
                      <span className="text-red-400">Errors:</span>
                      <span className="text-red-400 text-right">{metrics?.errors}</span>
                    </>
                  )}
                </div>
              </div>

              {idx < services.length - 1 && (
                <div className="flex items-center px-2 text-gray-600">
                  <div className="w-6 h-0.5 bg-gray-600" />
                  <ChevronRight className="w-4 h-4 -ml-1" />
                </div>
              )}
            </div>
          );
        })}
      </div>

      {/* Legend */}
      <div className="flex items-center gap-6 mt-6 pt-4 border-t border-gray-700 text-xs text-gray-400">
        <div className="flex items-center gap-2">
          <div className="w-3 h-3 rounded border-2 border-green-500 bg-green-500/10" />
          <span>Healthy</span>
        </div>
        <div className="flex items-center gap-2">
          <div className="w-3 h-3 rounded border-2 border-yellow-500 bg-yellow-500/10" />
          <span>Degraded</span>
        </div>
        <div className="flex items-center gap-2">
          <div className="w-3 h-3 rounded border-2 border-red-500 bg-red-500/10" />
          <span>Error</span>
        </div>
      </div>
    </div>
  );
}

// Logs Panel Component
function LogsPanel({ logs, filter }: { logs: LogEntry[]; filter: string }) {
  const [expandedLogs, setExpandedLogs] = useState<Set<number>>(new Set());
  const [levelFilter, setLevelFilter] = useState<string>('all');

  const filteredLogs = useMemo(() => {
    return logs.filter(log => {
      if (levelFilter !== 'all' && log.level.toUpperCase() !== levelFilter) return false;
      if (filter && !log.message.toLowerCase().includes(filter.toLowerCase()) &&
          !log.service.toLowerCase().includes(filter.toLowerCase())) return false;
      return true;
    });
  }, [logs, filter, levelFilter]);

  const levelCounts = useMemo(() => {
    const counts: Record<string, number> = { all: logs.length };
    logs.forEach(log => {
      const level = log.level.toUpperCase();
      counts[level] = (counts[level] || 0) + 1;
    });
    return counts;
  }, [logs]);

  const getLevelColor = (level: string) => {
    switch (level.toUpperCase()) {
      case 'ERROR':
      case 'FATAL':
        return 'bg-red-900/50 text-red-300 border-red-800';
      case 'WARN':
      case 'WARNING':
        return 'bg-yellow-900/50 text-yellow-300 border-yellow-800';
      case 'INFO':
        return 'bg-blue-900/50 text-blue-300 border-blue-800';
      case 'DEBUG':
        return 'bg-gray-700 text-gray-300 border-gray-600';
      default:
        return 'bg-gray-700 text-gray-300 border-gray-600';
    }
  };

  const formatTime = (ts: string) => {
    const date = new Date(ts);
    const time = date.toLocaleTimeString('en-US', {
      hour12: false,
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
    const ms = date.getMilliseconds().toString().padStart(3, '0');
    return `${time}.${ms}`;
  };

  if (logs.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-48 text-gray-400">
        <FileText className="w-8 h-8 mb-2 opacity-50" />
        <p className="text-sm">No logs found for this correlation</p>
        <p className="text-xs text-gray-500 mt-1">Logs are linked via trace context</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Level filter tabs */}
      <div className="flex items-center gap-1 mb-3 overflow-x-auto pb-2">
        {['all', 'ERROR', 'WARN', 'INFO', 'DEBUG'].map(level => (
          <button
            key={level}
            onClick={() => setLevelFilter(level)}
            className={clsx(
              'px-3 py-1.5 text-xs font-medium rounded transition-colors whitespace-nowrap',
              levelFilter === level
                ? level === 'ERROR' ? 'bg-red-600 text-white'
                : level === 'WARN' ? 'bg-yellow-600 text-white'
                : 'bg-blue-600 text-white'
                : 'bg-gray-700 text-gray-400 hover:bg-gray-600'
            )}
          >
            {level === 'all' ? 'All' : level}
            <span className="ml-1 opacity-70">({levelCounts[level] || 0})</span>
          </button>
        ))}
      </div>

      {/* Logs list */}
      <div className="flex-1 overflow-auto space-y-1.5">
        {filteredLogs.map((log, idx) => {
          const isExpanded = expandedLogs.has(idx);
          const isError = log.level.toUpperCase() === 'ERROR' || log.level.toUpperCase() === 'FATAL';

          return (
            <div
              key={idx}
              className={clsx(
                'rounded-lg border transition-colors',
                isError ? 'border-red-800/50 bg-red-900/20' : 'border-gray-700 bg-gray-800/50',
                'hover:border-gray-600'
              )}
            >
              <button
                onClick={() => {
                  const next = new Set(expandedLogs);
                  if (isExpanded) next.delete(idx);
                  else next.add(idx);
                  setExpandedLogs(next);
                }}
                className="w-full text-left p-2"
              >
                <div className="flex items-start gap-2">
                  <span className="text-xs text-gray-500 font-mono flex-shrink-0 w-20">
                    {formatTime(log.timestamp)}
                  </span>
                  <span className={clsx(
                    'text-xs font-medium px-1.5 py-0.5 rounded flex-shrink-0',
                    getLevelColor(log.level)
                  )}>
                    {log.level.toUpperCase().slice(0, 4)}
                  </span>
                  <span className="text-xs text-cyan-400 flex-shrink-0 w-24 truncate" title={log.service}>
                    {log.service}
                  </span>
                  <span className={clsx(
                    'text-xs flex-1 truncate font-mono',
                    isError ? 'text-red-300' : 'text-gray-300'
                  )}>
                    {log.message}
                  </span>
                  {isExpanded ? (
                    <ChevronUp className="w-4 h-4 text-gray-500 flex-shrink-0" />
                  ) : (
                    <ChevronDown className="w-4 h-4 text-gray-500 flex-shrink-0" />
                  )}
                </div>
              </button>

              {isExpanded && (
                <div className="px-3 pb-3 pt-1 border-t border-gray-700/50">
                  <pre className="text-xs text-gray-300 font-mono whitespace-pre-wrap break-all bg-gray-900/50 p-2 rounded">
                    {log.message}
                  </pre>
                  {log.traceId && (
                    <div className="mt-2 flex items-center gap-4 text-xs text-gray-500">
                      <span>Trace: <span className="text-blue-400 font-mono">{log.traceId.slice(0, 16)}...</span></span>
                      {log.spanId && <span>Span: <span className="text-purple-400 font-mono">{log.spanId.slice(0, 8)}</span></span>}
                    </div>
                  )}
                  {log.attributes && Object.keys(log.attributes).length > 0 && (
                    <div className="mt-2 text-xs">
                      <div className="text-gray-500 mb-1">Attributes:</div>
                      <div className="grid grid-cols-2 gap-1">
                        {Object.entries(log.attributes).slice(0, 6).map(([k, v]) => (
                          <div key={k} className="truncate">
                            <span className="text-gray-500">{k}:</span>{' '}
                            <span className="text-gray-300">{v}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// Traces Panel Component
function TracesPanel({ spans, onNavigate }: { spans: SpanData[]; onNavigate: (traceId: string) => void }) {
  // Group spans by traceId
  const traceGroups = useMemo(() => {
    const groups = new Map<string, SpanData[]>();
    spans.forEach(span => {
      const existing = groups.get(span.traceId) || [];
      existing.push(span);
      groups.set(span.traceId, existing);
    });
    return Array.from(groups.entries()).map(([traceId, spans]) => ({
      traceId,
      spans,
      rootSpan: spans.find(s => !s.parentSpanId || s.parentSpanId === '') || spans[0],
      totalDuration: Math.max(...spans.map(s => s.duration)),
      hasError: spans.some(s => s.status === 'STATUS_CODE_ERROR' || s.status === 'error'),
      services: [...new Set(spans.map(s => s.serviceName))],
    }));
  }, [spans]);

  const formatDuration = (ns: number) => {
    const ms = ns / 1000000;
    if (ms < 1) return `${Math.round(ns / 1000)}µs`;
    if (ms < 1000) return `${ms.toFixed(1)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  if (traceGroups.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-48 text-gray-400">
        <GitBranch className="w-8 h-8 mb-2 opacity-50" />
        <p className="text-sm">No traces found</p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {traceGroups.map((trace) => (
        <button
          key={trace.traceId}
          onClick={() => onNavigate(trace.traceId)}
          className={clsx(
            'w-full text-left p-3 rounded-lg border transition-colors',
            trace.hasError
              ? 'border-red-800/50 bg-red-900/20 hover:bg-red-900/30'
              : 'border-gray-700 bg-gray-800/50 hover:bg-gray-700/50'
          )}
        >
          <div className="flex items-center justify-between mb-2">
            <div className="flex items-center gap-2">
              {trace.hasError ? (
                <AlertTriangle className="w-4 h-4 text-red-400" />
              ) : (
                <CheckCircle className="w-4 h-4 text-green-400" />
              )}
              <span className="font-mono text-sm text-blue-400">
                {trace.traceId.slice(0, 16)}...
              </span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-sm text-gray-400">{formatDuration(trace.totalDuration)}</span>
              <ExternalLink className="w-4 h-4 text-gray-500" />
            </div>
          </div>

          <div className="flex items-center gap-3 text-xs text-gray-500">
            <span className="flex items-center gap-1">
              <Database className="w-3 h-3" />
              {trace.rootSpan?.serviceName}
            </span>
            <span className="flex items-center gap-1">
              <Terminal className="w-3 h-3" />
              {trace.rootSpan?.operationName}
            </span>
            <span className="flex items-center gap-1">
              <Layers className="w-3 h-3" />
              {trace.spans.length} spans
            </span>
            <span className="flex items-center gap-1">
              <GitBranch className="w-3 h-3" />
              {trace.services.length} services
            </span>
          </div>

          {/* Mini service flow */}
          <div className="flex items-center gap-1 mt-2">
            {trace.services.slice(0, 5).map((svc, idx) => (
              <span key={svc} className="flex items-center">
                <span className="px-1.5 py-0.5 text-xs bg-gray-700 rounded text-gray-300">
                  {svc}
                </span>
                {idx < Math.min(trace.services.length, 5) - 1 && (
                  <ChevronRight className="w-3 h-3 text-gray-600 mx-0.5" />
                )}
              </span>
            ))}
            {trace.services.length > 5 && (
              <span className="text-xs text-gray-500">+{trace.services.length - 5}</span>
            )}
          </div>
        </button>
      ))}
    </div>
  );
}

// Main Component
export default function CorrelationsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // State
  const [correlations, setCorrelations] = useState<CorrelationSummary[]>([]);
  const [selectedCorrelation, setSelectedCorrelation] = useState<string | null>(searchParams.get('id'));
  const [details, setDetails] = useState<CorrelationDetails | null>(null);
  const [loading, setLoading] = useState(true);
  const [detailsLoading, setDetailsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // UI State
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [activeTab, setActiveTab] = useState<'flow' | 'timeline' | 'traces' | 'logs'>('timeline');
  const [searchQuery, setSearchQuery] = useState('');
  const [hasErrorsOnly, setHasErrorsOnly] = useState(false);
  const [logFilter] = useState('');

  // Fetch correlations
  useEffect(() => {
    fetchCorrelations();
    const interval = setInterval(fetchCorrelations, 30000);
    return () => clearInterval(interval);
  }, [hasErrorsOnly]);

  // Fetch details when selection changes
  useEffect(() => {
    if (selectedCorrelation) {
      fetchDetails(selectedCorrelation);
      setSearchParams({ id: selectedCorrelation });
    } else {
      setSearchParams({});
      setDetails(null);
    }
  }, [selectedCorrelation]);

  const fetchCorrelations = async () => {
    try {
      setLoading(true);
      const params = new URLSearchParams({ limit: '100' });
      if (hasErrorsOnly) params.append('hasErrors', 'true');

      const response = await fetch(`${API_URL}/api/v1/correlate?${params}`);
      if (!response.ok) throw new Error('Failed to fetch correlations');

      const data = await response.json();
      setCorrelations(data.correlations || []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load correlations');
    } finally {
      setLoading(false);
    }
  };

  const fetchDetails = async (correlationId: string) => {
    try {
      setDetailsLoading(true);
      console.log('Fetching details for:', correlationId);

      const [contextRes, tracesRes, logsRes] = await Promise.all([
        fetch(`${API_URL}/api/v1/correlate/${correlationId}?details=true`),
        fetch(`${API_URL}/api/v1/correlate/${correlationId}/traces`),
        fetch(`${API_URL}/api/v1/correlate/${correlationId}/logs`),
      ]);

      console.log('API responses:', { contextOk: contextRes.ok, tracesOk: tracesRes.ok, logsOk: logsRes.ok });

      const context = contextRes.ok ? await contextRes.json() : null;
      const tracesData = tracesRes.ok ? await tracesRes.json() : { traces: [] };
      const logsData = logsRes.ok ? await logsRes.json() : { logs: [] };

      console.log('Parsed data:', { context, tracesData, logsData });

      // Map traces to spans format for the waterfall timeline
      const spans = (tracesData.traces || []).map((t: any) => ({
        traceId: t.traceId,
        spanId: t.spanId || t.traceId,
        parentSpanId: t.parentSpanId || '',
        serviceName: t.serviceName,
        operationName: t.operationName,
        duration: t.duration,
        status: t.statusCode || t.status,
        timestamp: t.timestamp,
        attributes: t.attributes,
      }));

      const detailsObj = {
        correlationId,
        firstSeen: context?.firstSeen || '',
        lastSeen: context?.lastSeen || '',
        services: context?.services || [],
        spanCount: context?.spanCount || spans.length,
        errorCount: context?.errorCount || spans.filter((s: SpanData) =>
          s.status === 'STATUS_CODE_ERROR' || s.status === 'error'
        ).length,
        totalDuration: context?.duration || Math.max(...spans.map((s: SpanData) => s.duration), 0),
        spans,
        logs: logsData.logs || [],
      };
      console.log('Setting details:', detailsObj);
      setDetails(detailsObj);
    } catch (err) {
      console.error('Failed to fetch correlation details:', err);
    } finally {
      setDetailsLoading(false);
    }
  };

  const formatTimestamp = (ts: string) => {
    const date = new Date(ts);
    return date.toLocaleTimeString('en-US', { hour12: true, hour: 'numeric', minute: '2-digit', second: '2-digit' });
  };

  const formatDuration = (ns: number) => {
    const ms = ns / 1000000;
    if (ms < 1) return `${Math.round(ns / 1000)}µs`;
    if (ms < 1000) return `${ms.toFixed(0)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  const filteredCorrelations = correlations.filter(c =>
    !searchQuery || c.correlationId.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="h-[calc(100vh-7rem)] flex flex-col">
      {/* Minimal Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <Link2 className="w-5 h-5 text-blue-400" />
          <h1 className="text-xl font-semibold">Correlations</h1>
          <span className="text-sm text-gray-500">
            {correlations.length} flows
          </span>
        </div>
        <button
          onClick={fetchCorrelations}
          className="flex items-center gap-2 px-3 py-1.5 bg-gray-700 hover:bg-gray-600 rounded-lg text-sm"
        >
          <RefreshCw className={clsx('w-4 h-4', loading && 'animate-spin')} />
          Refresh
        </button>
      </div>

      {error && (
        <div className="bg-red-900/20 border border-red-500 text-red-400 px-4 py-2 rounded mb-4 text-sm">
          {error}
        </div>
      )}

      <div className="flex-1 flex gap-4 overflow-hidden">
        {/* Collapsible Sidebar */}
        <div
          className={clsx(
            'flex flex-col bg-gray-800 rounded-lg overflow-hidden transition-all duration-300',
            sidebarCollapsed ? 'w-12' : 'w-80'
          )}
        >
          {/* Sidebar Header */}
          <div className="flex items-center justify-between p-2 border-b border-gray-700">
            {!sidebarCollapsed && (
              <div className="flex-1 pr-2">
                <div className="relative">
                  <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
                  <input
                    type="text"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    placeholder="Search..."
                    className="w-full pl-8 pr-3 py-1.5 bg-gray-700 border border-gray-600 rounded text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
                  />
                </div>
              </div>
            )}
            <button
              onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
              className="p-1.5 hover:bg-gray-700 rounded"
            >
              {sidebarCollapsed ? <ChevronRight className="w-4 h-4" /> : <ChevronLeft className="w-4 h-4" />}
            </button>
          </div>

          {/* Filters */}
          {!sidebarCollapsed && (
            <div className="px-3 py-2 border-b border-gray-700">
              <label className="flex items-center gap-2 text-xs text-gray-400 cursor-pointer">
                <input
                  type="checkbox"
                  checked={hasErrorsOnly}
                  onChange={(e) => setHasErrorsOnly(e.target.checked)}
                  className="rounded bg-gray-700 border-gray-600 text-red-500 focus:ring-red-500"
                />
                Errors only
              </label>
            </div>
          )}

          {/* Correlation List */}
          <div className="flex-1 overflow-auto">
            {loading && correlations.length === 0 ? (
              <div className="flex items-center justify-center h-24">
                <Loader2 className="w-5 h-5 animate-spin text-blue-500" />
              </div>
            ) : sidebarCollapsed ? (
              <div className="p-2 space-y-2">
                {filteredCorrelations.slice(0, 20).map((c) => (
                  <button
                    key={c.correlationId}
                    onClick={() => setSelectedCorrelation(c.correlationId)}
                    className={clsx(
                      'w-8 h-8 rounded flex items-center justify-center',
                      selectedCorrelation === c.correlationId ? 'bg-blue-600' : 'bg-gray-700 hover:bg-gray-600',
                      c.errorCount > 0 && 'border border-red-500'
                    )}
                  >
                    {c.errorCount > 0 ? (
                      <AlertTriangle className="w-4 h-4 text-red-400" />
                    ) : (
                      <CheckCircle className="w-4 h-4 text-green-400" />
                    )}
                  </button>
                ))}
              </div>
            ) : (
              <div className="divide-y divide-gray-700/50">
                {filteredCorrelations.map((c) => (
                  <button
                    key={c.correlationId}
                    onClick={() => setSelectedCorrelation(c.correlationId)}
                    className={clsx(
                      'w-full text-left p-3 hover:bg-gray-750 transition-colors',
                      selectedCorrelation === c.correlationId && 'bg-blue-900/30 border-l-2 border-blue-500'
                    )}
                  >
                    <div className="flex items-center gap-2 mb-1">
                      {c.errorCount > 0 ? (
                        <AlertTriangle className="w-3.5 h-3.5 text-red-400 flex-shrink-0" />
                      ) : (
                        <CheckCircle className="w-3.5 h-3.5 text-green-400 flex-shrink-0" />
                      )}
                      <span className="font-mono text-xs text-blue-400 truncate">
                        {c.correlationId}
                      </span>
                    </div>
                    <div className="flex items-center gap-3 text-xs text-gray-500 pl-5">
                      <span>{c.services?.length || 0} svc</span>
                      <span>{c.spanCount} spans</span>
                      {c.errorCount > 0 && (
                        <span className="text-red-400">{c.errorCount} err</span>
                      )}
                    </div>
                    <div className="text-xs text-gray-600 pl-5 mt-0.5">
                      {formatTimestamp(c.firstSeen)}
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Main Content */}
        <div className="flex-1 bg-gray-800 rounded-lg overflow-hidden flex flex-col">
          {!selectedCorrelation ? (
            <div className="flex flex-col items-center justify-center h-full text-gray-400">
              <Link2 className="w-12 h-12 mb-4 opacity-30" />
              <p className="text-lg">Select a correlation to view details</p>
              <p className="text-sm text-gray-500 mt-1">
                Choose from the list on the left
              </p>
            </div>
          ) : detailsLoading ? (
            <div className="flex items-center justify-center h-full">
              <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
            </div>
          ) : details ? (
            <>
              {/* Compact Header */}
              <div className="px-4 py-3 border-b border-gray-700 flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-sm text-blue-400">{details.correlationId}</span>
                      <button
                        onClick={() => copyToClipboard(details.correlationId)}
                        className="p-1 hover:bg-gray-700 rounded"
                      >
                        <Copy className="w-3 h-3 text-gray-500" />
                      </button>
                    </div>
                  </div>

                  {details.errorCount > 0 ? (
                    <span className="px-2 py-0.5 text-xs bg-red-900/50 text-red-400 rounded">
                      {details.errorCount} errors
                    </span>
                  ) : (
                    <span className="px-2 py-0.5 text-xs bg-green-900/50 text-green-400 rounded">
                      Healthy
                    </span>
                  )}
                </div>

                {/* Quick Stats */}
                <div className="flex items-center gap-6 text-sm">
                  <div className="text-center">
                    <div className="text-gray-500 text-xs">Services</div>
                    <div className="font-semibold">{details.services.length}</div>
                  </div>
                  <div className="text-center">
                    <div className="text-gray-500 text-xs">Spans</div>
                    <div className="font-semibold">{details.spanCount}</div>
                  </div>
                  <div className="text-center">
                    <div className="text-gray-500 text-xs">Duration</div>
                    <div className="font-semibold">{formatDuration(details.totalDuration)}</div>
                  </div>
                  <div className="text-center">
                    <div className="text-gray-500 text-xs">First Seen</div>
                    <div className="font-semibold text-xs">{formatTimestamp(details.firstSeen)}</div>
                  </div>
                </div>
              </div>

              {/* Tabs */}
              <div className="flex border-b border-gray-700 px-2">
                {[
                  { id: 'timeline', icon: Activity, label: 'Timeline' },
                  { id: 'flow', icon: Share2, label: 'Flow' },
                  { id: 'traces', icon: GitBranch, label: 'Traces', count: details.spans.length },
                  { id: 'logs', icon: FileText, label: 'Logs', count: details.logs.length },
                ].map(tab => (
                  <button
                    key={tab.id}
                    onClick={() => setActiveTab(tab.id as any)}
                    className={clsx(
                      'flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors',
                      activeTab === tab.id
                        ? 'border-blue-500 text-blue-400'
                        : 'border-transparent text-gray-400 hover:text-white'
                    )}
                  >
                    <tab.icon className="w-4 h-4" />
                    {tab.label}
                    {tab.count !== undefined && (
                      <span className={clsx(
                        'px-1.5 py-0.5 text-xs rounded',
                        activeTab === tab.id ? 'bg-blue-900/50' : 'bg-gray-700'
                      )}>
                        {tab.count}
                      </span>
                    )}
                  </button>
                ))}
              </div>

              {/* Tab Content */}
              <div className="flex-1 overflow-auto p-4">
                {activeTab === 'timeline' && (
                  <WaterfallTimeline spans={details.spans} totalDuration={details.totalDuration} />
                )}
                {activeTab === 'flow' && (
                  <ServiceFlow spans={details.spans} services={details.services} />
                )}
                {activeTab === 'traces' && (
                  <TracesPanel spans={details.spans} onNavigate={(id) => navigate(`/traces/${id}`)} />
                )}
                {activeTab === 'logs' && (
                  <LogsPanel logs={details.logs} filter={logFilter} />
                )}
              </div>
            </>
          ) : null}
        </div>
      </div>
    </div>
  );
}
