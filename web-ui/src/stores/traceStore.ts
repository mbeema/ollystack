import { create } from 'zustand';
import type { Trace, Span, SpanFilter, TraceViewMode, ServiceMap } from '../types/trace';

interface TraceState {
  // Current trace data
  trace: Trace | null;
  isLoading: boolean;
  error: string | null;

  // View state
  viewMode: TraceViewMode;
  selectedSpanId: string | null;
  hoveredSpanId: string | null;
  expandedSpanIds: Set<string>;

  // Zoom/pan state
  timeRange: { start: number; end: number } | null;
  zoomLevel: number;

  // Filter state
  filter: SpanFilter;
  filteredSpans: Span[];
  searchMatches: string[];
  currentMatchIndex: number;

  // Service map
  serviceMap: ServiceMap | null;

  // Actions
  setTrace: (trace: Trace) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;

  setViewMode: (mode: TraceViewMode) => void;
  selectSpan: (spanId: string | null) => void;
  hoverSpan: (spanId: string | null) => void;
  toggleSpanExpanded: (spanId: string) => void;
  expandAll: () => void;
  collapseAll: () => void;

  setTimeRange: (range: { start: number; end: number } | null) => void;
  setZoomLevel: (level: number) => void;
  zoomIn: () => void;
  zoomOut: () => void;
  resetZoom: () => void;

  setFilter: (filter: Partial<SpanFilter>) => void;
  clearFilter: () => void;
  setSearchQuery: (query: string) => void;
  nextMatch: () => void;
  prevMatch: () => void;
  setServiceFilter: (services: string[]) => void;
  setStatusFilter: (status: ('OK' | 'ERROR' | 'UNSET')[]) => void;
  setDurationFilter: (min: number | null, max: number | null) => void;

  setServiceMap: (map: ServiceMap) => void;
}

const defaultFilter: SpanFilter = {
  services: [],
  minDuration: null,
  maxDuration: null,
  status: [],
  searchQuery: '',
  attributes: {},
};

export const useTraceStore = create<TraceState>((set, get) => ({
  // Initial state
  trace: null,
  isLoading: false,
  error: null,
  viewMode: 'waterfall',
  selectedSpanId: null,
  hoveredSpanId: null,
  expandedSpanIds: new Set(),
  timeRange: null,
  zoomLevel: 1,
  filter: defaultFilter,
  filteredSpans: [],
  searchMatches: [],
  currentMatchIndex: 0,
  serviceMap: null,

  // Actions
  setTrace: (trace) => {
    const allSpanIds = new Set(trace.spans.map(s => s.spanId));
    set({
      trace,
      filteredSpans: trace.spans,
      expandedSpanIds: allSpanIds,
      timeRange: { start: trace.startTime, end: trace.endTime },
      selectedSpanId: trace.rootSpan.spanId,
    });
  },

  setLoading: (isLoading) => set({ isLoading }),
  setError: (error) => set({ error }),

  setViewMode: (viewMode) => set({ viewMode }),

  selectSpan: (spanId) => set({ selectedSpanId: spanId }),

  hoverSpan: (spanId) => set({ hoveredSpanId: spanId }),

  toggleSpanExpanded: (spanId) => {
    const { expandedSpanIds } = get();
    const newSet = new Set(expandedSpanIds);
    if (newSet.has(spanId)) {
      newSet.delete(spanId);
    } else {
      newSet.add(spanId);
    }
    set({ expandedSpanIds: newSet });
  },

  expandAll: () => {
    const { trace } = get();
    if (trace) {
      set({ expandedSpanIds: new Set(trace.spans.map(s => s.spanId)) });
    }
  },

  collapseAll: () => {
    const { trace } = get();
    if (trace) {
      set({ expandedSpanIds: new Set([trace.rootSpan.spanId]) });
    }
  },

  setTimeRange: (timeRange) => set({ timeRange }),

  setZoomLevel: (zoomLevel) => set({ zoomLevel: Math.max(0.1, Math.min(10, zoomLevel)) }),

  zoomIn: () => {
    const { zoomLevel } = get();
    set({ zoomLevel: Math.min(10, zoomLevel * 1.5) });
  },

  zoomOut: () => {
    const { zoomLevel } = get();
    set({ zoomLevel: Math.max(0.1, zoomLevel / 1.5) });
  },

  resetZoom: () => {
    const { trace } = get();
    if (trace) {
      set({
        zoomLevel: 1,
        timeRange: { start: trace.startTime, end: trace.endTime },
      });
    }
  },

  setFilter: (filterUpdate) => {
    const { trace, filter } = get();
    const newFilter = { ...filter, ...filterUpdate };

    if (trace) {
      const filteredSpans = applyFilters(trace.spans, newFilter);
      set({ filter: newFilter, filteredSpans });
    } else {
      set({ filter: newFilter });
    }
  },

  clearFilter: () => {
    const { trace } = get();
    set({
      filter: defaultFilter,
      filteredSpans: trace?.spans || [],
      searchMatches: [],
      currentMatchIndex: 0,
    });
  },

  setSearchQuery: (searchQuery) => {
    const { trace } = get();
    if (!trace) return;

    const query = searchQuery.toLowerCase();
    const matches: string[] = [];

    if (query) {
      trace.spans.forEach(span => {
        const matchesName = span.operationName.toLowerCase().includes(query);
        const matchesService = span.serviceName.toLowerCase().includes(query);
        const matchesAttributes = Object.entries(span.attributes).some(
          ([key, value]) =>
            key.toLowerCase().includes(query) ||
            String(value).toLowerCase().includes(query)
        );
        if (matchesName || matchesService || matchesAttributes) {
          matches.push(span.spanId);
        }
      });
    }

    set({
      filter: { ...get().filter, searchQuery },
      searchMatches: matches,
      currentMatchIndex: matches.length > 0 ? 0 : -1,
      selectedSpanId: matches.length > 0 ? matches[0] : get().selectedSpanId,
    });
  },

  nextMatch: () => {
    const { searchMatches, currentMatchIndex } = get();
    if (searchMatches.length === 0) return;

    const nextIndex = (currentMatchIndex + 1) % searchMatches.length;
    set({
      currentMatchIndex: nextIndex,
      selectedSpanId: searchMatches[nextIndex],
    });
  },

  prevMatch: () => {
    const { searchMatches, currentMatchIndex } = get();
    if (searchMatches.length === 0) return;

    const prevIndex = currentMatchIndex === 0 ? searchMatches.length - 1 : currentMatchIndex - 1;
    set({
      currentMatchIndex: prevIndex,
      selectedSpanId: searchMatches[prevIndex],
    });
  },

  setServiceFilter: (services) => {
    const { trace, filter } = get();
    const newFilter = { ...filter, services };

    if (trace) {
      const filteredSpans = applyFilters(trace.spans, newFilter);
      set({ filter: newFilter, filteredSpans });
    } else {
      set({ filter: newFilter });
    }
  },

  setStatusFilter: (status) => {
    const { trace, filter } = get();
    const newFilter = { ...filter, status };

    if (trace) {
      const filteredSpans = applyFilters(trace.spans, newFilter);
      set({ filter: newFilter, filteredSpans });
    } else {
      set({ filter: newFilter });
    }
  },

  setDurationFilter: (minDuration, maxDuration) => {
    const { trace, filter } = get();
    const newFilter = { ...filter, minDuration, maxDuration };

    if (trace) {
      const filteredSpans = applyFilters(trace.spans, newFilter);
      set({ filter: newFilter, filteredSpans });
    } else {
      set({ filter: newFilter });
    }
  },

  setServiceMap: (serviceMap) => set({ serviceMap }),
}));

// Helper function to apply all filters
function applyFilters(spans: Span[], filter: SpanFilter): Span[] {
  return spans.filter(span => {
    // Service filter
    if (filter.services.length > 0 && !filter.services.includes(span.serviceName)) {
      return false;
    }
    // Duration filter (values are in microseconds)
    if (filter.minDuration !== null && span.duration < filter.minDuration) {
      return false;
    }
    if (filter.maxDuration !== null && span.duration > filter.maxDuration) {
      return false;
    }
    // Status filter
    if (filter.status.length > 0 && !filter.status.includes(span.status)) {
      return false;
    }
    return true;
  });
}
