#!/bin/sh
set -e

# Runtime configuration injection
# This allows overriding API_URL at container startup

CONFIG_FILE="/usr/share/nginx/html/config.js"

# Create runtime config with environment variables
cat > "$CONFIG_FILE" << EOF
window.__OLLYSTACK_CONFIG__ = {
  API_URL: "${API_URL:-/api}",
  AI_ENGINE_URL: "${AI_ENGINE_URL:-/ai}",
  OTLP_URL: "${OTLP_URL:-}",
  VERSION: "${VERSION:-unknown}",
  ENVIRONMENT: "${ENVIRONMENT:-production}"
};
EOF

echo "Runtime config written to $CONFIG_FILE"
cat "$CONFIG_FILE"

# Update nginx config if API_URL is set to external URL
if [ -n "$API_URL" ] && [ "$API_URL" != "/api" ]; then
  echo "External API URL configured: $API_URL"
fi

# Execute the main command
exec "$@"
