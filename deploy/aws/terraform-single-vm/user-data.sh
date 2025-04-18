#!/bin/bash
set -e

# =============================================================================
# OllyStack MVP - Single VM Setup Script
# =============================================================================

exec > >(tee /var/log/user-data.log) 2>&1
echo "Starting OllyStack setup at $(date)"

# Variables from Terraform
CLICKHOUSE_PASSWORD="${clickhouse_password}"
JWT_SECRET="${jwt_secret}"
DOMAIN_NAME="${domain_name}"
LETSENCRYPT_EMAIL="${letsencrypt_email}"

# =============================================================================
# Swap Configuration (2GB)
# =============================================================================

echo "Setting up swap..."
SWAP_FILE="/swapfile"
if [ ! -f $SWAP_FILE ]; then
  dd if=/dev/zero of=$SWAP_FILE bs=1M count=2048
  chmod 600 $SWAP_FILE
  mkswap $SWAP_FILE
  swapon $SWAP_FILE
  echo "$SWAP_FILE swap swap defaults 0 0" >> /etc/fstab
fi

# =============================================================================
# System Updates & Dependencies
# =============================================================================

echo "Installing dependencies..."
dnf update -y
dnf install -y docker git

# =============================================================================
# Docker Setup
# =============================================================================

echo "Setting up Docker..."
systemctl enable docker
systemctl start docker
usermod -aG docker ec2-user

# Configure Docker daemon with log rotation to prevent disk filling
mkdir -p /etc/docker
cat > /etc/docker/daemon.json << 'DOCKER_DAEMON_EOF'
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "100m",
    "max-file": "3"
  }
}
DOCKER_DAEMON_EOF

# Restart Docker to apply log rotation settings
systemctl restart docker

# Install Docker Compose
DOCKER_COMPOSE_VERSION="v2.24.0"
curl -L "https://github.com/docker/compose/releases/download/$${DOCKER_COMPOSE_VERSION}/docker-compose-linux-x86_64" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose
ln -sf /usr/local/bin/docker-compose /usr/bin/docker-compose

# =============================================================================
# Mount Data Volume
# =============================================================================

echo "Setting up data volume..."
DATA_DEVICE="/dev/sdf"
DATA_MOUNT="/data"

# Wait for volume to be attached
while [ ! -e $DATA_DEVICE ]; do
  echo "Waiting for data volume..."
  sleep 5
done

# Format if needed (check if already formatted)
if ! blkid $DATA_DEVICE; then
  echo "Formatting data volume..."
  mkfs.xfs $DATA_DEVICE
fi

# Mount
mkdir -p $DATA_MOUNT
if ! grep -q "$DATA_MOUNT" /etc/fstab; then
  echo "$DATA_DEVICE $DATA_MOUNT xfs defaults,nofail 0 2" >> /etc/fstab
fi
mount -a

# Set permissions
chown -R 1000:1000 $DATA_MOUNT

# =============================================================================
# Application Setup
# =============================================================================

APP_DIR="/opt/ollystack"
mkdir -p $APP_DIR
cd $APP_DIR

# Create docker-compose.yml for MVP (using Terraform-injected variables)
cat > docker-compose.yml << COMPOSE_EOF
version: '3.8'

services:
  # ClickHouse - Storage
  clickhouse:
    image: clickhouse/clickhouse-server:24.1
    container_name: ollystack-clickhouse
    restart: unless-stopped
    ports:
      - "8123:8123"
      - "9000:9000"
    volumes:
      - /data/clickhouse:/var/lib/clickhouse
      - ./config/clickhouse:/docker-entrypoint-initdb.d:ro
    environment:
      CLICKHOUSE_DB: ollystack
      CLICKHOUSE_USER: ollystack
      CLICKHOUSE_PASSWORD: $CLICKHOUSE_PASSWORD
    healthcheck:
      test: ["CMD", "clickhouse-client", "--query", "SELECT 1"]
      interval: 10s
      timeout: 5s
      retries: 10
    deploy:
      resources:
        limits:
          memory: 2G

  # Redis - Caching
  redis:
    image: redis:7-alpine
    container_name: ollystack-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    volumes:
      - /data/redis:/data
    command: redis-server --appendonly yes --maxmemory 256mb --maxmemory-policy allkeys-lru
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  # OpenTelemetry Collector - Ingestion
  collector:
    image: otel/opentelemetry-collector-contrib:0.96.0
    container_name: ollystack-collector
    restart: unless-stopped
    user: "0:0"  # Run as root to read Docker container logs
    ports:
      - "4317:4317"
      - "4318:4318"
      - "8888:8888"
    volumes:
      - ./config/otel-collector.yaml:/etc/otelcol-contrib/config.yaml:ro
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
    command: ["--config=/etc/otelcol-contrib/config.yaml"]
    logging:
      driver: "none"  # Prevent collector logs from being re-ingested (feedback loop)
    depends_on:
      clickhouse:
        condition: service_healthy

  # API Server
  api-server:
    image: ollystack/api-server:latest
    container_name: ollystack-api
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - CLICKHOUSE_HOST=clickhouse
      - CLICKHOUSE_PORT=8123
      - CLICKHOUSE_DATABASE=ollystack
      - CLICKHOUSE_USER=ollystack
      - CLICKHOUSE_PASSWORD=$CLICKHOUSE_PASSWORD
      - REDIS_URL=redis://redis:6379
      - JWT_SECRET=$JWT_SECRET
    depends_on:
      clickhouse:
        condition: service_healthy
      redis:
        condition: service_healthy

  # Web UI
  web-ui:
    image: ollystack/web-ui:latest
    container_name: ollystack-ui
    restart: unless-stopped
    ports:
      - "3000:80"
    environment:
      - API_URL=http://api-server:8080
    depends_on:
      - api-server

  # ===========================================
  # CONTROL PLANE - OpAMP Server
  # ===========================================

  # OpAMP Server - Central Configuration Management
  opamp-server:
    image: ollystack/opamp-server:latest
    container_name: ollystack-opamp-server
    restart: unless-stopped
    ports:
      - "4320:4320"
    environment:
      - HTTP_PORT=4320
      - REDIS_URL=redis:6379
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://localhost:4320/api/v1/health"]
      interval: 10s
      timeout: 5s
      retries: 5
    depends_on:
      redis:
        condition: service_healthy

  # ===========================================
  # eBPF INSTRUMENTATION (Optional)
  # ===========================================

  # Grafana Beyla - Zero-code instrumentation for Go/Rust/C++
  # Uncomment to enable eBPF-based auto-instrumentation
  # beyla:
  #   image: grafana/beyla:latest
  #   container_name: ollystack-beyla
  #   restart: unless-stopped
  #   privileged: true
  #   pid: "host"
  #   environment:
  #     - BEYLA_OPEN_PORT=8080-8090
  #     - BEYLA_SERVICE_NAMESPACE=ollystack
  #     - OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4317
  #     - OTEL_EXPORTER_OTLP_PROTOCOL=grpc
  #   volumes:
  #     - /sys/kernel/security:/sys/kernel/security:ro
  #     - /sys/fs/cgroup:/sys/fs/cgroup:ro
  #   depends_on:
  #     - collector

networks:
  default:
    name: ollystack

COMPOSE_EOF

# Create config directories
mkdir -p config/clickhouse

# Create ClickHouse init script
cat > config/clickhouse/init.sql << 'SQL_EOF'
-- OllyStack ClickHouse Schema (Minimal)

CREATE DATABASE IF NOT EXISTS ollystack;

-- Traces table
CREATE TABLE IF NOT EXISTS ollystack.traces
(
    timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1)),
    parent_span_id String CODEC(ZSTD(1)),
    service_name LowCardinality(String) CODEC(ZSTD(1)),
    operation_name String CODEC(ZSTD(1)),
    span_kind LowCardinality(String) CODEC(ZSTD(1)),
    duration_ns UInt64 CODEC(Delta, ZSTD(1)),
    status_code LowCardinality(String) CODEC(ZSTD(1)),
    attributes Map(String, String) CODEC(ZSTD(1)),
    resource_attributes Map(String, String) CODEC(ZSTD(1)),
    events Nested(
        timestamp DateTime64(9),
        name String,
        attributes Map(String, String)
    ) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (service_name, timestamp, trace_id)
TTL toDateTime(timestamp) + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;

-- Metrics table
CREATE TABLE IF NOT EXISTS ollystack.metrics
(
    timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    metric_name LowCardinality(String) CODEC(ZSTD(1)),
    metric_type LowCardinality(String) CODEC(ZSTD(1)),
    value Float64 CODEC(ZSTD(1)),
    attributes Map(String, String) CODEC(ZSTD(1)),
    resource_attributes Map(String, String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (metric_name, timestamp)
TTL toDateTime(timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Logs table
CREATE TABLE IF NOT EXISTS ollystack.logs
(
    timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    severity_text LowCardinality(String) CODEC(ZSTD(1)),
    severity_number UInt8 CODEC(ZSTD(1)),
    service_name LowCardinality(String) CODEC(ZSTD(1)),
    body String CODEC(ZSTD(1)),
    attributes Map(String, String) CODEC(ZSTD(1)),
    resource_attributes Map(String, String) CODEC(ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (service_name, severity_number, timestamp)
TTL toDateTime(timestamp) + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;

SQL_EOF

# Create OpenTelemetry Collector config (using Terraform-injected password)
# Includes filelog receiver for zero-code log instrumentation
cat > config/otel-collector.yaml << OTEL_EOF
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

  # Filelog receiver for Docker container logs (zero-code instrumentation)
  # Note: The collector container uses logging driver "none" to prevent feedback loop
  filelog/docker:
    include:
      - /var/lib/docker/containers/*/*.log
    include_file_path: true
    include_file_name: false
    start_at: end  # Only collect new logs, not historical
    poll_interval: 500ms
    operators:
      # Parse Docker JSON log format first
      - type: json_parser
        id: docker_parser
        timestamp:
          parse_from: attributes.time
          layout: '%Y-%m-%dT%H:%M:%S.%LZ'
        on_error: drop

      # Extract container ID from file path
      - type: regex_parser
        id: container_id_parser
        regex: '/var/lib/docker/containers/(?P<container_id>[a-f0-9]+)/'
        parse_from: attributes["log.file.path"]
        parse_to: attributes
        on_error: send

      # Route based on whether the log body looks like JSON from our services
      - type: router
        id: format_router
        routes:
          # Route JSON logs from our services (contain "service" field)
          - expr: 'attributes.log != nil and attributes.log matches "^\\\\{.*\"service\".*\\\\}"'
            output: service_json_parser
        default: plain_log_handler

      # Parse JSON logs from our instrumented services
      - type: json_parser
        id: service_json_parser
        parse_from: attributes.log
        parse_to: body
        on_error: send
        output: move_trace_id

      # Handle plain text logs (non-JSON)
      - type: add
        id: plain_log_handler
        field: body
        value: EXPR(attributes.log)
        output: noop_end

      # Move parsed fields to attributes for correlation
      - type: move
        id: move_trace_id
        from: body.trace_id
        to: attributes["trace_id"]
        on_error: send
      - type: move
        id: move_span_id
        from: body.span_id
        to: attributes["span_id"]
        on_error: send
      - type: move
        id: move_correlation_id
        from: body.correlation_id
        to: attributes["correlation_id"]
        on_error: send
      - type: move
        id: move_service
        from: body.service
        to: resource["service.name"]
        on_error: send
      - type: move
        id: move_level
        from: body.level
        to: attributes["level"]
        on_error: send
      - type: move
        id: move_message
        from: body.message
        to: body
        on_error: send

      - type: noop
        id: noop_end

processors:
  batch:
    timeout: 5s
    send_batch_size: 10000
  memory_limiter:
    check_interval: 1s
    limit_mib: 512
    spike_limit_mib: 128
  # Filter out logs without service name (system/infra logs)
  filter/logs:
    logs:
      exclude:
        match_type: regexp
        resource_attributes:
          - key: service.name
            value: "^$"

exporters:
  clickhouse:
    endpoint: tcp://clickhouse:9000?dial_timeout=10s&compress=lz4
    database: ollystack
    username: ollystack
    password: $CLICKHOUSE_PASSWORD
    ttl: 168h
    logs_table_name: logs
    traces_table_name: traces
    metrics_table_name: metrics
    timeout: 10s
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 300s

  debug:
    verbosity: basic

service:
  telemetry:
    logs:
      level: warn  # Reduce collector log verbosity
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [clickhouse]
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [clickhouse]
    logs:
      receivers: [otlp, filelog/docker]
      processors: [memory_limiter, filter/logs, batch]
      exporters: [clickhouse]

OTEL_EOF

# Create .env file
cat > .env << ENV_EOF
CLICKHOUSE_PASSWORD=$CLICKHOUSE_PASSWORD
JWT_SECRET=$JWT_SECRET
ENV_EOF

# =============================================================================
# Create systemd service for auto-start
# =============================================================================

cat > /etc/systemd/system/ollystack.service << 'SERVICE_EOF'
[Unit]
Description=OllyStack Observability Stack
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/ollystack
ExecStart=/usr/local/bin/docker-compose up -d
ExecStop=/usr/local/bin/docker-compose down
TimeoutStartSec=300

[Install]
WantedBy=multi-user.target
SERVICE_EOF

systemctl daemon-reload
systemctl enable ollystack.service

# =============================================================================
# Start the stack
# =============================================================================

echo "Starting OllyStack stack..."
cd $APP_DIR

# Pull images first (api-server and web-ui may need to be built or use public images)
# For MVP, we'll use the collector and backing services, API can be added later
docker-compose pull clickhouse redis collector || true

# Start services
docker-compose up -d clickhouse redis
sleep 30  # Wait for ClickHouse to initialize

docker-compose up -d collector

# =============================================================================
# Create API config file
# =============================================================================

mkdir -p $APP_DIR/config/api
cat > $APP_DIR/config/api/config.yaml << 'API_CONFIG_EOF'
server:
  address: ":8080"
  mode: "release"
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s
  cors:
    enabled: true
    allowed_origins:
      - "*"
    allowed_methods:
      - GET
      - POST
      - PUT
      - DELETE
      - OPTIONS
    allowed_headers:
      - Authorization
      - Content-Type
      - X-Correlation-ID

auth:
  enabled: false

storage:
  metrics:
    type: clickhouse
    endpoint: "ollystack-clickhouse:9000"
    database: ollystack
    username: ollystack
    password: "CLICKHOUSE_PASSWORD_PLACEHOLDER"
  logs:
    type: clickhouse
    endpoint: "ollystack-clickhouse:9000"
    database: ollystack
    username: ollystack
    password: "CLICKHOUSE_PASSWORD_PLACEHOLDER"
  traces:
    type: clickhouse
    endpoint: "ollystack-clickhouse:9000"
    database: ollystack
    username: ollystack
    password: "CLICKHOUSE_PASSWORD_PLACEHOLDER"

ai:
  enabled: false

features:
  service_map: true
  anomaly_detection: true
  natural_language: true
API_CONFIG_EOF

# Replace password placeholder
sed -i "s/CLICKHOUSE_PASSWORD_PLACEHOLDER/$CLICKHOUSE_PASSWORD/g" $APP_DIR/config/api/config.yaml

# =============================================================================
# Nginx Reverse Proxy Setup
# =============================================================================

echo "Installing Nginx..."
dnf install -y nginx

# Create nginx configuration for reverse proxy
mkdir -p /etc/nginx/conf.d

# JSON log format with correlation ID for observability
cat > /etc/nginx/conf.d/00-log-format.conf << 'LOGFORMAT_EOF'
# Custom JSON log format with correlation ID (request_id)
log_format json_combined escape=json '{'
    '"time":"$msec",'
    '"time_iso":"$time_iso8601",'
    '"request_id":"$request_id",'
    '"remote_addr":"$remote_addr",'
    '"remote_user":"$remote_user",'
    '"request":"$request",'
    '"status":$status,'
    '"body_bytes_sent":$body_bytes_sent,'
    '"request_time":$request_time,'
    '"http_referrer":"$http_referer",'
    '"http_user_agent":"$http_user_agent",'
    '"http_x_forwarded_for":"$http_x_forwarded_for",'
    '"upstream_response_time":"$upstream_response_time",'
    '"upstream_addr":"$upstream_addr",'
    '"ssl_protocol":"$ssl_protocol",'
    '"ssl_cipher":"$ssl_cipher",'
    '"request_length":$request_length,'
    '"host":"$host",'
    '"uri":"$uri"'
'}';

# Use JSON format for access logs (enables correlation with backend services)
access_log /var/log/nginx/access.json json_combined;
LOGFORMAT_EOF

cat > /etc/nginx/conf.d/ollystack.conf << 'NGINX_EOF'
# OllyStack Reverse Proxy Configuration
# Proxies all services through nginx

upstream api_backend {
    server 127.0.0.1:8080;
    keepalive 32;
}

upstream ai_backend {
    server 127.0.0.1:8090;
    keepalive 32;
}

upstream ui_backend {
    server 127.0.0.1:3000;
    keepalive 32;
}

upstream grafana_backend {
    server 127.0.0.1:3001;
    keepalive 32;
}

upstream otlp_http_backend {
    server 127.0.0.1:4318;
    keepalive 32;
}

# Rate limiting zones
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=100r/s;
limit_req_zone $binary_remote_addr zone=otlp_limit:10m rate=1000r/s;

# HTTP server - redirect to HTTPS (when SSL is configured)
server {
    listen 80;
    server_name _;

    # Let's Encrypt challenge
    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    # Health check
    location /health {
        access_log off;
        return 200 "healthy\n";
        add_header Content-Type text/plain;
    }

    # Redirect to HTTPS when cert exists
    location / {
        if (-f /etc/letsencrypt/live/$host/fullchain.pem) {
            return 301 https://$host$request_uri;
        }

        # Fallback to HTTP proxy if no SSL
        proxy_pass http://ui_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
    }
}
NGINX_EOF

# Create SSL configuration template (activated after certbot runs)
cat > /etc/nginx/conf.d/ollystack-ssl.conf.disabled << 'SSL_NGINX_EOF'
# OllyStack HTTPS Configuration
# Enable by renaming to .conf after obtaining SSL certificate

server {
    listen 443 ssl http2;
    server_name DOMAIN_PLACEHOLDER;

    # SSL Configuration
    ssl_certificate /etc/letsencrypt/live/DOMAIN_PLACEHOLDER/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/DOMAIN_PLACEHOLDER/privkey.pem;
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:50m;
    ssl_session_tickets off;

    # Modern SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;

    # HSTS
    add_header Strict-Transport-Security "max-age=63072000" always;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # Gzip compression
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css text/xml application/json application/javascript application/xml+rss text/javascript;

    # Health check
    location /health {
        access_log off;
        return 200 "healthy\n";
        add_header Content-Type text/plain;
    }

    # API proxy
    location /api/ {
        limit_req zone=api_limit burst=50 nodelay;

        proxy_pass http://api_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
        proxy_read_timeout 300;
        proxy_connect_timeout 300;
        proxy_buffering off;
    }

    # AI Engine proxy
    location /ai/ {
        limit_req zone=api_limit burst=20 nodelay;

        rewrite ^/ai/(.*) /$1 break;
        proxy_pass http://ai_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
        proxy_read_timeout 300;
        proxy_connect_timeout 300;
    }

    # Grafana proxy
    location /grafana/ {
        rewrite ^/grafana/(.*) /$1 break;
        proxy_pass http://grafana_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
    }

    # OTLP HTTP ingestion (for external telemetry)
    location /v1/traces {
        limit_req zone=otlp_limit burst=100 nodelay;

        proxy_pass http://otlp_http_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
        proxy_read_timeout 60;
        client_max_body_size 10m;
    }

    location /v1/metrics {
        limit_req zone=otlp_limit burst=100 nodelay;

        proxy_pass http://otlp_http_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
        proxy_read_timeout 60;
        client_max_body_size 10m;
    }

    location /v1/logs {
        limit_req zone=otlp_limit burst=100 nodelay;

        proxy_pass http://otlp_http_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
        proxy_read_timeout 60;
        client_max_body_size 10m;
    }

    # UI - default route
    location / {
        proxy_pass http://ui_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
        proxy_cache_bypass $http_upgrade;
    }
}
SSL_NGINX_EOF

# Create certbot setup script
mkdir -p /var/www/certbot

cat > /opt/ollystack/setup-ssl.sh << 'SSL_SETUP_EOF'
#!/bin/bash
set -e

DOMAIN=$1
EMAIL=$2

if [ -z "$DOMAIN" ]; then
    echo "Usage: $0 <domain> [email]"
    exit 1
fi

echo "Setting up SSL for $DOMAIN..."

# Install certbot
dnf install -y certbot python3-certbot-nginx

# Get certificate
if [ -n "$EMAIL" ]; then
    certbot certonly --nginx -d $DOMAIN --non-interactive --agree-tos -m $EMAIL
else
    certbot certonly --nginx -d $DOMAIN --non-interactive --agree-tos --register-unsafely-without-email
fi

# Enable SSL configuration
if [ -f /etc/nginx/conf.d/ollystack-ssl.conf.disabled ]; then
    sed "s/DOMAIN_PLACEHOLDER/$DOMAIN/g" /etc/nginx/conf.d/ollystack-ssl.conf.disabled > /etc/nginx/conf.d/ollystack-ssl.conf
    rm /etc/nginx/conf.d/ollystack-ssl.conf.disabled
fi

# Test and reload nginx
nginx -t && systemctl reload nginx

# Setup auto-renewal
echo "0 0,12 * * * root certbot renew --quiet --post-hook 'systemctl reload nginx'" > /etc/cron.d/certbot-renew

echo "SSL setup complete for $DOMAIN"
echo "Access your site at: https://$DOMAIN"
SSL_SETUP_EOF

chmod +x /opt/ollystack/setup-ssl.sh

# Start nginx
systemctl enable nginx
systemctl start nginx

echo "=============================================="
echo "OllyStack MVP setup complete!"
echo "=============================================="
echo ""
echo "Services running:"
echo "  - ClickHouse: http://localhost:8123"
echo "  - Redis: localhost:6379"
echo "  - OTLP gRPC: localhost:4317"
echo "  - OTLP HTTP: localhost:4318"
echo "  - Nginx: http://localhost:80"
echo ""
if [ -n "$DOMAIN_NAME" ]; then
echo "To enable HTTPS for $DOMAIN_NAME:"
echo "  1. Point DNS A record to this server's public IP"
echo "  2. Run: /opt/ollystack/setup-ssl.sh $DOMAIN_NAME $LETSENCRYPT_EMAIL"
echo ""
fi
echo "Setup completed at $(date)"
