/**
 * Error Tracking Plugin
 *
 * Captures JavaScript errors, unhandled promise rejections,
 * and console errors with stack traces.
 */

import { RUMPlugin, RUMSDKInterface, ErrorData } from '../types';

export class ErrorPlugin implements RUMPlugin {
  name = 'errors';

  private sdk: RUMSDKInterface | null = null;
  private originalOnError: OnErrorEventHandler | null = null;
  private originalOnUnhandledRejection: ((ev: PromiseRejectionEvent) => void) | null = null;
  private originalConsoleError: typeof console.error | null = null;
  private reportedErrors: Set<string> = new Set();

  init(sdk: RUMSDKInterface): void {
    this.sdk = sdk;
    this.sdk.log('debug', 'Error plugin initializing');

    if (typeof window === 'undefined') return;

    this.setupErrorHandler();
    this.setupUnhandledRejectionHandler();
    this.setupConsoleErrorHandler();
  }

  destroy(): void {
    if (typeof window === 'undefined') return;

    // Restore original handlers
    if (this.originalOnError !== null) {
      window.onerror = this.originalOnError;
    }

    if (this.originalOnUnhandledRejection !== null) {
      window.removeEventListener('unhandledrejection', this.handleUnhandledRejection);
    }

    if (this.originalConsoleError !== null) {
      console.error = this.originalConsoleError;
    }
  }

  /**
   * Manually capture an error
   */
  captureError(error: Error, context?: Record<string, unknown>): void {
    this.reportError({
      message: error.message,
      stack: error.stack,
      type: error.name || 'Error',
      handled: true,
      context,
    });
  }

  /**
   * Capture a message as an error
   */
  captureMessage(message: string, level: 'error' | 'warning' = 'error', context?: Record<string, unknown>): void {
    this.reportError({
      message,
      type: level === 'error' ? 'Error' : 'Warning',
      handled: true,
      context,
    });
  }

  private setupErrorHandler(): void {
    this.originalOnError = window.onerror;

    window.onerror = (
      message: string | Event,
      filename?: string,
      lineno?: number,
      colno?: number,
      error?: Error
    ) => {
      this.handleError(message, filename, lineno, colno, error);

      // Call original handler
      if (this.originalOnError) {
        return this.originalOnError.call(window, message, filename, lineno, colno, error);
      }
      return false;
    };
  }

  private setupUnhandledRejectionHandler(): void {
    this.handleUnhandledRejection = this.handleUnhandledRejection.bind(this);
    window.addEventListener('unhandledrejection', this.handleUnhandledRejection);
  }

  private setupConsoleErrorHandler(): void {
    this.originalConsoleError = console.error;

    console.error = (...args: unknown[]) => {
      this.handleConsoleError(args);

      // Call original console.error
      if (this.originalConsoleError) {
        this.originalConsoleError.apply(console, args);
      }
    };
  }

  private handleError(
    message: string | Event,
    filename?: string,
    lineno?: number,
    colno?: number,
    error?: Error
  ): void {
    const msgString = typeof message === 'string' ? message : message.type;

    // Skip if this is an ignored error
    if (this.shouldIgnoreError(msgString)) return;

    const errorData: ErrorData = {
      message: msgString,
      stack: error?.stack || this.generateStackTrace(),
      type: error?.name || 'Error',
      filename,
      lineno,
      colno,
      handled: false,
    };

    this.reportError(errorData);
  }

  private handleUnhandledRejection(event: PromiseRejectionEvent): void {
    const reason = event.reason;

    let message: string;
    let stack: string | undefined;
    let type = 'UnhandledPromiseRejection';

    if (reason instanceof Error) {
      message = reason.message;
      stack = reason.stack;
      type = reason.name || type;
    } else if (typeof reason === 'string') {
      message = reason;
    } else {
      try {
        message = JSON.stringify(reason);
      } catch {
        message = 'Unknown promise rejection';
      }
    }

    if (this.shouldIgnoreError(message)) return;

    this.reportError({
      message,
      stack,
      type,
      handled: false,
      context: {
        unhandledRejection: true,
      },
    });
  }

  private handleConsoleError(args: unknown[]): void {
    // Don't capture console errors that look like they're from the SDK
    const firstArg = args[0];
    if (typeof firstArg === 'string' && firstArg.includes('[OllyStack')) return;

    const message = args.map((arg) => {
      if (arg instanceof Error) {
        return arg.message;
      }
      if (typeof arg === 'object') {
        try {
          return JSON.stringify(arg);
        } catch {
          return String(arg);
        }
      }
      return String(arg);
    }).join(' ');

    if (this.shouldIgnoreError(message)) return;

    // Check if any argument is an Error
    const error = args.find((arg) => arg instanceof Error) as Error | undefined;

    this.reportError({
      message,
      stack: error?.stack,
      type: 'ConsoleError',
      handled: true,
      context: {
        source: 'console.error',
      },
    });
  }

  private reportError(errorData: ErrorData): void {
    // Deduplicate errors
    const key = `${errorData.type}:${errorData.message}:${errorData.filename}:${errorData.lineno}`;
    if (this.reportedErrors.has(key)) return;
    this.reportedErrors.add(key);

    // Clear old errors from set (keep last 100)
    if (this.reportedErrors.size > 100) {
      const entries = Array.from(this.reportedErrors);
      entries.slice(0, 50).forEach((e) => this.reportedErrors.delete(e));
    }

    // Parse stack trace
    const parsedStack = this.parseStackTrace(errorData.stack);

    this.sdk?.sendEvent({
      type: 'error',
      data: {
        ...errorData,
        parsedStack,
      },
    });

    this.sdk?.log('debug', `Error captured: ${errorData.message}`);
  }

  private shouldIgnoreError(message: string): boolean {
    const ignorePatterns = this.sdk?.config.ignoreErrors || [];

    for (const pattern of ignorePatterns) {
      if (typeof pattern === 'string') {
        if (message.includes(pattern)) return true;
      } else if (pattern instanceof RegExp) {
        if (pattern.test(message)) return true;
      }
    }

    // Also ignore common browser extension errors
    const extensionPatterns = [
      /^Script error\.?$/,
      /chrome-extension:\/\//,
      /moz-extension:\/\//,
      /safari-extension:\/\//,
      /webkit-masked-url/,
    ];

    for (const pattern of extensionPatterns) {
      if (pattern.test(message)) return true;
    }

    return false;
  }

  private parseStackTrace(stack?: string): StackFrame[] {
    if (!stack) return [];

    const frames: StackFrame[] = [];
    const lines = stack.split('\n');

    for (const line of lines) {
      const frame = this.parseStackFrame(line);
      if (frame) {
        frames.push(frame);
      }
    }

    return frames;
  }

  private parseStackFrame(line: string): StackFrame | null {
    // Chrome/Edge format: "    at functionName (file.js:line:col)"
    const chromeMatch = line.match(/^\s*at\s+(?:(.+?)\s+\()?(.*?):(\d+):(\d+)\)?$/);
    if (chromeMatch) {
      return {
        function: chromeMatch[1] || '(anonymous)',
        filename: chromeMatch[2],
        lineno: parseInt(chromeMatch[3], 10),
        colno: parseInt(chromeMatch[4], 10),
      };
    }

    // Firefox format: "functionName@file.js:line:col"
    const firefoxMatch = line.match(/^(.*)@(.*):(\d+):(\d+)$/);
    if (firefoxMatch) {
      return {
        function: firefoxMatch[1] || '(anonymous)',
        filename: firefoxMatch[2],
        lineno: parseInt(firefoxMatch[3], 10),
        colno: parseInt(firefoxMatch[4], 10),
      };
    }

    return null;
  }

  private generateStackTrace(): string {
    try {
      throw new Error('Stack trace');
    } catch (e) {
      if (e instanceof Error && e.stack) {
        // Remove the first few lines (this function and the caller)
        const lines = e.stack.split('\n');
        return lines.slice(3).join('\n');
      }
    }
    return '';
  }
}

interface StackFrame {
  function: string;
  filename: string;
  lineno: number;
  colno: number;
}

export function createErrorPlugin(): RUMPlugin {
  return new ErrorPlugin();
}
