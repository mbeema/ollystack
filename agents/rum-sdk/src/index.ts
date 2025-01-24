/**
 * OllyStack RUM SDK
 *
 * Real User Monitoring SDK for browser and mobile applications.
 *
 * Features:
 * - Core Web Vitals monitoring (LCP, FID, CLS, INP, FCP, TTFB)
 * - JavaScript error tracking with stack traces
 * - Network request monitoring (XHR, Fetch)
 * - User interaction tracking
 * - SPA navigation tracking
 * - Distributed tracing (W3C Trace Context)
 * - Session management
 *
 * @example
 * ```typescript
 * import { init } from '@ollystack/rum-sdk';
 *
 * init({
 *   applicationName: 'my-app',
 *   endpoint: 'https://ollystack.example.com',
 *   apiKey: 'your-api-key',
 *   environment: 'production',
 *   version: '1.0.0',
 * });
 * ```
 */

// Core SDK
export {
  OllyStackRUM,
  init,
  getInstance,
  setUser,
  trackEvent,
  captureError,
  setTag,
  getSessionId,
  getTraceContext,
  createSpan,
  flush,
  destroy,
} from './core/sdk';

// Session management
export { SessionManager } from './core/session';

// Trace context
export { TraceContextManager, Span } from './core/context';

// Transport
export { BeaconTransport, createTransport } from './transport/beacon';

// Plugins
export { PerformancePlugin, createPerformancePlugin } from './plugins/performance';
export { ErrorPlugin, createErrorPlugin } from './plugins/errors';
export { NetworkPlugin, createNetworkPlugin } from './plugins/network';
export { InteractionsPlugin, createInteractionsPlugin } from './plugins/interactions';
export { NavigationPlugin, createNavigationPlugin } from './plugins/navigation';

// Types
export type {
  RUMConfig,
  RUMEvent,
  RUMEventType,
  RUMPlugin,
  RUMSDKInterface,
  UserInfo,
  SessionInfo,
  TraceContext,
  SpanInterface,
  Transport,
  // Performance types
  WebVitals,
  PerformanceData,
  NavigationTiming,
  ResourceTiming,
  LongTask,
  // Error types
  ErrorData,
  // Network types
  NetworkData,
  // Interaction types
  InteractionData,
  // Page view types
  PageViewData,
} from './types';

// Version
export const VERSION = '0.1.0';

// Default export for UMD bundle
import { init, setUser, trackEvent, captureError, setTag, getSessionId, flush, destroy } from './core/sdk';

export default {
  init,
  setUser,
  trackEvent,
  captureError,
  setTag,
  getSessionId,
  flush,
  destroy,
  VERSION: '0.1.0',
};
