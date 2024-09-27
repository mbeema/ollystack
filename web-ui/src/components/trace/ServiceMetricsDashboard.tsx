import { useState, useEffect } from 'react';
import {
  BarChart3,
  AlertTriangle,
  Layers,
  RefreshCw,
  ChevronDown,
  ChevronRight,
  TrendingUp,
  TrendingDown,
  Activity,
  Clock,
  AlertCircle,
  Fingerprint,
  Filter
} from 'lucide-react';
import clsx from 'clsx';
import { getApiUrl } from '../../lib/config';

// Types
interface ServiceAggregation {
  serviceName: string;
  operationName: string;
  requestCount: number;
  errorCount: number;
  errorRate: number;
  p50Ms: number;
  p90Ms: number;
  p99Ms: number;
  avgMs: number;
  minMs: number;
  maxMs: number;
}

interface AggregationResponse {
  aggregations: ServiceAggregation[];
  totalRequests: number;
  totalErrors: number;
  overallP50Ms: number;
  overallP99Ms: number;
  serviceCount: number;
  operationCount: number;
  timeRange: {
    start: string;
    end: string;
  };
}

interface ErrorFingerprint {
  fingerprint: string;
  count: number;
  errorType: string;
  normalizedMessage: string;
  sampleMessage: string;
  services: string[];
  operations: string[];
  firstSeen: string;
  lastSeen: string;
  sampleTraceIds: string[];
}

interface FingerprintResponse {
  fingerprints: ErrorFingerprint[];
  totalErrors: number;
  uniqueFingerprints: number;
}

interface GroupResult {
  groupKey: string;
  groupValues: Record<string, string>;
  count: number;
  errorCount: number;
  errorRate: number;
  p50Ms: number;
  p99Ms: number;
  avgMs: number;
}

interface GroupingResponse {
  groups: GroupResult[];
  groupBy: string[];
  totalGroups: number;
}

type Tab = 'overview' | 'errors' | 'explore';

interface Props {
  className?: string;
}

export function ServiceMetricsDashboard({ className }: Props) {
  const [activeTab, setActiveTab] = useState<Tab>('overview');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Data states
  const [aggregations, setAggregations] = useState<AggregationResponse | null>(null);
  const [fingerprints, setFingerprints] = useState<FingerprintResponse | null>(null);
  const [grouping, setGrouping] = useState<GroupingResponse | null>(null);

  // Filter states
  const [groupByFields, setGroupByFields] = useState<string[]>(['service']);
  const [expandedServices, setExpandedServices] = useState<Set<string>>(new Set());
  const [expandedFingerprints, setExpandedFingerprints] = useState<Set<string>>(new Set());

  const API_URL = getApiUrl();

  useEffect(() => {
    fetchData();
  }, [activeTab, groupByFields]);

  const fetchData = async () => {
    setLoading(true);
    setError(null);

    try {
      switch (activeTab) {
        case 'overview':
          const aggRes = await fetch(`${API_URL}/api/v1/traces/aggregate`);
          if (!aggRes.ok) throw new Error('Failed to fetch aggregations');
          setAggregations(await aggRes.json());
          break;

        case 'errors':
          const fpRes = await fetch(`${API_URL}/api/v1/traces/errors/fingerprints`);
          if (!fpRes.ok) throw new Error('Failed to fetch error fingerprints');
          setFingerprints(await fpRes.json());
          break;

        case 'explore':
          const groupRes = await fetch(`${API_URL}/api/v1/traces/group?groupBy=${groupByFields.join(',')}`);
          if (!groupRes.ok) throw new Error('Failed to fetch grouping');
          setGrouping(await groupRes.json());
          break;
      }
    } catch (err) {
      console.error('Failed to fetch data:', err);
      setError(err instanceof Error ? err.message : 'Failed to load data');
    } finally {
      setLoading(false);
    }
  };

  const formatDuration = (ms: number) => {
    if (ms < 1) return `${(ms * 1000).toFixed(0)}us`;
    if (ms < 1000) return `${ms.toFixed(1)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const formatNumber = (num: number) => {
    if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`;
    if (num >= 1000) return `${(num / 1000).toFixed(1)}K`;
    return num.toString();
  };

  const toggleService = (service: string) => {
    const newSet = new Set(expandedServices);
    if (newSet.has(service)) {
      newSet.delete(service);
    } else {
      newSet.add(service);
    }
    setExpandedServices(newSet);
  };

  const toggleFingerprint = (fp: string) => {
    const newSet = new Set(expandedFingerprints);
    if (newSet.has(fp)) {
      newSet.delete(fp);
    } else {
      newSet.add(fp);
    }
    setExpandedFingerprints(newSet);
  };

  const renderOverviewTab = () => {
    if (!aggregations) return null;

    // Group by service
    const serviceGroups: Record<string, ServiceAggregation[]> = {};
    aggregations.aggregations.forEach(agg => {
      if (!serviceGroups[agg.serviceName]) {
        serviceGroups[agg.serviceName] = [];
      }
      serviceGroups[agg.serviceName].push(agg);
    });

    return (
      <div className="space-y-4">
        {/* Summary Cards */}
        <div className="grid grid-cols-5 gap-4">
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <Activity className="w-4 h-4" />
              Total Requests
            </div>
            <div className="text-2xl font-bold">{formatNumber(aggregations.totalRequests)}</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <AlertTriangle className="w-4 h-4 text-red-400" />
              Total Errors
            </div>
            <div className="text-2xl font-bold text-red-400">{formatNumber(aggregations.totalErrors)}</div>
            <div className="text-xs text-gray-500">
              {((aggregations.totalErrors / aggregations.totalRequests) * 100).toFixed(2)}% error rate
            </div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <Clock className="w-4 h-4" />
              P50 Latency
            </div>
            <div className="text-2xl font-bold">{formatDuration(aggregations.overallP50Ms)}</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <TrendingUp className="w-4 h-4 text-yellow-400" />
              P99 Latency
            </div>
            <div className={clsx(
              "text-2xl font-bold",
              aggregations.overallP99Ms > 1000 ? "text-yellow-400" : ""
            )}>
              {formatDuration(aggregations.overallP99Ms)}
            </div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <Layers className="w-4 h-4 text-purple-400" />
              Services
            </div>
            <div className="text-2xl font-bold">{aggregations.serviceCount}</div>
            <div className="text-xs text-gray-500">{aggregations.operationCount} operations</div>
          </div>
        </div>

        {/* Service List */}
        <div className="bg-gray-800 rounded-lg overflow-hidden">
          <div className="px-4 py-3 border-b border-gray-700">
            <h3 className="font-semibold flex items-center gap-2">
              <BarChart3 className="w-5 h-5" />
              RED Metrics by Service
            </h3>
          </div>
          <div className="divide-y divide-gray-700">
            {Object.entries(serviceGroups).map(([service, operations]) => {
              const isExpanded = expandedServices.has(service);
              const totalReqs = operations.reduce((sum, o) => sum + o.requestCount, 0);
              const totalErrs = operations.reduce((sum, o) => sum + o.errorCount, 0);
              const avgP99 = operations.reduce((sum, o) => sum + o.p99Ms, 0) / operations.length;

              return (
                <div key={service}>
                  <div
                    className="px-4 py-3 flex items-center justify-between hover:bg-gray-750 cursor-pointer"
                    onClick={() => toggleService(service)}
                  >
                    <div className="flex items-center gap-3">
                      {isExpanded ? (
                        <ChevronDown className="w-4 h-4 text-gray-400" />
                      ) : (
                        <ChevronRight className="w-4 h-4 text-gray-400" />
                      )}
                      <span className="font-medium">{service}</span>
                      <span className="text-xs text-gray-500">({operations.length} operations)</span>
                    </div>
                    <div className="flex items-center gap-6 text-sm">
                      <div className="text-right">
                        <div className="text-gray-400">Rate</div>
                        <div className="font-mono">{formatNumber(totalReqs)}</div>
                      </div>
                      <div className="text-right">
                        <div className="text-gray-400">Errors</div>
                        <div className={clsx(
                          "font-mono",
                          totalErrs > 0 ? "text-red-400" : "text-green-400"
                        )}>
                          {totalErrs > 0 ? totalErrs : '0'}
                        </div>
                      </div>
                      <div className="text-right">
                        <div className="text-gray-400">P99</div>
                        <div className={clsx(
                          "font-mono",
                          avgP99 > 1000 ? "text-yellow-400" : ""
                        )}>
                          {formatDuration(avgP99)}
                        </div>
                      </div>
                    </div>
                  </div>
                  {isExpanded && (
                    <div className="bg-gray-850 border-t border-gray-700">
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="text-gray-400 text-left">
                            <th className="px-8 py-2 font-medium">Operation</th>
                            <th className="px-4 py-2 font-medium text-right">Requests</th>
                            <th className="px-4 py-2 font-medium text-right">Errors</th>
                            <th className="px-4 py-2 font-medium text-right">Error Rate</th>
                            <th className="px-4 py-2 font-medium text-right">P50</th>
                            <th className="px-4 py-2 font-medium text-right">P90</th>
                            <th className="px-4 py-2 font-medium text-right">P99</th>
                          </tr>
                        </thead>
                        <tbody>
                          {operations.map((op, idx) => (
                            <tr key={idx} className="border-t border-gray-700 hover:bg-gray-800">
                              <td className="px-8 py-2 font-mono text-blue-400">{op.operationName}</td>
                              <td className="px-4 py-2 text-right font-mono">{formatNumber(op.requestCount)}</td>
                              <td className={clsx(
                                "px-4 py-2 text-right font-mono",
                                op.errorCount > 0 ? "text-red-400" : "text-green-400"
                              )}>
                                {op.errorCount}
                              </td>
                              <td className={clsx(
                                "px-4 py-2 text-right font-mono",
                                op.errorRate > 5 ? "text-red-400" : op.errorRate > 1 ? "text-yellow-400" : ""
                              )}>
                                {op.errorRate.toFixed(2)}%
                              </td>
                              <td className="px-4 py-2 text-right font-mono">{formatDuration(op.p50Ms)}</td>
                              <td className="px-4 py-2 text-right font-mono">{formatDuration(op.p90Ms)}</td>
                              <td className={clsx(
                                "px-4 py-2 text-right font-mono",
                                op.p99Ms > 1000 ? "text-yellow-400" : ""
                              )}>
                                {formatDuration(op.p99Ms)}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      </div>
    );
  };

  const renderErrorsTab = () => {
    if (!fingerprints) return null;

    return (
      <div className="space-y-4">
        {/* Summary */}
        <div className="grid grid-cols-3 gap-4">
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <AlertTriangle className="w-4 h-4 text-red-400" />
              Total Errors
            </div>
            <div className="text-2xl font-bold text-red-400">{formatNumber(fingerprints.totalErrors)}</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <Fingerprint className="w-4 h-4 text-purple-400" />
              Unique Patterns
            </div>
            <div className="text-2xl font-bold text-purple-400">{fingerprints.uniqueFingerprints}</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center gap-2 text-gray-400 text-sm mb-1">
              <TrendingDown className="w-4 h-4 text-green-400" />
              Noise Reduction
            </div>
            <div className="text-2xl font-bold text-green-400">
              {fingerprints.totalErrors > 0
                ? `${((1 - fingerprints.uniqueFingerprints / fingerprints.totalErrors) * 100).toFixed(0)}%`
                : '0%'}
            </div>
            <div className="text-xs text-gray-500">errors grouped into patterns</div>
          </div>
        </div>

        {/* Fingerprints List */}
        <div className="bg-gray-800 rounded-lg overflow-hidden">
          <div className="px-4 py-3 border-b border-gray-700">
            <h3 className="font-semibold flex items-center gap-2">
              <Fingerprint className="w-5 h-5" />
              Error Fingerprints
              <span className="text-sm text-gray-400 font-normal ml-2">
                Similar errors grouped together
              </span>
            </h3>
          </div>
          <div className="divide-y divide-gray-700">
            {fingerprints.fingerprints.map((fp) => {
              const isExpanded = expandedFingerprints.has(fp.fingerprint);

              return (
                <div key={fp.fingerprint}>
                  <div
                    className="px-4 py-3 hover:bg-gray-750 cursor-pointer"
                    onClick={() => toggleFingerprint(fp.fingerprint)}
                  >
                    <div className="flex items-start justify-between">
                      <div className="flex items-start gap-3 flex-1">
                        {isExpanded ? (
                          <ChevronDown className="w-4 h-4 text-gray-400 mt-1 flex-shrink-0" />
                        ) : (
                          <ChevronRight className="w-4 h-4 text-gray-400 mt-1 flex-shrink-0" />
                        )}
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 mb-1">
                            <span className={clsx(
                              "px-2 py-0.5 rounded text-xs font-medium",
                              fp.errorType === 'exception' ? 'bg-red-900/50 text-red-300' :
                              fp.errorType === 'http_error' ? 'bg-orange-900/50 text-orange-300' :
                              'bg-gray-700 text-gray-300'
                            )}>
                              {fp.errorType}
                            </span>
                            <span className="font-mono text-xs text-gray-500">
                              #{fp.fingerprint}
                            </span>
                          </div>
                          <div className="text-sm text-gray-300 truncate" title={fp.normalizedMessage}>
                            {fp.normalizedMessage}
                          </div>
                          <div className="flex items-center gap-4 mt-1 text-xs text-gray-500">
                            <span>{fp.services.slice(0, 3).join(', ')}{fp.services.length > 3 ? ` +${fp.services.length - 3}` : ''}</span>
                            <span>First: {new Date(fp.firstSeen).toLocaleString()}</span>
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-4 ml-4">
                        <div className="text-right">
                          <div className="text-2xl font-bold text-red-400">{fp.count}</div>
                          <div className="text-xs text-gray-500">occurrences</div>
                        </div>
                      </div>
                    </div>
                  </div>
                  {isExpanded && (
                    <div className="bg-gray-850 border-t border-gray-700 px-4 py-3 space-y-3">
                      <div>
                        <div className="text-xs text-gray-400 mb-1">Sample Error Message:</div>
                        <div className="bg-gray-900 rounded p-2 text-sm font-mono text-red-300 overflow-x-auto">
                          {fp.sampleMessage}
                        </div>
                      </div>
                      <div className="grid grid-cols-2 gap-4">
                        <div>
                          <div className="text-xs text-gray-400 mb-1">Affected Services:</div>
                          <div className="flex flex-wrap gap-1">
                            {fp.services.map((svc, i) => (
                              <span key={i} className="px-2 py-0.5 bg-gray-700 rounded text-xs">
                                {svc}
                              </span>
                            ))}
                          </div>
                        </div>
                        <div>
                          <div className="text-xs text-gray-400 mb-1">Affected Operations:</div>
                          <div className="flex flex-wrap gap-1">
                            {fp.operations.map((op, i) => (
                              <span key={i} className="px-2 py-0.5 bg-gray-700 rounded text-xs font-mono">
                                {op}
                              </span>
                            ))}
                          </div>
                        </div>
                      </div>
                      <div>
                        <div className="text-xs text-gray-400 mb-1">Sample Traces:</div>
                        <div className="flex flex-wrap gap-2">
                          {fp.sampleTraceIds.slice(0, 5).map((tid, i) => (
                            <a
                              key={i}
                              href={`/traces/${tid}`}
                              className="font-mono text-xs text-blue-400 hover:text-blue-300"
                            >
                              {tid.substring(0, 16)}...
                            </a>
                          ))}
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
            {fingerprints.fingerprints.length === 0 && (
              <div className="px-4 py-8 text-center text-gray-400">
                <AlertCircle className="w-12 h-12 mx-auto mb-3 opacity-50" />
                <p>No errors found in the selected time range</p>
              </div>
            )}
          </div>
        </div>
      </div>
    );
  };

  const renderExploreTab = () => {
    const groupByOptions = [
      { value: 'service', label: 'Service' },
      { value: 'operation', label: 'Operation' },
      { value: 'status', label: 'Status' },
      { value: 'http.method', label: 'HTTP Method' },
      { value: 'http.status_code', label: 'HTTP Status' },
    ];

    return (
      <div className="space-y-4">
        {/* Group By Selector */}
        <div className="bg-gray-800 rounded-lg p-4">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2 text-gray-400">
              <Filter className="w-4 h-4" />
              <span>Group By:</span>
            </div>
            <div className="flex flex-wrap gap-2">
              {groupByOptions.map((opt) => (
                <button
                  key={opt.value}
                  onClick={() => {
                    if (groupByFields.includes(opt.value)) {
                      setGroupByFields(groupByFields.filter(f => f !== opt.value));
                    } else {
                      setGroupByFields([...groupByFields, opt.value]);
                    }
                  }}
                  className={clsx(
                    "px-3 py-1.5 rounded text-sm transition-colors",
                    groupByFields.includes(opt.value)
                      ? "bg-blue-600 text-white"
                      : "bg-gray-700 text-gray-300 hover:bg-gray-600"
                  )}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* Results */}
        {grouping && (
          <div className="bg-gray-800 rounded-lg overflow-hidden">
            <div className="px-4 py-3 border-b border-gray-700 flex items-center justify-between">
              <h3 className="font-semibold flex items-center gap-2">
                <Layers className="w-5 h-5" />
                Dynamic Grouping Results
              </h3>
              <span className="text-sm text-gray-400">{grouping.totalGroups} groups</span>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-700 text-left">
                    {groupByFields.map((field) => (
                      <th key={field} className="px-4 py-3 font-medium text-gray-400">{field}</th>
                    ))}
                    <th className="px-4 py-3 font-medium text-gray-400 text-right">Requests</th>
                    <th className="px-4 py-3 font-medium text-gray-400 text-right">Errors</th>
                    <th className="px-4 py-3 font-medium text-gray-400 text-right">Error Rate</th>
                    <th className="px-4 py-3 font-medium text-gray-400 text-right">P50</th>
                    <th className="px-4 py-3 font-medium text-gray-400 text-right">P99</th>
                  </tr>
                </thead>
                <tbody>
                  {grouping.groups.map((group, idx) => (
                    <tr key={idx} className="border-b border-gray-700 hover:bg-gray-750">
                      {groupByFields.map((field) => (
                        <td key={field} className="px-4 py-3">
                          <span className="px-2 py-0.5 bg-gray-700 rounded text-xs">
                            {group.groupValues[field] || '-'}
                          </span>
                        </td>
                      ))}
                      <td className="px-4 py-3 text-right font-mono">{formatNumber(group.count)}</td>
                      <td className={clsx(
                        "px-4 py-3 text-right font-mono",
                        group.errorCount > 0 ? "text-red-400" : "text-green-400"
                      )}>
                        {group.errorCount}
                      </td>
                      <td className={clsx(
                        "px-4 py-3 text-right font-mono",
                        group.errorRate > 5 ? "text-red-400" : group.errorRate > 1 ? "text-yellow-400" : ""
                      )}>
                        {group.errorRate.toFixed(2)}%
                      </td>
                      <td className="px-4 py-3 text-right font-mono">{formatDuration(group.p50Ms)}</td>
                      <td className={clsx(
                        "px-4 py-3 text-right font-mono",
                        group.p99Ms > 1000 ? "text-yellow-400" : ""
                      )}>
                        {formatDuration(group.p99Ms)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {grouping.groups.length === 0 && (
              <div className="px-4 py-8 text-center text-gray-400">
                <Layers className="w-12 h-12 mx-auto mb-3 opacity-50" />
                <p>No data found for selected grouping</p>
              </div>
            )}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className={clsx("space-y-4", className)}>
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h2 className="text-xl font-bold">Service Metrics</h2>
          {/* Tabs */}
          <div className="flex bg-gray-800 rounded-lg p-1">
            <button
              onClick={() => setActiveTab('overview')}
              className={clsx(
                "px-4 py-1.5 rounded text-sm transition-colors flex items-center gap-2",
                activeTab === 'overview'
                  ? "bg-blue-600 text-white"
                  : "text-gray-400 hover:text-white"
              )}
            >
              <BarChart3 className="w-4 h-4" />
              RED Metrics
            </button>
            <button
              onClick={() => setActiveTab('errors')}
              className={clsx(
                "px-4 py-1.5 rounded text-sm transition-colors flex items-center gap-2",
                activeTab === 'errors'
                  ? "bg-red-600 text-white"
                  : "text-gray-400 hover:text-white"
              )}
            >
              <Fingerprint className="w-4 h-4" />
              Error Fingerprints
            </button>
            <button
              onClick={() => setActiveTab('explore')}
              className={clsx(
                "px-4 py-1.5 rounded text-sm transition-colors flex items-center gap-2",
                activeTab === 'explore'
                  ? "bg-purple-600 text-white"
                  : "text-gray-400 hover:text-white"
              )}
            >
              <Layers className="w-4 h-4" />
              Dynamic Grouping
            </button>
          </div>
        </div>
        <button
          onClick={fetchData}
          disabled={loading}
          className="p-2 hover:bg-gray-700 rounded transition-colors disabled:opacity-50"
        >
          <RefreshCw className={clsx("w-4 h-4", loading && "animate-spin")} />
        </button>
      </div>

      {/* Error */}
      {error && (
        <div className="bg-red-900/20 border border-red-500 text-red-400 px-4 py-3 rounded">
          {error}
        </div>
      )}

      {/* Loading */}
      {loading && !aggregations && !fingerprints && !grouping && (
        <div className="bg-gray-800 rounded-lg p-8 text-center">
          <RefreshCw className="w-8 h-8 animate-spin mx-auto mb-2 text-gray-400" />
          <p className="text-gray-400">Loading metrics...</p>
        </div>
      )}

      {/* Tab Content */}
      {activeTab === 'overview' && renderOverviewTab()}
      {activeTab === 'errors' && renderErrorsTab()}
      {activeTab === 'explore' && renderExploreTab()}
    </div>
  );
}
