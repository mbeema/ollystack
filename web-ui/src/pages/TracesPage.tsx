import { useState, useEffect } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Search, ChevronRight, AlertCircle, CheckCircle, Loader2, GitBranch, TrendingUp, Clock, Activity, AlertTriangle, Zap, GitCompare, BarChart3, List } from 'lucide-react';
import clsx from 'clsx';
import { getApiUrl } from '../lib/config';
import { TraceComparison } from '../components/trace/TraceComparison';
import { ServiceMetricsDashboard } from '../components/trace/ServiceMetricsDashboard';

interface Trace {
  traceId: string;
  serviceName: string;
  operationName: string;
  duration: number;
  spanCount: number;
  status: string;
  timestamp: string;
}

interface TraceStats {
  totalTraces: number;
  errorCount: number;
  errorRate: number;
  p50LatencyMs: number;
  p90LatencyMs: number;
  p99LatencyMs: number;
  serviceCount: number;
  services: Record<string, number>;
}

interface ErrorPattern {
  serviceName: string;
  operationName: string;
  count: number;
  firstSeen: string;
  lastSeen: string;
}

export default function TracesPage() {
  const [searchParams] = useSearchParams();
  const [traces, setTraces] = useState<Trace[]>([]);
  const [services, setServices] = useState<string[]>([]);
  const [stats, setStats] = useState<TraceStats | null>(null);
  const [errorPatterns, setErrorPatterns] = useState<ErrorPattern[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filters, setFilters] = useState({
    service: searchParams.get('service') || '',
    minDuration: searchParams.get('minDuration') || '',
    status: searchParams.get('status') || '',
    search: searchParams.get('search') || '',
  });
  const [compareMode, setCompareMode] = useState(false);
  const [selectedForCompare, setSelectedForCompare] = useState<string | null>(null);
  const [showComparison, setShowComparison] = useState(false);
  const [compareTraceId, setCompareTraceId] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<'traces' | 'metrics'>('traces');

  const API_URL = getApiUrl();

  useEffect(() => {
    fetchData();
  }, [filters.service, filters.minDuration, filters.status]);

  const fetchData = async () => {
    try {
      setLoading(true);
      const params = new URLSearchParams();
      if (filters.service) params.set('service', filters.service);
      if (filters.minDuration) params.set('minDuration', filters.minDuration);
      if (filters.status) params.set('status', filters.status);

      // Fetch all data in parallel
      const [tracesRes, statsRes, patternsRes] = await Promise.all([
        fetch(`${API_URL}/api/v1/traces?${params}`),
        fetch(`${API_URL}/api/v1/traces/stats`),
        fetch(`${API_URL}/api/v1/traces/errors/patterns`),
      ]);

      if (!tracesRes.ok) throw new Error('Failed to fetch traces');

      const tracesData = await tracesRes.json();
      setTraces(tracesData.traces || []);

      const uniqueServices = [...new Set((tracesData.traces || []).map((t: Trace) => t.serviceName))];
      setServices(uniqueServices as string[]);

      if (statsRes.ok) {
        const statsData = await statsRes.json();
        setStats(statsData);
      }

      if (patternsRes.ok) {
        const patternsData = await patternsRes.json();
        setErrorPatterns(patternsData.patterns || []);
      }

      setError(null);
    } catch (err) {
      console.error('Failed to fetch data:', err);
      setError('Failed to load traces');
    } finally {
      setLoading(false);
    }
  };

  const filteredTraces = traces.filter((trace) => {
    if (filters.search) {
      const search = filters.search.toLowerCase();
      return (
        trace.traceId.toLowerCase().includes(search) ||
        trace.serviceName.toLowerCase().includes(search) ||
        trace.operationName.toLowerCase().includes(search)
      );
    }
    return true;
  });

  const formatDuration = (ns: number) => {
    const ms = ns / 1000000;
    if (ms < 1) return `${Math.round(ns / 1000)}us`;
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const formatTime = (timestamp: string) => {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);

    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffMins < 1440) return `${Math.floor(diffMins / 60)}h ago`;
    return date.toLocaleDateString();
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold">Traces</h1>
          {/* View Mode Toggle */}
          <div className="flex bg-gray-800 rounded-lg p-1">
            <button
              onClick={() => setViewMode('traces')}
              className={clsx(
                'flex items-center gap-2 px-3 py-1.5 rounded text-sm transition-colors',
                viewMode === 'traces'
                  ? 'bg-blue-600 text-white'
                  : 'text-gray-400 hover:text-white'
              )}
            >
              <List className="w-4 h-4" />
              Trace List
            </button>
            <button
              onClick={() => setViewMode('metrics')}
              className={clsx(
                'flex items-center gap-2 px-3 py-1.5 rounded text-sm transition-colors',
                viewMode === 'metrics'
                  ? 'bg-blue-600 text-white'
                  : 'text-gray-400 hover:text-white'
              )}
            >
              <BarChart3 className="w-4 h-4" />
              Service Metrics
            </button>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {loading && <Loader2 className="w-4 h-4 animate-spin text-gray-400" />}
          {viewMode === 'traces' && (
            <>
              <button
                onClick={() => {
                  if (compareMode && selectedForCompare) {
                    setShowComparison(true);
                  }
                  setCompareMode(!compareMode);
                }}
                className={clsx(
                  'flex items-center gap-2 px-3 py-1.5 rounded text-sm',
                  compareMode
                    ? 'bg-purple-600 hover:bg-purple-700'
                    : 'bg-gray-700 hover:bg-gray-600'
                )}
              >
                <GitCompare className="w-4 h-4" />
                {compareMode ? (selectedForCompare ? 'Compare Selected' : 'Select Trace') : 'Compare'}
              </button>
              <button
                onClick={fetchData}
                className="px-3 py-1.5 bg-blue-600 hover:bg-blue-700 rounded text-sm"
              >
                Refresh
              </button>
            </>
          )}
        </div>
      </div>

      {/* Service Metrics Dashboard View */}
      {viewMode === 'metrics' && <ServiceMetricsDashboard />}

      {/* Traces List View */}
      {viewMode === 'traces' && (
        <>
      {/* Stats Dashboard */}
      {stats && (
        <div className="grid grid-cols-5 gap-4">
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <Activity className="w-4 h-4" />
              Total Traces (24h)
            </div>
            <div className="text-2xl font-bold">{stats.totalTraces.toLocaleString()}</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <AlertTriangle className="w-4 h-4 text-red-400" />
              Error Rate
            </div>
            <div className={clsx(
              "text-2xl font-bold",
              stats.errorRate > 5 ? "text-red-400" : stats.errorRate > 1 ? "text-yellow-400" : "text-green-400"
            )}>
              {stats.errorRate.toFixed(2)}%
            </div>
            <div className="text-xs text-gray-500">{stats.errorCount} errors</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <Clock className="w-4 h-4" />
              P50 Latency
            </div>
            <div className="text-2xl font-bold">{stats.p50LatencyMs.toFixed(1)}ms</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <TrendingUp className="w-4 h-4 text-yellow-400" />
              P99 Latency
            </div>
            <div className={clsx(
              "text-2xl font-bold",
              stats.p99LatencyMs > 1000 ? "text-yellow-400" : "text-gray-100"
            )}>
              {stats.p99LatencyMs > 1000 ? `${(stats.p99LatencyMs/1000).toFixed(2)}s` : `${stats.p99LatencyMs.toFixed(0)}ms`}
            </div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <Zap className="w-4 h-4 text-purple-400" />
              Services
            </div>
            <div className="text-2xl font-bold">{stats.serviceCount}</div>
          </div>
        </div>
      )}

      {/* Error Patterns */}
      {errorPatterns.length > 0 && (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-4">
          <h3 className="text-red-400 font-semibold mb-3 flex items-center gap-2">
            <AlertCircle className="w-5 h-5" />
            Error Patterns Detected
          </h3>
          <div className="space-y-2">
            {errorPatterns.slice(0, 3).map((pattern, idx) => (
              <div key={idx} className="flex items-center justify-between bg-red-900/30 rounded p-2">
                <div>
                  <span className="text-red-300 font-medium">{pattern.serviceName}</span>
                  <span className="text-gray-400 mx-2">/</span>
                  <span className="text-gray-300">{pattern.operationName}</span>
                </div>
                <div className="flex items-center gap-4">
                  <span className="text-red-400 font-mono">{pattern.count} errors</span>
                  <span className="text-gray-500 text-sm">Last: {formatTime(pattern.lastSeen)}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Filters */}
      <div className="bg-gray-800 rounded-lg p-4">
        <div className="flex items-center space-x-4">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
            <input
              type="text"
              placeholder="Search by trace ID, service, or operation..."
              value={filters.search}
              onChange={(e) => setFilters({ ...filters, search: e.target.value })}
              className="w-full pl-10 pr-4 py-2 bg-gray-700 border border-gray-600 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>

          <select
            value={filters.service}
            onChange={(e) => setFilters({ ...filters, service: e.target.value })}
            className="px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="">All Services</option>
            {services.map((service) => (
              <option key={service} value={service}>
                {service}
              </option>
            ))}
          </select>

          <select
            value={filters.minDuration}
            onChange={(e) => setFilters({ ...filters, minDuration: e.target.value })}
            className="px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="">Any Duration</option>
            <option value="100ms">&gt; 100ms</option>
            <option value="500ms">&gt; 500ms</option>
            <option value="1s">&gt; 1s</option>
            <option value="5s">&gt; 5s</option>
          </select>

          <select
            value={filters.status}
            onChange={(e) => setFilters({ ...filters, status: e.target.value })}
            className="px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="">All Traces</option>
            <option value="error">Errors Only</option>
            <option value="ok">Successful Only</option>
          </select>
        </div>
      </div>

      {error && (
        <div className="bg-red-900/20 border border-red-500 text-red-400 px-4 py-3 rounded">
          {error}
        </div>
      )}

      {/* Traces Table */}
      <div className="bg-gray-800 rounded-lg overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-gray-700 text-left text-sm text-gray-400">
              {compareMode && <th className="px-4 py-3 w-10"></th>}
              <th className="px-4 py-3 font-medium">Trace ID</th>
              <th className="px-4 py-3 font-medium">Operation</th>
              <th className="px-4 py-3 font-medium">Service</th>
              <th className="px-4 py-3 font-medium">Duration</th>
              <th className="px-4 py-3 font-medium">Spans</th>
              <th className="px-4 py-3 font-medium">Status</th>
              <th className="px-4 py-3 font-medium">Time</th>
              <th className="px-4 py-3"></th>
            </tr>
          </thead>
          <tbody>
            {loading && traces.length === 0 ? (
              <tr>
                <td colSpan={compareMode ? 9 : 8} className="px-4 py-8 text-center text-gray-400">
                  <Loader2 className="w-6 h-6 animate-spin mx-auto mb-2" />
                  Loading traces...
                </td>
              </tr>
            ) : filteredTraces.length === 0 ? (
              <tr>
                <td colSpan={compareMode ? 9 : 8} className="px-4 py-8 text-center text-gray-400">
                  <GitBranch className="w-12 h-12 mx-auto mb-3 opacity-50" />
                  <p>No traces found</p>
                  <p className="text-sm mt-1">Send some requests to see traces here</p>
                </td>
              </tr>
            ) : (
              filteredTraces.map((trace) => (
                <tr
                  key={trace.traceId}
                  className={clsx(
                    "border-b border-gray-700 hover:bg-gray-750 transition-colors",
                    trace.status === 'error' && "bg-red-900/10",
                    compareMode && selectedForCompare === trace.traceId && "bg-purple-900/20"
                  )}
                >
                  {compareMode && (
                    <td className="px-4 py-3">
                      <input
                        type="radio"
                        name="compareTrace"
                        checked={selectedForCompare === trace.traceId}
                        onChange={() => setSelectedForCompare(trace.traceId)}
                        className="w-4 h-4 text-purple-600 focus:ring-purple-500"
                      />
                    </td>
                  )}
                  <td className="px-4 py-3">
                    <Link
                      to={`/traces/${trace.traceId}`}
                      className="font-mono text-sm text-blue-400 hover:text-blue-300"
                    >
                      {trace.traceId.substring(0, 16)}...
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <div className="text-sm font-medium truncate max-w-xs" title={trace.operationName}>
                      {trace.operationName || 'Unknown'}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span className="px-2 py-0.5 bg-gray-700 rounded text-xs">
                      {trace.serviceName}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={clsx(
                        'text-sm font-medium font-mono',
                        trace.duration > 1000000000 ? 'text-red-400' :
                        trace.duration > 500000000 ? 'text-yellow-400' : 'text-gray-300'
                      )}
                    >
                      {formatDuration(trace.duration)}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-400">
                    {trace.spanCount}
                  </td>
                  <td className="px-4 py-3">
                    {trace.status === 'error' ? (
                      <span className="flex items-center text-red-400 text-sm">
                        <AlertCircle className="w-4 h-4 mr-1" />
                        Error
                      </span>
                    ) : (
                      <span className="flex items-center text-green-400 text-sm">
                        <CheckCircle className="w-4 h-4 mr-1" />
                        OK
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-400">
                    {formatTime(trace.timestamp)}
                  </td>
                  <td className="px-4 py-3">
                    <Link
                      to={`/traces/${trace.traceId}`}
                      className="p-1 hover:bg-gray-700 rounded transition-colors inline-flex items-center gap-1 text-gray-400 hover:text-white"
                    >
                      <span className="text-xs">Analyze</span>
                      <ChevronRight className="w-4 h-4" />
                    </Link>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Footer */}
      <div className="text-sm text-gray-500 text-center">
        Showing {filteredTraces.length} of {traces.length} traces | Tail-based sampling enabled (keeps all errors + slow traces)
      </div>
        </>
      )}

      {/* Trace Comparison Modal */}
      {showComparison && selectedForCompare && (
        <TraceComparison
          baseTraceId={selectedForCompare}
          compareTraceId={compareTraceId || undefined}
          onClose={() => {
            setShowComparison(false);
            setCompareMode(false);
            setSelectedForCompare(null);
            setCompareTraceId(null);
          }}
          onSelectCompareTrace={(traceId) => setCompareTraceId(traceId)}
        />
      )}
    </div>
  );
}
