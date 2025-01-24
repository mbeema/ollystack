/**
 * Transport Layer
 *
 * Handles batching and sending telemetry data to the backend.
 * Uses sendBeacon for reliable delivery, with fetch fallback.
 */

import { RUMEvent, Transport, QueueItem, RUMConfig } from '../types';

const MAX_RETRIES = 3;
const RETRY_DELAY = 1000;

export class BeaconTransport implements Transport {
  private queue: QueueItem[] = [];
  private config: RUMConfig;
  private flushTimer: ReturnType<typeof setTimeout> | null = null;
  private isFlushing = false;

  constructor(config: RUMConfig) {
    this.config = config;
    this.setupFlushInterval();
    this.setupUnloadHandler();
  }

  /**
   * Queue an event for sending
   */
  async send(events: RUMEvent[]): Promise<void> {
    events.forEach((event) => {
      this.queue.push({ event, retries: 0 });
    });

    // Flush if batch size reached
    if (this.queue.length >= (this.config.batchSize || 10)) {
      await this.flush();
    }
  }

  /**
   * Flush all queued events
   */
  async flush(): Promise<void> {
    if (this.isFlushing || this.queue.length === 0) {
      return;
    }

    this.isFlushing = true;

    try {
      // Take items from queue
      const items = this.queue.splice(0, this.config.batchSize || 10);
      const events = items.map((item) => item.event);

      const success = await this.sendBatch(events);

      if (!success) {
        // Re-queue failed items for retry
        items.forEach((item) => {
          if (item.retries < MAX_RETRIES) {
            item.retries++;
            this.queue.unshift(item);
          } else {
            this.log('warn', `Dropping event after ${MAX_RETRIES} retries`);
          }
        });
      }
    } finally {
      this.isFlushing = false;
    }
  }

  /**
   * Get queue length
   */
  getQueueLength(): number {
    return this.queue.length;
  }

  /**
   * Destroy transport
   */
  destroy(): void {
    if (this.flushTimer) {
      clearInterval(this.flushTimer);
    }
    // Final flush
    this.flushSync();
  }

  private setupFlushInterval(): void {
    const interval = this.config.flushInterval || 5000;
    this.flushTimer = setInterval(() => {
      this.flush().catch((err) => {
        this.log('error', 'Flush error', err);
      });
    }, interval);
  }

  private setupUnloadHandler(): void {
    if (typeof window === 'undefined') return;

    // Use visibilitychange for more reliable unload detection
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'hidden') {
        this.flushSync();
      }
    });

    // Also handle pagehide for Safari
    window.addEventListener('pagehide', () => {
      this.flushSync();
    });

    // Fallback to beforeunload
    window.addEventListener('beforeunload', () => {
      this.flushSync();
    });
  }

  private async sendBatch(events: RUMEvent[]): Promise<boolean> {
    if (events.length === 0) return true;

    const payload = JSON.stringify({
      events,
      metadata: {
        sdkVersion: '0.1.0',
        sdkName: '@ollystack/rum-sdk',
      },
    });

    const url = this.buildUrl();
    const headers = this.buildHeaders();

    // Try sendBeacon first (most reliable for unload)
    if (this.useSendBeacon(events)) {
      const blob = new Blob([payload], { type: 'application/json' });
      const success = navigator.sendBeacon(url, blob);
      if (success) return true;
    }

    // Fallback to fetch
    try {
      const response = await fetch(url, {
        method: 'POST',
        headers,
        body: payload,
        keepalive: true, // Allows request to outlive page
      });

      return response.ok;
    } catch (error) {
      this.log('error', 'Failed to send events', error);
      return false;
    }
  }

  private flushSync(): void {
    if (this.queue.length === 0) return;

    const events = this.queue.map((item) => item.event);
    this.queue = [];

    const payload = JSON.stringify({
      events,
      metadata: {
        sdkVersion: '0.1.0',
        sdkName: '@ollystack/rum-sdk',
      },
    });

    const url = this.buildUrl();

    // Use sendBeacon for synchronous send
    if (typeof navigator !== 'undefined' && navigator.sendBeacon) {
      const blob = new Blob([payload], { type: 'application/json' });
      navigator.sendBeacon(url, blob);
    }
  }

  private useSendBeacon(events: RUMEvent[]): boolean {
    // sendBeacon has a 64KB limit in most browsers
    const payloadSize = JSON.stringify(events).length;
    return (
      typeof navigator !== 'undefined' &&
      navigator.sendBeacon &&
      payloadSize < 60000
    );
  }

  private buildUrl(): string {
    const base = this.config.endpoint.replace(/\/$/, '');
    return `${base}/v1/rum/events`;
  }

  private buildHeaders(): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    if (this.config.apiKey) {
      headers['Authorization'] = `Bearer ${this.config.apiKey}`;
    }

    return headers;
  }

  private log(level: 'debug' | 'info' | 'warn' | 'error', message: string, data?: unknown): void {
    if (!this.config.debug && level === 'debug') return;

    const prefix = '[OllyStack RUM]';
    const logFn = console[level] || console.log;

    if (data) {
      logFn(`${prefix} ${message}`, data);
    } else {
      logFn(`${prefix} ${message}`);
    }
  }
}

/**
 * Create a transport instance
 */
export function createTransport(config: RUMConfig): Transport {
  return new BeaconTransport(config);
}
