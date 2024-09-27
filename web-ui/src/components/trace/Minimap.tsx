import { useRef, useCallback, useMemo, useState } from 'react';
import { useTraceStore } from '../../stores/traceStore';
import type { Span } from '../../types/trace';

interface MinimapProps {
  width: number;
  height?: number;
}

export function Minimap({ width, height = 40 }: MinimapProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = useState(false);

  const {
    trace,
    timeRange,
    selectedSpanId,
    filteredSpans,
    setTimeRange,
  } = useTraceStore();

  // Calculate span positions for minimap
  const spanBars = useMemo(() => {
    if (!trace) return [];

    const timeScale = width / trace.duration;

    return filteredSpans.map(span => ({
      span,
      x: (span.startTime - trace.startTime) * timeScale,
      width: Math.max(1, span.duration * timeScale),
    }));
  }, [trace, filteredSpans, width]);

  // Calculate viewport indicator
  const viewport = useMemo(() => {
    if (!trace || !timeRange) return null;

    const timeScale = width / trace.duration;
    const x = (timeRange.start - trace.startTime) * timeScale;
    const viewportWidth = (timeRange.end - timeRange.start) * timeScale;

    return { x, width: viewportWidth };
  }, [trace, timeRange, width]);

  // Handle click/drag to change viewport
  const updateViewport = useCallback(
    (clientX: number) => {
      if (!containerRef.current || !trace) return;

      const rect = containerRef.current.getBoundingClientRect();
      const x = clientX - rect.left;
      const timeScale = trace.duration / width;

      // Calculate new center time
      const centerTime = trace.startTime + x * timeScale;

      // Keep same viewport width
      const currentWidth = timeRange ? timeRange.end - timeRange.start : trace.duration;
      const halfWidth = currentWidth / 2;

      const newStart = Math.max(trace.startTime, centerTime - halfWidth);
      const newEnd = Math.min(trace.endTime, newStart + currentWidth);

      setTimeRange({ start: newStart, end: newEnd });
    },
    [trace, timeRange, width, setTimeRange]
  );

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      setIsDragging(true);
      updateViewport(e.clientX);
    },
    [updateViewport]
  );

  const handleMouseMove = useCallback(
    (e: React.MouseEvent) => {
      if (isDragging) {
        updateViewport(e.clientX);
      }
    },
    [isDragging, updateViewport]
  );

  const handleMouseUp = useCallback(() => {
    setIsDragging(false);
  }, []);

  const handleMouseLeave = useCallback(() => {
    setIsDragging(false);
  }, []);

  if (!trace) return null;

  return (
    <div
      ref={containerRef}
      className="relative bg-gray-800 rounded-lg overflow-hidden cursor-crosshair select-none"
      style={{ width, height }}
      onMouseDown={handleMouseDown}
      onMouseMove={handleMouseMove}
      onMouseUp={handleMouseUp}
      onMouseLeave={handleMouseLeave}
    >
      {/* Span bars */}
      <svg width={width} height={height} className="absolute inset-0">
        {spanBars.map(({ span, x, width: barWidth }) => {
          const isSelected = span.spanId === selectedSpanId;
          const isError = span.status === 'ERROR';

          return (
            <rect
              key={span.spanId}
              x={x}
              y={height * 0.2}
              width={barWidth}
              height={height * 0.6}
              fill={isError ? '#ef4444' : span.serviceColor}
              opacity={isSelected ? 1 : 0.6}
              rx={1}
            />
          );
        })}
      </svg>

      {/* Viewport indicator */}
      {viewport && (
        <>
          {/* Dimmed areas outside viewport */}
          <div
            className="absolute top-0 bottom-0 bg-black/50"
            style={{ left: 0, width: viewport.x }}
          />
          <div
            className="absolute top-0 bottom-0 bg-black/50"
            style={{ left: viewport.x + viewport.width, right: 0 }}
          />

          {/* Viewport border */}
          <div
            className="absolute top-0 bottom-0 border-2 border-blue-500 bg-blue-500/10"
            style={{ left: viewport.x, width: viewport.width }}
          >
            {/* Resize handles */}
            <div className="absolute left-0 top-0 bottom-0 w-1 cursor-ew-resize bg-blue-500" />
            <div className="absolute right-0 top-0 bottom-0 w-1 cursor-ew-resize bg-blue-500" />
          </div>
        </>
      )}

      {/* Selection indicator */}
      {selectedSpanId && (
        <div
          className="absolute top-0 bottom-0 w-0.5 bg-white"
          style={{
            left:
              ((trace.spans.find(s => s.spanId === selectedSpanId)?.startTime || trace.startTime) -
                trace.startTime) *
              (width / trace.duration),
          }}
        />
      )}
    </div>
  );
}

export default Minimap;
