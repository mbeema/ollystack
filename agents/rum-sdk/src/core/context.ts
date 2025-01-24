/**
 * Trace Context Management
 *
 * Implements W3C Trace Context for distributed tracing.
 * Propagates trace context to backend services via HTTP headers.
 */

import { TraceContext, SpanInterface } from '../types';
import { SessionManager } from './session';

const TRACEPARENT_HEADER = 'traceparent';
const TRACESTATE_HEADER = 'tracestate';

export class TraceContextManager {
  private sessionManager: SessionManager;
  private currentContext: TraceContext | null = null;
  private activeSpans: Map<string, Span> = new Map();

  constructor(sessionManager: SessionManager) {
    this.sessionManager = sessionManager;
  }

  /**
   * Get or create trace context for current page
   */
  getTraceContext(): TraceContext {
    if (!this.currentContext) {
      this.currentContext = this.createNewContext();
    }
    return this.currentContext;
  }

  /**
   * Create a new trace context
   */
  createNewContext(): TraceContext {
    return {
      traceId: this.sessionManager.generateTraceId(),
      spanId: this.sessionManager.generateSpanId(),
      sampled: true,
    };
  }

  /**
   * Create a child span
   */
  createSpan(name: string, parentContext?: TraceContext): Span {
    const parent = parentContext || this.currentContext || this.createNewContext();
    const span = new Span(
      name,
      parent.traceId,
      this.sessionManager.generateSpanId(),
      parent.spanId
    );
    this.activeSpans.set(span.spanId, span);
    return span;
  }

  /**
   * End and remove a span
   */
  endSpan(spanId: string): void {
    this.activeSpans.delete(spanId);
  }

  /**
   * Parse traceparent header (W3C format)
   * Format: version-traceId-spanId-flags
   * Example: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
   */
  parseTraceparent(header: string): TraceContext | null {
    const parts = header.trim().split('-');
    if (parts.length !== 4) return null;

    const [version, traceId, spanId, flags] = parts;

    // Only support version 00
    if (version !== '00') return null;

    // Validate lengths
    if (traceId.length !== 32 || spanId.length !== 16) return null;

    // Validate hex
    if (!/^[0-9a-f]+$/.test(traceId) || !/^[0-9a-f]+$/.test(spanId)) return null;

    return {
      traceId,
      spanId,
      sampled: (parseInt(flags, 16) & 0x01) === 0x01,
    };
  }

  /**
   * Generate traceparent header
   */
  generateTraceparent(context?: TraceContext): string {
    const ctx = context || this.getTraceContext();
    const flags = ctx.sampled ? '01' : '00';
    return `00-${ctx.traceId}-${ctx.spanId}-${flags}`;
  }

  /**
   * Generate tracestate header
   */
  generateTracestate(context?: TraceContext): string {
    const ctx = context || this.getTraceContext();
    return `ollystack=${ctx.spanId}`;
  }

  /**
   * Get headers to inject into outgoing requests
   */
  getHeadersForPropagation(context?: TraceContext): Record<string, string> {
    const ctx = context || this.getTraceContext();
    return {
      [TRACEPARENT_HEADER]: this.generateTraceparent(ctx),
      [TRACESTATE_HEADER]: this.generateTracestate(ctx),
    };
  }

  /**
   * Start a new page context (called on navigation)
   */
  startPageContext(): void {
    this.currentContext = this.createNewContext();
    this.activeSpans.clear();
  }

  /**
   * Get all active spans
   */
  getActiveSpans(): Span[] {
    return Array.from(this.activeSpans.values());
  }
}

/**
 * Span implementation
 */
export class Span implements SpanInterface {
  readonly traceId: string;
  readonly spanId: string;
  readonly name: string;
  readonly startTime: number;
  readonly parentSpanId?: string;

  private endTime?: number;
  private status: 'ok' | 'error' = 'ok';
  private attributes: Map<string, string | number | boolean> = new Map();
  private events: Array<{ name: string; timestamp: number; attributes?: Record<string, unknown> }> = [];

  constructor(name: string, traceId: string, spanId: string, parentSpanId?: string) {
    this.name = name;
    this.traceId = traceId;
    this.spanId = spanId;
    this.parentSpanId = parentSpanId;
    this.startTime = performance.now();
  }

  /**
   * End the span
   */
  end(): void {
    if (!this.endTime) {
      this.endTime = performance.now();
    }
  }

  /**
   * Get span duration in milliseconds
   */
  getDuration(): number {
    const end = this.endTime || performance.now();
    return end - this.startTime;
  }

  /**
   * Set span status
   */
  setStatus(status: 'ok' | 'error'): void {
    this.status = status;
  }

  /**
   * Get span status
   */
  getStatus(): 'ok' | 'error' {
    return this.status;
  }

  /**
   * Set an attribute
   */
  setAttribute(key: string, value: string | number | boolean): void {
    this.attributes.set(key, value);
  }

  /**
   * Get all attributes
   */
  getAttributes(): Record<string, string | number | boolean> {
    return Object.fromEntries(this.attributes);
  }

  /**
   * Add an event to the span
   */
  addEvent(name: string, attributes?: Record<string, unknown>): void {
    this.events.push({
      name,
      timestamp: performance.now(),
      attributes,
    });
  }

  /**
   * Get all events
   */
  getEvents(): Array<{ name: string; timestamp: number; attributes?: Record<string, unknown> }> {
    return this.events;
  }

  /**
   * Check if span is ended
   */
  isEnded(): boolean {
    return this.endTime !== undefined;
  }

  /**
   * Convert to JSON-serializable object
   */
  toJSON(): Record<string, unknown> {
    return {
      traceId: this.traceId,
      spanId: this.spanId,
      parentSpanId: this.parentSpanId,
      name: this.name,
      startTime: this.startTime,
      endTime: this.endTime,
      duration: this.getDuration(),
      status: this.status,
      attributes: this.getAttributes(),
      events: this.events,
    };
  }
}
