import { useEffect, useRef, useState, useMemo } from 'react';
import { useParams, Link } from 'react-router-dom';
import { ArrowLeft, Loader2, Brain, X } from 'lucide-react';
import { useTraceStore } from '../stores/traceStore';
import { getApiUrl } from '../lib/config';
import { TraceHeader } from '../components/trace/TraceHeader';
import { FlameGraph } from '../components/trace/FlameGraph';
import { WaterfallView } from '../components/trace/WaterfallView';
import { SpanList } from '../components/trace/SpanList';
import { SpanDetails } from '../components/trace/SpanDetails';
import { Minimap } from '../components/trace/Minimap';
import { ServiceMap } from '../components/trace/ServiceMap';
import { AIInsightsPanel } from '../components/trace/AIInsightsPanel';
import type { Trace, Span } from '../types/trace';
import { getServiceColor, buildSpanTree, flattenSpanTree } from '../types/trace';

export function TraceDetailPage() {
  const { traceId } = useParams<{ traceId: string }>();
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(0);
  const [showAIInsights, setShowAIInsights] = useState(false);

  const {
    trace,
    isLoading,
    error,
    viewMode,
    selectedSpanId,
    setTrace,
    setLoading,
    setError,
    selectSpan,
  } = useTraceStore();

  // Measure container width for responsive visualizations
  useEffect(() => {
    const updateWidth = () => {
      if (containerRef.current) {
        setContainerWidth(containerRef.current.offsetWidth);
      }
    };

    updateWidth();
    window.addEventListener('resize', updateWidth);
    return () => window.removeEventListener('resize', updateWidth);
  }, []);

  // Fetch trace data
  useEffect(() => {
    if (!traceId) return;

    const fetchTrace = async () => {
      setLoading(true);
      setError(null);

      try {
        const API_URL = getApiUrl();
        const response = await fetch(`${API_URL}/api/v1/traces/${traceId}`);
        if (!response.ok) {
          throw new Error('Failed to fetch trace');
        }
        const data = await response.json();

        // Transform API response to Trace format
        const trace = transformApiResponse(data, traceId);
        setTrace(trace);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load trace');
      } finally {
        setLoading(false);
      }
    };

    fetchTrace();
  }, [traceId, setTrace, setLoading, setError]);

  // Calculate visualization width (minus details panel and AI insights panel)
  const visualizationWidth = useMemo(() => {
    const detailsPanelWidth = selectedSpanId ? 400 : 0;
    const aiPanelWidth = showAIInsights ? 380 : 0;
    return Math.max(0, containerWidth - detailsPanelWidth - aiPanelWidth);
  }, [containerWidth, selectedSpanId, showAIInsights]);

  // Handle span click from AI Insights panel
  const handleAISpanClick = (spanId: string) => {
    selectSpan(spanId);
  };

  if (isLoading) {
    return (
      <div className="h-full flex items-center justify-center">
        <Loader2 className="w-8 h-8 text-blue-500 animate-spin" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-full flex flex-col items-center justify-center text-center p-8">
        <div className="text-red-400 text-lg mb-2">Error loading trace</div>
        <div className="text-gray-500 mb-4">{error}</div>
        <Link
          to="/traces"
          className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg text-white"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Traces
        </Link>
      </div>
    );
  }

  if (!trace) {
    return (
      <div className="h-full flex flex-col items-center justify-center text-center p-8">
        <div className="text-gray-400 text-lg mb-4">Trace not found</div>
        <Link
          to="/traces"
          className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg text-white"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Traces
        </Link>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-gray-900">
      {/* Back link and AI toggle */}
      <div className="flex-shrink-0 px-4 py-2 border-b border-gray-800 flex items-center justify-between">
        <Link
          to="/traces"
          className="inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Traces
        </Link>
        <button
          onClick={() => setShowAIInsights(!showAIInsights)}
          className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm transition-colors ${
            showAIInsights
              ? 'bg-purple-600 text-white'
              : 'bg-gray-800 text-gray-300 hover:bg-gray-700 hover:text-white'
          }`}
        >
          <Brain className="w-4 h-4" />
          AI Insights
        </button>
      </div>

      {/* Trace header with controls */}
      <TraceHeader />

      {/* Main content area */}
      <div ref={containerRef} className="flex-1 flex overflow-hidden">
        {/* Visualization area */}
        <div className="flex-1 flex flex-col overflow-hidden">
          {/* Minimap */}
          <div className="flex-shrink-0 p-3 border-b border-gray-700">
            <Minimap width={visualizationWidth - 24} height={40} />
          </div>

          {/* Main visualization */}
          <div className="flex-1 overflow-auto">
            {viewMode === 'flamegraph' && (
              <FlameGraph width={visualizationWidth} height={600} />
            )}
            {viewMode === 'waterfall' && <WaterfallView width={visualizationWidth} />}
            {viewMode === 'spanlist' && <SpanList />}
            {viewMode === 'map' && (
              <ServiceMap width={visualizationWidth} height={600} />
            )}
          </div>
        </div>

        {/* Span details panel */}
        {selectedSpanId && (
          <div className="w-[400px] flex-shrink-0 border-l border-gray-700">
            <SpanDetails />
          </div>
        )}

        {/* AI Insights panel */}
        {showAIInsights && traceId && (
          <div className="w-[380px] flex-shrink-0 border-l border-gray-700 relative">
            <button
              onClick={() => setShowAIInsights(false)}
              className="absolute top-2 right-2 z-10 p-1 hover:bg-gray-700 rounded text-gray-400 hover:text-white"
            >
              <X className="w-4 h-4" />
            </button>
            <AIInsightsPanel traceId={traceId} onSpanClick={handleAISpanClick} />
          </div>
        )}
      </div>
    </div>
  );
}

// Transform API response to Trace format
function transformApiResponse(data: any, traceId: string): Trace {
  const spans: Span[] = (data.spans || []).map((apiSpan: any, index: number) => {
    const serviceName = apiSpan.serviceName || 'unknown';
    return {
      spanId: apiSpan.spanId,
      traceId: apiSpan.traceId || traceId,
      parentSpanId: apiSpan.parentSpanId || null,
      operationName: apiSpan.operationName || apiSpan.name || 'unknown',
      serviceName,
      serviceColor: getServiceColor(serviceName, index),
      startTime: apiSpan.startTime || 0,
      duration: apiSpan.duration || 0,
      status: apiSpan.status || 'OK',
      statusMessage: apiSpan.statusMessage,
      kind: apiSpan.kind || 'INTERNAL',
      attributes: apiSpan.attributes || {},
      events: apiSpan.events || [],
      links: apiSpan.links || [],
      depth: 0,
      children: [],
      selfTime: apiSpan.duration || 0,
      percentOfTrace: 0,
    } as Span;
  });

  // Build span tree and calculate depths
  const spanMap = new Map<string, Span>();
  spans.forEach(span => spanMap.set(span.spanId, span));

  // Find root span and build tree
  let rootSpan: Span | null = null;
  spans.forEach(span => {
    if (!span.parentSpanId) {
      rootSpan = span;
    } else if (spanMap.has(span.parentSpanId)) {
      const parent = spanMap.get(span.parentSpanId)!;
      parent.children.push(span);
    }
  });

  // Calculate depths
  const calculateDepth = (span: Span, depth: number) => {
    span.depth = depth;
    span.children.forEach(child => calculateDepth(child, depth + 1));
  };
  if (rootSpan) {
    calculateDepth(rootSpan, 0);
  }

  // Calculate trace duration and percentages
  const traceStartTime = spans.length > 0 ? Math.min(...spans.map(s => s.startTime)) : 0;
  const traceEndTime = spans.length > 0 ? Math.max(...spans.map(s => s.startTime + s.duration)) : 0;
  const traceDuration = traceEndTime - traceStartTime;

  spans.forEach(span => {
    span.percentOfTrace = traceDuration > 0 ? (span.duration / traceDuration) * 100 : 0;
  });

  // Calculate service info
  const serviceNames = [...new Set(spans.map(s => s.serviceName))];
  const services = serviceNames.map((name, i) => {
    const serviceSpans = spans.filter(s => s.serviceName === name);
    const totalDuration = serviceSpans.reduce((sum, s) => sum + s.duration, 0);
    return {
      name,
      color: getServiceColor(name, i),
      spanCount: serviceSpans.length,
      errorCount: serviceSpans.filter(s => s.status === 'ERROR').length,
      totalDuration,
      avgDuration: serviceSpans.length > 0 ? totalDuration / serviceSpans.length : 0,
    };
  });

  return {
    traceId,
    rootSpan: rootSpan || spans[0],
    spans,
    services,
    startTime: traceStartTime,
    endTime: traceEndTime,
    duration: traceDuration,
    spanCount: spans.length,
    errorCount: spans.filter(s => s.status === 'ERROR').length,
    serviceCount: services.length,
  };
}

export default TraceDetailPage;
