/**
 * Performance Plugin
 *
 * Monitors Core Web Vitals and navigation timing.
 * Uses PerformanceObserver for accurate metrics.
 */

import {
  RUMPlugin,
  RUMSDKInterface,
  WebVitals,
  NavigationTiming,
  ResourceTiming,
  LongTask,
} from '../types';

export class PerformancePlugin implements RUMPlugin {
  name = 'performance';

  private sdk: RUMSDKInterface | null = null;
  private observers: PerformanceObserver[] = [];
  private webVitals: WebVitals = {};
  private reported = false;

  init(sdk: RUMSDKInterface): void {
    this.sdk = sdk;
    this.sdk.log('debug', 'Performance plugin initializing');

    if (typeof window === 'undefined' || !window.PerformanceObserver) {
      this.sdk.log('warn', 'PerformanceObserver not available');
      return;
    }

    this.observeNavigationTiming();
    this.observeWebVitals();
    this.observeLongTasks();
    this.observeResources();

    // Report on page hide
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'hidden') {
        this.reportMetrics();
      }
    });
  }

  destroy(): void {
    this.observers.forEach((observer) => observer.disconnect());
    this.observers = [];
    this.reportMetrics();
  }

  private observeNavigationTiming(): void {
    // Wait for page to be fully loaded
    if (document.readyState === 'complete') {
      this.collectNavigationTiming();
    } else {
      window.addEventListener('load', () => {
        // Defer to ensure all timing data is available
        setTimeout(() => this.collectNavigationTiming(), 0);
      });
    }
  }

  private collectNavigationTiming(): void {
    const entries = performance.getEntriesByType('navigation') as PerformanceNavigationTiming[];
    if (entries.length === 0) return;

    const nav = entries[0];
    const timing: NavigationTiming = {
      loadTime: nav.loadEventEnd - nav.startTime,
      domContentLoaded: nav.domContentLoadedEventEnd - nav.startTime,
      domInteractive: nav.domInteractive - nav.startTime,
      dnsTime: nav.domainLookupEnd - nav.domainLookupStart,
      connectTime: nav.connectEnd - nav.connectStart,
      tlsTime: nav.secureConnectionStart > 0 ? nav.connectEnd - nav.secureConnectionStart : 0,
      ttfb: nav.responseStart - nav.requestStart,
      responseTime: nav.responseEnd - nav.responseStart,
      domProcessingTime: nav.domComplete - nav.domInteractive,
      redirectTime: nav.redirectEnd - nav.redirectStart,
    };

    this.webVitals.ttfb = timing.ttfb;

    this.sdk?.sendEvent({
      type: 'performance',
      data: {
        navigation: timing,
        type: 'navigation',
      },
    });
  }

  private observeWebVitals(): void {
    // Largest Contentful Paint (LCP)
    this.createObserver('largest-contentful-paint', (entries) => {
      const lastEntry = entries[entries.length - 1] as PerformanceLCPEntry;
      this.webVitals.lcp = lastEntry.startTime;
      this.sdk?.log('debug', `LCP: ${lastEntry.startTime.toFixed(2)}ms`);
    });

    // First Input Delay (FID)
    this.createObserver('first-input', (entries) => {
      const entry = entries[0] as PerformanceEventTiming;
      this.webVitals.fid = entry.processingStart - entry.startTime;
      this.sdk?.log('debug', `FID: ${this.webVitals.fid.toFixed(2)}ms`);
    });

    // Cumulative Layout Shift (CLS)
    let clsValue = 0;
    let clsEntries: PerformanceEntry[] = [];
    let sessionValue = 0;
    let sessionEntries: PerformanceEntry[] = [];

    this.createObserver('layout-shift', (entries) => {
      for (const entry of entries as LayoutShiftEntry[]) {
        if (!entry.hadRecentInput) {
          const firstEntry = sessionEntries[0];
          const lastEntry = sessionEntries[sessionEntries.length - 1];

          // Start new session if gap > 1s or session > 5s
          if (
            sessionValue > 0 &&
            (entry.startTime - lastEntry.startTime > 1000 ||
              entry.startTime - firstEntry.startTime > 5000)
          ) {
            if (sessionValue > clsValue) {
              clsValue = sessionValue;
              clsEntries = [...sessionEntries];
            }
            sessionValue = 0;
            sessionEntries = [];
          }

          sessionValue += entry.value;
          sessionEntries.push(entry);
        }
      }

      // Update CLS if current session is largest
      if (sessionValue > clsValue) {
        clsValue = sessionValue;
        clsEntries = [...sessionEntries];
      }

      this.webVitals.cls = clsValue;
      this.sdk?.log('debug', `CLS: ${clsValue.toFixed(4)}`);
    });

    // First Contentful Paint (FCP)
    this.createObserver('paint', (entries) => {
      for (const entry of entries) {
        if (entry.name === 'first-contentful-paint') {
          this.webVitals.fcp = entry.startTime;
          this.sdk?.log('debug', `FCP: ${entry.startTime.toFixed(2)}ms`);
        }
      }
    });

    // Interaction to Next Paint (INP)
    const interactionMap = new Map<number, number>();

    this.createObserver('event', (entries) => {
      for (const entry of entries as PerformanceEventTiming[]) {
        // Only count discrete events
        if (
          entry.interactionId &&
          ['pointerdown', 'pointerup', 'keydown', 'keyup', 'click'].includes(entry.name)
        ) {
          const duration = entry.duration;
          const existing = interactionMap.get(entry.interactionId) || 0;
          interactionMap.set(entry.interactionId, Math.max(existing, duration));

          // Calculate INP as 98th percentile of interactions
          const values = Array.from(interactionMap.values()).sort((a, b) => b - a);
          const index = Math.floor(values.length * 0.02);
          this.webVitals.inp = values[index] || values[0];

          this.sdk?.log('debug', `INP: ${this.webVitals.inp?.toFixed(2)}ms`);
        }
      }
    }, { durationThreshold: 40 });
  }

  private observeLongTasks(): void {
    this.createObserver('longtask', (entries) => {
      for (const entry of entries as PerformanceLongTaskTiming[]) {
        const longTask: LongTask = {
          name: entry.name,
          duration: entry.duration,
          startTime: entry.startTime,
          attribution: entry.attribution?.map((a) => a.containerType || 'unknown') || [],
        };

        this.sdk?.sendEvent({
          type: 'long_task',
          data: longTask,
        });

        if (entry.duration > 100) {
          this.sdk?.log(
            'debug',
            `Long task detected: ${entry.duration.toFixed(2)}ms`,
            longTask
          );
        }
      }
    });
  }

  private observeResources(): void {
    this.createObserver('resource', (entries) => {
      const resources: ResourceTiming[] = [];

      for (const entry of entries as PerformanceResourceTiming[]) {
        // Skip RUM SDK's own requests
        if (entry.name.includes('/v1/rum/')) continue;

        resources.push({
          name: entry.name,
          type: entry.initiatorType,
          duration: entry.duration,
          transferSize: entry.transferSize,
          encodedBodySize: entry.encodedBodySize,
          decodedBodySize: entry.decodedBodySize,
          startTime: entry.startTime,
        });
      }

      if (resources.length > 0) {
        this.sdk?.sendEvent({
          type: 'resource',
          data: { resources },
        });
      }
    });
  }

  private createObserver(
    type: string,
    callback: (entries: PerformanceEntry[]) => void,
    options?: PerformanceObserverInit
  ): void {
    try {
      const observer = new PerformanceObserver((list) => {
        callback(list.getEntries());
      });

      const observerOptions: PerformanceObserverInit = {
        type,
        buffered: true,
        ...options,
      };

      observer.observe(observerOptions);
      this.observers.push(observer);
    } catch (error) {
      this.sdk?.log('debug', `PerformanceObserver for ${type} not supported`);
    }
  }

  private reportMetrics(): void {
    if (this.reported) return;
    this.reported = true;

    // Only report if we have meaningful data
    if (Object.keys(this.webVitals).length === 0) return;

    this.sdk?.sendEvent({
      type: 'performance',
      data: {
        webVitals: this.webVitals,
        type: 'web-vitals',
      },
    });

    this.sdk?.log('debug', 'Web Vitals reported', this.webVitals);
  }
}

// Type augmentations for Web Vitals APIs

interface PerformanceLCPEntry extends PerformanceEntry {
  element?: Element;
  renderTime: number;
  loadTime: number;
  size: number;
  id: string;
  url: string;
}

interface PerformanceEventTiming extends PerformanceEntry {
  processingStart: number;
  processingEnd: number;
  interactionId?: number;
  cancelable: boolean;
}

interface LayoutShiftEntry extends PerformanceEntry {
  value: number;
  hadRecentInput: boolean;
  lastInputTime: number;
  sources: LayoutShiftAttribution[];
}

interface LayoutShiftAttribution {
  node?: Node;
  previousRect: DOMRect;
  currentRect: DOMRect;
}

interface PerformanceLongTaskTiming extends PerformanceEntry {
  attribution: TaskAttributionTiming[];
}

interface TaskAttributionTiming extends PerformanceEntry {
  containerType: string;
  containerSrc: string;
  containerId: string;
  containerName: string;
}

export function createPerformancePlugin(): RUMPlugin {
  return new PerformancePlugin();
}
