import { useState, useEffect, useCallback } from 'react';
import {
  Search, RefreshCw, Filter, ChevronDown, ChevronRight,
  Clock, Box, AlertCircle, Info, AlertTriangle, Bug,
  Link2, X, Play, Pause, Download
} from 'lucide-react';
import { getApiUrl } from '../lib/config';
import { useNavigate, useSearchParams } from 'react-router-dom';

interface LogEntry {
  timestamp: string;
  severityText: string;
  serviceName: string;
  body: string;
  attributes?: Record<string, string>;
}

interface ParsedLog {
  timestamp: string;
  level: string;
  service: string;
  message: string;
  correlationId?: string;
  container?: string;
  raw: string;
  extra: Record<string, unknown>;
}

const TIME_RANGES = [
  { label: 'Last 15 minutes', value: '15m' },
  { label: 'Last 1 hour', value: '1h' },
  { label: 'Last 6 hours', value: '6h' },
  { label: 'Last 24 hours', value: '24h' },
  { label: 'Last 7 days', value: '7d' },
];

const LOG_LEVELS = [
  { value: 'error', label: 'Error', color: 'bg-red-500', textColor: 'text-red-400', bgColor: 'bg-red-500/20' },
  { value: 'warn', label: 'Warn', color: 'bg-yellow-500', textColor: 'text-yellow-400', bgColor: 'bg-yellow-500/20' },
  { value: 'info', label: 'Info', color: 'bg-blue-500', textColor: 'text-blue-400', bgColor: 'bg-blue-500/20' },
  { value: 'debug', label: 'Debug', color: 'bg-gray-500', textColor: 'text-gray-400', bgColor: 'bg-gray-500/20' },
];

export default function LogsPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const initialService = searchParams.get('service');

  const [logs, setLogs] = useState<ParsedLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [limit, setLimit] = useState(200);
  const [timeRange, setTimeRange] = useState('1h');
  const [selectedLevels, setSelectedLevels] = useState<string[]>(['error', 'warn', 'info', 'debug']);
  const [selectedServices, setSelectedServices] = useState<string[]>(initialService ? [initialService] : []);
  const [availableServices, setAvailableServices] = useState<string[]>([]);
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
  const [showFilters, setShowFilters] = useState(true);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [correlationFilter, setCorrelationFilter] = useState('');

  const parseLogBody = (log: LogEntry): ParsedLog => {
    let parsed: ParsedLog = {
      timestamp: log.timestamp,
      level: log.severityText?.toLowerCase() || 'info',
      service: log.serviceName || '',
      message: log.body,
      raw: log.body,
      container: log.attributes?.container_id || '',
      extra: {},
    };

    // Try to parse JSON body
    try {
      const body = log.body.replace(/\\n$/, '');
      if (body.startsWith('{')) {
        const json = JSON.parse(body);
        parsed.level = json.level || parsed.level;
        parsed.service = json.service || parsed.service;
        parsed.message = json.message || body;
        parsed.correlationId = json.correlation_id;
        // Store extra fields
        const { level, service, message, correlation_id, timestamp, ...rest } = json;
        parsed.extra = rest;
      }
    } catch {
      // Keep original body
    }

    return parsed;
  };

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    try {
      const API_URL = getApiUrl();
      const params = new URLSearchParams({ limit: limit.toString(), timeRange });
      if (search) params.append('query', search);
      if (selectedLevels.length > 0 && selectedLevels.length < 4) {
        params.append('severity', selectedLevels.join(','));
      }
      if (selectedServices.length === 1) {
        params.append('service', selectedServices[0]);
      }

      const response = await fetch(`${API_URL}/api/v1/logs/query?${params}`);
      const data = await response.json();

      const parsedLogs = (data.logs || []).map(parseLogBody);
      setLogs(parsedLogs);

      // Extract unique services
      const services = [...new Set(parsedLogs.map((l: ParsedLog) => l.service).filter(Boolean))] as string[];
      setAvailableServices(services.sort());
    } catch (error) {
      console.error('Failed to fetch logs:', error);
    } finally {
      setLoading(false);
    }
  }, [limit, search, timeRange, selectedLevels, selectedServices]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  useEffect(() => {
    if (autoRefresh) {
      const interval = setInterval(fetchLogs, 5000);
      return () => clearInterval(interval);
    }
  }, [autoRefresh, fetchLogs]);

  const toggleLevel = (level: string) => {
    setSelectedLevels(prev =>
      prev.includes(level) ? prev.filter(l => l !== level) : [...prev, level]
    );
  };

  const toggleService = (service: string) => {
    setSelectedServices(prev =>
      prev.includes(service) ? prev.filter(s => s !== service) : [...prev, service]
    );
  };

  const toggleRow = (index: number) => {
    setExpandedRows(prev => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  };

  const filteredLogs = logs.filter(log => {
    if (!selectedLevels.includes(log.level)) return false;
    if (selectedServices.length > 0 && !selectedServices.includes(log.service)) return false;
    if (correlationFilter && log.correlationId !== correlationFilter) return false;
    return true;
  });

  const getLevelIcon = (level: string) => {
    switch (level) {
      case 'error': return <AlertCircle className="w-4 h-4" />;
      case 'warn': return <AlertTriangle className="w-4 h-4" />;
      case 'info': return <Info className="w-4 h-4" />;
      case 'debug': return <Bug className="w-4 h-4" />;
      default: return <Info className="w-4 h-4" />;
    }
  };

  const getLevelStyle = (level: string) => {
    const config = LOG_LEVELS.find(l => l.value === level) || LOG_LEVELS[2];
    return { textColor: config.textColor, bgColor: config.bgColor };
  };

  const formatTime = (ts: string) => {
    try {
      const date = new Date(ts);
      const time = date.toLocaleTimeString('en-US', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      });
      const ms = date.getMilliseconds().toString().padStart(3, '0');
      return `${time}.${ms}`;
    } catch {
      return ts;
    }
  };

  const formatDate = (ts: string) => {
    try {
      return new Date(ts).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    } catch {
      return '';
    }
  };

  const exportLogs = () => {
    const data = filteredLogs.map(l => ({
      timestamp: l.timestamp,
      level: l.level,
      service: l.service,
      message: l.message,
      correlation_id: l.correlationId,
      ...l.extra
    }));
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `logs-${new Date().toISOString()}.json`;
    a.click();
  };

  const levelCounts = logs.reduce((acc, log) => {
    acc[log.level] = (acc[log.level] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);

  return (
    <div className="h-full flex bg-gray-900 text-white">
      {/* Filters Sidebar */}
      {showFilters && (
        <div className="w-64 flex-shrink-0 border-r border-gray-700 overflow-y-auto">
          <div className="p-4 space-y-6">
            {/* Time Range */}
            <div>
              <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2 flex items-center gap-2">
                <Clock className="w-4 h-4" />
                Time Range
              </h3>
              <div className="space-y-1">
                {TIME_RANGES.map(range => (
                  <button
                    key={range.value}
                    onClick={() => setTimeRange(range.value)}
                    className={`w-full text-left px-3 py-1.5 rounded text-sm transition-colors ${
                      timeRange === range.value
                        ? 'bg-blue-600 text-white'
                        : 'text-gray-300 hover:bg-gray-800'
                    }`}
                  >
                    {range.label}
                  </button>
                ))}
              </div>
            </div>

            {/* Log Levels */}
            <div>
              <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2">
                Log Level
              </h3>
              <div className="space-y-1">
                {LOG_LEVELS.map(level => (
                  <label
                    key={level.value}
                    className="flex items-center gap-3 px-3 py-1.5 rounded hover:bg-gray-800 cursor-pointer"
                  >
                    <input
                      type="checkbox"
                      checked={selectedLevels.includes(level.value)}
                      onChange={() => toggleLevel(level.value)}
                      className="hidden"
                    />
                    <div className={`w-4 h-4 rounded flex items-center justify-center ${
                      selectedLevels.includes(level.value) ? level.color : 'bg-gray-700'
                    }`}>
                      {selectedLevels.includes(level.value) && (
                        <svg className="w-3 h-3 text-white" fill="currentColor" viewBox="0 0 20 20">
                          <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                        </svg>
                      )}
                    </div>
                    <span className={`text-sm ${level.textColor}`}>{level.label}</span>
                    <span className="text-xs text-gray-500 ml-auto">
                      {levelCounts[level.value] || 0}
                    </span>
                  </label>
                ))}
              </div>
            </div>

            {/* Services */}
            <div>
              <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2 flex items-center gap-2">
                <Box className="w-4 h-4" />
                Service / Container
              </h3>
              {availableServices.length === 0 ? (
                <p className="text-sm text-gray-500 px-3">No services found</p>
              ) : (
                <div className="space-y-1 max-h-48 overflow-y-auto">
                  {availableServices.map(service => (
                    <label
                      key={service}
                      className="flex items-center gap-3 px-3 py-1.5 rounded hover:bg-gray-800 cursor-pointer"
                    >
                      <input
                        type="checkbox"
                        checked={selectedServices.length === 0 || selectedServices.includes(service)}
                        onChange={() => toggleService(service)}
                        className="hidden"
                      />
                      <div className={`w-4 h-4 rounded border ${
                        selectedServices.length === 0 || selectedServices.includes(service)
                          ? 'bg-purple-500 border-purple-500'
                          : 'border-gray-600'
                      } flex items-center justify-center`}>
                        {(selectedServices.length === 0 || selectedServices.includes(service)) && (
                          <svg className="w-3 h-3 text-white" fill="currentColor" viewBox="0 0 20 20">
                            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                          </svg>
                        )}
                      </div>
                      <span className="text-sm text-gray-300 truncate">{service}</span>
                    </label>
                  ))}
                </div>
              )}
            </div>

            {/* Correlation ID Filter */}
            {correlationFilter && (
              <div>
                <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2 flex items-center gap-2">
                  <Link2 className="w-4 h-4" />
                  Correlation Filter
                </h3>
                <div className="flex items-center gap-2 px-3 py-2 bg-gray-800 rounded">
                  <span className="text-xs text-blue-400 truncate flex-1">{correlationFilter}</span>
                  <button
                    onClick={() => setCorrelationFilter('')}
                    className="text-gray-400 hover:text-white"
                  >
                    <X className="w-4 h-4" />
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Main Content */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* Header */}
        <div className="flex-shrink-0 p-4 border-b border-gray-700 bg-gray-900/95 backdrop-blur">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-3">
              <button
                onClick={() => setShowFilters(!showFilters)}
                className={`p-2 rounded-lg transition-colors ${
                  showFilters ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400 hover:text-white'
                }`}
              >
                <Filter className="w-5 h-5" />
              </button>
              <h1 className="text-xl font-semibold">Logs Explorer</h1>
              <span className="text-sm text-gray-500">
                {filteredLogs.length.toLocaleString()} of {logs.length.toLocaleString()} logs
              </span>
            </div>

            <div className="flex items-center gap-2">
              <button
                onClick={() => setAutoRefresh(!autoRefresh)}
                className={`flex items-center gap-2 px-3 py-2 rounded-lg transition-colors ${
                  autoRefresh ? 'bg-green-600 text-white' : 'bg-gray-800 text-gray-400 hover:text-white'
                }`}
              >
                {autoRefresh ? <Pause className="w-4 h-4" /> : <Play className="w-4 h-4" />}
                {autoRefresh ? 'Live' : 'Paused'}
              </button>
              <button
                onClick={fetchLogs}
                disabled={loading}
                className="flex items-center gap-2 px-3 py-2 bg-gray-800 hover:bg-gray-700 rounded-lg transition-colors"
              >
                <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
                Refresh
              </button>
              <button
                onClick={exportLogs}
                className="flex items-center gap-2 px-3 py-2 bg-gray-800 hover:bg-gray-700 rounded-lg transition-colors"
              >
                <Download className="w-4 h-4" />
                Export
              </button>
            </div>
          </div>

          {/* Search Bar */}
          <div className="flex gap-3">
            <div className="flex-1 relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-5 h-5 text-gray-400" />
              <input
                type="text"
                placeholder="Search logs by message, service, or correlation ID..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && fetchLogs()}
                className="w-full pl-11 pr-4 py-2.5 bg-gray-800 border border-gray-700 rounded-lg focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 text-sm"
              />
            </div>
            <select
              value={limit}
              onChange={(e) => setLimit(Number(e.target.value))}
              className="px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-sm focus:outline-none focus:border-blue-500"
            >
              <option value={100}>100 logs</option>
              <option value={200}>200 logs</option>
              <option value={500}>500 logs</option>
              <option value={1000}>1000 logs</option>
            </select>
          </div>
        </div>

        {/* Logs List */}
        <div className="flex-1 overflow-auto">
          {loading && logs.length === 0 ? (
            <div className="flex items-center justify-center h-32">
              <RefreshCw className="w-8 h-8 animate-spin text-blue-500" />
            </div>
          ) : filteredLogs.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-64 text-gray-500">
              <AlertCircle className="w-12 h-12 mb-4 opacity-50" />
              <p className="text-lg">No logs found</p>
              <p className="text-sm mt-1">Try adjusting your filters or time range</p>
            </div>
          ) : (
            <div className="font-mono text-sm">
              {filteredLogs.map((log, index) => {
                const { textColor, bgColor } = getLevelStyle(log.level);
                const isExpanded = expandedRows.has(index);

                return (
                  <div key={index} className="border-b border-gray-800 hover:bg-gray-800/50">
                    {/* Log Row */}
                    <div
                      className="flex items-start gap-3 px-4 py-2 cursor-pointer"
                      onClick={() => toggleRow(index)}
                    >
                      {/* Expand Icon */}
                      <button className="mt-0.5 text-gray-500">
                        {isExpanded ? (
                          <ChevronDown className="w-4 h-4" />
                        ) : (
                          <ChevronRight className="w-4 h-4" />
                        )}
                      </button>

                      {/* Timestamp */}
                      <div className="flex-shrink-0 w-28 text-gray-500">
                        <div className="text-xs text-gray-600">{formatDate(log.timestamp)}</div>
                        <div>{formatTime(log.timestamp)}</div>
                      </div>

                      {/* Level Badge */}
                      <div className={`flex-shrink-0 flex items-center gap-1.5 px-2 py-0.5 rounded ${bgColor} ${textColor}`}>
                        {getLevelIcon(log.level)}
                        <span className="text-xs font-medium uppercase">{log.level}</span>
                      </div>

                      {/* Service Badge */}
                      {log.service && (
                        <span className="flex-shrink-0 px-2 py-0.5 rounded bg-purple-500/20 text-purple-400 text-xs">
                          {log.service}
                        </span>
                      )}

                      {/* Message */}
                      <span className="flex-1 text-gray-200 truncate">
                        {log.message}
                      </span>

                      {/* Correlation ID Link */}
                      {log.correlationId && (
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            navigate(`/correlations?id=${log.correlationId}`);
                          }}
                          className="flex-shrink-0 flex items-center gap-1 px-2 py-0.5 rounded bg-blue-500/20 text-blue-400 text-xs hover:bg-blue-500/30 transition-colors"
                          title={`View correlation ${log.correlationId}`}
                        >
                          <Link2 className="w-3 h-3" />
                          {log.correlationId.slice(0, 12)}...
                        </button>
                      )}
                    </div>

                    {/* Expanded Details */}
                    {isExpanded && (
                      <div className="px-4 py-3 bg-gray-800/50 border-t border-gray-700">
                        <div className="grid grid-cols-2 gap-4 text-xs mb-3">
                          <div>
                            <span className="text-gray-500">Full Timestamp:</span>
                            <span className="ml-2 text-gray-300">{log.timestamp}</span>
                          </div>
                          {log.container && (
                            <div>
                              <span className="text-gray-500">Container:</span>
                              <span className="ml-2 text-gray-300">{log.container}</span>
                            </div>
                          )}
                          {log.correlationId && (
                            <div className="col-span-2 flex items-center gap-4">
                              <div>
                                <span className="text-gray-500">Correlation ID:</span>
                                <span className="ml-2 text-blue-400 font-mono">{log.correlationId}</span>
                              </div>
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  navigate(`/correlations?id=${log.correlationId}`);
                                }}
                                className="px-2 py-1 rounded bg-blue-600 hover:bg-blue-700 text-white text-xs transition-colors"
                              >
                                View in Correlations
                              </button>
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setCorrelationFilter(log.correlationId!);
                                }}
                                className="px-2 py-1 rounded bg-gray-700 hover:bg-gray-600 text-gray-300 text-xs transition-colors"
                              >
                                Filter Logs
                              </button>
                            </div>
                          )}
                        </div>

                        {/* Extra Fields */}
                        {Object.keys(log.extra).length > 0 && (
                          <div className="mt-2">
                            <div className="text-xs text-gray-500 mb-1">Additional Fields:</div>
                            <pre className="text-xs text-gray-300 bg-gray-900 rounded p-2 overflow-x-auto">
                              {JSON.stringify(log.extra, null, 2)}
                            </pre>
                          </div>
                        )}

                        {/* Raw Log */}
                        <div className="mt-2">
                          <div className="text-xs text-gray-500 mb-1">Raw Log:</div>
                          <pre className="text-xs text-gray-400 bg-gray-900 rounded p-2 overflow-x-auto whitespace-pre-wrap break-all">
                            {log.raw}
                          </pre>
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex-shrink-0 px-4 py-2 border-t border-gray-700 bg-gray-900/95 flex items-center justify-between text-xs text-gray-500">
          <div className="flex items-center gap-4">
            <span>Showing {filteredLogs.length.toLocaleString()} logs</span>
            {autoRefresh && (
              <span className="flex items-center gap-1 text-green-400">
                <span className="w-2 h-2 bg-green-400 rounded-full animate-pulse" />
                Live updating
              </span>
            )}
          </div>
          <div className="flex items-center gap-4">
            {LOG_LEVELS.map(level => (
              <span key={level.value} className={`flex items-center gap-1 ${level.textColor}`}>
                <span className={`w-2 h-2 rounded-full ${level.color}`} />
                {levelCounts[level.value] || 0} {level.label}
              </span>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
