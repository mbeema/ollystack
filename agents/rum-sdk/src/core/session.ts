/**
 * Session Management
 *
 * Handles user session tracking with persistence and timeout.
 */

import { SessionInfo } from '../types';

const SESSION_STORAGE_KEY = 'ollystack_rum_session';
const DEFAULT_SESSION_TIMEOUT = 30 * 60 * 1000; // 30 minutes

export class SessionManager {
  private session: SessionInfo | null = null;
  private sessionTimeout: number;
  private activityTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(sessionTimeout: number = DEFAULT_SESSION_TIMEOUT) {
    this.sessionTimeout = sessionTimeout;
    this.loadSession();
    this.setupActivityTracking();
  }

  /**
   * Get current session ID, creating new session if needed
   */
  getSessionId(): string {
    if (!this.session || this.isSessionExpired()) {
      this.createNewSession();
    }
    return this.session!.id;
  }

  /**
   * Get full session info
   */
  getSession(): SessionInfo {
    if (!this.session || this.isSessionExpired()) {
      this.createNewSession();
    }
    return this.session!;
  }

  /**
   * Update user ID for current session
   */
  setUserId(userId: string): void {
    if (this.session) {
      this.session.userId = userId;
      this.saveSession();
    }
  }

  /**
   * Record a page view
   */
  recordPageView(): void {
    if (this.session) {
      this.session.pageViews++;
      this.updateActivity();
      this.saveSession();
    }
  }

  /**
   * Update last activity time
   */
  updateActivity(): void {
    if (this.session) {
      this.session.lastActivity = Date.now();
      this.resetActivityTimer();
    }
  }

  /**
   * Generate new trace ID
   */
  generateTraceId(): string {
    return this.generateId(32);
  }

  /**
   * Generate new span ID
   */
  generateSpanId(): string {
    return this.generateId(16);
  }

  /**
   * Destroy session
   */
  destroy(): void {
    if (this.activityTimer) {
      clearTimeout(this.activityTimer);
    }
    this.session = null;
    try {
      sessionStorage.removeItem(SESSION_STORAGE_KEY);
    } catch {
      // Ignore storage errors
    }
  }

  private loadSession(): void {
    try {
      const stored = sessionStorage.getItem(SESSION_STORAGE_KEY);
      if (stored) {
        const parsed = JSON.parse(stored) as SessionInfo;
        if (!this.isSessionExpired(parsed)) {
          this.session = parsed;
        }
      }
    } catch {
      // Ignore storage errors, create new session
    }
  }

  private saveSession(): void {
    if (!this.session) return;
    try {
      sessionStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(this.session));
    } catch {
      // Ignore storage errors
    }
  }

  private createNewSession(): void {
    const now = Date.now();
    this.session = {
      id: this.generateId(32),
      startTime: now,
      lastActivity: now,
      pageViews: 0,
    };
    this.saveSession();
    this.resetActivityTimer();
  }

  private isSessionExpired(session?: SessionInfo): boolean {
    const s = session || this.session;
    if (!s) return true;
    return Date.now() - s.lastActivity > this.sessionTimeout;
  }

  private setupActivityTracking(): void {
    if (typeof window === 'undefined') return;

    const events = ['click', 'keydown', 'scroll', 'mousemove', 'touchstart'];
    const throttledUpdate = this.throttle(() => this.updateActivity(), 5000);

    events.forEach((event) => {
      window.addEventListener(event, throttledUpdate, { passive: true });
    });

    // Handle visibility change
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') {
        // Check if session expired while tab was hidden
        if (this.isSessionExpired()) {
          this.createNewSession();
        }
      }
    });
  }

  private resetActivityTimer(): void {
    if (this.activityTimer) {
      clearTimeout(this.activityTimer);
    }
    this.activityTimer = setTimeout(() => {
      // Session will be considered expired on next access
    }, this.sessionTimeout);
  }

  private generateId(length: number): string {
    const chars = 'abcdef0123456789';
    let result = '';

    // Use crypto.getRandomValues if available
    if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
      const array = new Uint8Array(length / 2);
      crypto.getRandomValues(array);
      result = Array.from(array, (byte) => byte.toString(16).padStart(2, '0')).join('');
    } else {
      // Fallback to Math.random
      for (let i = 0; i < length; i++) {
        result += chars.charAt(Math.floor(Math.random() * chars.length));
      }
    }

    return result;
  }

  private throttle<T extends (...args: unknown[]) => void>(
    fn: T,
    limit: number
  ): (...args: Parameters<T>) => void {
    let inThrottle = false;
    return (...args: Parameters<T>) => {
      if (!inThrottle) {
        fn(...args);
        inThrottle = true;
        setTimeout(() => (inThrottle = false), limit);
      }
    };
  }
}
