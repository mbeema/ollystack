import { useState, useMemo } from 'react';
import {
  X,
  Filter,
  ChevronDown,
  ChevronUp,
  Check,
  AlertCircle,
  Clock,
  Server,
  Tag,
  RotateCcw,
} from 'lucide-react';
import { useTraceStore } from '../../stores/traceStore';
import { formatDuration } from '../../types/trace';
import { clsx } from 'clsx';

interface FilterPanelProps {
  isOpen: boolean;
  onClose: () => void;
}

export function FilterPanel({ isOpen, onClose }: FilterPanelProps) {
  const {
    trace,
    filter,
    setServiceFilter,
    setStatusFilter,
    setDurationFilter,
    filteredSpans,
  } = useTraceStore();

  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set(['services', 'status', 'duration'])
  );

  // Get available services and their stats
  const serviceStats = useMemo(() => {
    if (!trace) return [];

    const stats = new Map<string, { count: number; errorCount: number; color: string }>();

    trace.spans.forEach(span => {
      if (!stats.has(span.serviceName)) {
        stats.set(span.serviceName, {
          count: 0,
          errorCount: 0,
          color: span.serviceColor,
        });
      }
      const stat = stats.get(span.serviceName)!;
      stat.count++;
      if (span.status === 'ERROR') stat.errorCount++;
    });

    return Array.from(stats.entries())
      .map(([name, stat]) => ({
        name,
        ...stat,
      }))
      .sort((a, b) => b.count - a.count);
  }, [trace]);

  // Duration stats
  const durationStats = useMemo(() => {
    if (!trace) return { min: 0, max: 0, avg: 0 };

    const durations = trace.spans.map(s => s.duration);
    return {
      min: Math.min(...durations),
      max: Math.max(...durations),
      avg: durations.reduce((a, b) => a + b, 0) / durations.length,
    };
  }, [trace]);

  const toggleSection = (section: string) => {
    const newSet = new Set(expandedSections);
    if (newSet.has(section)) {
      newSet.delete(section);
    } else {
      newSet.add(section);
    }
    setExpandedSections(newSet);
  };

  const handleServiceToggle = (serviceName: string) => {
    const newServices = filter.services.includes(serviceName)
      ? filter.services.filter(s => s !== serviceName)
      : [...filter.services, serviceName];
    setServiceFilter(newServices);
  };

  const handleStatusToggle = (status: 'OK' | 'ERROR') => {
    const newStatuses = filter.status.includes(status)
      ? filter.status.filter(s => s !== status)
      : [...filter.status, status];
    setStatusFilter(newStatuses);
  };

  const handleDurationChange = (type: 'min' | 'max', value: string) => {
    const numValue = value === '' ? null : parseFloat(value) * 1000; // Convert ms to μs
    setDurationFilter(
      type === 'min' ? numValue : filter.minDuration,
      type === 'max' ? numValue : filter.maxDuration
    );
  };

  const resetFilters = () => {
    setServiceFilter([]);
    setStatusFilter([]);
    setDurationFilter(null, null);
  };

  const activeFilterCount =
    filter.services.length +
    filter.status.length +
    (filter.minDuration !== null ? 1 : 0) +
    (filter.maxDuration !== null ? 1 : 0);

  if (!isOpen) return null;

  return (
    <div className="absolute top-full left-0 mt-2 w-80 bg-gray-800 rounded-lg border border-gray-700 shadow-xl z-50">
      {/* Header */}
      <div className="flex items-center justify-between p-3 border-b border-gray-700">
        <div className="flex items-center gap-2">
          <Filter className="w-4 h-4 text-gray-400" />
          <span className="font-medium text-white">Filter Spans</span>
          {activeFilterCount > 0 && (
            <span className="px-1.5 py-0.5 bg-blue-500 text-white text-xs rounded-full">
              {activeFilterCount}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          {activeFilterCount > 0 && (
            <button
              onClick={resetFilters}
              className="p-1 rounded hover:bg-gray-700 text-gray-400 hover:text-white"
              title="Reset filters"
            >
              <RotateCcw className="w-4 h-4" />
            </button>
          )}
          <button
            onClick={onClose}
            className="p-1 rounded hover:bg-gray-700 text-gray-400 hover:text-white"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Filter results count */}
      <div className="px-3 py-2 bg-gray-700/30 text-xs text-gray-400">
        Showing {filteredSpans.length} of {trace?.spanCount || 0} spans
      </div>

      <div className="max-h-[400px] overflow-auto">
        {/* Services Section */}
        <FilterSection
          title="Services"
          icon={<Server className="w-4 h-4" />}
          isExpanded={expandedSections.has('services')}
          onToggle={() => toggleSection('services')}
          badge={filter.services.length > 0 ? filter.services.length : undefined}
        >
          <div className="space-y-1">
            {serviceStats.map(service => (
              <label
                key={service.name}
                className={clsx(
                  'flex items-center gap-2 p-2 rounded cursor-pointer transition-colors',
                  filter.services.includes(service.name)
                    ? 'bg-blue-500/20'
                    : 'hover:bg-gray-700/50'
                )}
              >
                <input
                  type="checkbox"
                  checked={
                    filter.services.length === 0 || filter.services.includes(service.name)
                  }
                  onChange={() => handleServiceToggle(service.name)}
                  className="sr-only"
                />
                <div
                  className={clsx(
                    'w-4 h-4 rounded border-2 flex items-center justify-center transition-colors',
                    filter.services.includes(service.name) || filter.services.length === 0
                      ? 'bg-blue-500 border-blue-500'
                      : 'border-gray-500'
                  )}
                >
                  {(filter.services.includes(service.name) || filter.services.length === 0) && (
                    <Check className="w-3 h-3 text-white" />
                  )}
                </div>
                <div
                  className="w-2 h-2 rounded-full flex-shrink-0"
                  style={{ backgroundColor: service.color }}
                />
                <span className="text-sm text-gray-200 flex-1 truncate">{service.name}</span>
                <span className="text-xs text-gray-500">{service.count}</span>
                {service.errorCount > 0 && (
                  <span className="text-xs text-red-400">({service.errorCount} err)</span>
                )}
              </label>
            ))}
          </div>
        </FilterSection>

        {/* Status Section */}
        <FilterSection
          title="Status"
          icon={<Tag className="w-4 h-4" />}
          isExpanded={expandedSections.has('status')}
          onToggle={() => toggleSection('status')}
          badge={filter.status.length > 0 ? filter.status.length : undefined}
        >
          <div className="space-y-1">
            <label
              className={clsx(
                'flex items-center gap-2 p-2 rounded cursor-pointer transition-colors',
                filter.status.includes('OK') ? 'bg-blue-500/20' : 'hover:bg-gray-700/50'
              )}
            >
              <input
                type="checkbox"
                checked={filter.status.length === 0 || filter.status.includes('OK')}
                onChange={() => handleStatusToggle('OK')}
                className="sr-only"
              />
              <div
                className={clsx(
                  'w-4 h-4 rounded border-2 flex items-center justify-center transition-colors',
                  filter.status.includes('OK') || filter.status.length === 0
                    ? 'bg-blue-500 border-blue-500'
                    : 'border-gray-500'
                )}
              >
                {(filter.status.includes('OK') || filter.status.length === 0) && (
                  <Check className="w-3 h-3 text-white" />
                )}
              </div>
              <span className="px-2 py-0.5 bg-green-500/20 text-green-400 text-xs rounded">
                OK
              </span>
              <span className="text-xs text-gray-500">
                {trace?.spans.filter(s => s.status === 'OK').length || 0}
              </span>
            </label>

            <label
              className={clsx(
                'flex items-center gap-2 p-2 rounded cursor-pointer transition-colors',
                filter.status.includes('ERROR') ? 'bg-blue-500/20' : 'hover:bg-gray-700/50'
              )}
            >
              <input
                type="checkbox"
                checked={filter.status.length === 0 || filter.status.includes('ERROR')}
                onChange={() => handleStatusToggle('ERROR')}
                className="sr-only"
              />
              <div
                className={clsx(
                  'w-4 h-4 rounded border-2 flex items-center justify-center transition-colors',
                  filter.status.includes('ERROR') || filter.status.length === 0
                    ? 'bg-blue-500 border-blue-500'
                    : 'border-gray-500'
                )}
              >
                {(filter.status.includes('ERROR') || filter.status.length === 0) && (
                  <Check className="w-3 h-3 text-white" />
                )}
              </div>
              <span className="flex items-center gap-1 px-2 py-0.5 bg-red-500/20 text-red-400 text-xs rounded">
                <AlertCircle className="w-3 h-3" />
                Error
              </span>
              <span className="text-xs text-gray-500">
                {trace?.spans.filter(s => s.status === 'ERROR').length || 0}
              </span>
            </label>
          </div>
        </FilterSection>

        {/* Duration Section */}
        <FilterSection
          title="Duration"
          icon={<Clock className="w-4 h-4" />}
          isExpanded={expandedSections.has('duration')}
          onToggle={() => toggleSection('duration')}
          badge={
            filter.minDuration !== null || filter.maxDuration !== null ? 1 : undefined
          }
        >
          <div className="space-y-3">
            {/* Duration range info */}
            <div className="text-xs text-gray-400 space-y-1">
              <div className="flex justify-between">
                <span>Min:</span>
                <span className="font-mono">{formatDuration(durationStats.min)}</span>
              </div>
              <div className="flex justify-between">
                <span>Max:</span>
                <span className="font-mono">{formatDuration(durationStats.max)}</span>
              </div>
              <div className="flex justify-between">
                <span>Avg:</span>
                <span className="font-mono">{formatDuration(durationStats.avg)}</span>
              </div>
            </div>

            {/* Duration inputs */}
            <div className="flex items-center gap-2">
              <div className="flex-1">
                <label className="text-xs text-gray-400 block mb-1">Min (ms)</label>
                <input
                  type="number"
                  placeholder="0"
                  value={filter.minDuration !== null ? filter.minDuration / 1000 : ''}
                  onChange={e => handleDurationChange('min', e.target.value)}
                  className="w-full px-2 py-1.5 bg-gray-700 border border-gray-600 rounded text-sm text-white placeholder-gray-500 focus:outline-none focus:border-blue-500"
                />
              </div>
              <span className="text-gray-500 mt-5">-</span>
              <div className="flex-1">
                <label className="text-xs text-gray-400 block mb-1">Max (ms)</label>
                <input
                  type="number"
                  placeholder="∞"
                  value={filter.maxDuration !== null ? filter.maxDuration / 1000 : ''}
                  onChange={e => handleDurationChange('max', e.target.value)}
                  className="w-full px-2 py-1.5 bg-gray-700 border border-gray-600 rounded text-sm text-white placeholder-gray-500 focus:outline-none focus:border-blue-500"
                />
              </div>
            </div>

            {/* Quick duration filters */}
            <div className="flex flex-wrap gap-1">
              <QuickDurationButton
                label="> 10ms"
                onClick={() => setDurationFilter(10000, null)}
                active={filter.minDuration === 10000}
              />
              <QuickDurationButton
                label="> 50ms"
                onClick={() => setDurationFilter(50000, null)}
                active={filter.minDuration === 50000}
              />
              <QuickDurationButton
                label="> 100ms"
                onClick={() => setDurationFilter(100000, null)}
                active={filter.minDuration === 100000}
              />
              <QuickDurationButton
                label="> 500ms"
                onClick={() => setDurationFilter(500000, null)}
                active={filter.minDuration === 500000}
              />
            </div>
          </div>
        </FilterSection>
      </div>

      {/* Apply/Clear footer */}
      <div className="p-3 border-t border-gray-700 flex items-center justify-between">
        <button
          onClick={resetFilters}
          className="px-3 py-1.5 text-sm text-gray-400 hover:text-white"
        >
          Clear All
        </button>
        <button
          onClick={onClose}
          className="px-4 py-1.5 bg-blue-600 hover:bg-blue-700 rounded text-sm text-white font-medium"
        >
          Done
        </button>
      </div>
    </div>
  );
}

// Collapsible filter section
function FilterSection({
  title,
  icon,
  isExpanded,
  onToggle,
  badge,
  children,
}: {
  title: string;
  icon: React.ReactNode;
  isExpanded: boolean;
  onToggle: () => void;
  badge?: number;
  children: React.ReactNode;
}) {
  return (
    <div className="border-b border-gray-700/50 last:border-0">
      <button
        onClick={onToggle}
        className="w-full flex items-center justify-between p-3 hover:bg-gray-700/30 transition-colors"
      >
        <div className="flex items-center gap-2">
          <span className="text-gray-400">{icon}</span>
          <span className="text-sm font-medium text-white">{title}</span>
          {badge !== undefined && (
            <span className="px-1.5 py-0.5 bg-blue-500 text-white text-[10px] rounded-full">
              {badge}
            </span>
          )}
        </div>
        {isExpanded ? (
          <ChevronUp className="w-4 h-4 text-gray-400" />
        ) : (
          <ChevronDown className="w-4 h-4 text-gray-400" />
        )}
      </button>
      {isExpanded && <div className="px-3 pb-3">{children}</div>}
    </div>
  );
}

// Quick duration filter button
function QuickDurationButton({
  label,
  onClick,
  active,
}: {
  label: string;
  onClick: () => void;
  active: boolean;
}) {
  return (
    <button
      onClick={onClick}
      className={clsx(
        'px-2 py-1 text-xs rounded transition-colors',
        active
          ? 'bg-blue-500 text-white'
          : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
      )}
    >
      {label}
    </button>
  );
}

export default FilterPanel;
