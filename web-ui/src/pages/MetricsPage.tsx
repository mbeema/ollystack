import { useState, useEffect, useMemo } from 'react';
import {
  RefreshCw, Activity, AlertTriangle, Clock, Server,
  TrendingUp, TrendingDown, Minus, ChevronDown, ChevronRight,
  Zap, Target, BarChart3, Filter
} from 'lucide-react';
import { getApiUrl, getAiEngineUrl } from '../lib/config';

// Types
interface ServiceHealth {
  service: string;
  status: 'healthy' | 'degraded' | 'critical';
  metrics: {
    total_requests: number;
    errors: number;
    error_rate: number;
    avg_latency_ms: number;
    p95_latency_ms: number;
    p99_latency_ms: number;
  };
}

interface ServiceHealthResponse {
  services: ServiceHealth[];
  window_minutes: number;
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

interface ServiceTimeSeries {
  timestamps: string[];
  requests: number[];
  errors: number[];
  avg_latency_ms: number[];
  p50_ms: number[];
  p95_ms: number[];
  p99_ms: number[];
}

interface TimeSeriesResponse {
  services: Record<string, ServiceTimeSeries>;
  window_minutes: number;
  bucket_minutes: number;
  timestamp: string;
}

// Sparkline component
function Sparkline({ data, color = '#3b82f6', height = 30, width = 100 }: {
  data: number[], color?: string, height?: number, width?: number
}) {
  if (data.length < 2) return <div className="text-gray-500 text-xs">No data</div>;

  const max = Math.max(...data);
  const min = Math.min(...data);
  const range = max - min || 1;

  const points = data.map((value, i) => {
    const x = (i / (data.length - 1)) * width;
    const y = height - ((value - min) / range) * height;
    return `${x},${y}`;
  }).join(' ');

  return (
    <svg width={width} height={height} className="overflow-visible">
      <polyline
        fill="none"
        stroke={color}
        strokeWidth="1.5"
        points={points}
      />
      <circle
        cx={(data.length - 1) / (data.length - 1) * width}
        cy={height - ((data[data.length - 1] - min) / range) * height}
        r="2"
        fill={color}
      />
    </svg>
  );
}

// Status badge component
function StatusBadge({ status }: { status: string }) {
  const config = {
    healthy: { bg: 'bg-green-500/20', text: 'text-green-400', dot: 'bg-green-400' },
    degraded: { bg: 'bg-yellow-500/20', text: 'text-yellow-400', dot: 'bg-yellow-400' },
    critical: { bg: 'bg-red-500/20', text: 'text-red-400', dot: 'bg-red-400' },
  }[status] || { bg: 'bg-gray-500/20', text: 'text-gray-400', dot: 'bg-gray-400' };

  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${config.bg} ${config.text}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${config.dot}`} />
      {status}
    </span>
  );
}

// Metric card component
function MetricCard({
  title, value, unit, icon: Icon, trend, color = 'blue', subtitle
}: {
  title: string;
  value: string | number;
  unit?: string;
  icon: React.ElementType;
  trend?: number;
  color?: string;
  subtitle?: string;
}) {
  const colorClasses = {
    blue: 'bg-blue-500/20 text-blue-400',
    green: 'bg-green-500/20 text-green-400',
    yellow: 'bg-yellow-500/20 text-yellow-400',
    red: 'bg-red-500/20 text-red-400',
    purple: 'bg-purple-500/20 text-purple-400',
    orange: 'bg-orange-500/20 text-orange-400',
  }[color] || 'bg-blue-500/20 text-blue-400';

  return (
    <div className="bg-gray-800/50 border border-gray-700/50 rounded-xl p-4 hover:border-gray-600/50 transition-colors">
      <div className="flex items-start justify-between">
        <div className={`p-2 rounded-lg ${colorClasses}`}>
          <Icon className="w-5 h-5" />
        </div>
        {trend !== undefined && (
          <div className={`flex items-center gap-1 text-xs ${
            trend > 0 ? 'text-red-400' : trend < 0 ? 'text-green-400' : 'text-gray-400'
          }`}>
            {trend > 0 ? <TrendingUp className="w-3 h-3" /> :
             trend < 0 ? <TrendingDown className="w-3 h-3" /> :
             <Minus className="w-3 h-3" />}
            {Math.abs(trend).toFixed(1)}%
          </div>
        )}
      </div>
      <div className="mt-3">
        <div className="text-2xl font-bold text-white">
          {value}
          {unit && <span className="text-sm font-normal text-gray-400 ml-1">{unit}</span>}
        </div>
        <div className="text-sm text-gray-400 mt-0.5">{title}</div>
        {subtitle && <div className="text-xs text-gray-500 mt-1">{subtitle}</div>}
      </div>
    </div>
  );
}

// Service row component
function ServiceRow({ service, expanded, onToggle, timeSeries }: {
  service: ServiceHealth;
  expanded: boolean;
  onToggle: () => void;
  timeSeries?: ServiceTimeSeries;
}) {
  const formatLatency = (ms: number) => {
    if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
    return `${ms.toFixed(1)}ms`;
  };

  const formatNumber = (n: number) => {
    if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
    if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
    return n.toString();
  };

  // Use real time-series data or fall back to empty array
  const sparklineData = timeSeries?.avg_latency_ms || [];

  return (
    <div className="border-b border-gray-700/50 last:border-0">
      <div
        className="flex items-center gap-4 p-4 hover:bg-gray-800/30 cursor-pointer transition-colors"
        onClick={onToggle}
      >
        <div className="text-gray-400">
          {expanded ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <Server className="w-4 h-4 text-gray-400" />
            <span className="font-medium text-white truncate">{service.service}</span>
            <StatusBadge status={service.status} />
          </div>
        </div>

        <div className="hidden md:block w-28">
          <Sparkline data={sparklineData} color={
            service.status === 'healthy' ? '#22c55e' :
            service.status === 'degraded' ? '#eab308' : '#ef4444'
          } />
        </div>

        <div className="text-right w-24">
          <div className="text-sm font-medium text-white">{formatNumber(service.metrics.total_requests)}</div>
          <div className="text-xs text-gray-400">requests</div>
        </div>

        <div className="text-right w-20">
          <div className={`text-sm font-medium ${
            service.metrics.error_rate > 5 ? 'text-red-400' :
            service.metrics.error_rate > 1 ? 'text-yellow-400' : 'text-green-400'
          }`}>
            {service.metrics.error_rate.toFixed(2)}%
          </div>
          <div className="text-xs text-gray-400">errors</div>
        </div>

        <div className="text-right w-24">
          <div className={`text-sm font-medium ${
            service.metrics.avg_latency_ms > 500 ? 'text-red-400' :
            service.metrics.avg_latency_ms > 200 ? 'text-yellow-400' : 'text-white'
          }`}>
            {formatLatency(service.metrics.avg_latency_ms)}
          </div>
          <div className="text-xs text-gray-400">avg latency</div>
        </div>

        <div className="text-right w-24">
          <div className="text-sm font-medium text-white">{formatLatency(service.metrics.p99_latency_ms)}</div>
          <div className="text-xs text-gray-400">p99</div>
        </div>
      </div>

      {expanded && (
        <div className="px-4 pb-4 pl-12 bg-gray-800/20">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 p-4 bg-gray-900/50 rounded-lg">
            <div>
              <div className="text-xs text-gray-400 mb-1">Total Requests</div>
              <div className="text-lg font-semibold text-white">{formatNumber(service.metrics.total_requests)}</div>
            </div>
            <div>
              <div className="text-xs text-gray-400 mb-1">Error Count</div>
              <div className="text-lg font-semibold text-red-400">{service.metrics.errors}</div>
            </div>
            <div>
              <div className="text-xs text-gray-400 mb-1">P95 Latency</div>
              <div className="text-lg font-semibold text-white">{formatLatency(service.metrics.p95_latency_ms)}</div>
            </div>
            <div>
              <div className="text-xs text-gray-400 mb-1">P99 Latency</div>
              <div className="text-lg font-semibold text-white">{formatLatency(service.metrics.p99_latency_ms)}</div>
            </div>
          </div>

          {/* Latency distribution bar */}
          <div className="mt-4">
            <div className="text-xs text-gray-400 mb-2">Latency Distribution</div>
            <div className="flex items-center gap-1 h-8">
              <div
                className="h-full bg-green-500/60 rounded-l"
                style={{ width: `${Math.min(50, (service.metrics.avg_latency_ms / service.metrics.p99_latency_ms) * 100)}%` }}
                title={`Avg: ${formatLatency(service.metrics.avg_latency_ms)}`}
              />
              <div
                className="h-full bg-yellow-500/60"
                style={{ width: `${Math.min(30, ((service.metrics.p95_latency_ms - service.metrics.avg_latency_ms) / service.metrics.p99_latency_ms) * 100)}%` }}
                title={`P95: ${formatLatency(service.metrics.p95_latency_ms)}`}
              />
              <div
                className="h-full bg-red-500/60 rounded-r"
                style={{ width: `${Math.min(20, ((service.metrics.p99_latency_ms - service.metrics.p95_latency_ms) / service.metrics.p99_latency_ms) * 100)}%` }}
                title={`P99: ${formatLatency(service.metrics.p99_latency_ms)}`}
              />
            </div>
            <div className="flex justify-between text-xs text-gray-500 mt-1">
              <span>Avg</span>
              <span>P95</span>
              <span>P99</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default function MetricsPage() {
  const [services, setServices] = useState<ServiceHealth[]>([]);
  const [stats, setStats] = useState<TraceStats | null>(null);
  const [timeSeries, setTimeSeries] = useState<Record<string, ServiceTimeSeries>>({});
  const [loading, setLoading] = useState(true);
  const [timeWindow, setTimeWindow] = useState(60);
  const [expandedServices, setExpandedServices] = useState<Set<string>>(new Set());
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [sortBy, setSortBy] = useState<'name' | 'requests' | 'errors' | 'latency'>('requests');

  const fetchData = async () => {
    setLoading(true);
    try {
      const AI_URL = getAiEngineUrl();
      const API_URL = getApiUrl();

      // Calculate bucket size based on time window
      const bucketMinutes = timeWindow <= 15 ? 1 : timeWindow <= 60 ? 5 : 15;

      const [healthRes, statsRes, timeSeriesRes] = await Promise.all([
        fetch(`${AI_URL}/api/v1/services/health?window_minutes=${timeWindow}`),
        fetch(`${API_URL}/api/v1/traces/stats`),
        fetch(`${AI_URL}/api/v1/services/metrics/timeseries?window_minutes=${timeWindow}&bucket_minutes=${bucketMinutes}`),
      ]);

      if (healthRes.ok) {
        const healthData: ServiceHealthResponse = await healthRes.json();
        setServices(healthData.services || []);
      }

      if (statsRes.ok) {
        const statsData: TraceStats = await statsRes.json();
        setStats(statsData);
      }

      if (timeSeriesRes.ok) {
        const tsData: TimeSeriesResponse = await timeSeriesRes.json();
        setTimeSeries(tsData.services || {});
      }
    } catch (error) {
      console.error('Failed to fetch metrics:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, [timeWindow]);

  const toggleService = (serviceName: string) => {
    setExpandedServices(prev => {
      const next = new Set(prev);
      if (next.has(serviceName)) {
        next.delete(serviceName);
      } else {
        next.add(serviceName);
      }
      return next;
    });
  };

  // Filter and sort services
  const filteredServices = useMemo(() => {
    let result = [...services];

    if (statusFilter !== 'all') {
      result = result.filter(s => s.status === statusFilter);
    }

    result.sort((a, b) => {
      switch (sortBy) {
        case 'name': return a.service.localeCompare(b.service);
        case 'requests': return b.metrics.total_requests - a.metrics.total_requests;
        case 'errors': return b.metrics.error_rate - a.metrics.error_rate;
        case 'latency': return b.metrics.avg_latency_ms - a.metrics.avg_latency_ms;
        default: return 0;
      }
    });

    return result;
  }, [services, statusFilter, sortBy]);

  // Calculate summary stats
  const summary = useMemo(() => {
    const healthy = services.filter(s => s.status === 'healthy').length;
    const degraded = services.filter(s => s.status === 'degraded').length;
    const critical = services.filter(s => s.status === 'critical').length;
    const totalRequests = services.reduce((sum, s) => sum + s.metrics.total_requests, 0);
    const totalErrors = services.reduce((sum, s) => sum + s.metrics.errors, 0);
    const avgLatency = services.length > 0
      ? services.reduce((sum, s) => sum + s.metrics.avg_latency_ms, 0) / services.length
      : 0;

    return { healthy, degraded, critical, totalRequests, totalErrors, avgLatency };
  }, [services]);

  return (
    <div className="h-full flex flex-col bg-gray-900 text-white overflow-auto">
      {/* Header */}
      <div className="flex-shrink-0 p-4 border-b border-gray-700/50">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold">Metrics</h1>
            <p className="text-sm text-gray-400 mt-1">Service performance and health monitoring</p>
          </div>
          <div className="flex items-center gap-3">
            {/* Time window selector */}
            <select
              value={timeWindow}
              onChange={(e) => setTimeWindow(Number(e.target.value))}
              className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              <option value={5}>Last 5 minutes</option>
              <option value={15}>Last 15 minutes</option>
              <option value={60}>Last 1 hour</option>
              <option value={360}>Last 6 hours</option>
              <option value={1440}>Last 24 hours</option>
            </select>

            <button
              onClick={fetchData}
              disabled={loading}
              className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 rounded-lg transition-colors"
            >
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
          </div>
        </div>
      </div>

      {/* Summary Cards */}
      <div className="p-4 grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        <MetricCard
          title="Total Requests"
          value={summary.totalRequests >= 1000 ? `${(summary.totalRequests / 1000).toFixed(1)}K` : summary.totalRequests}
          icon={Activity}
          color="blue"
          subtitle={`Last ${timeWindow} min`}
        />
        <MetricCard
          title="Error Rate"
          value={stats?.errorRate?.toFixed(2) || '0'}
          unit="%"
          icon={AlertTriangle}
          color={stats?.errorRate && stats.errorRate > 5 ? 'red' : stats?.errorRate && stats.errorRate > 1 ? 'yellow' : 'green'}
        />
        <MetricCard
          title="Avg Latency"
          value={summary.avgLatency.toFixed(1)}
          unit="ms"
          icon={Clock}
          color={summary.avgLatency > 500 ? 'red' : summary.avgLatency > 200 ? 'yellow' : 'green'}
        />
        <MetricCard
          title="P99 Latency"
          value={stats?.p99LatencyMs ? (stats.p99LatencyMs >= 1000 ? `${(stats.p99LatencyMs / 1000).toFixed(2)}s` : `${stats.p99LatencyMs.toFixed(0)}ms`) : '-'}
          icon={Zap}
          color="purple"
        />
        <MetricCard
          title="Services"
          value={services.length}
          icon={Server}
          color="blue"
          subtitle={`${summary.healthy} healthy`}
        />
        <MetricCard
          title="Health Score"
          value={services.length > 0 ? Math.round((summary.healthy / services.length) * 100) : 100}
          unit="%"
          icon={Target}
          color={summary.critical > 0 ? 'red' : summary.degraded > 0 ? 'yellow' : 'green'}
        />
      </div>

      {/* Status summary bar */}
      <div className="px-4 pb-4">
        <div className="flex items-center gap-4 p-3 bg-gray-800/50 rounded-lg border border-gray-700/50">
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-green-400" />
            <span className="text-sm text-gray-300">{summary.healthy} Healthy</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-yellow-400" />
            <span className="text-sm text-gray-300">{summary.degraded} Degraded</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-red-400" />
            <span className="text-sm text-gray-300">{summary.critical} Critical</span>
          </div>
          <div className="flex-1" />
          <div className="text-xs text-gray-500">
            Auto-refresh every 30s
          </div>
        </div>
      </div>

      {/* Services Table */}
      <div className="flex-1 px-4 pb-4">
        <div className="bg-gray-800/30 border border-gray-700/50 rounded-xl overflow-hidden">
          {/* Table header */}
          <div className="flex items-center gap-4 p-4 border-b border-gray-700/50 bg-gray-800/50">
            <div className="w-4" />
            <div className="flex-1 flex items-center gap-4">
              <span className="text-sm font-medium text-gray-300">Service</span>

              {/* Filters */}
              <div className="flex items-center gap-2 ml-4">
                <Filter className="w-4 h-4 text-gray-400" />
                <select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className="bg-gray-700 border-none rounded px-2 py-1 text-xs focus:outline-none focus:ring-2 focus:ring-blue-500"
                >
                  <option value="all">All Status</option>
                  <option value="healthy">Healthy</option>
                  <option value="degraded">Degraded</option>
                  <option value="critical">Critical</option>
                </select>
              </div>
            </div>

            <div className="hidden md:block w-28 text-xs text-gray-400">Trend</div>

            <button
              onClick={() => setSortBy('requests')}
              className={`w-24 text-right text-xs ${sortBy === 'requests' ? 'text-blue-400' : 'text-gray-400'} hover:text-blue-400`}
            >
              Requests {sortBy === 'requests' && '↓'}
            </button>

            <button
              onClick={() => setSortBy('errors')}
              className={`w-20 text-right text-xs ${sortBy === 'errors' ? 'text-blue-400' : 'text-gray-400'} hover:text-blue-400`}
            >
              Errors {sortBy === 'errors' && '↓'}
            </button>

            <button
              onClick={() => setSortBy('latency')}
              className={`w-24 text-right text-xs ${sortBy === 'latency' ? 'text-blue-400' : 'text-gray-400'} hover:text-blue-400`}
            >
              Avg Latency {sortBy === 'latency' && '↓'}
            </button>

            <div className="w-24 text-right text-xs text-gray-400">P99</div>
          </div>

          {/* Services list */}
          {loading && services.length === 0 ? (
            <div className="flex items-center justify-center py-12">
              <RefreshCw className="w-6 h-6 animate-spin text-blue-500" />
            </div>
          ) : filteredServices.length === 0 ? (
            <div className="text-center py-12 text-gray-500">
              No services found
            </div>
          ) : (
            <div>
              {filteredServices.map((service) => (
                <ServiceRow
                  key={service.service}
                  service={service}
                  expanded={expandedServices.has(service.service)}
                  onToggle={() => toggleService(service.service)}
                  timeSeries={timeSeries[service.service]}
                />
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Quick Stats Footer */}
      {stats && (
        <div className="flex-shrink-0 p-4 border-t border-gray-700/50 bg-gray-800/30">
          <div className="flex items-center justify-between text-sm">
            <div className="flex items-center gap-6">
              <div className="flex items-center gap-2">
                <BarChart3 className="w-4 h-4 text-gray-400" />
                <span className="text-gray-400">24h Overview:</span>
              </div>
              <div>
                <span className="text-gray-400">Traces:</span>
                <span className="ml-2 text-white font-medium">{stats.totalTraces.toLocaleString()}</span>
              </div>
              <div>
                <span className="text-gray-400">Errors:</span>
                <span className="ml-2 text-red-400 font-medium">{stats.errorCount.toLocaleString()}</span>
              </div>
              <div>
                <span className="text-gray-400">P50:</span>
                <span className="ml-2 text-white font-medium">{stats.p50LatencyMs.toFixed(1)}ms</span>
              </div>
              <div>
                <span className="text-gray-400">P90:</span>
                <span className="ml-2 text-white font-medium">{stats.p90LatencyMs.toFixed(1)}ms</span>
              </div>
            </div>
            <div className="text-xs text-gray-500">
              {stats.serviceCount} services tracked
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
