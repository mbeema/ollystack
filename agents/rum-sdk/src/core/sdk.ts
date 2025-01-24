/**
 * OllyStack RUM SDK Core
 *
 * Main SDK class that orchestrates all components.
 */

import {
  RUMConfig,
  RUMEvent,
  RUMPlugin,
  RUMSDKInterface,
  TraceContext,
  SpanInterface,
  UserInfo,
} from '../types';
import { SessionManager } from './session';
import { TraceContextManager, Span } from './context';
import { BeaconTransport } from '../transport/beacon';
import { createPerformancePlugin } from '../plugins/performance';
import { createErrorPlugin, ErrorPlugin } from '../plugins/errors';
import { createNetworkPlugin } from '../plugins/network';
import { createInteractionsPlugin } from '../plugins/interactions';
import { createNavigationPlugin } from '../plugins/navigation';

const DEFAULT_CONFIG: Partial<RUMConfig> = {
  environment: 'production',
  sampleRate: 1.0,
  sessionTimeout: 30 * 60 * 1000, // 30 minutes
  batchSize: 10,
  flushInterval: 5000,
  enablePerformance: true,
  enableErrors: true,
  enableNetwork: true,
  enableInteractions: true,
  enableSessionReplay: false,
  debug: false,
};

export class OllyStackRUM implements RUMSDKInterface {
  config: RUMConfig;

  private sessionManager: SessionManager;
  private traceManager: TraceContextManager;
  private transport: BeaconTransport;
  private plugins: Map<string, RUMPlugin> = new Map();
  private initialized = false;
  private sampled = true;

  constructor(config: RUMConfig) {
    // Validate required config
    if (!config.applicationName) {
      throw new Error('applicationName is required');
    }
    if (!config.endpoint) {
      throw new Error('endpoint is required');
    }

    // Merge with defaults
    this.config = { ...DEFAULT_CONFIG, ...config } as RUMConfig;

    // Determine if this session should be sampled
    this.sampled = Math.random() < (this.config.sampleRate || 1.0);

    // Initialize core components
    this.sessionManager = new SessionManager(this.config.sessionTimeout);
    this.traceManager = new TraceContextManager(this.sessionManager);
    this.transport = new BeaconTransport(this.config);

    this.log('debug', 'SDK constructed', {
      applicationName: this.config.applicationName,
      sampled: this.sampled,
    });
  }

  /**
   * Initialize the SDK and start collecting data
   */
  init(): OllyStackRUM {
    if (this.initialized) {
      this.log('warn', 'SDK already initialized');
      return this;
    }

    if (typeof window === 'undefined') {
      this.log('warn', 'SDK only works in browser environment');
      return this;
    }

    // Check if sampled
    if (!this.sampled) {
      this.log('info', 'Session not sampled, skipping initialization');
      return this;
    }

    this.log('info', `Initializing OllyStack RUM SDK v0.1.0`);
    this.log('debug', 'Config', this.config);

    // Load default plugins based on config
    this.loadDefaultPlugins();

    this.initialized = true;
    this.log('info', 'SDK initialized successfully');

    return this;
  }

  /**
   * Add a plugin
   */
  use(plugin: RUMPlugin): OllyStackRUM {
    if (this.plugins.has(plugin.name)) {
      this.log('warn', `Plugin ${plugin.name} already registered`);
      return this;
    }

    this.plugins.set(plugin.name, plugin);
    plugin.init(this);
    this.log('debug', `Plugin ${plugin.name} loaded`);

    return this;
  }

  /**
   * Get session ID
   */
  getSessionId(): string {
    return this.sessionManager.getSessionId();
  }

  /**
   * Get current trace context
   */
  getTraceContext(): TraceContext {
    return this.traceManager.getTraceContext();
  }

  /**
   * Create a new span for tracing
   */
  createSpan(name: string): SpanInterface {
    return this.traceManager.createSpan(name);
  }

  /**
   * Set user information
   */
  setUser(user: UserInfo | null): void {
    if (user?.id) {
      this.sessionManager.setUserId(user.id);
    }
    this.log('debug', 'User set', user);
  }

  /**
   * Set a custom tag that will be added to all events
   */
  setTag(key: string, value: string): void {
    if (!this.config.tags) {
      this.config.tags = {};
    }
    this.config.tags[key] = value;
  }

  /**
   * Send a custom event
   */
  trackEvent(name: string, data?: Record<string, unknown>): void {
    this.sendEvent({
      type: 'custom',
      data: {
        name,
        ...data,
      },
    });
  }

  /**
   * Capture an error manually
   */
  captureError(error: Error, context?: Record<string, unknown>): void {
    const errorPlugin = this.plugins.get('errors') as ErrorPlugin | undefined;
    if (errorPlugin) {
      errorPlugin.captureError(error, context);
    } else {
      this.sendEvent({
        type: 'error',
        data: {
          message: error.message,
          stack: error.stack,
          type: error.name,
          handled: true,
          context,
        },
      });
    }
  }

  /**
   * Send an event
   */
  sendEvent(
    event: Omit<
      RUMEvent,
      'timestamp' | 'sessionId' | 'application' | 'url' | 'userAgent'
    >
  ): void {
    if (!this.sampled) return;

    const fullEvent: RUMEvent = {
      ...event,
      timestamp: Date.now(),
      sessionId: this.getSessionId(),
      application: this.config.applicationName,
      version: this.config.version,
      environment: this.config.environment,
      url: typeof window !== 'undefined' ? window.location.href : '',
      userAgent: typeof navigator !== 'undefined' ? navigator.userAgent : '',
      user: this.config.getUser?.() || undefined,
      tags: this.config.tags,
    };

    // Apply trace context if available
    const traceCtx = this.traceManager.getTraceContext();
    fullEvent.traceId = traceCtx.traceId;
    fullEvent.spanId = traceCtx.spanId;

    // Apply beforeSend hook
    if (this.config.beforeSend) {
      const result = this.config.beforeSend(fullEvent);
      if (result === false) {
        this.log('debug', 'Event dropped by beforeSend hook');
        return;
      }
    }

    this.transport.send([fullEvent]);
  }

  /**
   * Flush pending events
   */
  async flush(): Promise<void> {
    await this.transport.flush();
  }

  /**
   * Destroy the SDK
   */
  destroy(): void {
    this.log('info', 'Destroying SDK');

    // Destroy all plugins
    for (const plugin of this.plugins.values()) {
      if (plugin.destroy) {
        plugin.destroy();
      }
    }
    this.plugins.clear();

    // Destroy transport (will flush)
    this.transport.destroy();

    // Destroy session
    this.sessionManager.destroy();

    this.initialized = false;
  }

  /**
   * Log a message
   */
  log(
    level: 'debug' | 'info' | 'warn' | 'error',
    message: string,
    data?: unknown
  ): void {
    if (!this.config.debug && level === 'debug') return;

    const prefix = '[OllyStack RUM]';
    const logFn = console[level] || console.log;

    if (data !== undefined) {
      logFn(`${prefix} ${message}`, data);
    } else {
      logFn(`${prefix} ${message}`);
    }
  }

  private loadDefaultPlugins(): void {
    // Navigation plugin (always enabled for page views)
    this.use(createNavigationPlugin());

    // Performance plugin
    if (this.config.enablePerformance) {
      this.use(createPerformancePlugin());
    }

    // Error plugin
    if (this.config.enableErrors) {
      this.use(createErrorPlugin());
    }

    // Network plugin
    if (this.config.enableNetwork) {
      this.use(createNetworkPlugin());
    }

    // Interactions plugin
    if (this.config.enableInteractions) {
      this.use(createInteractionsPlugin());
    }
  }
}

// Global instance holder
let globalInstance: OllyStackRUM | null = null;

/**
 * Initialize the SDK
 */
export function init(config: RUMConfig): OllyStackRUM {
  if (globalInstance) {
    globalInstance.log('warn', 'SDK already initialized, returning existing instance');
    return globalInstance;
  }

  globalInstance = new OllyStackRUM(config);
  globalInstance.init();

  return globalInstance;
}

/**
 * Get the global SDK instance
 */
export function getInstance(): OllyStackRUM | null {
  return globalInstance;
}

/**
 * Set user information
 */
export function setUser(user: UserInfo | null): void {
  globalInstance?.setUser(user);
}

/**
 * Track a custom event
 */
export function trackEvent(name: string, data?: Record<string, unknown>): void {
  globalInstance?.trackEvent(name, data);
}

/**
 * Capture an error
 */
export function captureError(error: Error, context?: Record<string, unknown>): void {
  globalInstance?.captureError(error, context);
}

/**
 * Set a custom tag
 */
export function setTag(key: string, value: string): void {
  globalInstance?.setTag(key, value);
}

/**
 * Get session ID
 */
export function getSessionId(): string | null {
  return globalInstance?.getSessionId() || null;
}

/**
 * Get trace context
 */
export function getTraceContext(): TraceContext | null {
  return globalInstance?.getTraceContext() || null;
}

/**
 * Create a span
 */
export function createSpan(name: string): SpanInterface | null {
  return globalInstance?.createSpan(name) || null;
}

/**
 * Flush pending events
 */
export async function flush(): Promise<void> {
  await globalInstance?.flush();
}

/**
 * Destroy the SDK
 */
export function destroy(): void {
  globalInstance?.destroy();
  globalInstance = null;
}
