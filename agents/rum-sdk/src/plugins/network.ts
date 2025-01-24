/**
 * Network Plugin
 *
 * Monitors XHR and Fetch requests with automatic trace context propagation.
 * Captures request/response details, timing, and errors.
 */

import { RUMPlugin, RUMSDKInterface, NetworkData } from '../types';

export class NetworkPlugin implements RUMPlugin {
  name = 'network';

  private sdk: RUMSDKInterface | null = null;
  private originalXHROpen: typeof XMLHttpRequest.prototype.open | null = null;
  private originalXHRSend: typeof XMLHttpRequest.prototype.send | null = null;
  private originalFetch: typeof fetch | null = null;

  init(sdk: RUMSDKInterface): void {
    this.sdk = sdk;
    this.sdk.log('debug', 'Network plugin initializing');

    if (typeof window === 'undefined') return;

    this.patchXHR();
    this.patchFetch();
  }

  destroy(): void {
    // Restore original XHR methods
    if (this.originalXHROpen) {
      XMLHttpRequest.prototype.open = this.originalXHROpen;
    }
    if (this.originalXHRSend) {
      XMLHttpRequest.prototype.send = this.originalXHRSend;
    }

    // Restore original fetch
    if (this.originalFetch && typeof window !== 'undefined') {
      window.fetch = this.originalFetch;
    }
  }

  private patchXHR(): void {
    const plugin = this;
    const sdk = this.sdk!;

    this.originalXHROpen = XMLHttpRequest.prototype.open;
    this.originalXHRSend = XMLHttpRequest.prototype.send;

    XMLHttpRequest.prototype.open = function (
      method: string,
      url: string | URL,
      async: boolean = true,
      username?: string | null,
      password?: string | null
    ) {
      const xhr = this as XMLHttpRequestWithMeta;
      xhr._ollystack = {
        method,
        url: String(url),
        startTime: 0,
        span: null,
      };

      return plugin.originalXHROpen!.call(this, method, url, async, username, password);
    };

    XMLHttpRequest.prototype.send = function (body?: Document | XMLHttpRequestBodyInit | null) {
      const xhr = this as XMLHttpRequestWithMeta;

      if (!xhr._ollystack || plugin.shouldIgnoreUrl(xhr._ollystack.url)) {
        return plugin.originalXHRSend!.call(this, body);
      }

      // Create span for this request
      const span = sdk.createSpan(`HTTP ${xhr._ollystack.method}`);
      xhr._ollystack.span = span;
      xhr._ollystack.startTime = performance.now();

      // Inject trace headers if allowed
      if (plugin.shouldPropagateTrace(xhr._ollystack.url)) {
        const context = sdk.getTraceContext();
        const headers = plugin.getTraceHeaders(context);
        for (const [name, value] of Object.entries(headers)) {
          try {
            this.setRequestHeader(name, value);
          } catch {
            // Header might already be set
          }
        }
      }

      // Track request size
      if (body) {
        xhr._ollystack.requestSize = plugin.getBodySize(body);
      }

      // Listen for completion
      this.addEventListener('loadend', function () {
        plugin.handleXHRComplete(xhr);
      });

      this.addEventListener('error', function () {
        plugin.handleXHRError(xhr, 'Network error');
      });

      this.addEventListener('timeout', function () {
        plugin.handleXHRError(xhr, 'Request timeout');
      });

      this.addEventListener('abort', function () {
        plugin.handleXHRError(xhr, 'Request aborted');
      });

      return plugin.originalXHRSend!.call(this, body);
    };
  }

  private handleXHRComplete(xhr: XMLHttpRequestWithMeta): void {
    if (!xhr._ollystack) return;

    const duration = performance.now() - xhr._ollystack.startTime;
    const span = xhr._ollystack.span;

    span?.setAttribute('http.method', xhr._ollystack.method);
    span?.setAttribute('http.url', xhr._ollystack.url);
    span?.setAttribute('http.status_code', xhr.status);
    span?.setStatus(xhr.status >= 400 ? 'error' : 'ok');
    span?.end();

    const networkData: NetworkData = {
      method: xhr._ollystack.method,
      url: xhr._ollystack.url,
      status: xhr.status,
      statusText: xhr.statusText,
      duration,
      requestSize: xhr._ollystack.requestSize,
      responseSize: this.getResponseSize(xhr),
      type: 'xhr',
      traceId: span?.traceId,
      spanId: span?.spanId,
    };

    this.sdk?.sendEvent({
      type: 'network',
      data: networkData,
    });

    this.sdk?.log('debug', `XHR: ${xhr._ollystack.method} ${xhr._ollystack.url} ${xhr.status} (${duration.toFixed(0)}ms)`);
  }

  private handleXHRError(xhr: XMLHttpRequestWithMeta, error: string): void {
    if (!xhr._ollystack) return;

    const duration = performance.now() - xhr._ollystack.startTime;
    const span = xhr._ollystack.span;

    span?.setStatus('error');
    span?.setAttribute('error', error);
    span?.end();

    const networkData: NetworkData = {
      method: xhr._ollystack.method,
      url: xhr._ollystack.url,
      status: 0,
      duration,
      type: 'xhr',
      error,
      traceId: span?.traceId,
      spanId: span?.spanId,
    };

    this.sdk?.sendEvent({
      type: 'network',
      data: networkData,
    });

    this.sdk?.log('debug', `XHR error: ${xhr._ollystack.method} ${xhr._ollystack.url} - ${error}`);
  }

  private getResponseSize(xhr: XMLHttpRequest): number | undefined {
    try {
      const contentLength = xhr.getResponseHeader('content-length');
      if (contentLength) {
        return parseInt(contentLength, 10);
      }
      if (xhr.responseText) {
        return new Blob([xhr.responseText]).size;
      }
    } catch {
      // Ignore errors
    }
    return undefined;
  }

  private patchFetch(): void {
    if (typeof fetch === 'undefined') return;

    const plugin = this;
    const sdk = this.sdk!;

    this.originalFetch = window.fetch;

    window.fetch = async function (
      input: RequestInfo | URL,
      init?: RequestInit
    ): Promise<Response> {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
      const method = init?.method || (typeof input !== 'string' && !(input instanceof URL) ? input.method : 'GET') || 'GET';

      if (plugin.shouldIgnoreUrl(url)) {
        return plugin.originalFetch!.call(window, input, init);
      }

      const span = sdk.createSpan(`HTTP ${method}`);
      const startTime = performance.now();

      // Inject trace headers if allowed
      let headers = new Headers(init?.headers);
      if (plugin.shouldPropagateTrace(url)) {
        const context = sdk.getTraceContext();
        const traceHeaders = plugin.getTraceHeaders(context);
        for (const [name, value] of Object.entries(traceHeaders)) {
          headers.set(name, value);
        }
      }

      const modifiedInit: RequestInit = {
        ...init,
        headers,
      };

      let requestSize: number | undefined;
      if (init?.body) {
        requestSize = plugin.getBodySize(init.body);
      }

      try {
        const response = await plugin.originalFetch!.call(window, input, modifiedInit);
        const duration = performance.now() - startTime;

        span.setAttribute('http.method', method);
        span.setAttribute('http.url', url);
        span.setAttribute('http.status_code', response.status);
        span.setStatus(response.ok ? 'ok' : 'error');
        span.end();

        const responseSize = parseInt(response.headers.get('content-length') || '0', 10) || undefined;

        const networkData: NetworkData = {
          method,
          url,
          status: response.status,
          statusText: response.statusText,
          duration,
          requestSize,
          responseSize,
          type: 'fetch',
          traceId: span.traceId,
          spanId: span.spanId,
        };

        sdk.sendEvent({
          type: 'network',
          data: networkData,
        });

        sdk.log('debug', `Fetch: ${method} ${url} ${response.status} (${duration.toFixed(0)}ms)`);

        return response;
      } catch (error) {
        const duration = performance.now() - startTime;

        span.setStatus('error');
        span.setAttribute('error', error instanceof Error ? error.message : 'Unknown error');
        span.end();

        const networkData: NetworkData = {
          method,
          url,
          status: 0,
          duration,
          type: 'fetch',
          error: error instanceof Error ? error.message : 'Unknown error',
          traceId: span.traceId,
          spanId: span.spanId,
        };

        sdk.sendEvent({
          type: 'network',
          data: networkData,
        });

        sdk.log('debug', `Fetch error: ${method} ${url} - ${error}`);

        throw error;
      }
    };
  }

  private shouldIgnoreUrl(url: string): boolean {
    // Ignore RUM SDK's own requests
    if (url.includes('/v1/rum/')) return true;

    const ignorePatterns = this.sdk?.config.ignoreUrls || [];

    for (const pattern of ignorePatterns) {
      if (typeof pattern === 'string') {
        if (url.includes(pattern)) return true;
      } else if (pattern instanceof RegExp) {
        if (pattern.test(url)) return true;
      }
    }

    return false;
  }

  private shouldPropagateTrace(url: string): boolean {
    const propagatePatterns = this.sdk?.config.propagateTraceHeaderCorsUrls || [];

    // If no patterns specified, propagate to same-origin only
    if (propagatePatterns.length === 0) {
      try {
        const urlObj = new URL(url, window.location.href);
        return urlObj.origin === window.location.origin;
      } catch {
        return false;
      }
    }

    for (const pattern of propagatePatterns) {
      if (typeof pattern === 'string') {
        if (url.includes(pattern)) return true;
      } else if (pattern instanceof RegExp) {
        if (pattern.test(url)) return true;
      }
    }

    return false;
  }

  private getTraceHeaders(context: { traceId: string; spanId: string; sampled?: boolean }): Record<string, string> {
    const flags = context.sampled !== false ? '01' : '00';
    return {
      'traceparent': `00-${context.traceId}-${context.spanId}-${flags}`,
      'tracestate': `ollystack=${context.spanId}`,
    };
  }

  private getBodySize(body: BodyInit | Document | XMLHttpRequestBodyInit): number | undefined {
    try {
      if (typeof body === 'string') {
        return new Blob([body]).size;
      }
      if (body instanceof Blob) {
        return body.size;
      }
      if (body instanceof ArrayBuffer) {
        return body.byteLength;
      }
      if (body instanceof FormData) {
        // Can't easily calculate FormData size
        return undefined;
      }
      if (body instanceof URLSearchParams) {
        return new Blob([body.toString()]).size;
      }
    } catch {
      // Ignore errors
    }
    return undefined;
  }
}

interface XMLHttpRequestWithMeta extends XMLHttpRequest {
  _ollystack?: {
    method: string;
    url: string;
    startTime: number;
    requestSize?: number;
    span: { traceId: string; spanId: string; setAttribute: (k: string, v: unknown) => void; setStatus: (s: string) => void; end: () => void } | null;
  };
}

export function createNetworkPlugin(): RUMPlugin {
  return new NetworkPlugin();
}
