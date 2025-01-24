/**
 * OllyStack RUM SDK Types
 */

// Configuration types

export interface RUMConfig {
  /** Application/service name */
  applicationName: string;

  /** Application version */
  version?: string;

  /** Environment (production, staging, development) */
  environment?: string;

  /** Endpoint to send telemetry data */
  endpoint: string;

  /** API key for authentication */
  apiKey?: string;

  /** Enable debug logging */
  debug?: boolean;

  /** Sample rate (0.0 - 1.0) */
  sampleRate?: number;

  /** Session timeout in milliseconds (default: 30 minutes) */
  sessionTimeout?: number;

  /** Maximum events per batch */
  batchSize?: number;

  /** Flush interval in milliseconds */
  flushInterval?: number;

  /** Enable performance monitoring */
  enablePerformance?: boolean;

  /** Enable error tracking */
  enableErrors?: boolean;

  /** Enable network request tracking */
  enableNetwork?: boolean;

  /** Enable user interaction tracking */
  enableInteractions?: boolean;

  /** Enable session replay (experimental) */
  enableSessionReplay?: boolean;

  /** URLs to ignore for network tracking (regex patterns) */
  ignoreUrls?: (string | RegExp)[];

  /** Error messages to ignore (regex patterns) */
  ignoreErrors?: (string | RegExp)[];

  /** Custom tags to add to all events */
  tags?: Record<string, string>;

  /** User identification callback */
  getUser?: () => UserInfo | null;

  /** Before send hook - return false to drop event */
  beforeSend?: (event: RUMEvent) => RUMEvent | false;

  /** Propagate trace context to these domains */
  propagateTraceHeaderCorsUrls?: (string | RegExp)[];
}

// User info

export interface UserInfo {
  id?: string;
  email?: string;
  username?: string;
  name?: string;
  [key: string]: string | undefined;
}

// Session info

export interface SessionInfo {
  id: string;
  startTime: number;
  lastActivity: number;
  pageViews: number;
  userId?: string;
}

// Trace context for distributed tracing

export interface TraceContext {
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  sampled: boolean;
}

// Event types

export type RUMEventType =
  | 'page_view'
  | 'performance'
  | 'error'
  | 'network'
  | 'interaction'
  | 'resource'
  | 'long_task'
  | 'custom';

export interface RUMEvent {
  type: RUMEventType;
  timestamp: number;
  sessionId: string;
  traceId?: string;
  spanId?: string;
  application: string;
  version?: string;
  environment?: string;
  url: string;
  userAgent: string;
  user?: UserInfo;
  tags?: Record<string, string>;
  data: Record<string, unknown>;
}

// Performance metrics

export interface WebVitals {
  /** Largest Contentful Paint (ms) */
  lcp?: number;
  /** First Input Delay (ms) */
  fid?: number;
  /** Cumulative Layout Shift */
  cls?: number;
  /** First Contentful Paint (ms) */
  fcp?: number;
  /** Time to First Byte (ms) */
  ttfb?: number;
  /** Interaction to Next Paint (ms) */
  inp?: number;
}

export interface PerformanceData {
  /** Navigation timing */
  navigation?: NavigationTiming;
  /** Core Web Vitals */
  webVitals?: WebVitals;
  /** Resource timing entries */
  resources?: ResourceTiming[];
  /** Long tasks */
  longTasks?: LongTask[];
}

export interface NavigationTiming {
  /** Page fully loaded */
  loadTime: number;
  /** DOM content loaded */
  domContentLoaded: number;
  /** DOM interactive */
  domInteractive: number;
  /** DNS lookup time */
  dnsTime: number;
  /** TCP connection time */
  connectTime: number;
  /** TLS handshake time */
  tlsTime: number;
  /** Time to first byte */
  ttfb: number;
  /** Response time */
  responseTime: number;
  /** DOM processing time */
  domProcessingTime: number;
  /** Redirect time */
  redirectTime: number;
}

export interface ResourceTiming {
  name: string;
  type: string;
  duration: number;
  transferSize: number;
  encodedBodySize: number;
  decodedBodySize: number;
  startTime: number;
}

export interface LongTask {
  name: string;
  duration: number;
  startTime: number;
  attribution: string[];
}

// Error data

export interface ErrorData {
  message: string;
  stack?: string;
  type: string;
  filename?: string;
  lineno?: number;
  colno?: number;
  handled: boolean;
  context?: Record<string, unknown>;
}

// Network request data

export interface NetworkData {
  method: string;
  url: string;
  status?: number;
  statusText?: string;
  duration: number;
  requestSize?: number;
  responseSize?: number;
  requestHeaders?: Record<string, string>;
  responseHeaders?: Record<string, string>;
  type: 'xhr' | 'fetch' | 'beacon';
  traceId?: string;
  spanId?: string;
  error?: string;
}

// User interaction data

export interface InteractionData {
  type: 'click' | 'input' | 'scroll' | 'submit' | 'change' | 'focus' | 'blur';
  target: string;
  targetId?: string;
  targetClass?: string;
  targetTag: string;
  targetText?: string;
  value?: string;
  x?: number;
  y?: number;
  timestamp: number;
}

// Page view data

export interface PageViewData {
  url: string;
  path: string;
  title: string;
  referrer: string;
  previousPage?: string;
  loadTime?: number;
}

// Plugin interface

export interface RUMPlugin {
  name: string;
  init(sdk: RUMSDKInterface): void;
  destroy?(): void;
}

// SDK interface for plugins

export interface RUMSDKInterface {
  config: RUMConfig;
  getSessionId(): string;
  getTraceContext(): TraceContext;
  createSpan(name: string): SpanInterface;
  sendEvent(event: Omit<RUMEvent, 'timestamp' | 'sessionId' | 'application' | 'url' | 'userAgent'>): void;
  log(level: 'debug' | 'info' | 'warn' | 'error', message: string, data?: unknown): void;
}

// Span interface

export interface SpanInterface {
  traceId: string;
  spanId: string;
  name: string;
  startTime: number;
  end(): void;
  setStatus(status: 'ok' | 'error'): void;
  setAttribute(key: string, value: string | number | boolean): void;
  addEvent(name: string, attributes?: Record<string, unknown>): void;
}

// Transport interface

export interface Transport {
  send(events: RUMEvent[]): Promise<void>;
  flush(): Promise<void>;
}

// Internal queue item

export interface QueueItem {
  event: RUMEvent;
  retries: number;
}
