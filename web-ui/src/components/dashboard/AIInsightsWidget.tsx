import { useState, useEffect } from 'react';
import {
  Brain,
  AlertTriangle,
  TrendingUp,
  Lightbulb,
  Activity,
  CheckCircle,
  XCircle,
  Loader2,
  RefreshCw,
  ChevronRight,
  Zap,
} from 'lucide-react';
import { clsx } from 'clsx';
import { getAiEngineUrl } from '../../lib/config';

const AI_ENGINE_URL = getAiEngineUrl();

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

interface Insight {
  type: string;
  severity: 'high' | 'medium' | 'low';
  title: string;
  description: string;
  recommendation: string;
}

interface Anomaly {
  timestamp: string;
  service: string;
  metric: string;
  value: number;
  baseline: number;
  deviation: number;
  severity: string;
  description: string;
}

export default function AIInsightsWidget() {
  const [services, setServices] = useState<ServiceHealth[]>([]);
  const [insights, setInsights] = useState<Insight[]>([]);
  const [anomalies, setAnomalies] = useState<Anomaly[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'health' | 'insights' | 'anomalies'>('health');

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, []);

  const fetchData = async () => {
    try {
      setLoading(true);

      const [healthRes, insightsRes, anomaliesRes] = await Promise.all([
        fetch(`${AI_ENGINE_URL}/api/v1/services/health?window_minutes=5`),
        fetch(`${AI_ENGINE_URL}/api/v1/insights?hours=1`),
        fetch(`${AI_ENGINE_URL}/api/v1/anomalies/detect`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ metric: 'latency', window_minutes: 30, sensitivity: 2.0 }),
        }),
      ]);

      if (healthRes.ok) {
        const healthData = await healthRes.json();
        setServices(healthData.services || []);
      }

      if (insightsRes.ok) {
        const insightsData = await insightsRes.json();
        setInsights(insightsData.insights || []);
      }

      if (anomaliesRes.ok) {
        const anomaliesData = await anomaliesRes.json();
        setAnomalies(anomaliesData.anomalies || []);
      }

      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch AI insights');
    } finally {
      setLoading(false);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'healthy':
        return 'text-green-400';
      case 'degraded':
        return 'text-yellow-400';
      case 'critical':
        return 'text-red-400';
      default:
        return 'text-gray-400';
    }
  };

  const getStatusBg = (status: string) => {
    switch (status) {
      case 'healthy':
        return 'bg-green-900/30';
      case 'degraded':
        return 'bg-yellow-900/30';
      case 'critical':
        return 'bg-red-900/30';
      default:
        return 'bg-gray-800';
    }
  };

  const getSeverityIcon = (severity: string) => {
    switch (severity) {
      case 'high':
      case 'critical':
        return <XCircle className="w-4 h-4 text-red-400" />;
      case 'medium':
      case 'warning':
        return <AlertTriangle className="w-4 h-4 text-yellow-400" />;
      default:
        return <Lightbulb className="w-4 h-4 text-blue-400" />;
    }
  };

  const healthyCount = services.filter((s) => s.status === 'healthy').length;
  const degradedCount = services.filter((s) => s.status === 'degraded').length;
  const criticalCount = services.filter((s) => s.status === 'critical').length;

  return (
    <div className="bg-gray-800 rounded-lg overflow-hidden">
      {/* Header */}
      <div className="p-4 border-b border-gray-700 bg-gradient-to-r from-purple-900/30 to-blue-900/30">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Brain className="w-5 h-5 text-purple-400" />
            <span className="font-semibold">AI Insights</span>
          </div>
          <button
            onClick={fetchData}
            className="p-1.5 hover:bg-gray-700 rounded text-gray-400 hover:text-white"
            title="Refresh"
          >
            <RefreshCw className={clsx('w-4 h-4', loading && 'animate-spin')} />
          </button>
        </div>

        {/* Quick Stats */}
        <div className="flex items-center gap-4 mt-3 text-sm">
          <div className="flex items-center gap-1">
            <CheckCircle className="w-4 h-4 text-green-400" />
            <span className="text-green-400">{healthyCount}</span>
            <span className="text-gray-500">healthy</span>
          </div>
          {degradedCount > 0 && (
            <div className="flex items-center gap-1">
              <AlertTriangle className="w-4 h-4 text-yellow-400" />
              <span className="text-yellow-400">{degradedCount}</span>
              <span className="text-gray-500">degraded</span>
            </div>
          )}
          {criticalCount > 0 && (
            <div className="flex items-center gap-1">
              <XCircle className="w-4 h-4 text-red-400" />
              <span className="text-red-400">{criticalCount}</span>
              <span className="text-gray-500">critical</span>
            </div>
          )}
          {anomalies.length > 0 && (
            <div className="flex items-center gap-1 ml-auto">
              <Zap className="w-4 h-4 text-orange-400" />
              <span className="text-orange-400">{anomalies.length}</span>
              <span className="text-gray-500">anomalies</span>
            </div>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-gray-700">
        <button
          onClick={() => setActiveTab('health')}
          className={clsx(
            'flex-1 px-4 py-2 text-sm font-medium transition-colors',
            activeTab === 'health'
              ? 'bg-gray-750 text-white border-b-2 border-blue-500'
              : 'text-gray-400 hover:text-white'
          )}
        >
          <Activity className="w-4 h-4 inline-block mr-1" />
          Health
        </button>
        <button
          onClick={() => setActiveTab('insights')}
          className={clsx(
            'flex-1 px-4 py-2 text-sm font-medium transition-colors',
            activeTab === 'insights'
              ? 'bg-gray-750 text-white border-b-2 border-blue-500'
              : 'text-gray-400 hover:text-white'
          )}
        >
          <Lightbulb className="w-4 h-4 inline-block mr-1" />
          Insights
          {insights.length > 0 && (
            <span className="ml-1 px-1.5 py-0.5 bg-blue-600 text-xs rounded">
              {insights.length}
            </span>
          )}
        </button>
        <button
          onClick={() => setActiveTab('anomalies')}
          className={clsx(
            'flex-1 px-4 py-2 text-sm font-medium transition-colors',
            activeTab === 'anomalies'
              ? 'bg-gray-750 text-white border-b-2 border-blue-500'
              : 'text-gray-400 hover:text-white'
          )}
        >
          <TrendingUp className="w-4 h-4 inline-block mr-1" />
          Anomalies
          {anomalies.length > 0 && (
            <span className="ml-1 px-1.5 py-0.5 bg-orange-600 text-xs rounded">
              {anomalies.length}
            </span>
          )}
        </button>
      </div>

      {/* Content */}
      <div className="p-4 max-h-80 overflow-auto">
        {loading && services.length === 0 ? (
          <div className="flex items-center justify-center h-32">
            <Loader2 className="w-6 h-6 animate-spin text-blue-500" />
          </div>
        ) : error ? (
          <div className="text-center py-8 text-gray-400">
            <AlertTriangle className="w-8 h-8 mx-auto mb-2 text-yellow-400" />
            <p className="text-sm">{error}</p>
            <p className="text-xs text-gray-500 mt-1">AI Engine may not be running</p>
          </div>
        ) : (
          <>
            {/* Health Tab */}
            {activeTab === 'health' && (
              <div className="space-y-2">
                {services.length === 0 ? (
                  <div className="text-center py-8 text-gray-400">
                    <Activity className="w-8 h-8 mx-auto mb-2 opacity-50" />
                    <p className="text-sm">No services detected</p>
                  </div>
                ) : (
                  services.map((service) => (
                    <div
                      key={service.service}
                      className={clsx(
                        'p-3 rounded-lg flex items-center justify-between',
                        getStatusBg(service.status)
                      )}
                    >
                      <div>
                        <div className="flex items-center gap-2">
                          <span className={clsx('text-sm font-medium', getStatusColor(service.status))}>
                            {service.service}
                          </span>
                          <span
                            className={clsx(
                              'text-xs px-1.5 py-0.5 rounded capitalize',
                              service.status === 'healthy'
                                ? 'bg-green-900/50 text-green-300'
                                : service.status === 'degraded'
                                ? 'bg-yellow-900/50 text-yellow-300'
                                : 'bg-red-900/50 text-red-300'
                            )}
                          >
                            {service.status}
                          </span>
                        </div>
                        <div className="flex items-center gap-3 text-xs text-gray-400 mt-1">
                          <span>{service.metrics.total_requests} requests</span>
                          <span>{service.metrics.avg_latency_ms.toFixed(0)}ms avg</span>
                          {service.metrics.error_rate > 0 && (
                            <span className="text-red-400">
                              {service.metrics.error_rate.toFixed(1)}% errors
                            </span>
                          )}
                        </div>
                      </div>
                      <ChevronRight className="w-4 h-4 text-gray-500" />
                    </div>
                  ))
                )}
              </div>
            )}

            {/* Insights Tab */}
            {activeTab === 'insights' && (
              <div className="space-y-2">
                {insights.length === 0 ? (
                  <div className="text-center py-8 text-gray-400">
                    <Lightbulb className="w-8 h-8 mx-auto mb-2 opacity-50" />
                    <p className="text-sm">No insights available</p>
                    <p className="text-xs text-gray-500 mt-1">System is operating normally</p>
                  </div>
                ) : (
                  insights.map((insight, idx) => (
                    <div
                      key={idx}
                      className={clsx(
                        'p-3 rounded-lg border',
                        insight.severity === 'high'
                          ? 'bg-red-900/20 border-red-800'
                          : insight.severity === 'medium'
                          ? 'bg-yellow-900/20 border-yellow-800'
                          : 'bg-blue-900/20 border-blue-800'
                      )}
                    >
                      <div className="flex items-start gap-2">
                        {getSeverityIcon(insight.severity)}
                        <div className="flex-1">
                          <div className="text-sm font-medium text-white">{insight.title}</div>
                          <div className="text-xs text-gray-400 mt-1">{insight.description}</div>
                          <div className="text-xs text-blue-400 mt-2 flex items-center gap-1">
                            <Zap className="w-3 h-3" />
                            {insight.recommendation}
                          </div>
                        </div>
                      </div>
                    </div>
                  ))
                )}
              </div>
            )}

            {/* Anomalies Tab */}
            {activeTab === 'anomalies' && (
              <div className="space-y-2">
                {anomalies.length === 0 ? (
                  <div className="text-center py-8 text-gray-400">
                    <TrendingUp className="w-8 h-8 mx-auto mb-2 opacity-50" />
                    <p className="text-sm">No anomalies detected</p>
                    <p className="text-xs text-gray-500 mt-1">All metrics within normal range</p>
                  </div>
                ) : (
                  anomalies.map((anomaly, idx) => (
                    <div
                      key={idx}
                      className={clsx(
                        'p-3 rounded-lg border',
                        anomaly.severity === 'critical'
                          ? 'bg-red-900/20 border-red-800'
                          : 'bg-yellow-900/20 border-yellow-800'
                      )}
                    >
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-sm font-medium text-white">{anomaly.service}</span>
                        <span
                          className={clsx(
                            'text-xs px-1.5 py-0.5 rounded',
                            anomaly.severity === 'critical'
                              ? 'bg-red-900/50 text-red-300'
                              : 'bg-yellow-900/50 text-yellow-300'
                          )}
                        >
                          {anomaly.severity}
                        </span>
                      </div>
                      <div className="text-xs text-gray-400">{anomaly.description}</div>
                      <div className="flex items-center gap-4 text-xs text-gray-500 mt-2">
                        <span>Value: {anomaly.value}</span>
                        <span>Baseline: {anomaly.baseline}</span>
                        <span>Deviation: {anomaly.deviation.toFixed(1)}σ</span>
                      </div>
                    </div>
                  ))
                )}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
