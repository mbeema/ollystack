// Trace and Span types for visualization

export interface Span {
  spanId: string;
  traceId: string;
  parentSpanId: string | null;
  operationName: string;
  serviceName: string;
  serviceColor: string;
  startTime: number; // microseconds
  duration: number; // microseconds
  status: 'OK' | 'ERROR' | 'UNSET';
  statusMessage?: string;
  kind: 'INTERNAL' | 'SERVER' | 'CLIENT' | 'PRODUCER' | 'CONSUMER';
  attributes: Record<string, string | number | boolean>;
  events: SpanEvent[];
  links: SpanLink[];
  // Computed fields
  depth: number;
  children: Span[];
  selfTime: number; // Time not spent in children
  percentOfTrace: number;
}

export interface SpanEvent {
  name: string;
  timestamp: number;
  attributes: Record<string, string | number | boolean>;
}

export interface SpanLink {
  traceId: string;
  spanId: string;
  attributes: Record<string, string | number | boolean>;
}

export interface Trace {
  traceId: string;
  rootSpan: Span;
  spans: Span[];
  services: ServiceInfo[];
  startTime: number;
  endTime: number;
  duration: number;
  spanCount: number;
  errorCount: number;
  serviceCount: number;
}

export interface ServiceInfo {
  name: string;
  color: string;
  spanCount: number;
  errorCount: number;
  totalDuration: number;
  avgDuration: number;
}

// Service Map types
export interface ServiceNode {
  id: string;
  name: string;
  type: 'service' | 'database' | 'cache' | 'queue' | 'external';
  metrics: {
    requestRate: number;
    errorRate: number;
    p50Latency: number;
    p99Latency: number;
  };
  health: 'healthy' | 'degraded' | 'error';
}

export interface ServiceEdge {
  id: string;
  source: string;
  target: string;
  metrics: {
    requestRate: number;
    errorRate: number;
    avgLatency: number;
  };
}

export interface ServiceMap {
  nodes: ServiceNode[];
  edges: ServiceEdge[];
}

// View modes
export type TraceViewMode = 'flamegraph' | 'waterfall' | 'spanlist' | 'map';

// Filter types
export interface SpanFilter {
  services: string[];
  minDuration: number | null;
  maxDuration: number | null;
  status: ('OK' | 'ERROR' | 'UNSET')[];
  searchQuery: string;
  attributes: Record<string, string>;
}

// Color palette for services
export const SERVICE_COLORS = [
  '#6366f1', // indigo
  '#8b5cf6', // violet
  '#a855f7', // purple
  '#d946ef', // fuchsia
  '#ec4899', // pink
  '#f43f5e', // rose
  '#ef4444', // red
  '#f97316', // orange
  '#f59e0b', // amber
  '#eab308', // yellow
  '#84cc16', // lime
  '#22c55e', // green
  '#10b981', // emerald
  '#14b8a6', // teal
  '#06b6d4', // cyan
  '#0ea5e9', // sky
  '#3b82f6', // blue
];

// Helper to get service color
export function getServiceColor(serviceName: string, index: number): string {
  return SERVICE_COLORS[index % SERVICE_COLORS.length];
}

// Helper to format duration
export function formatDuration(microseconds: number): string {
  if (microseconds < 1000) {
    return `${microseconds.toFixed(0)}μs`;
  } else if (microseconds < 1000000) {
    return `${(microseconds / 1000).toFixed(2)}ms`;
  } else {
    return `${(microseconds / 1000000).toFixed(2)}s`;
  }
}

// Helper to format timestamp
export function formatTimestamp(timestamp: number): string {
  const date = new Date(timestamp / 1000); // Convert microseconds to milliseconds
  return date.toISOString().replace('T', ' ').replace('Z', '');
}

// Helper to calculate span tree
export function buildSpanTree(spans: Span[]): Span {
  const spanMap = new Map<string, Span>();
  let rootSpan: Span | null = null;

  // First pass: create map and initialize children
  spans.forEach(span => {
    spanMap.set(span.spanId, { ...span, children: [], depth: 0 });
  });

  // Second pass: build tree
  spanMap.forEach(span => {
    if (span.parentSpanId && spanMap.has(span.parentSpanId)) {
      const parent = spanMap.get(span.parentSpanId)!;
      parent.children.push(span);
      span.depth = parent.depth + 1;
    } else {
      rootSpan = span;
    }
  });

  // Sort children by start time
  const sortChildren = (span: Span) => {
    span.children.sort((a, b) => a.startTime - b.startTime);
    span.children.forEach(sortChildren);
  };

  if (rootSpan) {
    sortChildren(rootSpan);
  }

  return rootSpan || spans[0];
}

// Helper to flatten span tree for list view
export function flattenSpanTree(root: Span): Span[] {
  const result: Span[] = [];

  const traverse = (span: Span) => {
    result.push(span);
    span.children.forEach(traverse);
  };

  traverse(root);
  return result;
}

// Helper to filter spans
export function filterSpans(spans: Span[], filter: SpanFilter): Span[] {
  return spans.filter(span => {
    // Service filter
    if (filter.services.length > 0 && !filter.services.includes(span.serviceName)) {
      return false;
    }

    // Duration filter
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

    // Search query
    if (filter.searchQuery) {
      const query = filter.searchQuery.toLowerCase();
      const matchesName = span.operationName.toLowerCase().includes(query);
      const matchesService = span.serviceName.toLowerCase().includes(query);
      const matchesAttributes = Object.entries(span.attributes).some(
        ([key, value]) =>
          key.toLowerCase().includes(query) ||
          String(value).toLowerCase().includes(query)
      );
      if (!matchesName && !matchesService && !matchesAttributes) {
        return false;
      }
    }

    // Attribute filter
    for (const [key, value] of Object.entries(filter.attributes)) {
      if (span.attributes[key] !== value) {
        return false;
      }
    }

    return true;
  });
}
