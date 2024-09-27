import { WebTracerProvider } from '@opentelemetry/sdk-trace-web';
import { BatchSpanProcessor } from '@opentelemetry/sdk-trace-base';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { ZoneContextManager } from '@opentelemetry/context-zone';
import { registerInstrumentations } from '@opentelemetry/instrumentation';
import { DocumentLoadInstrumentation } from '@opentelemetry/instrumentation-document-load';
import { FetchInstrumentation } from '@opentelemetry/instrumentation-fetch';
import { XMLHttpRequestInstrumentation } from '@opentelemetry/instrumentation-xml-http-request';
import { UserInteractionInstrumentation } from '@opentelemetry/instrumentation-user-interaction';
import { LongTaskInstrumentation } from '@opentelemetry/instrumentation-long-task';
import { Resource } from '@opentelemetry/resources';
import { trace, SpanStatusCode } from '@opentelemetry/api';
import { onCLS, onLCP, onTTFB, onINP, type Metric } from 'web-vitals';

// =============================================================================
// Session Management
// =============================================================================

const SESSION_KEY = 'ollystack_session';
const SESSION_TIMEOUT = 30 * 60 * 1000; // 30 minutes

interface SessionData {
  id: string;
  startTime: number;
  lastActivity: number;
  pageViews: number;
  userId?: string;
}

function generateSessionId(): string {
  return `${Date.now().toString(36)}-${Math.random().toString(36).substring(2, 11)}`;
}

function getOrCreateSession(): SessionData {
  try {
    const stored = sessionStorage.getItem(SESSION_KEY);
    if (stored) {
      const session: SessionData = JSON.parse(stored);
      const now = Date.now();
      // Check if session is still valid (not timed out)
      if (now - session.lastActivity < SESSION_TIMEOUT) {
        session.lastActivity = now;
        sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
        return session;
      }
    }
  } catch {
    // Ignore storage errors
  }

  // Create new session
  const session: SessionData = {
    id: generateSessionId(),
    startTime: Date.now(),
    lastActivity: Date.now(),
    pageViews: 0,
  };

  try {
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
  } catch {
    // Ignore storage errors
  }

  return session;
}

function updateSessionPageView(): SessionData {
  const session = getOrCreateSession();
  session.pageViews++;
  session.lastActivity = Date.now();
  try {
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
  } catch {
    // Ignore storage errors
  }
  return session;
}

// Export session for use in other parts of the app
export function getSession(): SessionData {
  return getOrCreateSession();
}

// =============================================================================
// Configuration
// =============================================================================

function getOtelEndpoint(): string {
  if (typeof window !== 'undefined' && (window as any).__OLLYSTACK_CONFIG__?.OTLP_URL) {
    return (window as any).__OLLYSTACK_CONFIG__.OTLP_URL;
  }
  // Legacy support
  if (typeof window !== 'undefined' && (window as any).__OTEL_ENDPOINT__) {
    return (window as any).__OTEL_ENDPOINT__;
  }
  return window.location.origin;
}

// =============================================================================
// Tracer instance (for custom spans)
// =============================================================================

let tracerInstance: ReturnType<typeof trace.getTracer> | null = null;

export function getTracer() {
  if (!tracerInstance) {
    tracerInstance = trace.getTracer('ollystack-rum', '1.0.0');
  }
  return tracerInstance;
}

// =============================================================================
// Core Web Vitals
// =============================================================================

function initWebVitals() {
  const tracer = getTracer();

  const reportMetric = (metric: Metric) => {
    const span = tracer.startSpan(`web-vital.${metric.name}`, {
      attributes: {
        'web_vital.name': metric.name,
        'web_vital.value': metric.value,
        'web_vital.rating': metric.rating,
        'web_vital.delta': metric.delta,
        'web_vital.id': metric.id,
        'web_vital.navigation_type': metric.navigationType || 'unknown',
        'session.id': getSession().id,
        'page.url': window.location.href,
        'page.path': window.location.pathname,
      },
    });
    span.end();
  };

  // Core Web Vitals
  onCLS(reportMetric);  // Cumulative Layout Shift
  onLCP(reportMetric);  // Largest Contentful Paint
  onTTFB(reportMetric); // Time to First Byte
  onINP(reportMetric);  // Interaction to Next Paint
}

// =============================================================================
// Error Tracking
// =============================================================================

function initErrorTracking() {
  const tracer = getTracer();

  // Global error handler
  window.addEventListener('error', (event) => {
    const span = tracer.startSpan('browser.error', {
      attributes: {
        'error.type': 'uncaught_exception',
        'error.message': event.message,
        'error.filename': event.filename,
        'error.lineno': event.lineno,
        'error.colno': event.colno,
        'session.id': getSession().id,
        'page.url': window.location.href,
        'page.path': window.location.pathname,
      },
    });
    span.setStatus({ code: SpanStatusCode.ERROR, message: event.message });
    span.end();
  });

  // Unhandled promise rejection handler
  window.addEventListener('unhandledrejection', (event) => {
    const message = event.reason?.message || event.reason?.toString() || 'Unknown rejection';
    const span = tracer.startSpan('browser.error', {
      attributes: {
        'error.type': 'unhandled_rejection',
        'error.message': message,
        'error.stack': event.reason?.stack || '',
        'session.id': getSession().id,
        'page.url': window.location.href,
        'page.path': window.location.pathname,
      },
    });
    span.setStatus({ code: SpanStatusCode.ERROR, message });
    span.end();
  });
}

// =============================================================================
// Route Change Tracking (User Journey)
// =============================================================================

function initRouteTracking() {
  const tracer = getTracer();
  let currentPath = window.location.pathname;
  let pageViewSpan: ReturnType<typeof tracer.startSpan> | null = null;

  const trackPageView = (fromPath: string, toPath: string) => {
    const session = updateSessionPageView();

    // End previous page view span
    if (pageViewSpan) {
      pageViewSpan.end();
    }

    // Start new page view span
    pageViewSpan = tracer.startSpan('page.view', {
      attributes: {
        'page.url': window.location.href,
        'page.path': toPath,
        'page.title': document.title,
        'page.referrer': fromPath !== toPath ? fromPath : document.referrer,
        'page.view_number': session.pageViews,
        'session.id': session.id,
        'session.duration_ms': Date.now() - session.startTime,
        'navigation.type': fromPath === toPath ? 'initial' : 'spa_navigation',
      },
    });

    // Create a separate navigation span that ends immediately
    const navSpan = tracer.startSpan('navigation', {
      attributes: {
        'navigation.from': fromPath,
        'navigation.to': toPath,
        'session.id': session.id,
      },
    });
    navSpan.end();
  };

  // Track initial page load
  trackPageView('', currentPath);

  // Track SPA route changes using History API
  const originalPushState = history.pushState;
  const originalReplaceState = history.replaceState;

  history.pushState = function (...args) {
    const result = originalPushState.apply(this, args);
    const newPath = window.location.pathname;
    if (newPath !== currentPath) {
      trackPageView(currentPath, newPath);
      currentPath = newPath;
    }
    return result;
  };

  history.replaceState = function (...args) {
    const result = originalReplaceState.apply(this, args);
    const newPath = window.location.pathname;
    if (newPath !== currentPath) {
      trackPageView(currentPath, newPath);
      currentPath = newPath;
    }
    return result;
  };

  // Handle browser back/forward navigation
  window.addEventListener('popstate', () => {
    const newPath = window.location.pathname;
    if (newPath !== currentPath) {
      trackPageView(currentPath, newPath);
      currentPath = newPath;
    }
  });
}

// =============================================================================
// Custom Event Tracking (for use in components)
// =============================================================================

export function trackEvent(
  eventName: string,
  attributes: Record<string, string | number | boolean> = {}
) {
  const tracer = getTracer();
  const span = tracer.startSpan(`event.${eventName}`, {
    attributes: {
      'event.name': eventName,
      'session.id': getSession().id,
      'page.url': window.location.href,
      'page.path': window.location.pathname,
      ...attributes,
    },
  });
  span.end();
}

export function trackUserAction(
  action: string,
  target: string,
  attributes: Record<string, string | number | boolean> = {}
) {
  trackEvent('user_action', {
    'user_action.type': action,
    'user_action.target': target,
    ...attributes,
  });
}

// =============================================================================
// Main Initialization
// =============================================================================

let initialized = false;

export function initTelemetry() {
  if (typeof window === 'undefined' || initialized) {
    return;
  }

  try {
    const otelEndpoint = getOtelEndpoint();
    const session = getOrCreateSession();

    const resource = new Resource({
      'service.name': 'ollystack-web-ui',
      'service.version': '1.0.0',
      'deployment.environment': (window as any).__OLLYSTACK_CONFIG__?.ENVIRONMENT || 'production',
      'session.id': session.id,
      'browser.user_agent': navigator.userAgent,
      'browser.language': navigator.language,
      'screen.width': window.screen.width,
      'screen.height': window.screen.height,
      'viewport.width': window.innerWidth,
      'viewport.height': window.innerHeight,
    });

    const provider = new WebTracerProvider({ resource });

    const exporter = new OTLPTraceExporter({
      url: `${otelEndpoint}/v1/traces`,
    });

    provider.addSpanProcessor(new BatchSpanProcessor(exporter, {
      maxQueueSize: 100,
      maxExportBatchSize: 10,
      scheduledDelayMillis: 5000,
    }));

    provider.register({
      contextManager: new ZoneContextManager(),
    });

    // Register OpenTelemetry instrumentations
    registerInstrumentations({
      instrumentations: [
        // Page load performance
        new DocumentLoadInstrumentation(),

        // API call tracking
        new FetchInstrumentation({
          propagateTraceHeaderCorsUrls: [
            /localhost/,
            /127\.0\.0\.1/,
            /ollystack\.com/,
            new RegExp(window.location.origin.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')),
          ],
          clearTimingResources: true,
          applyCustomAttributesOnSpan: (span) => {
            span.setAttribute('session.id', getSession().id);
          },
        }),

        new XMLHttpRequestInstrumentation({
          propagateTraceHeaderCorsUrls: [
            /localhost/,
            /127\.0\.0\.1/,
            /ollystack\.com/,
            new RegExp(window.location.origin.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')),
          ],
        }),

        // User interaction tracking (clicks, form submissions)
        new UserInteractionInstrumentation({
          eventNames: ['click', 'submit', 'change'],
          shouldPreventSpanCreation: (eventType, element) => {
            // Skip tracking for non-interactive elements
            const tagName = element.tagName.toLowerCase();
            if (eventType === 'click' && !['a', 'button', 'input', 'select', 'textarea'].includes(tagName)) {
              // Check if element has role or is inside a button
              if (!element.getAttribute('role') && !element.closest('button, a, [role="button"]')) {
                return true;
              }
            }
            return false;
          },
        }),

        // Long task detection (UI blocking operations)
        new LongTaskInstrumentation(),
      ],
    });

    // Initialize custom tracking
    initWebVitals();
    initErrorTracking();
    initRouteTracking();

    initialized = true;
    console.log('RUM initialized - session:', session.id, 'endpoint:', otelEndpoint);
  } catch (error) {
    console.warn('Failed to initialize RUM:', error);
  }
}

// Auto-initialize on import (can be disabled via config)
if (typeof window !== 'undefined' && !(window as any).__OLLYSTACK_CONFIG__?.DISABLE_RUM) {
  // Defer initialization to not block initial render
  if (document.readyState === 'complete') {
    initTelemetry();
  } else {
    window.addEventListener('load', initTelemetry);
  }
}
