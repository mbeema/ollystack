import { useState, useEffect } from 'react';
import {
  Bell,
  AlertTriangle,
  AlertCircle,
  Info,
  CheckCircle,
  Clock,
  RefreshCw,
  Filter,
  Plus,
  X,
  ExternalLink,
  TrendingUp,
  TrendingDown,
  Activity,
  Server,
  Zap,
  Target,
  ChevronRight,
} from 'lucide-react';
import clsx from 'clsx';
import { useNavigate } from 'react-router-dom';

interface Alert {
  id: string;
  name: string;
  type: 'error_rate' | 'latency' | 'availability' | 'throughput';
  severity: 'critical' | 'warning' | 'info';
  status: 'firing' | 'resolved' | 'acknowledged';
  service: string;
  summary: string;
  description: string;
  currentValue: number;
  thresholdValue: number;
  timestamp: string;
  resolvedAt?: string;
  acknowledgedBy?: string;
}

interface AlertRule {
  id: string;
  name: string;
  type: 'error_rate' | 'latency' | 'availability' | 'throughput';
  condition: string;
  threshold: number;
  duration: string;
  severity: 'critical' | 'warning' | 'info';
  services: string[];
  enabled: boolean;
}

// Mock data for alerts
const mockAlerts: Alert[] = [
  {
    id: 'alert-001',
    name: 'High Error Rate',
    type: 'error_rate',
    severity: 'critical',
    status: 'firing',
    service: 'notification-service',
    summary: 'Error rate exceeded 5% threshold',
    description: 'The notification-service has been experiencing elevated error rates for the past 15 minutes.',
    currentValue: 7.2,
    thresholdValue: 5,
    timestamp: new Date(Date.now() - 15 * 60000).toISOString(),
  },
  {
    id: 'alert-002',
    name: 'High P99 Latency',
    type: 'latency',
    severity: 'warning',
    status: 'firing',
    service: 'payment-service',
    summary: 'P99 latency exceeded 1000ms threshold',
    description: 'Payment service P99 latency has spiked, potentially affecting user experience.',
    currentValue: 1250,
    thresholdValue: 1000,
    timestamp: new Date(Date.now() - 8 * 60000).toISOString(),
  },
  {
    id: 'alert-003',
    name: 'Service Degradation',
    type: 'availability',
    severity: 'warning',
    status: 'acknowledged',
    service: 'order-service',
    summary: 'Service availability below 99.9%',
    description: 'Order service availability has dropped below SLO target.',
    currentValue: 99.5,
    thresholdValue: 99.9,
    timestamp: new Date(Date.now() - 45 * 60000).toISOString(),
    acknowledgedBy: 'admin@example.com',
  },
  {
    id: 'alert-004',
    name: 'Low Throughput',
    type: 'throughput',
    severity: 'info',
    status: 'resolved',
    service: 'api-gateway',
    summary: 'Request throughput dropped below baseline',
    description: 'API gateway throughput was significantly lower than expected baseline.',
    currentValue: 850,
    thresholdValue: 1000,
    timestamp: new Date(Date.now() - 120 * 60000).toISOString(),
    resolvedAt: new Date(Date.now() - 60 * 60000).toISOString(),
  },
];

const mockRules: AlertRule[] = [
  {
    id: 'rule-001',
    name: 'High Error Rate',
    type: 'error_rate',
    condition: 'error_rate > threshold',
    threshold: 5,
    duration: '5m',
    severity: 'critical',
    services: ['*'],
    enabled: true,
  },
  {
    id: 'rule-002',
    name: 'High P99 Latency',
    type: 'latency',
    condition: 'p99_latency > threshold',
    threshold: 1000,
    duration: '10m',
    severity: 'warning',
    services: ['*'],
    enabled: true,
  },
  {
    id: 'rule-003',
    name: 'Service Availability SLO',
    type: 'availability',
    condition: 'availability < threshold',
    threshold: 99.9,
    duration: '15m',
    severity: 'warning',
    services: ['*'],
    enabled: true,
  },
  {
    id: 'rule-004',
    name: 'Low Throughput',
    type: 'throughput',
    condition: 'throughput < threshold',
    threshold: 500,
    duration: '30m',
    severity: 'info',
    services: ['api-gateway'],
    enabled: false,
  },
];

const severityConfig = {
  critical: { icon: AlertCircle, color: 'text-red-400', bg: 'bg-red-400/10', border: 'border-red-400/30' },
  warning: { icon: AlertTriangle, color: 'text-yellow-400', bg: 'bg-yellow-400/10', border: 'border-yellow-400/30' },
  info: { icon: Info, color: 'text-blue-400', bg: 'bg-blue-400/10', border: 'border-blue-400/30' },
};

const statusConfig = {
  firing: { color: 'text-red-400', bg: 'bg-red-400/10', label: 'Firing' },
  acknowledged: { color: 'text-yellow-400', bg: 'bg-yellow-400/10', label: 'Acknowledged' },
  resolved: { color: 'text-green-400', bg: 'bg-green-400/10', label: 'Resolved' },
};

const typeConfig = {
  error_rate: { icon: AlertTriangle, label: 'Error Rate', unit: '%' },
  latency: { icon: Clock, label: 'Latency', unit: 'ms' },
  availability: { icon: Activity, label: 'Availability', unit: '%' },
  throughput: { icon: TrendingUp, label: 'Throughput', unit: 'req/s' },
};

export default function AlertsPage() {
  const navigate = useNavigate();
  const [alerts, setAlerts] = useState<Alert[]>(mockAlerts);
  const [rules, setRules] = useState<AlertRule[]>(mockRules);
  const [activeTab, setActiveTab] = useState<'alerts' | 'rules' | 'slo'>('alerts');
  const [severityFilter, setSeverityFilter] = useState<string>('all');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [selectedAlert, setSelectedAlert] = useState<Alert | null>(null);
  const [loading, setLoading] = useState(false);

  const refreshAlerts = () => {
    setLoading(true);
    setTimeout(() => setLoading(false), 500);
  };

  const filteredAlerts = alerts.filter(alert => {
    if (severityFilter !== 'all' && alert.severity !== severityFilter) return false;
    if (statusFilter !== 'all' && alert.status !== statusFilter) return false;
    return true;
  });

  const alertCounts = {
    firing: alerts.filter(a => a.status === 'firing').length,
    acknowledged: alerts.filter(a => a.status === 'acknowledged').length,
    resolved: alerts.filter(a => a.status === 'resolved').length,
    critical: alerts.filter(a => a.severity === 'critical' && a.status === 'firing').length,
    warning: alerts.filter(a => a.severity === 'warning' && a.status === 'firing').length,
  };

  const formatTimeAgo = (timestamp: string) => {
    const diff = Date.now() - new Date(timestamp).getTime();
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(minutes / 60);
    if (hours > 0) return `${hours}h ${minutes % 60}m ago`;
    return `${minutes}m ago`;
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Alerts & SLOs</h1>
          <p className="text-gray-400 mt-1">Monitor service health and SLO compliance</p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={refreshAlerts}
            className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors"
          >
            <RefreshCw className={clsx('w-4 h-4', loading && 'animate-spin')} />
            Refresh
          </button>
          <button className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-lg transition-colors">
            <Plus className="w-4 h-4" />
            Create Rule
          </button>
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
        <div className="bg-gray-800 rounded-xl p-4 border border-gray-700">
          <div className="flex items-center justify-between">
            <div className="p-2 bg-red-400/10 rounded-lg">
              <Bell className="w-5 h-5 text-red-400" />
            </div>
            <span className="text-2xl font-bold text-red-400">{alertCounts.firing}</span>
          </div>
          <p className="mt-2 text-sm text-gray-400">Active Alerts</p>
        </div>
        <div className="bg-gray-800 rounded-xl p-4 border border-gray-700">
          <div className="flex items-center justify-between">
            <div className="p-2 bg-red-400/10 rounded-lg">
              <AlertCircle className="w-5 h-5 text-red-400" />
            </div>
            <span className="text-2xl font-bold">{alertCounts.critical}</span>
          </div>
          <p className="mt-2 text-sm text-gray-400">Critical</p>
        </div>
        <div className="bg-gray-800 rounded-xl p-4 border border-gray-700">
          <div className="flex items-center justify-between">
            <div className="p-2 bg-yellow-400/10 rounded-lg">
              <AlertTriangle className="w-5 h-5 text-yellow-400" />
            </div>
            <span className="text-2xl font-bold">{alertCounts.warning}</span>
          </div>
          <p className="mt-2 text-sm text-gray-400">Warning</p>
        </div>
        <div className="bg-gray-800 rounded-xl p-4 border border-gray-700">
          <div className="flex items-center justify-between">
            <div className="p-2 bg-yellow-400/10 rounded-lg">
              <Clock className="w-5 h-5 text-yellow-400" />
            </div>
            <span className="text-2xl font-bold">{alertCounts.acknowledged}</span>
          </div>
          <p className="mt-2 text-sm text-gray-400">Acknowledged</p>
        </div>
        <div className="bg-gray-800 rounded-xl p-4 border border-gray-700">
          <div className="flex items-center justify-between">
            <div className="p-2 bg-green-400/10 rounded-lg">
              <CheckCircle className="w-5 h-5 text-green-400" />
            </div>
            <span className="text-2xl font-bold">{alertCounts.resolved}</span>
          </div>
          <p className="mt-2 text-sm text-gray-400">Resolved (24h)</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-2 border-b border-gray-700 pb-4">
        <button
          onClick={() => setActiveTab('alerts')}
          className={clsx(
            'flex items-center gap-2 px-4 py-2 rounded-lg transition-all',
            activeTab === 'alerts' ? 'bg-blue-600 text-white' : 'text-gray-400 hover:bg-gray-800'
          )}
        >
          <Bell className="w-4 h-4" />
          Alerts
          {alertCounts.firing > 0 && (
            <span className="px-2 py-0.5 text-xs rounded-full bg-red-500">{alertCounts.firing}</span>
          )}
        </button>
        <button
          onClick={() => setActiveTab('rules')}
          className={clsx(
            'flex items-center gap-2 px-4 py-2 rounded-lg transition-all',
            activeTab === 'rules' ? 'bg-blue-600 text-white' : 'text-gray-400 hover:bg-gray-800'
          )}
        >
          <Zap className="w-4 h-4" />
          Alert Rules
          <span className="px-2 py-0.5 text-xs rounded-full bg-gray-700">{rules.length}</span>
        </button>
        <button
          onClick={() => setActiveTab('slo')}
          className={clsx(
            'flex items-center gap-2 px-4 py-2 rounded-lg transition-all',
            activeTab === 'slo' ? 'bg-blue-600 text-white' : 'text-gray-400 hover:bg-gray-800'
          )}
        >
          <Target className="w-4 h-4" />
          SLO Dashboard
        </button>
      </div>

      {/* Content */}
      {activeTab === 'alerts' && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Alerts List */}
          <div className="lg:col-span-2 bg-gray-800 rounded-xl border border-gray-700">
            <div className="p-4 border-b border-gray-700 flex items-center justify-between">
              <h3 className="font-semibold">Alert History</h3>
              <div className="flex items-center gap-2">
                <select
                  value={severityFilter}
                  onChange={(e) => setSeverityFilter(e.target.value)}
                  className="px-3 py-1.5 bg-gray-700 border border-gray-600 rounded-lg text-sm"
                >
                  <option value="all">All Severity</option>
                  <option value="critical">Critical</option>
                  <option value="warning">Warning</option>
                  <option value="info">Info</option>
                </select>
                <select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className="px-3 py-1.5 bg-gray-700 border border-gray-600 rounded-lg text-sm"
                >
                  <option value="all">All Status</option>
                  <option value="firing">Firing</option>
                  <option value="acknowledged">Acknowledged</option>
                  <option value="resolved">Resolved</option>
                </select>
              </div>
            </div>
            <div className="divide-y divide-gray-700">
              {filteredAlerts.map((alert) => {
                const SeverityIcon = severityConfig[alert.severity].icon;
                const TypeIcon = typeConfig[alert.type].icon;
                return (
                  <div
                    key={alert.id}
                    onClick={() => setSelectedAlert(alert)}
                    className={clsx(
                      'p-4 cursor-pointer transition-colors',
                      selectedAlert?.id === alert.id ? 'bg-gray-700' : 'hover:bg-gray-750'
                    )}
                  >
                    <div className="flex items-start gap-4">
                      <div className={clsx('p-2 rounded-lg', severityConfig[alert.severity].bg)}>
                        <SeverityIcon className={clsx('w-5 h-5', severityConfig[alert.severity].color)} />
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{alert.name}</span>
                          <span className={clsx('px-2 py-0.5 rounded-full text-xs', statusConfig[alert.status].bg, statusConfig[alert.status].color)}>
                            {statusConfig[alert.status].label}
                          </span>
                        </div>
                        <p className="text-sm text-gray-400 mt-1">{alert.summary}</p>
                        <div className="flex items-center gap-4 mt-2 text-xs text-gray-500">
                          <span className="flex items-center gap-1">
                            <Server className="w-3 h-3" />
                            {alert.service}
                          </span>
                          <span className="flex items-center gap-1">
                            <TypeIcon className="w-3 h-3" />
                            {alert.currentValue}{typeConfig[alert.type].unit}
                          </span>
                          <span className="flex items-center gap-1">
                            <Clock className="w-3 h-3" />
                            {formatTimeAgo(alert.timestamp)}
                          </span>
                        </div>
                      </div>
                      <ChevronRight className="w-4 h-4 text-gray-500" />
                    </div>
                  </div>
                );
              })}
              {filteredAlerts.length === 0 && (
                <div className="p-8 text-center text-gray-500">
                  <Bell className="w-12 h-12 mx-auto mb-3 opacity-50" />
                  <p>No alerts matching filters</p>
                </div>
              )}
            </div>
          </div>

          {/* Alert Details */}
          <div className="bg-gray-800 rounded-xl border border-gray-700">
            <div className="p-4 border-b border-gray-700">
              <h3 className="font-semibold">Alert Details</h3>
            </div>
            {selectedAlert ? (
              <div className="p-4 space-y-4">
                <div className="flex items-center gap-3">
                  <div className={clsx('p-3 rounded-lg', severityConfig[selectedAlert.severity].bg)}>
                    {(() => {
                      const Icon = severityConfig[selectedAlert.severity].icon;
                      return <Icon className={clsx('w-6 h-6', severityConfig[selectedAlert.severity].color)} />;
                    })()}
                  </div>
                  <div>
                    <h4 className="font-semibold">{selectedAlert.name}</h4>
                    <span className={clsx('px-2 py-0.5 rounded-full text-xs', statusConfig[selectedAlert.status].bg, statusConfig[selectedAlert.status].color)}>
                      {statusConfig[selectedAlert.status].label}
                    </span>
                  </div>
                </div>

                <div className="space-y-3">
                  <div>
                    <label className="text-sm text-gray-400">Service</label>
                    <p className="font-medium">{selectedAlert.service}</p>
                  </div>
                  <div>
                    <label className="text-sm text-gray-400">Description</label>
                    <p className="text-sm text-gray-300">{selectedAlert.description}</p>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="text-sm text-gray-400">Current Value</label>
                      <p className="text-xl font-bold text-red-400">
                        {selectedAlert.currentValue}{typeConfig[selectedAlert.type].unit}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm text-gray-400">Threshold</label>
                      <p className="text-xl font-bold">
                        {selectedAlert.thresholdValue}{typeConfig[selectedAlert.type].unit}
                      </p>
                    </div>
                  </div>
                  <div>
                    <label className="text-sm text-gray-400">Started</label>
                    <p className="text-sm">{new Date(selectedAlert.timestamp).toLocaleString()}</p>
                  </div>
                  {selectedAlert.resolvedAt && (
                    <div>
                      <label className="text-sm text-gray-400">Resolved</label>
                      <p className="text-sm">{new Date(selectedAlert.resolvedAt).toLocaleString()}</p>
                    </div>
                  )}
                  {selectedAlert.acknowledgedBy && (
                    <div>
                      <label className="text-sm text-gray-400">Acknowledged By</label>
                      <p className="text-sm">{selectedAlert.acknowledgedBy}</p>
                    </div>
                  )}
                </div>

                {selectedAlert.status === 'firing' && (
                  <div className="pt-4 flex gap-2">
                    <button className="flex-1 px-4 py-2 bg-yellow-600 hover:bg-yellow-700 rounded-lg text-sm font-medium transition-colors">
                      Acknowledge
                    </button>
                    <button className="flex-1 px-4 py-2 bg-green-600 hover:bg-green-700 rounded-lg text-sm font-medium transition-colors">
                      Resolve
                    </button>
                  </div>
                )}

                <div className="pt-4 flex gap-2">
                  <button
                    onClick={() => navigate(`/traces?service=${selectedAlert.service}`)}
                    className="flex-1 px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg text-sm transition-colors flex items-center justify-center gap-2"
                  >
                    View Traces
                    <ExternalLink className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => navigate(`/logs?service=${selectedAlert.service}`)}
                    className="flex-1 px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg text-sm transition-colors flex items-center justify-center gap-2"
                  >
                    View Logs
                    <ExternalLink className="w-4 h-4" />
                  </button>
                </div>
              </div>
            ) : (
              <div className="p-8 text-center text-gray-500">
                <Bell className="w-12 h-12 mx-auto mb-3 opacity-50" />
                <p>Select an alert to view details</p>
              </div>
            )}
          </div>
        </div>
      )}

      {activeTab === 'rules' && (
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700">
            <h3 className="font-semibold">Alert Rules</h3>
          </div>
          <div className="divide-y divide-gray-700">
            {rules.map((rule) => {
              const TypeIcon = typeConfig[rule.type].icon;
              return (
                <div key={rule.id} className="p-4 flex items-center gap-4">
                  <div className={clsx('p-2 rounded-lg', severityConfig[rule.severity].bg)}>
                    <TypeIcon className={clsx('w-5 h-5', severityConfig[rule.severity].color)} />
                  </div>
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{rule.name}</span>
                      <span className={clsx('px-2 py-0.5 rounded-full text-xs', severityConfig[rule.severity].bg, severityConfig[rule.severity].color)}>
                        {rule.severity}
                      </span>
                    </div>
                    <p className="text-sm text-gray-400 mt-1">
                      {typeConfig[rule.type].label} {rule.condition.includes('>') ? '>' : '<'} {rule.threshold}{typeConfig[rule.type].unit} for {rule.duration}
                    </p>
                    <p className="text-xs text-gray-500 mt-1">
                      Applies to: {rule.services.join(', ')}
                    </p>
                  </div>
                  <div className="flex items-center gap-4">
                    <label className="relative inline-flex items-center cursor-pointer">
                      <input
                        type="checkbox"
                        checked={rule.enabled}
                        onChange={() => {
                          setRules(rules.map(r =>
                            r.id === rule.id ? { ...r, enabled: !r.enabled } : r
                          ));
                        }}
                        className="sr-only peer"
                      />
                      <div className="w-11 h-6 bg-gray-700 peer-focus:ring-2 peer-focus:ring-blue-500 rounded-full peer peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-blue-600"></div>
                    </label>
                    <button className="p-2 hover:bg-gray-700 rounded-lg transition-colors">
                      <ChevronRight className="w-4 h-4 text-gray-400" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {activeTab === 'slo' && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {/* SLO Cards */}
          {[
            { name: 'API Gateway Availability', target: 99.9, current: 99.95, service: 'api-gateway', budget: 98.2 },
            { name: 'Order Service Latency P99', target: 500, current: 320, service: 'order-service', budget: 85.5, unit: 'ms' },
            { name: 'Payment Service Error Rate', target: 0.1, current: 0.08, service: 'payment-service', budget: 92.3, unit: '%' },
            { name: 'Notification Delivery', target: 99.5, current: 98.8, service: 'notification-service', budget: 45.2 },
            { name: 'Database Query Latency', target: 100, current: 45, service: 'postgres', budget: 99.1, unit: 'ms' },
            { name: 'Cache Hit Rate', target: 95, current: 97.2, service: 'redis', budget: 100 },
          ].map((slo, index) => (
            <div key={index} className="bg-gray-800 rounded-xl border border-gray-700 p-6">
              <div className="flex items-start justify-between">
                <div>
                  <h4 className="font-semibold">{slo.name}</h4>
                  <p className="text-sm text-gray-400 mt-1">{slo.service}</p>
                </div>
                <div className={clsx(
                  'px-2 py-1 rounded text-xs font-medium',
                  slo.budget > 50 ? 'bg-green-400/10 text-green-400' :
                  slo.budget > 20 ? 'bg-yellow-400/10 text-yellow-400' :
                  'bg-red-400/10 text-red-400'
                )}>
                  {slo.budget.toFixed(1)}% budget remaining
                </div>
              </div>

              <div className="mt-6 flex items-end justify-between">
                <div>
                  <p className="text-xs text-gray-400">Current</p>
                  <p className="text-2xl font-bold">
                    {slo.current}{slo.unit || '%'}
                  </p>
                </div>
                <div className="text-right">
                  <p className="text-xs text-gray-400">Target</p>
                  <p className="text-lg text-gray-300">
                    {slo.unit === 'ms' ? '<' : '>'}{slo.target}{slo.unit || '%'}
                  </p>
                </div>
              </div>

              {/* Progress bar */}
              <div className="mt-4">
                <div className="h-2 bg-gray-700 rounded-full overflow-hidden">
                  <div
                    className={clsx(
                      'h-full rounded-full transition-all',
                      slo.budget > 50 ? 'bg-green-500' :
                      slo.budget > 20 ? 'bg-yellow-500' : 'bg-red-500'
                    )}
                    style={{ width: `${Math.min(100, slo.budget)}%` }}
                  />
                </div>
                <div className="flex justify-between mt-1 text-xs text-gray-500">
                  <span>Error Budget</span>
                  <span>{slo.budget.toFixed(1)}%</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
