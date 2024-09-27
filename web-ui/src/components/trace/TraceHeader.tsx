import { useState } from 'react';
import {
  Search,
  ChevronUp,
  ChevronDown,
  X,
  ZoomIn,
  ZoomOut,
  Maximize2,
  Filter,
  Copy,
  Check,
  Clock,
  Activity,
  AlertCircle,
  Layers,
} from 'lucide-react';
import { useTraceStore } from '../../stores/traceStore';
import { formatDuration } from '../../types/trace';
import type { TraceViewMode } from '../../types/trace';
import { clsx } from 'clsx';
import { FilterPanel } from './FilterPanel';

export function TraceHeader() {
  const {
    trace,
    viewMode,
    setViewMode,
    filter,
    setSearchQuery,
    searchMatches,
    currentMatchIndex,
    nextMatch,
    prevMatch,
    zoomIn,
    zoomOut,
    resetZoom,
    expandAll,
    collapseAll,
    filteredSpans,
  } = useTraceStore();

  // Calculate active filter count
  const activeFilterCount =
    filter.services.length +
    filter.status.length +
    (filter.minDuration !== null ? 1 : 0) +
    (filter.maxDuration !== null ? 1 : 0);

  const [showSearch, setShowSearch] = useState(false);
  const [showFilter, setShowFilter] = useState(false);
  const [copiedTraceId, setCopiedTraceId] = useState(false);

  const handleCopyTraceId = async () => {
    if (trace) {
      await navigator.clipboard.writeText(trace.traceId);
      setCopiedTraceId(true);
      setTimeout(() => setCopiedTraceId(false), 2000);
    }
  };

  const viewModes: { id: TraceViewMode; label: string }[] = [
    { id: 'flamegraph', label: 'Flame Graph' },
    { id: 'waterfall', label: 'Waterfall' },
    { id: 'spanlist', label: 'Span List' },
    { id: 'map', label: 'Service Map' },
  ];

  if (!trace) return null;

  return (
    <div className="flex-shrink-0 bg-gray-800 border-b border-gray-700">
      {/* Top row - Trace info */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-700/50">
        <div className="flex items-center gap-4">
          {/* Root service & operation */}
          <div>
            <div className="flex items-center gap-2">
              <div
                className="w-3 h-3 rounded-full"
                style={{ backgroundColor: trace.rootSpan.serviceColor }}
              />
              <span className="font-medium text-white">{trace.rootSpan.serviceName}</span>
            </div>
            <div className="text-sm text-gray-400 mt-0.5">{trace.rootSpan.operationName}</div>
          </div>

          {/* Stats */}
          <div className="flex items-center gap-4 pl-4 border-l border-gray-700">
            <StatBadge icon={<Clock className="w-3.5 h-3.5" />} label="Duration" value={formatDuration(trace.duration)} />
            <StatBadge icon={<Layers className="w-3.5 h-3.5" />} label="Spans" value={trace.spanCount.toString()} />
            <StatBadge icon={<Activity className="w-3.5 h-3.5" />} label="Services" value={trace.serviceCount.toString()} />
            {trace.errorCount > 0 && (
              <StatBadge
                icon={<AlertCircle className="w-3.5 h-3.5" />}
                label="Errors"
                value={trace.errorCount.toString()}
                className="text-red-400"
              />
            )}
          </div>
        </div>

        {/* Trace ID */}
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-400">Trace ID:</span>
          <code className="text-xs text-gray-300 font-mono">{trace.traceId.slice(0, 16)}...</code>
          <button
            onClick={handleCopyTraceId}
            className="p-1 rounded hover:bg-gray-700 text-gray-400 hover:text-white"
          >
            {copiedTraceId ? (
              <Check className="w-3.5 h-3.5 text-green-400" />
            ) : (
              <Copy className="w-3.5 h-3.5" />
            )}
          </button>
        </div>
      </div>

      {/* Bottom row - Controls */}
      <div className="flex items-center justify-between px-4 py-2">
        {/* View mode toggle */}
        <div className="flex items-center gap-1 bg-gray-900 rounded-lg p-1">
          {viewModes.map(mode => (
            <button
              key={mode.id}
              onClick={() => setViewMode(mode.id)}
              className={clsx(
                'px-3 py-1.5 text-xs font-medium rounded-md transition-colors',
                viewMode === mode.id
                  ? 'bg-blue-600 text-white'
                  : 'text-gray-400 hover:text-white hover:bg-gray-700'
              )}
            >
              {mode.label}
            </button>
          ))}
        </div>

        {/* Middle controls */}
        <div className="flex items-center gap-2">
          {/* Search */}
          {showSearch ? (
            <div className="flex items-center gap-1 bg-gray-900 rounded-lg px-2 py-1">
              <Search className="w-4 h-4 text-gray-400" />
              <input
                type="text"
                placeholder="Search spans..."
                value={filter.searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
                className="bg-transparent text-sm text-white placeholder-gray-500 outline-none w-48"
                autoFocus
              />
              {searchMatches.length > 0 && (
                <>
                  <span className="text-xs text-gray-400 px-2 border-l border-gray-700">
                    {currentMatchIndex + 1}/{searchMatches.length}
                  </span>
                  <button
                    onClick={prevMatch}
                    className="p-0.5 rounded hover:bg-gray-700 text-gray-400"
                  >
                    <ChevronUp className="w-4 h-4" />
                  </button>
                  <button
                    onClick={nextMatch}
                    className="p-0.5 rounded hover:bg-gray-700 text-gray-400"
                  >
                    <ChevronDown className="w-4 h-4" />
                  </button>
                </>
              )}
              <button
                onClick={() => {
                  setShowSearch(false);
                  setSearchQuery('');
                }}
                className="p-0.5 rounded hover:bg-gray-700 text-gray-400"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          ) : (
            <button
              onClick={() => setShowSearch(true)}
              className="p-2 rounded-lg hover:bg-gray-700 text-gray-400 hover:text-white"
              title="Search spans (Ctrl+F)"
            >
              <Search className="w-4 h-4" />
            </button>
          )}

          {/* Filter button */}
          <div className="relative">
            <button
              onClick={() => setShowFilter(!showFilter)}
              className={clsx(
                'p-2 rounded-lg text-gray-400 hover:text-white flex items-center gap-1',
                showFilter ? 'bg-gray-700' : 'hover:bg-gray-700'
              )}
              title="Filter spans"
            >
              <Filter className="w-4 h-4" />
              {activeFilterCount > 0 && (
                <span className="px-1.5 py-0.5 bg-blue-500 text-white text-[10px] rounded-full">
                  {activeFilterCount}
                </span>
              )}
            </button>
            <FilterPanel isOpen={showFilter} onClose={() => setShowFilter(false)} />
          </div>
        </div>

        {/* Right controls */}
        <div className="flex items-center gap-1">
          {/* Expand/Collapse */}
          <button
            onClick={expandAll}
            className="px-2 py-1.5 text-xs text-gray-400 hover:text-white hover:bg-gray-700 rounded"
          >
            Expand All
          </button>
          <button
            onClick={collapseAll}
            className="px-2 py-1.5 text-xs text-gray-400 hover:text-white hover:bg-gray-700 rounded"
          >
            Collapse All
          </button>

          <div className="w-px h-6 bg-gray-700 mx-2" />

          {/* Zoom controls */}
          <button
            onClick={zoomOut}
            className="p-2 rounded-lg hover:bg-gray-700 text-gray-400 hover:text-white"
            title="Zoom out"
          >
            <ZoomOut className="w-4 h-4" />
          </button>
          <button
            onClick={zoomIn}
            className="p-2 rounded-lg hover:bg-gray-700 text-gray-400 hover:text-white"
            title="Zoom in"
          >
            <ZoomIn className="w-4 h-4" />
          </button>
          <button
            onClick={resetZoom}
            className="p-2 rounded-lg hover:bg-gray-700 text-gray-400 hover:text-white"
            title="Reset zoom"
          >
            <Maximize2 className="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>
  );
}

// Stat badge component
function StatBadge({
  icon,
  label,
  value,
  className,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div className={clsx('flex items-center gap-1.5', className)}>
      {icon}
      <span className="text-xs text-gray-500">{label}:</span>
      <span className="text-sm font-medium text-white">{value}</span>
    </div>
  );
}

export default TraceHeader;
