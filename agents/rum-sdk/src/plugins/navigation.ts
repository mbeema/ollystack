/**
 * Navigation Plugin
 *
 * Tracks page views and SPA navigation.
 * Supports History API and hash-based routing.
 */

import { RUMPlugin, RUMSDKInterface, PageViewData } from '../types';

export class NavigationPlugin implements RUMPlugin {
  name = 'navigation';

  private sdk: RUMSDKInterface | null = null;
  private currentPath: string = '';
  private previousPath: string = '';
  private originalPushState: typeof history.pushState | null = null;
  private originalReplaceState: typeof history.replaceState | null = null;
  private boundPopStateHandler: ((event: PopStateEvent) => void) | null = null;
  private boundHashChangeHandler: ((event: HashChangeEvent) => void) | null = null;

  init(sdk: RUMSDKInterface): void {
    this.sdk = sdk;
    this.sdk.log('debug', 'Navigation plugin initializing');

    if (typeof window === 'undefined') return;

    this.currentPath = this.getCurrentPath();

    // Track initial page view
    this.trackPageView();

    // Patch History API for SPA navigation
    this.patchHistoryAPI();

    // Handle popstate (back/forward navigation)
    this.boundPopStateHandler = this.handlePopState.bind(this);
    window.addEventListener('popstate', this.boundPopStateHandler);

    // Handle hash changes (for hash-based routing)
    this.boundHashChangeHandler = this.handleHashChange.bind(this);
    window.addEventListener('hashchange', this.boundHashChangeHandler);

    // Handle visibility change (tab switching)
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') {
        // User returned to tab - could track this as engagement metric
        this.sdk?.log('debug', 'Tab became visible');
      }
    });
  }

  destroy(): void {
    // Restore History API
    if (this.originalPushState && typeof history !== 'undefined') {
      history.pushState = this.originalPushState;
    }
    if (this.originalReplaceState && typeof history !== 'undefined') {
      history.replaceState = this.originalReplaceState;
    }

    // Remove event listeners
    if (this.boundPopStateHandler) {
      window.removeEventListener('popstate', this.boundPopStateHandler);
    }
    if (this.boundHashChangeHandler) {
      window.removeEventListener('hashchange', this.boundHashChangeHandler);
    }
  }

  /**
   * Manually track a page view (useful for custom SPA navigation)
   */
  trackPageView(customData?: Partial<PageViewData>): void {
    const path = customData?.path || this.getCurrentPath();
    const url = customData?.url || window.location.href;
    const title = customData?.title || document.title;

    const pageViewData: PageViewData = {
      url,
      path,
      title,
      referrer: document.referrer,
      previousPage: this.previousPath || undefined,
      ...customData,
    };

    this.sdk?.sendEvent({
      type: 'page_view',
      data: pageViewData,
    });

    this.sdk?.log('debug', `Page view: ${path}`);

    // Update tracking
    this.previousPath = this.currentPath;
    this.currentPath = path;
  }

  private patchHistoryAPI(): void {
    if (typeof history === 'undefined') return;

    const plugin = this;

    // Patch pushState
    this.originalPushState = history.pushState;
    history.pushState = function (
      data: unknown,
      unused: string,
      url?: string | URL | null
    ): void {
      plugin.originalPushState!.call(this, data, unused, url);
      plugin.handleNavigation('pushState');
    };

    // Patch replaceState
    this.originalReplaceState = history.replaceState;
    history.replaceState = function (
      data: unknown,
      unused: string,
      url?: string | URL | null
    ): void {
      plugin.originalReplaceState!.call(this, data, unused, url);
      plugin.handleNavigation('replaceState');
    };
  }

  private handleNavigation(source: string): void {
    const newPath = this.getCurrentPath();

    // Only track if path actually changed
    if (newPath !== this.currentPath) {
      this.sdk?.log('debug', `Navigation (${source}): ${this.currentPath} -> ${newPath}`);
      this.trackPageView();
    }
  }

  private handlePopState(_event: PopStateEvent): void {
    const newPath = this.getCurrentPath();

    if (newPath !== this.currentPath) {
      this.sdk?.log('debug', `Navigation (popstate): ${this.currentPath} -> ${newPath}`);
      this.trackPageView();
    }
  }

  private handleHashChange(_event: HashChangeEvent): void {
    const newPath = this.getCurrentPath();

    if (newPath !== this.currentPath) {
      this.sdk?.log('debug', `Navigation (hashchange): ${this.currentPath} -> ${newPath}`);
      this.trackPageView();
    }
  }

  private getCurrentPath(): string {
    // Include hash for hash-based routing
    const path = window.location.pathname;
    const hash = window.location.hash;

    // For hash-based routing (e.g., /#/dashboard), use hash as path
    if (hash.startsWith('#/')) {
      return hash.slice(1);
    }

    return path + (hash || '');
  }
}

export function createNavigationPlugin(): RUMPlugin {
  return new NavigationPlugin();
}
