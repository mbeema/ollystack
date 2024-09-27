// Runtime configuration helper
// Reads from window.__OLLYSTACK_CONFIG__ injected by docker-entrypoint.sh

declare global {
  interface Window {
    __OLLYSTACK_CONFIG__?: {
      API_URL: string;
      AI_ENGINE_URL: string;
      OPAMP_URL: string;
      VERSION: string;
      ENVIRONMENT: string;
    };
  }
}

export function getApiUrl(): string {
  // Check runtime config first (set by docker-entrypoint.sh)
  if (typeof window !== 'undefined' && window.__OLLYSTACK_CONFIG__?.API_URL) {
    const url = window.__OLLYSTACK_CONFIG__.API_URL;
    // If it's "/api", return empty since code already adds /api
    return url === '/api' ? '' : url;
  }
  // Fallback to build-time env or default (empty for relative paths)
  return import.meta.env.VITE_API_URL || '';
}

export function getAiEngineUrl(): string {
  // Check runtime config first
  if (typeof window !== 'undefined' && window.__OLLYSTACK_CONFIG__?.AI_ENGINE_URL) {
    return window.__OLLYSTACK_CONFIG__.AI_ENGINE_URL;
  }
  // Fallback to build-time env or default
  return import.meta.env.VITE_AI_ENGINE_URL || '/ai';
}

export function getOpampUrl(): string {
  // Check runtime config first
  if (typeof window !== 'undefined' && window.__OLLYSTACK_CONFIG__?.OPAMP_URL) {
    return window.__OLLYSTACK_CONFIG__.OPAMP_URL;
  }
  // Fallback to build-time env or default
  return import.meta.env.VITE_OPAMP_URL || '/opamp';
}

export const API_URL = getApiUrl();
export const AI_ENGINE_URL = getAiEngineUrl();
export const OPAMP_URL = getOpampUrl();
