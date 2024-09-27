import { useEffect, useState } from 'react';
import { BarChart3, GitBranch, AlertTriangle, CheckCircle, TrendingUp, Clock, Loader2, Link2 } from 'lucide-react';
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, AreaChart, Area } from 'recharts';
import { getApiUrl } from '../lib/config';
import AIInsightsWidget from '../components/dashboard/AIInsightsWidget';
import NLQueryWidget from '../components/dashboard/NLQueryWidget';

const API_URL = getApiUrl();

interface DashboardStats {
  totalTraces: number;
  errorCount: number;
  avgLatency: number;
  services: number;
}

interface Trace {
  traceId: string;
  serviceName: string;
  operationName: string;
  duration: number;
  status: string;
  timestamp: string;
}

export default function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [recentTraces, setRecentTraces] = useState<Trace[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchData() {
      try {
        setLoading(true);

        // Fetch recent traces
        const tracesRes = await fetch(`${API_URL}/api/v1/traces?limit=10`);
        if (tracesRes.ok) {
          const tracesData = await tracesRes.json();
          setRecentTraces(tracesData.traces || []);

          // Calculate stats from traces
          const traces = tracesData.traces || [];
          const errorCount = traces.filter((t: Trace) => t.status === 'error').length;
          const avgLatency = traces.length > 0
            ? Math.round(traces.reduce((sum: number, t: Trace) => sum + t.duration, 0) / traces.length / 1000)
            : 0;
          const services = new Set(traces.map((t: Trace) => t.serviceName)).size;

          setStats({
            totalTraces: tracesData.total || traces.length,
            errorCount,
            avgLatency,
            services,
          });
        }

        setError(null);
      } catch (err) {
        console.error('Failed to fetch dashboard data:', err);
        setError('Failed to load dashboard data');
      } finally {
        setLoading(false);
      }
    }

    fetchData();
    const interval = setInterval(fetchData, 30000); // Refresh every 30s
    return () => clearInterval(interval);
  }, []);

  if (loading && !stats) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
      </div>
    );
  }

  const errorRate = stats && stats.totalTraces > 0
    ? ((stats.errorCount / stats.totalTraces) * 100).toFixed(2)
    : '0.00';

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        {loading && <Loader2 className="w-4 h-4 animate-spin text-gray-400" />}
      </div>

      {error && (
        <div className="bg-red-900/20 border border-red-500 text-red-400 px-4 py-3 rounded mb-6">
          {error}
        </div>
      )}

      {/* Stats Grid */}
      <div className="grid grid-cols-4 gap-6 mb-6">
        <StatCard
          title="Total Traces"
          value={stats?.totalTraces?.toLocaleString() || '0'}
          icon={BarChart3}
        />
        <StatCard
          title="Services"
          value={stats?.services?.toString() || '0'}
          icon={GitBranch}
        />
        <StatCard
          title="Error Rate"
          value={`${errorRate}%`}
          icon={AlertTriangle}
        />
        <StatCard
          title="Avg Latency"
          value={`${stats?.avgLatency || 0}ms`}
          icon={Clock}
        />
      </div>

      {/* NLQ Widget - Full Width */}
      <div className="mb-6">
        <NLQueryWidget />
      </div>

      {/* Main Content Grid */}
      <div className="grid grid-cols-2 gap-6">
        {/* AI Insights Widget */}
        <AIInsightsWidget />

        {/* Recent Traces */}
        <div className="bg-gray-800 rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold">Recent Traces</h3>
            <a href="/traces" className="text-sm text-blue-400 hover:text-blue-300">
              View all
            </a>
          </div>

          {recentTraces.length === 0 ? (
            <div className="text-center py-8 text-gray-400">
              <GitBranch className="w-12 h-12 mx-auto mb-3 opacity-50" />
              <p>No traces yet</p>
              <p className="text-sm mt-1">Send some requests to see traces here</p>
            </div>
          ) : (
            <div className="space-y-3 max-h-80 overflow-auto">
              {recentTraces.slice(0, 8).map((trace) => (
                <a
                  key={trace.traceId}
                  href={`/traces/${trace.traceId}`}
                  className="flex items-center justify-between p-3 bg-gray-750 rounded-lg hover:bg-gray-700 transition-colors cursor-pointer block"
                >
                  <div className="flex items-center">
                    {trace.status === 'error' ? (
                      <AlertTriangle className="w-4 h-4 text-red-400 mr-3" />
                    ) : (
                      <CheckCircle className="w-4 h-4 text-green-400 mr-3" />
                    )}
                    <div>
                      <div className="text-sm font-medium">{trace.operationName || 'Unknown'}</div>
                      <div className="text-xs text-gray-400">{trace.serviceName}</div>
                    </div>
                  </div>
                  <div className="text-right">
                    <span className="text-sm text-gray-400">{Math.round(trace.duration / 1000)}ms</span>
                    <div className="text-xs text-gray-500">{trace.traceId.slice(0, 8)}...</div>
                  </div>
                </a>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

interface StatCardProps {
  title: string;
  value: string;
  icon: React.ComponentType<{ className?: string }>;
}

function StatCard({ title, value, icon: Icon }: StatCardProps) {
  return (
    <div className="bg-gray-800 rounded-lg p-6">
      <div className="flex items-center justify-between mb-4">
        <span className="text-gray-400 text-sm">{title}</span>
        <Icon className="w-5 h-5 text-gray-500" />
      </div>
      <div className="flex items-end justify-between">
        <span className="text-3xl font-bold">{value}</span>
      </div>
    </div>
  );
}
