import { useMemo, useState } from 'react';
import { ArrowUpDown, ArrowUp, ArrowDown, AlertCircle } from 'lucide-react';
import { useTraceStore } from '../../stores/traceStore';
import { formatDuration } from '../../types/trace';
import { clsx } from 'clsx';

type SortField = 'startTime' | 'duration' | 'service' | 'operation' | 'status';
type SortDirection = 'asc' | 'desc';

export function SpanList() {
  const {
    trace,
    selectedSpanId,
    hoveredSpanId,
    searchMatches,
    filteredSpans,
    selectSpan,
    hoverSpan,
  } = useTraceStore();

  const [sortField, setSortField] = useState<SortField>('startTime');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');

  // Sort spans
  const sortedSpans = useMemo(() => {
    const spans = [...filteredSpans];

    spans.sort((a, b) => {
      let comparison = 0;

      switch (sortField) {
        case 'startTime':
          comparison = a.startTime - b.startTime;
          break;
        case 'duration':
          comparison = a.duration - b.duration;
          break;
        case 'service':
          comparison = a.serviceName.localeCompare(b.serviceName);
          break;
        case 'operation':
          comparison = a.operationName.localeCompare(b.operationName);
          break;
        case 'status':
          comparison = a.status.localeCompare(b.status);
          break;
      }

      return sortDirection === 'asc' ? comparison : -comparison;
    });

    return spans;
  }, [filteredSpans, sortField, sortDirection]);

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(prev => (prev === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortField(field);
      setSortDirection('asc');
    }
  };

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) {
      return <ArrowUpDown className="w-3 h-3 text-gray-500" />;
    }
    return sortDirection === 'asc' ? (
      <ArrowUp className="w-3 h-3 text-blue-400" />
    ) : (
      <ArrowDown className="w-3 h-3 text-blue-400" />
    );
  };

  if (!trace) {
    return (
      <div className="flex items-center justify-center h-full text-gray-500">
        No trace data
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col">
      {/* Table header */}
      <div className="flex-shrink-0 grid grid-cols-12 gap-2 px-3 py-2 bg-gray-800/50 border-b border-gray-700 text-xs font-medium text-gray-400 uppercase">
        <button
          onClick={() => handleSort('service')}
          className="col-span-2 flex items-center gap-1 hover:text-white"
        >
          Service <SortIcon field="service" />
        </button>
        <button
          onClick={() => handleSort('operation')}
          className="col-span-4 flex items-center gap-1 hover:text-white"
        >
          Operation <SortIcon field="operation" />
        </button>
        <button
          onClick={() => handleSort('duration')}
          className="col-span-2 flex items-center gap-1 hover:text-white justify-end"
        >
          Duration <SortIcon field="duration" />
        </button>
        <button
          onClick={() => handleSort('startTime')}
          className="col-span-2 flex items-center gap-1 hover:text-white justify-end"
        >
          Offset <SortIcon field="startTime" />
        </button>
        <button
          onClick={() => handleSort('status')}
          className="col-span-2 flex items-center gap-1 hover:text-white justify-center"
        >
          Status <SortIcon field="status" />
        </button>
      </div>

      {/* Table body */}
      <div className="flex-1 overflow-auto">
        {sortedSpans.map(span => {
          const isSelected = span.spanId === selectedSpanId;
          const isHovered = span.spanId === hoveredSpanId;
          const isMatch = searchMatches.includes(span.spanId);
          const offset = span.startTime - trace.startTime;

          return (
            <div
              key={span.spanId}
              onClick={() => selectSpan(span.spanId)}
              onMouseEnter={() => hoverSpan(span.spanId)}
              onMouseLeave={() => hoverSpan(null)}
              className={clsx(
                'grid grid-cols-12 gap-2 px-3 py-2 border-b border-gray-800 cursor-pointer transition-colors',
                isSelected && 'bg-blue-500/10 border-blue-500/30',
                isHovered && !isSelected && 'bg-gray-700/30',
                isMatch && !isSelected && 'bg-yellow-500/10'
              )}
            >
              {/* Service */}
              <div className="col-span-2 flex items-center gap-2 overflow-hidden">
                <div
                  className="w-2 h-2 rounded-full flex-shrink-0"
                  style={{ backgroundColor: span.serviceColor }}
                />
                <span className="text-sm text-gray-300 truncate">{span.serviceName}</span>
              </div>

              {/* Operation */}
              <div className="col-span-4 flex items-center overflow-hidden">
                <span className="text-sm text-white truncate" title={span.operationName}>
                  {span.operationName}
                </span>
              </div>

              {/* Duration */}
              <div className="col-span-2 flex items-center justify-end">
                <span className="text-sm font-mono text-gray-300">
                  {formatDuration(span.duration)}
                </span>
              </div>

              {/* Offset */}
              <div className="col-span-2 flex items-center justify-end">
                <span className="text-sm font-mono text-gray-500">
                  +{formatDuration(offset)}
                </span>
              </div>

              {/* Status */}
              <div className="col-span-2 flex items-center justify-center">
                {span.status === 'ERROR' ? (
                  <span className="flex items-center gap-1 px-2 py-0.5 bg-red-500/20 text-red-400 text-xs rounded">
                    <AlertCircle className="w-3 h-3" />
                    Error
                  </span>
                ) : (
                  <span className="px-2 py-0.5 bg-green-500/20 text-green-400 text-xs rounded">
                    OK
                  </span>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* Summary footer */}
      <div className="flex-shrink-0 px-3 py-2 bg-gray-800/50 border-t border-gray-700 text-xs text-gray-400">
        Showing {sortedSpans.length} of {trace.spanCount} spans
      </div>
    </div>
  );
}

export default SpanList;
