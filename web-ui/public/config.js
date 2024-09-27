// Runtime configuration for OllyStack UI
// This file is loaded before the main bundle and sets window.__OLLYSTACK_CONFIG__
// For production, this file should be generated/replaced during deployment

window.__OLLYSTACK_CONFIG__ = {
  // API server URL - use relative path for nginx proxy, or full URL for direct access
  API_URL: "/api",

  // AI Engine URL - use relative path for nginx proxy
  AI_ENGINE_URL: "/ai",

  // OTLP endpoint URL - browser telemetry is sent here (nginx proxies to collector)
  // Leave empty or omit to use same-origin (recommended for nginx proxy setup)
  OTLP_URL: "",

  // Application version (set during build/deploy)
  VERSION: "dev",

  // Environment identifier
  ENVIRONMENT: "development"
};
