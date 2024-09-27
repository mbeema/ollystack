import { useMemo, useRef, useCallback } from 'react';
import { useTraceStore } from '../../stores/traceStore';
import type { Span } from '../../types/trace';
import { formatDuration } from '../../types/trace';
import { clsx } from 'clsx';

interface FlameGraphProps {
  width: number;
  height: number;
}

const ROW_HEIGHT = 24;
const ROW_GAP = 2;
const MIN_SPAN_WIDTH = 2;

export function FlameGraph({ width, height }: FlameGraphProps) {
  const containerRef = useRef<SVGSVGElement>(null);

  const {
    trace,
    timeRange,
    selectedSpanId,
    hoveredSpanId,
    searchMatches,
    selectSpan,
    hoverSpan,
    filteredSpans,
  } = useTraceStore();

  // Build rows for flame graph
  const rows = useMemo(() => {
    if (!trace || !timeRange) return [];

    const spanRows: { span: Span; row: number; x: number; width: number }[] = [];
    const rowEndTimes: number[] = [];

    // Sort spans by start time then by duration (longer spans first)
    const sortedSpans = [...filteredSpans].sort((a, b) => {
      if (a.startTime !== b.startTime) return a.startTime - b.startTime;
      return b.duration - a.duration;
    });

    const timeScale = width / (timeRange.end - timeRange.start);

    sortedSpans.forEach(span => {
      const x = (span.startTime - timeRange.start) * timeScale;
      const spanWidth = Math.max(MIN_SPAN_WIDTH, span.duration * timeScale);

      // Find the first row where this span fits
      let row = 0;
      while (rowEndTimes[row] !== undefined && rowEndTimes[row] > span.startTime) {
        row++;
      }

      rowEndTimes[row] = span.startTime + span.duration;
      spanRows.push({ span, row, x, width: spanWidth });
    });

    return spanRows;
  }, [trace, timeRange, filteredSpans, width]);

  // Calculate total height needed
  const maxRow = useMemo(() => {
    return rows.reduce((max, r) => Math.max(max, r.row), 0);
  }, [rows]);

  const totalHeight = (maxRow + 1) * (ROW_HEIGHT + ROW_GAP);

  const handleSpanClick = useCallback((spanId: string) => {
    selectSpan(spanId);
  }, [selectSpan]);

  const handleSpanHover = useCallback((spanId: string | null) => {
    hoverSpan(spanId);
  }, [hoverSpan]);

  if (!trace) {
    return (
      <div className="flex items-center justify-center h-full text-gray-500">
        No trace data
      </div>
    );
  }

  return (
    <div className="relative overflow-auto" style={{ height }}>
      <svg
        ref={containerRef}
        width={width}
        height={Math.max(totalHeight, height)}
        className="select-none"
      >
        {/* Time axis */}
        <TimeAxis width={width} timeRange={timeRange!} />

        {/* Spans */}
        <g transform="translate(0, 30)">
          {rows.map(({ span, row, x, width: spanWidth }) => {
            const isSelected = span.spanId === selectedSpanId;
            const isHovered = span.spanId === hoveredSpanId;
            const isMatch = searchMatches.includes(span.spanId);
            const isFiltered = !filteredSpans.includes(span);
            const y = row * (ROW_HEIGHT + ROW_GAP);

            return (
              <g
                key={span.spanId}
                transform={`translate(${x}, ${y})`}
                onClick={() => handleSpanClick(span.spanId)}
                onMouseEnter={() => handleSpanHover(span.spanId)}
                onMouseLeave={() => handleSpanHover(null)}
                className="cursor-pointer"
                opacity={isFiltered ? 0.3 : 1}
              >
                {/* Span bar */}
                <rect
                  width={spanWidth}
                  height={ROW_HEIGHT}
                  rx={3}
                  fill={span.serviceColor}
                  stroke={isSelected ? '#fff' : isMatch ? '#fbbf24' : 'transparent'}
                  strokeWidth={isSelected ? 2 : isMatch ? 2 : 0}
                  className={clsx(
                    'transition-all duration-150',
                    isHovered && 'brightness-110',
                    span.status === 'ERROR' && 'opacity-90'
                  )}
                />

                {/* Error indicator */}
                {span.status === 'ERROR' && (
                  <rect
                    x={0}
                    y={0}
                    width={4}
                    height={ROW_HEIGHT}
                    fill="#ef4444"
                    rx={3}
                  />
                )}

                {/* Span label (if wide enough) */}
                {spanWidth > 60 && (
                  <text
                    x={8}
                    y={ROW_HEIGHT / 2 + 4}
                    fontSize={11}
                    fill="#fff"
                    className="pointer-events-none font-medium"
                  >
                    {truncateText(span.operationName, spanWidth - 16)}
                  </text>
                )}

                {/* Selection highlight */}
                {isSelected && (
                  <rect
                    width={spanWidth}
                    height={ROW_HEIGHT}
                    rx={3}
                    fill="none"
                    stroke="#fff"
                    strokeWidth={2}
                    className="animate-pulse"
                  />
                )}
              </g>
            );
          })}
        </g>
      </svg>

      {/* Tooltip */}
      {hoveredSpanId && (
        <SpanTooltip
          span={trace.spans.find(s => s.spanId === hoveredSpanId)!}
          containerRef={containerRef}
        />
      )}
    </div>
  );
}

// Time axis component
function TimeAxis({ width, timeRange }: { width: number; timeRange: { start: number; end: number } }) {
  const ticks = useMemo(() => {
    const duration = timeRange.end - timeRange.start;
    const tickCount = Math.min(10, Math.floor(width / 100));
    const tickInterval = duration / tickCount;

    return Array.from({ length: tickCount + 1 }, (_, i) => ({
      x: (i / tickCount) * width,
      time: timeRange.start + i * tickInterval,
    }));
  }, [width, timeRange]);

  return (
    <g className="text-gray-400">
      {/* Axis line */}
      <line x1={0} y1={20} x2={width} y2={20} stroke="currentColor" strokeWidth={1} />

      {/* Tick marks and labels */}
      {ticks.map((tick, i) => (
        <g key={i} transform={`translate(${tick.x}, 0)`}>
          <line y1={15} y2={20} stroke="currentColor" strokeWidth={1} />
          <text
            y={12}
            fontSize={10}
            fill="currentColor"
            textAnchor={i === 0 ? 'start' : i === ticks.length - 1 ? 'end' : 'middle'}
          >
            {formatDuration(tick.time - timeRange.start)}
          </text>
        </g>
      ))}
    </g>
  );
}

// Tooltip component
function SpanTooltip({ span, containerRef }: { span: Span; containerRef: React.RefObject<SVGSVGElement> }) {
  if (!span) return null;

  return (
    <div
      className="absolute z-50 bg-gray-900 border border-gray-700 rounded-lg shadow-xl p-3 text-sm max-w-sm pointer-events-none"
      style={{
        left: '50%',
        top: '10px',
        transform: 'translateX(-50%)',
      }}
    >
      <div className="flex items-center gap-2 mb-2">
        <div
          className="w-3 h-3 rounded-full"
          style={{ backgroundColor: span.serviceColor }}
        />
        <span className="font-medium text-white">{span.serviceName}</span>
        {span.status === 'ERROR' && (
          <span className="px-1.5 py-0.5 bg-red-500/20 text-red-400 text-xs rounded">
            ERROR
          </span>
        )}
      </div>
      <div className="text-gray-300 mb-2 truncate">{span.operationName}</div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
        <div className="text-gray-500">Duration</div>
        <div className="text-white font-mono">{formatDuration(span.duration)}</div>
        <div className="text-gray-500">Self Time</div>
        <div className="text-white font-mono">{formatDuration(span.selfTime)}</div>
        <div className="text-gray-500">% of Trace</div>
        <div className="text-white font-mono">{span.percentOfTrace.toFixed(1)}%</div>
      </div>
    </div>
  );
}

// Helper to truncate text
function truncateText(text: string, maxWidth: number): string {
  const avgCharWidth = 6;
  const maxChars = Math.floor(maxWidth / avgCharWidth);
  if (text.length <= maxChars) return text;
  return text.slice(0, maxChars - 3) + '...';
}

export default FlameGraph;
