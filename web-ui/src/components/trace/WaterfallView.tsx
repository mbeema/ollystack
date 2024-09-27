import { useMemo, useCallback } from 'react';
import { ChevronRight, ChevronDown, AlertCircle } from 'lucide-react';
import { useTraceStore } from '../../stores/traceStore';
import type { Span } from '../../types/trace';
import { formatDuration } from '../../types/trace';
import { clsx } from 'clsx';

interface WaterfallViewProps {
  width: number;
}

const ROW_HEIGHT = 32;
const INDENT_WIDTH = 20;
const TIMELINE_START = 300; // px from left where timeline starts

export function WaterfallView({ width }: WaterfallViewProps) {
  const {
    trace,
    timeRange,
    selectedSpanId,
    hoveredSpanId,
    expandedSpanIds,
    searchMatches,
    selectSpan,
    hoverSpan,
    toggleSpanExpanded,
  } = useTraceStore();

  // Build visible span list respecting collapsed state
  const visibleSpans = useMemo(() => {
    if (!trace) return [];

    const result: { span: Span; depth: number; hasChildren: boolean }[] = [];

    const traverse = (span: Span, depth: number) => {
      const hasChildren = span.children.length > 0;
      result.push({ span, depth, hasChildren });

      if (hasChildren && expandedSpanIds.has(span.spanId)) {
        span.children.forEach(child => traverse(child, depth + 1));
      }
    };

    traverse(trace.rootSpan, 0);
    return result;
  }, [trace, expandedSpanIds]);

  const timelineWidth = width - TIMELINE_START - 20;

  const handleSpanClick = useCallback((spanId: string) => {
    selectSpan(spanId);
  }, [selectSpan]);

  const handleToggle = useCallback((spanId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    toggleSpanExpanded(spanId);
  }, [toggleSpanExpanded]);

  if (!trace || !timeRange) {
    return (
      <div className="flex items-center justify-center h-full text-gray-500">
        No trace data
      </div>
    );
  }

  const timeScale = timelineWidth / (timeRange.end - timeRange.start);

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center border-b border-gray-700 bg-gray-800/50 sticky top-0 z-10">
        <div className="flex-shrink-0 px-3 py-2 text-xs font-medium text-gray-400 uppercase" style={{ width: TIMELINE_START }}>
          Service / Operation
        </div>
        <div className="flex-1 px-3 py-2">
          <TimelineHeader timeRange={timeRange} width={timelineWidth} />
        </div>
      </div>

      {/* Span rows */}
      <div className="flex-1 overflow-auto">
        {visibleSpans.map(({ span, depth, hasChildren }) => {
          const isSelected = span.spanId === selectedSpanId;
          const isHovered = span.spanId === hoveredSpanId;
          const isMatch = searchMatches.includes(span.spanId);
          const isExpanded = expandedSpanIds.has(span.spanId);

          const barX = (span.startTime - timeRange.start) * timeScale;
          const barWidth = Math.max(2, span.duration * timeScale);

          return (
            <div
              key={span.spanId}
              className={clsx(
                'flex items-center border-b border-gray-800 cursor-pointer transition-colors',
                isSelected && 'bg-blue-500/10',
                isHovered && !isSelected && 'bg-gray-700/30',
                isMatch && 'bg-yellow-500/10'
              )}
              style={{ height: ROW_HEIGHT }}
              onClick={() => handleSpanClick(span.spanId)}
              onMouseEnter={() => hoverSpan(span.spanId)}
              onMouseLeave={() => hoverSpan(null)}
            >
              {/* Service/Operation column */}
              <div
                className="flex-shrink-0 flex items-center gap-1 px-2 overflow-hidden"
                style={{ width: TIMELINE_START, paddingLeft: depth * INDENT_WIDTH + 8 }}
              >
                {/* Expand/collapse toggle */}
                {hasChildren ? (
                  <button
                    onClick={(e) => handleToggle(span.spanId, e)}
                    className="p-0.5 rounded hover:bg-gray-600"
                  >
                    {isExpanded ? (
                      <ChevronDown className="w-3 h-3 text-gray-400" />
                    ) : (
                      <ChevronRight className="w-3 h-3 text-gray-400" />
                    )}
                  </button>
                ) : (
                  <div className="w-4" />
                )}

                {/* Service color dot */}
                <div
                  className="w-2 h-2 rounded-full flex-shrink-0"
                  style={{ backgroundColor: span.serviceColor }}
                />

                {/* Service name */}
                <span className="text-xs text-gray-400 truncate flex-shrink-0">
                  {span.serviceName}
                </span>

                {/* Operation name */}
                <span className="text-xs text-white truncate">
                  {span.operationName}
                </span>

                {/* Error indicator */}
                {span.status === 'ERROR' && (
                  <AlertCircle className="w-3 h-3 text-red-500 flex-shrink-0" />
                )}
              </div>

              {/* Timeline column */}
              <div className="flex-1 relative h-full px-2">
                {/* Grid lines */}
                <div className="absolute inset-0 flex">
                  {[0, 0.25, 0.5, 0.75, 1].map((p, i) => (
                    <div
                      key={i}
                      className="border-l border-gray-800"
                      style={{ left: `${p * 100}%`, position: 'absolute', height: '100%' }}
                    />
                  ))}
                </div>

                {/* Span bar */}
                <div
                  className={clsx(
                    'absolute top-1/2 -translate-y-1/2 rounded-sm transition-all',
                    span.status === 'ERROR' ? 'bg-red-500' : ''
                  )}
                  style={{
                    left: barX,
                    width: barWidth,
                    height: 16,
                    backgroundColor: span.status !== 'ERROR' ? span.serviceColor : undefined,
                  }}
                >
                  {/* Duration label */}
                  {barWidth > 50 && (
                    <span className="absolute inset-0 flex items-center justify-center text-[10px] text-white font-medium">
                      {formatDuration(span.duration)}
                    </span>
                  )}
                </div>

                {/* Duration on right if bar is too small */}
                {barWidth <= 50 && (
                  <span
                    className="absolute top-1/2 -translate-y-1/2 text-[10px] text-gray-400 ml-1"
                    style={{ left: barX + barWidth + 4 }}
                  >
                    {formatDuration(span.duration)}
                  </span>
                )}

                {/* Selection indicator */}
                {isSelected && (
                  <div
                    className="absolute top-0 bottom-0 border-2 border-blue-500 rounded pointer-events-none"
                    style={{ left: barX - 2, width: barWidth + 4 }}
                  />
                )}

                {/* Search match indicator */}
                {isMatch && !isSelected && (
                  <div
                    className="absolute top-0 bottom-0 border-2 border-yellow-500 rounded pointer-events-none"
                    style={{ left: barX - 2, width: barWidth + 4 }}
                  />
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// Timeline header component
function TimelineHeader({ timeRange, width }: { timeRange: { start: number; end: number }; width: number }) {
  const ticks = useMemo(() => {
    const duration = timeRange.end - timeRange.start;
    return [0, 0.25, 0.5, 0.75, 1].map(p => ({
      position: p * width,
      label: formatDuration(p * duration),
    }));
  }, [timeRange, width]);

  return (
    <div className="relative h-6">
      {ticks.map((tick, i) => (
        <span
          key={i}
          className="absolute text-[10px] text-gray-500 -translate-x-1/2"
          style={{ left: tick.position }}
        >
          {tick.label}
        </span>
      ))}
    </div>
  );
}

export default WaterfallView;
