# OllyStack Web UI - Multi-stage Dockerfile
# Supports both AMD64 and ARM64 (Graviton)

# =============================================================================
# Stage 1: Dependencies
# =============================================================================
FROM --platform=$BUILDPLATFORM node:20-alpine AS deps

WORKDIR /app

# Copy package files
COPY package.json ./

# Install dependencies
RUN npm install

# =============================================================================
# Stage 2: Build
# =============================================================================
FROM --platform=$BUILDPLATFORM node:20-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /app

# Copy dependencies from deps stage
COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Set build-time environment variables
ENV VITE_APP_VERSION=${VERSION}
ENV VITE_APP_COMMIT=${COMMIT}

# Build the application
RUN npm run build

# =============================================================================
# Stage 3: Production
# =============================================================================
FROM nginx:alpine

ARG VERSION=dev

LABEL org.opencontainers.image.title="OllyStack Web UI"
LABEL org.opencontainers.image.description="OllyStack observability platform web interface"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.source="https://github.com/ollystack/ollystack"

# Copy built assets
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy nginx configuration
COPY nginx.conf /etc/nginx/nginx.conf

# Create non-root user for nginx
RUN addgroup -g 1000 ollystack && \
    adduser -u 1000 -G ollystack -s /bin/sh -D ollystack && \
    chown -R ollystack:ollystack /usr/share/nginx/html && \
    chmod -R 644 /usr/share/nginx/html/* && \
    chmod 755 /usr/share/nginx/html /usr/share/nginx/html/assets && \
    chown -R ollystack:ollystack /var/cache/nginx && \
    chown -R ollystack:ollystack /var/log/nginx && \
    touch /var/run/nginx.pid && \
    chown -R ollystack:ollystack /var/run/nginx.pid

# Add runtime config script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Expose port
EXPOSE 80

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:80/health || exit 1

# Run as root for MVP (TODO: fix permissions for production)
# USER ollystack

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
