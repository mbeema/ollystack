# Zero-Code Observability Instrumentation Architecture

## Enterprise Guide for Multi-Language Telemetry Collection

**Version:** 1.0
**Last Updated:** February 2025
**Status:** Production Ready

---

### Document Information

| Field | Value |
|-------|-------|
| **Author** | Madhukar Beema |
| **Title** | Distinguished Engineer |
| **Email** | mbeema@gmail.com |
| **Organization** | Enterprise Architecture |
| **Classification** | Technical Reference |

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Architecture Overview](#2-architecture-overview)
3. [Components & Responsibilities](#3-components--responsibilities)
4. [Language-Specific Instrumentation](#4-language-specific-instrumentation)
5. [eBPF-Based Instrumentation](#5-ebpf-based-instrumentation)
   - [5.6 OTel vs eBPF Comparison](#56-otel-auto-instrumentation-vs-ebpf-comparison)
6. [Central Agent Management](#6-central-agent-management)
7. [OpenTelemetry Collector](#7-opentelemetry-collector)
8. [Correlation & Data Model](#8-correlation--data-model)
9. [Lifecycle Management](#9-lifecycle-management)
10. [Deployment Patterns](#10-deployment-patterns)
11. [Operational Runbook](#11-operational-runbook)

---

## 1. Executive Summary

### Purpose

This document provides a comprehensive architecture for implementing zero-code (automatic) instrumentation across all major programming languages. The goal is to collect traces, metrics, and logs with full correlation while minimizing developer effort.

### Key Principles

| Principle | Description |
|-----------|-------------|
| **Zero-Code First** | No application code changes required |
| **Unified Collection** | Single collector for all telemetry types |
| **Automatic Correlation** | trace_id/span_id propagation across signals |
| **Central Management** | Manage all agents from single control plane |
| **Language Agnostic** | Consistent experience across all languages |

### Supported Languages & Instrumentation Methods

| Language | Method | Maturity | Traces | Metrics | Logs |
|----------|--------|----------|--------|---------|------|
| Java | Agent (javaagent) | Stable | ✅ | ✅ | ✅ |
| Python | Auto-instrumentation | Stable | ✅ | ✅ | ✅ |
| .NET | CLR Profiler | Stable | ✅ | ✅ | ✅ |
| Node.js | Require hook | Stable | ✅ | ✅ | ✅ |
| Go | eBPF | Beta | ✅ | ⚠️ | ❌ |
| Ruby | Require hook | Beta | ✅ | ✅ | ⚠️ |
| PHP | Extension | Beta | ✅ | ✅ | ⚠️ |
| Rust | eBPF | Alpha | ✅ | ❌ | ❌ |
| C/C++ | eBPF | Alpha | ✅ | ❌ | ❌ |

---

## 2. Architecture Overview

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           CONTROL PLANE                                      │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │ Agent Manager   │  │ Config Server   │  │ Fleet Manager   │             │
│  │ (OpAMP Server)  │  │ (Remote Config) │  │ (Health/Status) │             │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘             │
└───────────┼────────────────────┼────────────────────┼───────────────────────┘
            │                    │                    │
            └────────────────────┼────────────────────┘
                                 │ OpAMP Protocol
┌────────────────────────────────┼────────────────────────────────────────────┐
│                           DATA PLANE                                         │
│                                │                                             │
│  ┌─────────────────────────────▼─────────────────────────────────┐          │
│  │                    OTel Collector Gateway                      │          │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐      │          │
│  │  │ OTLP Rcv │  │ Batch    │  │ Resource │  │ Exporters│      │          │
│  │  │ gRPC/HTTP│  │ Processor│  │ Enricher │  │ Multiple │      │          │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘      │          │
│  └─────────────────────────────▲─────────────────────────────────┘          │
│                                │                                             │
│         ┌──────────────────────┼──────────────────────┐                     │
│         │                      │                      │                     │
│  ┌──────┴──────┐  ┌────────────┴────────┐  ┌────────┴────────┐             │
│  │ Sidecar     │  │ DaemonSet           │  │ eBPF Agent      │             │
│  │ Collector   │  │ Collector           │  │ (Kernel Level)  │             │
│  └──────┬──────┘  └─────────┬───────────┘  └────────┬────────┘             │
│         │                   │                       │                       │
└─────────┼───────────────────┼───────────────────────┼───────────────────────┘
          │                   │                       │
┌─────────┼───────────────────┼───────────────────────┼───────────────────────┐
│         │              APPLICATION LAYER            │                       │
│  ┌──────▼──────┐  ┌────────▼────────┐  ┌───────────▼───────────┐           │
│  │ Java App   │  │ Python App      │  │ Go/Rust/C++ App       │           │
│  │ (javaagent)│  │ (auto-instr)    │  │ (eBPF instrumented)   │           │
│  └─────────────┘  └─────────────────┘  └───────────────────────┘           │
│  ┌─────────────┐  ┌─────────────────┐  ┌───────────────────────┐           │
│  │ .NET App   │  │ Node.js App     │  │ Ruby/PHP App          │           │
│  │ (profiler) │  │ (require hook)  │  │ (extension/hook)      │           │
│  └─────────────┘  └─────────────────┘  └───────────────────────┘           │
└─────────────────────────────────────────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           STORAGE LAYER                                      │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │ ClickHouse      │  │ Prometheus      │  │ Object Storage  │             │
│  │ (Traces/Logs)   │  │ (Metrics)       │  │ (Long-term)     │             │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
Application → Auto-Instrumentation → Local/Sidecar Collector → Gateway Collector → Backend Storage
     │                                        │
     └── Logs (stdout/file) ──────────────────┘
```

### Telemetry Signal Flow

| Signal | Source | Collection Method | Correlation Key |
|--------|--------|-------------------|-----------------|
| Traces | Auto-instrumented code | OTLP | trace_id, span_id |
| Metrics | Runtime + custom | OTLP/Prometheus | service.name, resource |
| Logs | Application logger | OTLP/Filelog | trace_id, span_id |

---

## 3. Components & Responsibilities

### 3.1 Component Overview

```
┌────────────────────────────────────────────────────────────────────────┐
│                        COMPONENT HIERARCHY                              │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  Level 1: Control Plane                                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                 │
│  │ OpAMP Server │  │ Config Store │  │ Fleet Manager│                 │
│  │ (Management) │  │ (etcd/consul)│  │ (Monitoring) │                 │
│  └──────────────┘  └──────────────┘  └──────────────┘                 │
│                                                                        │
│  Level 2: Collection Layer                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                 │
│  │ Gateway      │  │ Agent        │  │ eBPF         │                 │
│  │ Collector    │  │ Collector    │  │ Collector    │                 │
│  └──────────────┘  └──────────────┘  └──────────────┘                 │
│                                                                        │
│  Level 3: Instrumentation Layer                                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                 │
│  │ Language     │  │ eBPF         │  │ Log          │                 │
│  │ Agents       │  │ Probes       │  │ Shippers     │                 │
│  └──────────────┘  └──────────────┘  └──────────────┘                 │
│                                                                        │
│  Level 4: Application Layer                                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                 │
│  │ JVM Apps     │  │ Interpreted  │  │ Compiled     │                 │
│  │ (.NET, Java) │  │ (Python, JS) │  │ (Go, Rust)   │                 │
│  └──────────────┘  └──────────────┘  └──────────────┘                 │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Component Descriptions

#### Control Plane Components

| Component | Purpose | Technology | Owner |
|-----------|---------|------------|-------|
| **OpAMP Server** | Remote agent management, config updates | Custom/BindPlane | Platform Team |
| **Config Store** | Centralized configuration storage | etcd, Consul, K8s ConfigMaps | Platform Team |
| **Fleet Manager** | Agent health monitoring, version control | Custom Dashboard | Platform Team |

#### Data Plane Components

| Component | Purpose | Deployment | Owner |
|-----------|---------|------------|-------|
| **Gateway Collector** | Central aggregation, routing | K8s Deployment (HA) | Platform Team |
| **Agent Collector** | Node-level collection | K8s DaemonSet | Platform Team |
| **Sidecar Collector** | Pod-level collection | K8s Sidecar | App Team |
| **eBPF Agent** | Kernel-level instrumentation | K8s DaemonSet (privileged) | Platform Team |

#### Instrumentation Components

| Component | Languages | Method | Owner |
|-----------|-----------|--------|-------|
| **Java Agent** | Java, Kotlin, Scala | JVM -javaagent | Platform Team |
| **Python Auto** | Python | opentelemetry-instrument | Platform Team |
| **.NET Profiler** | C#, F#, VB.NET | CLR Profiler API | Platform Team |
| **Node.js Hook** | JavaScript, TypeScript | --require flag | Platform Team |
| **eBPF Probes** | Go, Rust, C, C++ | Kernel uprobes | Platform Team |

### 3.3 Responsibility Matrix (RACI)

| Activity | Platform Team | App Dev Team | SRE Team | Security Team |
|----------|---------------|--------------|----------|---------------|
| Agent deployment | R, A | I | C | C |
| Agent configuration | R, A | C | C | C |
| Agent upgrades | R, A | I | C | I |
| Collector management | R, A | I | C | I |
| Instrumentation config | R | A, C | I | I |
| Custom spans/metrics | I | R, A | C | I |
| Alert configuration | C | C | R, A | I |
| Security compliance | C | C | C | R, A |
| Sampling policies | R, A | C | C | I |
| Data retention | R | I | C | A |

**Legend:** R=Responsible, A=Accountable, C=Consulted, I=Informed

---

## 4. Language-Specific Instrumentation

### 4.1 Java (JVM Languages)

**Supported:** Java, Kotlin, Scala, Groovy, Clojure

#### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│                    JVM INSTRUMENTATION                          │
│                                                                 │
│  java -javaagent:opentelemetry-javaagent.jar -jar app.jar      │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                 JVM Agent Mechanism                      │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │   │
│  │  │ Premain     │  │ ByteBuddy   │  │ Instrumented│     │   │
│  │  │ Hook        │─▶│ Transform   │─▶│ Bytecode    │     │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Automatic Instrumentation                   │   │
│  │  • HTTP Clients (OkHttp, Apache, Java 11+)              │   │
│  │  • Web Frameworks (Spring, JAX-RS, Servlet)             │   │
│  │  • Databases (JDBC, Hibernate, MongoDB)                 │   │
│  │  • Messaging (Kafka, RabbitMQ, JMS)                     │   │
│  │  • Runtime Metrics (GC, Memory, Threads)                │   │
│  │  • Logging (Log4j, Logback, JUL) → trace correlation    │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

#### Deployment - Dockerfile

```dockerfile
# Multi-stage build for Java application
FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /app
COPY pom.xml .
COPY src ./src
RUN mvn clean package -DskipTests

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app

# Download OTel Java Agent
ARG OTEL_AGENT_VERSION=2.10.0
ADD https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/download/v${OTEL_AGENT_VERSION}/opentelemetry-javaagent.jar /otel/opentelemetry-javaagent.jar

COPY --from=build /app/target/*.jar app.jar

# Configure via environment variables (12-factor app)
ENV JAVA_TOOL_OPTIONS="-javaagent:/otel/opentelemetry-javaagent.jar"
ENV OTEL_SERVICE_NAME="java-service"
ENV OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4317"
ENV OTEL_EXPORTER_OTLP_PROTOCOL="grpc"
ENV OTEL_TRACES_EXPORTER="otlp"
ENV OTEL_METRICS_EXPORTER="otlp"
ENV OTEL_LOGS_EXPORTER="otlp"
ENV OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production"

# Enable log correlation
ENV OTEL_INSTRUMENTATION_LOG4J_MDC_ENABLED=true
ENV OTEL_INSTRUMENTATION_LOGBACK_MDC_ENABLED=true

EXPOSE 8080
ENTRYPOINT ["java", "-jar", "app.jar"]
```

#### Deployment - Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: java-app
  labels:
    app: java-app
    instrumentation: otel-java
spec:
  replicas: 3
  selector:
    matchLabels:
      app: java-app
  template:
    metadata:
      labels:
        app: java-app
      annotations:
        instrumentation.opentelemetry.io/inject-java: "true"  # OTel Operator
    spec:
      containers:
      - name: java-app
        image: myregistry/java-app:latest
        ports:
        - containerPort: 8080
        env:
        - name: OTEL_SERVICE_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.labels['app']
        - name: OTEL_RESOURCE_ATTRIBUTES
          value: "k8s.namespace=$(K8S_NAMESPACE),k8s.pod.name=$(K8S_POD_NAME)"
        - name: K8S_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: K8S_POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://otel-collector.observability:4317"
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
```

#### Configuration Options

```bash
# Performance tuning
OTEL_INSTRUMENTATION_COMMON_EXPERIMENTAL_CONTROLLER_TELEMETRY_ENABLED=false
OTEL_INSTRUMENTATION_COMMON_PEER_SERVICE_MAPPING="1.2.3.4=dbservice"

# Sampling
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.1  # 10% sampling

# Disable specific instrumentations
OTEL_INSTRUMENTATION_[NAME]_ENABLED=false
# Example: OTEL_INSTRUMENTATION_KAFKA_ENABLED=false

# Span limits
OTEL_SPAN_ATTRIBUTE_COUNT_LIMIT=128
OTEL_SPAN_EVENT_COUNT_LIMIT=128
OTEL_SPAN_LINK_COUNT_LIMIT=128
```

---

### 4.2 Python

**Supported:** Python 3.8+

#### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│                   PYTHON INSTRUMENTATION                        │
│                                                                 │
│  opentelemetry-instrument python app.py                        │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Python Instrumentation Mechanism            │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │   │
│  │  │ sitecustomize│  │ Module      │  │ Monkey      │     │   │
│  │  │ Hook        │─▶│ Import Hook │─▶│ Patching    │     │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Automatic Instrumentation                   │   │
│  │  • Web Frameworks (Django, Flask, FastAPI, AIOHTTP)     │   │
│  │  • HTTP Clients (requests, httpx, urllib3, aiohttp)     │   │
│  │  • Databases (psycopg2, pymysql, sqlalchemy, redis)     │   │
│  │  • Messaging (kafka-python, pika, celery)               │   │
│  │  • AWS SDK (boto3)                                      │   │
│  │  • Logging (stdlib logging) → trace correlation         │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

#### Deployment - Dockerfile

```dockerfile
FROM python:3.11-slim

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Install OTel instrumentation
RUN pip install --no-cache-dir \
    opentelemetry-distro \
    opentelemetry-exporter-otlp

# Auto-detect and install instrumentations for installed packages
RUN opentelemetry-bootstrap -a install

COPY . .

# Environment configuration
ENV OTEL_SERVICE_NAME="python-service"
ENV OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4317"
ENV OTEL_EXPORTER_OTLP_PROTOCOL="grpc"
ENV OTEL_TRACES_EXPORTER="otlp"
ENV OTEL_METRICS_EXPORTER="otlp"
ENV OTEL_LOGS_EXPORTER="otlp"
ENV OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production"

# Enable log correlation - CRITICAL for logs
ENV OTEL_PYTHON_LOG_CORRELATION="true"
ENV OTEL_PYTHON_LOGGING_AUTO_INSTRUMENTATION_ENABLED="true"
ENV OTEL_PYTHON_LOG_LEVEL="info"

EXPOSE 8000

# Use opentelemetry-instrument wrapper
CMD ["opentelemetry-instrument", "python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
```

#### Deployment - Kubernetes with Init Container

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: python-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: python-app
  template:
    metadata:
      labels:
        app: python-app
      annotations:
        instrumentation.opentelemetry.io/inject-python: "true"
    spec:
      initContainers:
      - name: otel-agent-installer
        image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-python:latest
        command: ["/bin/sh", "-c"]
        args:
        - cp -r /autoinstrumentation /otel-auto-instrumentation
        volumeMounts:
        - name: otel-auto-instrumentation
          mountPath: /otel-auto-instrumentation
      containers:
      - name: python-app
        image: myregistry/python-app:latest
        ports:
        - containerPort: 8000
        env:
        - name: PYTHONPATH
          value: "/otel-auto-instrumentation"
        - name: OTEL_SERVICE_NAME
          value: "python-app"
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://otel-collector.observability:4317"
        - name: OTEL_PYTHON_LOG_CORRELATION
          value: "true"
        - name: OTEL_PYTHON_LOGGING_AUTO_INSTRUMENTATION_ENABLED
          value: "true"
        volumeMounts:
        - name: otel-auto-instrumentation
          mountPath: /otel-auto-instrumentation
      volumes:
      - name: otel-auto-instrumentation
        emptyDir: {}
```

#### Configuration Options

```bash
# Sampling
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.1

# Disable specific instrumentations
OTEL_PYTHON_DISABLED_INSTRUMENTATIONS="flask,django"

# Log format (adds trace context to logs)
OTEL_PYTHON_LOG_FORMAT="%(asctime)s %(levelname)s [%(name)s] [trace_id=%(otelTraceID)s span_id=%(otelSpanID)s] %(message)s"

# Excluded URLs (don't trace health checks)
OTEL_PYTHON_EXCLUDED_URLS="health,ready,live,metrics"
```

---

### 4.3 .NET

**Supported:** .NET 6+, .NET Framework 4.6.2+

#### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│                   .NET INSTRUMENTATION                          │
│                                                                 │
│  CORECLR_ENABLE_PROFILING=1 dotnet MyApp.dll                   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              CLR Profiler Mechanism                      │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │   │
│  │  │ CLR Profiler│  │ IL Rewriting│  │ Startup     │     │   │
│  │  │ API         │─▶│ (Bytecode)  │─▶│ Hooks       │     │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Automatic Instrumentation                   │   │
│  │  • ASP.NET Core (MVC, Razor, Minimal APIs)              │   │
│  │  • HTTP Clients (HttpClient, WebClient)                 │   │
│  │  • Databases (EF Core, SqlClient, Npgsql)               │   │
│  │  • Messaging (Azure Service Bus, RabbitMQ)              │   │
│  │  • gRPC                                                  │   │
│  │  • ILogger → trace correlation                          │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

#### Deployment - Dockerfile

```dockerfile
# Build stage
FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build
WORKDIR /src
COPY ["MyApp.csproj", "."]
RUN dotnet restore
COPY . .
RUN dotnet publish -c Release -o /app/publish

# Runtime stage
FROM mcr.microsoft.com/dotnet/aspnet:8.0
WORKDIR /app

# Install OTel auto-instrumentation
ARG OTEL_VERSION=1.7.0
RUN apt-get update && apt-get install -y curl unzip && \
    curl -L -o otel-dotnet-install.sh https://github.com/open-telemetry/opentelemetry-dotnet-instrumentation/releases/download/v${OTEL_VERSION}/otel-dotnet-auto-install.sh && \
    chmod +x otel-dotnet-install.sh && \
    OTEL_DOTNET_AUTO_HOME=/otel ./otel-dotnet-install.sh && \
    rm otel-dotnet-install.sh && \
    apt-get remove -y curl unzip && apt-get autoremove -y

COPY --from=build /app/publish .

# CLR Profiler Configuration
ENV CORECLR_ENABLE_PROFILING=1
ENV CORECLR_PROFILER={918728DD-259F-4A6A-AC2B-B85E1B658571}
ENV CORECLR_PROFILER_PATH=/otel/linux-x64/OpenTelemetry.AutoInstrumentation.Native.so
ENV DOTNET_ADDITIONAL_DEPS=/otel/AdditionalDeps
ENV DOTNET_SHARED_STORE=/otel/store
ENV DOTNET_STARTUP_HOOKS=/otel/net/OpenTelemetry.AutoInstrumentation.StartupHook.dll
ENV OTEL_DOTNET_AUTO_HOME=/otel

# OTel Configuration
ENV OTEL_SERVICE_NAME="dotnet-service"
ENV OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4317"
ENV OTEL_TRACES_EXPORTER="otlp"
ENV OTEL_METRICS_EXPORTER="otlp"
ENV OTEL_LOGS_EXPORTER="otlp"
ENV OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production"

# Enable log correlation
ENV OTEL_DOTNET_AUTO_LOGS_INCLUDE_FORMATTED_MESSAGE=true

EXPOSE 8080
ENTRYPOINT ["dotnet", "MyApp.dll"]
```

#### Deployment - Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dotnet-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: dotnet-app
  template:
    metadata:
      labels:
        app: dotnet-app
      annotations:
        instrumentation.opentelemetry.io/inject-dotnet: "true"
    spec:
      containers:
      - name: dotnet-app
        image: myregistry/dotnet-app:latest
        ports:
        - containerPort: 8080
        env:
        # CLR Profiler (required)
        - name: CORECLR_ENABLE_PROFILING
          value: "1"
        - name: CORECLR_PROFILER
          value: "{918728DD-259F-4A6A-AC2B-B85E1B658571}"
        - name: CORECLR_PROFILER_PATH
          value: "/otel/linux-x64/OpenTelemetry.AutoInstrumentation.Native.so"
        - name: DOTNET_ADDITIONAL_DEPS
          value: "/otel/AdditionalDeps"
        - name: DOTNET_SHARED_STORE
          value: "/otel/store"
        - name: DOTNET_STARTUP_HOOKS
          value: "/otel/net/OpenTelemetry.AutoInstrumentation.StartupHook.dll"
        - name: OTEL_DOTNET_AUTO_HOME
          value: "/otel"
        # OTel Config
        - name: OTEL_SERVICE_NAME
          value: "dotnet-app"
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://otel-collector.observability:4317"
        - name: OTEL_DOTNET_AUTO_LOGS_INCLUDE_FORMATTED_MESSAGE
          value: "true"
        volumeMounts:
        - name: otel-auto-instrumentation
          mountPath: /otel
      initContainers:
      - name: otel-agent-installer
        image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-dotnet:latest
        command: ["/bin/sh", "-c"]
        args:
        - cp -r /autoinstrumentation /otel-auto-instrumentation
        volumeMounts:
        - name: otel-auto-instrumentation
          mountPath: /otel-auto-instrumentation
      volumes:
      - name: otel-auto-instrumentation
        emptyDir: {}
```

#### Configuration Options

```bash
# Sampling
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.1

# Disable specific instrumentations
OTEL_DOTNET_AUTO_TRACES_INSTRUMENTATION_ENABLED=AspNet=false

# Enable additional instrumentation
OTEL_DOTNET_AUTO_TRACES_ADDITIONAL_SOURCES="MyApp.*"

# Log filtering
OTEL_DOTNET_AUTO_LOGS_INCLUDE_FORMATTED_MESSAGE=true
```

---

### 4.4 Node.js

**Supported:** Node.js 14+, TypeScript

#### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│                   NODE.JS INSTRUMENTATION                       │
│                                                                 │
│  node --require @opentelemetry/auto-instrumentations-node app.js│
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Node.js Require Hook                        │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │   │
│  │  │ --require   │  │ Module      │  │ Shimmer     │     │   │
│  │  │ Flag        │─▶│ Interception│─▶│ Wrapping    │     │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Automatic Instrumentation                   │   │
│  │  • HTTP/HTTPS (http, https modules)                     │   │
│  │  • Express, Fastify, Koa, Hapi, Restify                 │   │
│  │  • Databases (pg, mysql, mongodb, redis, ioredis)       │   │
│  │  • GraphQL                                               │   │
│  │  • gRPC                                                  │   │
│  │  • AWS SDK, Azure SDK                                   │   │
│  │  • Winston, Bunyan, Pino → trace correlation            │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

#### Deployment - Dockerfile

```dockerfile
FROM node:20-alpine

WORKDIR /app

# Install dependencies
COPY package*.json ./
RUN npm ci --only=production

# Install OTel auto-instrumentation
RUN npm install --save \
    @opentelemetry/api \
    @opentelemetry/auto-instrumentations-node \
    @opentelemetry/sdk-node \
    @opentelemetry/exporter-trace-otlp-grpc \
    @opentelemetry/exporter-metrics-otlp-grpc \
    @opentelemetry/exporter-logs-otlp-grpc

COPY . .

# OTel Configuration
ENV OTEL_SERVICE_NAME="nodejs-service"
ENV OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4317"
ENV OTEL_TRACES_EXPORTER="otlp"
ENV OTEL_METRICS_EXPORTER="otlp"
ENV OTEL_LOGS_EXPORTER="otlp"
ENV OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production"
ENV OTEL_LOG_LEVEL="info"

# Node options for auto-instrumentation
ENV NODE_OPTIONS="--require @opentelemetry/auto-instrumentations-node/register"

EXPOSE 3000

CMD ["node", "server.js"]
```

#### Alternative: Instrumentation File

Create `instrumentation.js`:
```javascript
// instrumentation.js
const { NodeSDK } = require('@opentelemetry/sdk-node');
const { getNodeAutoInstrumentations } = require('@opentelemetry/auto-instrumentations-node');
const { OTLPTraceExporter } = require('@opentelemetry/exporter-trace-otlp-grpc');
const { OTLPMetricExporter } = require('@opentelemetry/exporter-metrics-otlp-grpc');
const { OTLPLogExporter } = require('@opentelemetry/exporter-logs-otlp-grpc');
const { PeriodicExportingMetricReader } = require('@opentelemetry/sdk-metrics');

const sdk = new NodeSDK({
  traceExporter: new OTLPTraceExporter(),
  metricReader: new PeriodicExportingMetricReader({
    exporter: new OTLPMetricExporter(),
  }),
  logRecordProcessor: new OTLPLogExporter(),
  instrumentations: [getNodeAutoInstrumentations({
    '@opentelemetry/instrumentation-fs': { enabled: false },
  })],
});

sdk.start();
```

Then in Dockerfile:
```dockerfile
ENV NODE_OPTIONS="--require ./instrumentation.js"
```

#### Deployment - Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nodejs-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nodejs-app
  template:
    metadata:
      labels:
        app: nodejs-app
      annotations:
        instrumentation.opentelemetry.io/inject-nodejs: "true"
    spec:
      containers:
      - name: nodejs-app
        image: myregistry/nodejs-app:latest
        ports:
        - containerPort: 3000
        env:
        - name: NODE_OPTIONS
          value: "--require @opentelemetry/auto-instrumentations-node/register"
        - name: OTEL_SERVICE_NAME
          value: "nodejs-app"
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://otel-collector.observability:4317"
        - name: OTEL_RESOURCE_ATTRIBUTES
          value: "k8s.namespace=$(K8S_NAMESPACE),k8s.pod.name=$(K8S_POD_NAME)"
        - name: K8S_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: K8S_POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
```

---

### 4.5 Go

**Status:** Requires eBPF (no traditional auto-instrumentation)

Go is a compiled language without a runtime that supports traditional auto-instrumentation. Options:

#### Option A: eBPF-Based (Recommended for Zero-Code)

See [Section 5: eBPF-Based Instrumentation](#5-ebpf-based-instrumentation)

#### Option B: Minimal Code (Recommended for Production)

```go
// main.go - Add minimal initialization code
package main

import (
    "context"
    "log"
    "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/trace"
)

func initTelemetry() func() {
    ctx := context.Background()
    exporter, _ := otlptracegrpc.New(ctx)
    tp := trace.NewTracerProvider(trace.WithBatcher(exporter))
    otel.SetTracerProvider(tp)
    return func() { tp.Shutdown(ctx) }
}

func main() {
    shutdown := initTelemetry()
    defer shutdown()

    // Wrap HTTP handler - this is automatic instrumentation
    handler := otelhttp.NewHandler(myHandler, "my-service")
    http.ListenAndServe(":8080", handler)
}
```

#### Option C: OpenTelemetry Go Auto (Experimental)

```yaml
# Kubernetes deployment with eBPF sidecar
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-app
spec:
  template:
    metadata:
      labels:
        app: go-app
        otel-go-auto-instrumentation: enabled  # Trigger eBPF agent
    spec:
      containers:
      - name: go-app
        image: myregistry/go-app:latest
        ports:
        - containerPort: 8080
```

---

### 4.6 Ruby

**Supported:** Ruby 2.7+, Rails 6+

#### Deployment - Dockerfile

```dockerfile
FROM ruby:3.2-slim

WORKDIR /app

# Install dependencies
COPY Gemfile Gemfile.lock ./
RUN bundle install --without development test

# Install OTel
RUN gem install opentelemetry-sdk \
    opentelemetry-exporter-otlp \
    opentelemetry-instrumentation-all

COPY . .

# OTel Configuration
ENV OTEL_SERVICE_NAME="ruby-service"
ENV OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4317"
ENV OTEL_TRACES_EXPORTER="otlp"
ENV OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production"

# Require instrumentation before app loads
ENV RUBYOPT="-ropentelemetry/instrumentation/all"

EXPOSE 3000

CMD ["bundle", "exec", "rails", "server", "-b", "0.0.0.0"]
```

#### Configuration - Initializer

Create `config/initializers/opentelemetry.rb`:
```ruby
# config/initializers/opentelemetry.rb
require 'opentelemetry/sdk'
require 'opentelemetry/exporter/otlp'
require 'opentelemetry/instrumentation/all'

OpenTelemetry::SDK.configure do |c|
  c.service_name = ENV.fetch('OTEL_SERVICE_NAME', 'ruby-service')
  c.use_all # Enables all auto-instrumentations
end
```

---

### 4.7 PHP

**Supported:** PHP 8.0+

#### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│                   PHP INSTRUMENTATION                           │
│                                                                 │
│  PHP with otel extension loaded                                │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              PHP Extension Mechanism                     │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │   │
│  │  │ Zend        │  │ Observer    │  │ Hook        │     │   │
│  │  │ Extension   │─▶│ API         │─▶│ Functions   │     │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Automatic Instrumentation                   │   │
│  │  • Laravel, Symfony, WordPress                          │   │
│  │  • PDO, MySQLi                                          │   │
│  │  • cURL, Guzzle                                         │   │
│  │  • Redis, Memcached                                     │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

#### Deployment - Dockerfile

```dockerfile
FROM php:8.2-fpm-alpine

# Install build dependencies
RUN apk add --no-cache $PHPIZE_DEPS linux-headers

# Install OTel extension
RUN pecl install opentelemetry && \
    docker-php-ext-enable opentelemetry

# Install Composer and OTel packages
COPY --from=composer:latest /usr/bin/composer /usr/bin/composer
COPY composer.json composer.lock ./
RUN composer require \
    open-telemetry/sdk \
    open-telemetry/exporter-otlp \
    open-telemetry/opentelemetry-auto-laravel  # or symfony, etc.

COPY . .

# OTel Configuration
ENV OTEL_PHP_AUTOLOAD_ENABLED=true
ENV OTEL_SERVICE_NAME="php-service"
ENV OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4317"
ENV OTEL_TRACES_EXPORTER="otlp"
ENV OTEL_METRICS_EXPORTER="otlp"
ENV OTEL_LOGS_EXPORTER="otlp"

# PHP configuration
RUN echo "opentelemetry.enabled=1" >> /usr/local/etc/php/conf.d/otel.ini

EXPOSE 9000

CMD ["php-fpm"]
```

#### Configuration - php.ini

```ini
; php.ini or conf.d/otel.ini
[opentelemetry]
opentelemetry.enabled = 1
```

#### Laravel Auto-Configuration

```php
// bootstrap/app.php (Laravel 11+)
<?php
use Illuminate\Foundation\Application;

return Application::configure(basePath: dirname(__DIR__))
    ->withRouting(...)
    ->withMiddleware(...)
    ->withExceptions(...)
    ->create();

// OTel auto-configures via OTEL_PHP_AUTOLOAD_ENABLED=true
```

---

## 5. eBPF-Based Instrumentation

### 5.1 What is eBPF?

**eBPF (extended Berkeley Packet Filter)** is a revolutionary Linux kernel technology that allows running sandboxed programs in kernel space without modifying kernel source code or loading kernel modules.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         eBPF ARCHITECTURE                                    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         USER SPACE                                   │   │
│  │                                                                      │   │
│  │  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐          │   │
│  │  │ eBPF Agent   │    │ OTel         │    │ Observability│          │   │
│  │  │ (Loader)     │    │ Collector    │    │ Backend      │          │   │
│  │  └──────┬───────┘    └──────▲───────┘    └──────────────┘          │   │
│  │         │                   │                                       │   │
│  │         │ Load              │ Ring Buffer / Perf Events            │   │
│  │         │ Programs          │ (telemetry data)                     │   │
│  │         │                   │                                       │   │
│  └─────────┼───────────────────┼───────────────────────────────────────┘   │
│            │                   │                                           │
│  ══════════╪═══════════════════╪═══════════════════════════════════════   │
│            │    KERNEL SPACE   │                                           │
│  ══════════╪═══════════════════╪═══════════════════════════════════════   │
│            ▼                   │                                           │
│  ┌─────────────────────────────┴───────────────────────────────────────┐   │
│  │                      eBPF VIRTUAL MACHINE                           │   │
│  │                                                                      │   │
│  │  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐          │   │
│  │  │ Verifier     │───▶│ JIT Compiler │───▶│ eBPF Program │          │   │
│  │  │ (Safety)     │    │ (Native code)│    │ (Running)    │          │   │
│  │  └──────────────┘    └──────────────┘    └──────┬───────┘          │   │
│  │                                                  │                  │   │
│  │  ┌───────────────────────────────────────────────┴──────────────┐  │   │
│  │  │                    ATTACHMENT POINTS                          │  │   │
│  │  │                                                               │  │   │
│  │  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐         │  │   │
│  │  │  │ kprobes │  │ uprobes │  │ trace-  │  │ network │         │  │   │
│  │  │  │ (kernel │  │ (user   │  │ points  │  │ (XDP,   │         │  │   │
│  │  │  │ funcs)  │  │ funcs)  │  │         │  │ TC)     │         │  │   │
│  │  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘         │  │   │
│  │  └───────────────────────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      APPLICATION PROCESSES                           │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐            │   │
│  │  │ Go App   │  │ Rust App │  │ C++ App  │  │ Any App  │            │   │
│  │  │ (binary) │  │ (binary) │  │ (binary) │  │ (binary) │            │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘            │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 eBPF Probe Types for Observability

| Probe Type | Target | Use Case | Overhead |
|------------|--------|----------|----------|
| **kprobes** | Kernel functions | Network calls, syscalls | Very Low |
| **uprobes** | User-space functions | Application function tracing | Low |
| **tracepoints** | Kernel static points | Scheduler, memory, network | Very Low |
| **USDT** | User static probes | Pre-defined app trace points | Very Low |
| **fentry/fexit** | Kernel functions | Modern kprobes alternative | Minimal |

### 5.3 How eBPF Tracing Works

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    eBPF TRACING FLOW                                     │
│                                                                         │
│  1. ATTACH UPROBE TO HTTP HANDLER                                       │
│  ─────────────────────────────────                                      │
│                                                                         │
│     Go Application Binary                                               │
│     ┌────────────────────────────────────────────┐                     │
│     │  func ServeHTTP(w, r *http.Request) {     │ ◄─── uprobe attach   │
│     │      // handler code                       │      (entry)        │
│     │  }                                         │ ◄─── uretprobe      │
│     └────────────────────────────────────────────┘      (exit)         │
│                                                                         │
│  2. WHEN FUNCTION EXECUTES                                              │
│  ─────────────────────────────                                          │
│                                                                         │
│     HTTP Request ───▶ ServeHTTP() ───▶ HTTP Response                   │
│                            │                                            │
│                            ▼                                            │
│     ┌────────────────────────────────────────────┐                     │
│     │           eBPF PROGRAM TRIGGERS            │                     │
│     │                                            │                     │
│     │  Entry:                                    │                     │
│     │  - Capture start timestamp                 │                     │
│     │  - Read function arguments                 │                     │
│     │  - Extract: URL, method, headers           │                     │
│     │  - Generate trace_id/span_id               │                     │
│     │                                            │                     │
│     │  Exit:                                     │                     │
│     │  - Capture end timestamp                   │                     │
│     │  - Calculate duration                      │                     │
│     │  - Read return value (status code)         │                     │
│     └────────────────────────────────────────────┘                     │
│                            │                                            │
│                            ▼                                            │
│  3. SEND TO USER SPACE                                                  │
│  ─────────────────────────                                              │
│                                                                         │
│     ┌────────────────────────────────────────────┐                     │
│     │              RING BUFFER                   │                     │
│     │  ┌──────────────────────────────────────┐ │                     │
│     │  │ Span {                               │ │                     │
│     │  │   trace_id: "abc123",                │ │                     │
│     │  │   span_id: "def456",                 │ │                     │
│     │  │   name: "GET /api/users",            │ │                     │
│     │  │   duration: 45ms,                    │ │                     │
│     │  │   status: 200                        │ │                     │
│     │  │ }                                    │ │                     │
│     │  └──────────────────────────────────────┘ │                     │
│     └────────────────────────────────────────────┘                     │
│                            │                                            │
│                            ▼                                            │
│     OTel Collector (OTLP export)                                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 5.4 eBPF Tools for Observability

#### Comparison Matrix

| Tool | Vendor | Languages | Traces | Metrics | Logs | Production Ready |
|------|--------|-----------|--------|---------|------|------------------|
| **OpenTelemetry Go Auto** | OTel | Go | ✅ | ⚠️ | ❌ | Beta |
| **Odigos** | Keyval | Go, Java, Python, Node, .NET | ✅ | ✅ | ✅ | Stable |
| **Pixie** | CNCF | All (network-level) | ✅ | ✅ | ❌ | Stable |
| **Beyla** | Grafana | Go, Java, Node, Python, Rust | ✅ | ✅ | ❌ | Beta |
| **Coroot** | Coroot | All | ✅ | ✅ | ❌ | Stable |
| **Cilium/Hubble** | Isovalent | Network-level | ✅ | ✅ | ❌ | Stable |

### 5.5 Odigos - Enterprise eBPF Instrumentation

Odigos provides automatic instrumentation for any application without code changes.

#### Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        ODIGOS ARCHITECTURE                              │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                      CONTROL PLANE                               │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │ Odigos UI    │  │ Instrumentor │  │ Scheduler    │          │   │
│  │  │ (Dashboard)  │  │ (CRD Manager)│  │              │          │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘          │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                       DATA PLANE (per node)                      │   │
│  │                                                                  │   │
│  │  ┌──────────────────────────────────────────────────────────┐  │   │
│  │  │                   Odiglet (DaemonSet)                     │  │   │
│  │  │  ┌────────────┐  ┌────────────┐  ┌────────────┐         │  │   │
│  │  │  │ Language   │  │ eBPF       │  │ OTel       │         │  │   │
│  │  │  │ Detector   │  │ Instrument │  │ Collector  │         │  │   │
│  │  │  └────────────┘  └────────────┘  └────────────┘         │  │   │
│  │  └──────────────────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    APPLICATION PODS                              │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │   │
│  │  │ Go App   │  │ Java App │  │ Python   │  │ Node.js  │        │   │
│  │  │ (eBPF)   │  │ (agent)  │  │ (auto)   │  │ (auto)   │        │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Deployment

```bash
# Install Odigos CLI
curl -s https://raw.githubusercontent.com/odigos-io/odigos/main/install.sh | bash

# Install in cluster
odigos install

# Select namespaces to instrument
odigos ui  # Opens web UI for configuration
```

#### Kubernetes CRDs

```yaml
# Select which applications to instrument
apiVersion: odigos.io/v1alpha1
kind: InstrumentedApplication
metadata:
  name: my-go-app
  namespace: production
spec:
  languages:
  - language: go
    containerName: app
---
# Configure destination
apiVersion: odigos.io/v1alpha1
kind: Destination
metadata:
  name: jaeger
spec:
  type: jaeger
  signals:
  - TRACES
  - METRICS
  data:
    JAEGER_URL: "jaeger-collector.observability:4317"
```

### 5.6 Grafana Beyla

Beyla is an eBPF-based auto-instrumentation tool from Grafana.

#### Deployment - Kubernetes DaemonSet

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: beyla
  namespace: observability
spec:
  selector:
    matchLabels:
      app: beyla
  template:
    metadata:
      labels:
        app: beyla
    spec:
      hostPID: true  # Required for eBPF
      serviceAccountName: beyla
      containers:
      - name: beyla
        image: grafana/beyla:latest
        securityContext:
          privileged: true  # Required for eBPF
          capabilities:
            add:
            - SYS_ADMIN
            - SYS_PTRACE
        env:
        - name: BEYLA_OPEN_PORT
          value: "80,443,8080,3000"  # Ports to instrument
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://otel-collector:4317"
        - name: BEYLA_SERVICE_NAME
          value: "auto-detected"  # Uses k8s metadata
        volumeMounts:
        - name: sys-kernel
          mountPath: /sys/kernel
          readOnly: true
      volumes:
      - name: sys-kernel
        hostPath:
          path: /sys/kernel
```

#### Configuration File

```yaml
# beyla-config.yaml
otel_metrics_export:
  endpoint: http://otel-collector:4317

otel_traces_export:
  endpoint: http://otel-collector:4317

# Discovery configuration
discovery:
  services:
  - name: my-go-service
    namespace: production
    open_ports: 8080

# Attribute enrichment
attributes:
  kubernetes:
    enable: true

# Routes for HTTP path grouping
routes:
  patterns:
  - /api/users/{id}
  - /api/orders/{id}
```

### 5.7 eBPF Requirements & Limitations

#### System Requirements

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| Linux Kernel | 4.14+ | 5.10+ |
| CPU Architecture | x86_64, arm64 | x86_64 |
| Privileges | CAP_SYS_ADMIN, CAP_BPF | privileged container |
| BTF (BPF Type Format) | Required for CO-RE | Kernel with CONFIG_DEBUG_INFO_BTF |

#### Limitations

| Limitation | Description | Workaround |
|------------|-------------|------------|
| **No Windows** | eBPF is Linux-only | Use language agents on Windows |
| **Kernel dependency** | Requires specific kernel features | Use distros with modern kernels |
| **Symbol stripping** | Stripped binaries harder to trace | Build with symbols or use DWARF |
| **Context propagation** | Limited automatic propagation | Manual header injection for distributed traces |
| **Log correlation** | No direct log access | Use sidecar log collection |

### 5.8 eBPF vs Traditional Instrumentation

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    COMPARISON: eBPF vs TRADITIONAL                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  TRADITIONAL (Agent-based)           eBPF (Kernel-based)               │
│  ─────────────────────────           ──────────────────────             │
│                                                                         │
│  ┌─────────────────────┐            ┌─────────────────────┐            │
│  │    Application      │            │    Application      │            │
│  │  ┌───────────────┐  │            │   (unmodified)      │            │
│  │  │ OTel SDK/Agent│  │            └──────────┬──────────┘            │
│  │  │ (in-process)  │  │                       │                       │
│  │  └───────┬───────┘  │                       │ uprobe                │
│  └──────────┼──────────┘                       │                       │
│             │                       ┌──────────▼──────────┐            │
│             │                       │   Linux Kernel      │            │
│             │                       │   ┌─────────────┐   │            │
│             │                       │   │ eBPF Program│   │            │
│             │                       │   └──────┬──────┘   │            │
│             │                       └──────────┼──────────┘            │
│             │                                  │                       │
│             ▼                                  ▼                       │
│  ┌─────────────────────┐            ┌─────────────────────┐            │
│  │   OTel Collector    │            │   OTel Collector    │            │
│  └─────────────────────┘            └─────────────────────┘            │
│                                                                         │
│  PROS:                              PROS:                              │
│  ✅ Deep instrumentation            ✅ Zero code changes               │
│  ✅ Full context propagation        ✅ Minimal overhead                │
│  ✅ Custom spans                    ✅ Language agnostic               │
│  ✅ Stable, production-ready        ✅ Works with any binary           │
│                                                                         │
│  CONS:                              CONS:                              │
│  ❌ Per-language setup              ❌ Linux only                      │
│  ❌ Memory/CPU overhead             ❌ Limited context propagation     │
│  ❌ App restart required            ❌ No custom spans                 │
│                                     ❌ Requires privileged access      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 5.9 Recommended Strategy

| Scenario | Recommendation |
|----------|----------------|
| JVM applications (Java, Kotlin, Scala) | Use Java Agent (mature, full-featured) |
| Python, Node.js, Ruby | Use language auto-instrumentation |
| .NET applications | Use CLR Profiler auto-instrumentation |
| Go applications | Use eBPF (Odigos or Beyla) |
| Rust, C, C++ applications | Use eBPF |
| Mixed environment | Combine: agents for JVM/interpreted, eBPF for compiled |
| Quick POC/evaluation | Use Odigos (handles all languages automatically) |

### 5.6 OTel Auto-Instrumentation vs eBPF Comparison

This section provides a comprehensive comparison to help choose the right instrumentation approach.

#### OpenTelemetry Auto-Instrumentation

**How it Works:**
- Uses language-specific hooks (javaagent, Python auto-instrumentation, Node.js require hooks, .NET CLR profiler)
- Runs in user-space, within the application process
- Instruments at the library/framework level

**Pros:**

| Advantage | Description |
|-----------|-------------|
| **Rich Context** | Full access to application context (HTTP headers, SQL queries, user IDs) |
| **Semantic Conventions** | Standardized span attributes across all languages |
| **Mature Ecosystem** | Stable support for Java, Python, .NET, Node.js |
| **Custom Attributes** | Easy to add business-specific metadata |
| **Log Correlation** | Native trace_id/span_id injection into logs |
| **Full Telemetry** | Traces, metrics, and logs with full correlation |

**Cons:**

| Disadvantage | Description |
|--------------|-------------|
| **Language-Specific** | Different agent per language |
| **Startup Overhead** | Adds ~100-500ms to startup time |
| **Memory Footprint** | Adds 50-200MB per process |
| **Deployment Changes** | Requires env vars or wrapper scripts |

#### eBPF-Based Instrumentation

**How it Works:**
- Runs in kernel space, intercepts syscalls and network traffic
- No application modification needed
- Instruments at the OS/network level

**Pros:**

| Advantage | Description |
|-----------|-------------|
| **Truly Zero-Code** | No env vars, no wrappers, no changes at all |
| **Language Agnostic** | Works for Go, Rust, C/C++ (compiled languages) |
| **Low Overhead** | <1% CPU impact, minimal memory |
| **Universal** | Single agent covers all processes on a node |
| **Network Visibility** | Sees all TCP/HTTP traffic regardless of library |
| **No Restart Required** | Can attach to running processes |

**Cons:**

| Disadvantage | Description |
|--------------|-------------|
| **Limited Context** | Cannot see application-level details (user IDs, business data) |
| **Kernel Requirement** | Needs Linux 4.14+ with BTF support |
| **Privileged Access** | Requires CAP_BPF or root |
| **Maturity** | Beta/Alpha for most implementations |
| **No Log Correlation** | Cannot inject trace context into application logs |
| **Limited Metrics** | Cannot access application-level metrics |

#### Side-by-Side Comparison Matrix

| Feature | OTel Auto-Instrumentation | eBPF |
|---------|---------------------------|------|
| **Deployment Complexity** | Medium (env vars, agents) | Low (DaemonSet only) |
| **Application Changes** | Minimal (env vars) | None |
| **Trace Context Propagation** | Full (headers, baggage) | Limited (HTTP only) |
| **Custom Attributes** | Yes | No |
| **Log Correlation** | Yes (trace_id injection) | No |
| **Metrics Collection** | Yes (runtime + custom) | Limited (network only) |
| **Startup Impact** | 100-500ms | None |
| **Runtime Overhead** | 2-5% CPU | <1% CPU |
| **Memory Overhead** | 50-200MB | <50MB (shared) |
| **Compiled Language Support** | Limited/None | Yes |
| **Kernel Requirements** | None | Linux 4.14+ with BTF |
| **Privileged Mode** | No | Yes |

#### Decision Matrix: When to Use Which

| Scenario | Recommendation | Rationale |
|----------|----------------|-----------|
| Java, Python, .NET, Node.js apps | **OTel Auto-Instrumentation** | Mature, full-featured, rich context |
| Go applications | **eBPF** | Only option for zero-code Go instrumentation |
| Rust, C, C++ applications | **eBPF** | Only option for compiled languages |
| Need business context in traces | **OTel** | eBPF cannot access application-level data |
| Need trace-log correlation | **OTel** | eBPF cannot inject trace context into logs |
| Cannot modify deployment at all | **eBPF** | Truly zero-touch instrumentation |
| Quick POC/visibility | **eBPF** | Fastest time to first trace |
| Production with SLOs | **OTel** | More reliable, mature, better context |
| Service mesh visibility | **eBPF** | Complements existing instrumentation |
| Mixed polyglot environment | **Both** | OTel for interpreted, eBPF for compiled |

#### Recommended Hybrid Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    HYBRID INSTRUMENTATION STRATEGY                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Interpreted/Managed Languages          Compiled Languages              │
│  ┌─────────────────────────┐           ┌─────────────────────────┐     │
│  │ Java      → javaagent   │           │ Go      → eBPF          │     │
│  │ Python    → auto-instr  │           │ Rust    → eBPF          │     │
│  │ Node.js   → require     │           │ C/C++   → eBPF          │     │
│  │ .NET      → CLR profiler│           │                         │     │
│  │ Ruby      → require     │           │                         │     │
│  │ PHP       → extension   │           │                         │     │
│  └─────────────────────────┘           └─────────────────────────┘     │
│           │                                       │                     │
│           │  OTel Auto-Instrumentation            │  eBPF Probes       │
│           │  • Rich context                       │  • Zero-code       │
│           │  • Log correlation                    │  • Low overhead    │
│           │  • Custom attributes                  │  • Network-level   │
│           │                                       │                     │
│           └───────────────────┬───────────────────┘                     │
│                               │                                         │
│                               ▼                                         │
│                    ┌─────────────────────┐                              │
│                    │  OTel Collector     │                              │
│                    │  (Unified Pipeline) │                              │
│                    └─────────────────────┘                              │
│                               │                                         │
│                               ▼                                         │
│                    ┌─────────────────────┐                              │
│                    │  Backend Storage    │                              │
│                    │  (ClickHouse, etc)  │                              │
│                    └─────────────────────┘                              │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Migration Path

For organizations starting fresh or migrating:

| Phase | Action | Duration |
|-------|--------|----------|
| **Phase 1** | Deploy eBPF agent for immediate visibility | Week 1 |
| **Phase 2** | Add OTel auto-instrumentation for Java/Python services | Weeks 2-3 |
| **Phase 3** | Enable log correlation for critical services | Weeks 4-5 |
| **Phase 4** | Add custom attributes for business context | Ongoing |

> **Best Practice:** Start with eBPF for quick wins and network visibility, then layer OTel auto-instrumentation for richer application context where needed.

---

## 6. Central Agent Management

### 6.1 OpAMP Protocol

**OpAMP (Open Agent Management Protocol)** is an open standard for managing observability agents remotely.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        OpAMP ARCHITECTURE                               │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                      OpAMP SERVER                                │   │
│  │                   (Control Plane)                                │   │
│  │                                                                  │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │ Agent        │  │ Config       │  │ Health       │          │   │
│  │  │ Registry     │  │ Management   │  │ Monitoring   │          │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘          │   │
│  │                                                                  │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │ Version      │  │ Package      │  │ Fleet        │          │   │
│  │  │ Management   │  │ Distribution │  │ Analytics    │          │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘          │   │
│  └──────────────────────────────┬──────────────────────────────────┘   │
│                                 │                                       │
│                                 │ OpAMP Protocol                        │
│                                 │ (WebSocket/HTTP)                      │
│                                 │                                       │
│  ┌──────────────────────────────┼──────────────────────────────────┐   │
│  │                              │                                   │   │
│  │  ┌───────────────────────────▼────────────────────────────────┐ │   │
│  │  │                    OTel Collector                          │ │   │
│  │  │                  (OpAMP Client)                            │ │   │
│  │  │  ┌────────────┐  ┌────────────┐  ┌────────────┐          │ │   │
│  │  │  │ Agent      │  │ Config     │  │ Status     │          │ │   │
│  │  │  │ Descriptor │  │ Receiver   │  │ Reporter   │          │ │   │
│  │  │  └────────────┘  └────────────┘  └────────────┘          │ │   │
│  │  └────────────────────────────────────────────────────────────┘ │   │
│  │                                                                  │   │
│  │                         NODE (Agent)                             │   │
│  └──────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

### 6.2 OpAMP Capabilities

| Capability | Description |
|------------|-------------|
| **Remote Configuration** | Push config changes without restart |
| **Agent Status Reporting** | Health, version, effective config |
| **Package Management** | Remote agent updates and rollbacks |
| **Connection Credentials** | Rotate credentials remotely |
| **Custom Messages** | Application-specific extensions |

### 6.3 OTel Collector with OpAMP

#### Collector Configuration

```yaml
# otel-collector-config.yaml
extensions:
  opamp:
    server:
      ws:
        endpoint: wss://opamp-server.example.com:4320/v1/opamp
        tls:
          insecure: false
          ca_file: /etc/ssl/certs/ca.crt
    instance_uid: ${env:OTEL_INSTANCE_ID}
    agent_description:
      identifying_attributes:
        service.name: otel-collector
        service.namespace: ${env:K8S_NAMESPACE}
        k8s.node.name: ${env:K8S_NODE_NAME}
      non_identifying_attributes:
        os.type: linux

receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

processors:
  batch:

exporters:
  otlp:
    endpoint: backend:4317

service:
  extensions: [opamp]
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlp]
```

### 6.4 OpAMP Server Implementations

#### Option A: BindPlane OP (Enterprise)

```yaml
# Kubernetes deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bindplane-server
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: bindplane
        image: observiq/bindplane-ee:latest
        ports:
        - containerPort: 3001  # UI
        - containerPort: 4320  # OpAMP
        env:
        - name: BINDPLANE_LICENSE
          valueFrom:
            secretKeyRef:
              name: bindplane-license
              key: license
```

#### Option B: Custom OpAMP Server

```go
// Simple OpAMP server implementation
package main

import (
    "github.com/open-telemetry/opamp-go/server"
    "github.com/open-telemetry/opamp-go/protobufs"
)

func main() {
    srv := server.New(&server.Settings{})

    srv.OnConnecting = func(request *http.Request) (bool, int) {
        // Authentication
        return true, http.StatusOK
    }

    srv.OnMessage = func(conn server.Connection, msg *protobufs.AgentToServer) {
        // Handle agent status reports
        if msg.StatusReport != nil {
            handleStatusReport(conn, msg.StatusReport)
        }
    }

    http.HandleFunc("/v1/opamp", srv.HandleHTTP)
    http.ListenAndServe(":4320", nil)
}
```

### 6.5 Fleet Management Dashboard

#### Key Metrics to Track

```yaml
# Prometheus metrics from OpAMP server
agent_fleet_total{status="healthy"} 150
agent_fleet_total{status="unhealthy"} 3
agent_fleet_total{status="disconnected"} 2

agent_config_version{agent_id="abc123"} "v1.2.3"
agent_last_seen_seconds{agent_id="abc123"} 30

agent_telemetry_rate{agent_id="abc123", signal="traces"} 1500
agent_telemetry_rate{agent_id="abc123", signal="metrics"} 5000
agent_telemetry_rate{agent_id="abc123", signal="logs"} 2000
```

#### Dashboard Requirements

| Panel | Description |
|-------|-------------|
| Fleet Overview | Total agents by status (healthy/unhealthy) |
| Version Distribution | Agents by collector version |
| Config Compliance | Agents with outdated configs |
| Telemetry Throughput | Data volume per agent |
| Error Rate | Agent errors and restarts |
| Resource Usage | CPU/memory per agent |

### 6.6 Centralized Configuration Management

#### Configuration Hierarchy

```
┌─────────────────────────────────────────────────────────────────┐
│                 CONFIGURATION HIERARCHY                          │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Level 1: Global Defaults                                 │   │
│  │ (Applied to all agents)                                  │   │
│  │ - Default exporters                                      │   │
│  │ - Global sampling rate                                   │   │
│  │ - Security settings                                      │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Level 2: Environment Override                            │   │
│  │ (dev, staging, production)                               │   │
│  │ - Environment-specific endpoints                         │   │
│  │ - Sampling adjustments                                   │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Level 3: Cluster/Region Override                         │   │
│  │ (us-east-1, eu-west-1)                                   │   │
│  │ - Regional backend endpoints                             │   │
│  │ - Compliance requirements                                │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Level 4: Namespace Override                              │   │
│  │ (payment-service, user-service)                          │   │
│  │ - Service-specific processors                            │   │
│  │ - Custom attributes                                      │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Level 5: Agent Override                                  │   │
│  │ (specific pod/instance)                                  │   │
│  │ - Debug settings                                         │   │
│  │ - Troubleshooting config                                 │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

#### GitOps Configuration

```yaml
# configs/global/base.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

processors:
  batch:
    timeout: 1s

  resource:
    attributes:
      - key: deployment.environment
        from_attribute: k8s.namespace.name
        action: insert

exporters:
  otlp:
    endpoint: ${OTEL_BACKEND_ENDPOINT}

---
# configs/production/override.yaml
processors:
  filter:
    traces:
      span:
        - 'attributes["http.target"] == "/health"'
        - 'attributes["http.target"] == "/ready"'

  probabilistic_sampler:
    sampling_percentage: 10

---
# configs/production/us-east-1/override.yaml
exporters:
  otlp:
    endpoint: otel-backend.us-east-1.example.com:4317
```

### 6.7 Agent Lifecycle Management

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    AGENT LIFECYCLE                                       │
│                                                                         │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐            │
│  │ DISCOVER │──▶│ DEPLOY   │──▶│ CONFIGURE│──▶│ MONITOR  │            │
│  └──────────┘   └──────────┘   └──────────┘   └──────────┘            │
│       │              │              │              │                    │
│       │              │              │              │                    │
│       ▼              ▼              ▼              ▼                    │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐            │
│  │ Detect   │   │ Install  │   │ Apply    │   │ Health   │            │
│  │ language │   │ agent/   │   │ config   │   │ checks   │            │
│  │ & runtime│   │ sidecar  │   │ via OpAMP│   │ & alerts │            │
│  └──────────┘   └──────────┘   └──────────┘   └──────────┘            │
│                                                                         │
│                                     │                                   │
│                                     ▼                                   │
│                              ┌──────────┐                              │
│                              │ UPDATE   │                              │
│                              └──────────┘                              │
│                                   │                                    │
│                                   ▼                                    │
│                            ┌────────────┐                              │
│                            │ Rolling    │                              │
│                            │ upgrade    │                              │
│                            │ via OpAMP  │                              │
│                            └────────────┘                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 6.8 OpenTelemetry Operator (Kubernetes)

The OTel Operator automates agent injection and management.

#### Installation

```bash
# Install cert-manager (required)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.0/cert-manager.yaml

# Install OTel Operator
kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/latest/download/opentelemetry-operator.yaml
```

#### Instrumentation CRD

```yaml
apiVersion: opentelemetry.io/v1alpha1
kind: Instrumentation
metadata:
  name: auto-instrumentation
  namespace: observability
spec:
  exporter:
    endpoint: http://otel-collector.observability:4317

  propagators:
    - tracecontext
    - baggage
    - b3

  sampler:
    type: parentbased_traceidratio
    argument: "0.1"

  # Language-specific configurations
  java:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-java:latest
    env:
      - name: OTEL_INSTRUMENTATION_KAFKA_ENABLED
        value: "false"

  python:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-python:latest
    env:
      - name: OTEL_PYTHON_LOG_CORRELATION
        value: "true"

  nodejs:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-nodejs:latest

  dotnet:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-dotnet:latest
```

#### Auto-Injection Annotations

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    metadata:
      annotations:
        # Inject based on detected language
        instrumentation.opentelemetry.io/inject-java: "true"
        # OR
        instrumentation.opentelemetry.io/inject-python: "true"
        # OR
        instrumentation.opentelemetry.io/inject-nodejs: "true"
        # OR
        instrumentation.opentelemetry.io/inject-dotnet: "true"
        # OR use specific Instrumentation resource
        instrumentation.opentelemetry.io/inject-java: "observability/auto-instrumentation"
    spec:
      containers:
      - name: app
        image: myapp:latest
```

---

## 7. OpenTelemetry Collector

### 7.1 Collector Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    OTel COLLECTOR PIPELINE                              │
│                                                                         │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐              │
│  │   RECEIVERS   │  │  PROCESSORS   │  │   EXPORTERS   │              │
│  │               │  │               │  │               │              │
│  │  ┌─────────┐  │  │  ┌─────────┐  │  │  ┌─────────┐  │              │
│  │  │  OTLP   │──┼──┼─▶│  Batch  │──┼──┼─▶│  OTLP   │  │              │
│  │  │(gRPC/HTTP)│  │  │  │         │  │  │  │         │  │              │
│  │  └─────────┘  │  │  └─────────┘  │  │  └─────────┘  │              │
│  │               │  │       │       │  │               │              │
│  │  ┌─────────┐  │  │       ▼       │  │  ┌─────────┐  │              │
│  │  │ Filelog │──┼──┼─▶┌─────────┐  │  │  │ClickHouse│ │              │
│  │  │         │  │  │  │Resource │──┼──┼─▶│         │  │              │
│  │  └─────────┘  │  │  │ Enrich  │  │  │  └─────────┘  │              │
│  │               │  │  └─────────┘  │  │               │              │
│  │  ┌─────────┐  │  │       │       │  │  ┌─────────┐  │              │
│  │  │Prometheus│──┼──┼─▶     ▼       │  │  │Prometheus│ │              │
│  │  │(scrape) │  │  │  ┌─────────┐──┼──┼─▶│ Remote  │  │              │
│  │  └─────────┘  │  │  │ Filter  │  │  │  └─────────┘  │              │
│  │               │  │  │         │  │  │               │              │
│  │  ┌─────────┐  │  │  └─────────┘  │  │  ┌─────────┐  │              │
│  │  │ Kafka   │──┼──┼─▶     │       │  │  │  Loki   │  │              │
│  │  │         │  │  │       ▼       │──┼─▶│         │  │              │
│  │  └─────────┘  │  │  ┌─────────┐  │  │  └─────────┘  │              │
│  │               │  │  │Transform│  │  │               │              │
│  └───────────────┘  │  │         │  │  └───────────────┘              │
│                     │  └─────────┘  │                                  │
│                     └───────────────┘                                  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 7.2 Deployment Patterns

#### Pattern 1: Agent (DaemonSet)

```yaml
# Best for: Node-level metrics, log collection
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: otel-collector-agent
  namespace: observability
spec:
  selector:
    matchLabels:
      app: otel-collector-agent
  template:
    metadata:
      labels:
        app: otel-collector-agent
    spec:
      serviceAccountName: otel-collector
      containers:
      - name: collector
        image: otel/opentelemetry-collector-contrib:latest
        args: ["--config=/etc/otel/config.yaml"]
        ports:
        - containerPort: 4317  # OTLP gRPC
        - containerPort: 4318  # OTLP HTTP
        env:
        - name: K8S_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        volumeMounts:
        - name: config
          mountPath: /etc/otel
        - name: varlog
          mountPath: /var/log
          readOnly: true
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
      volumes:
      - name: config
        configMap:
          name: otel-agent-config
      - name: varlog
        hostPath:
          path: /var/log
```

#### Pattern 2: Sidecar

```yaml
# Best for: Per-pod isolation, multi-tenant environments
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    spec:
      containers:
      - name: app
        image: myapp:latest
        ports:
        - containerPort: 8080
        env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://localhost:4317"  # Sidecar

      - name: otel-sidecar
        image: otel/opentelemetry-collector-contrib:latest
        args: ["--config=/etc/otel/config.yaml"]
        ports:
        - containerPort: 4317
        volumeMounts:
        - name: otel-config
          mountPath: /etc/otel
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
      volumes:
      - name: otel-config
        configMap:
          name: otel-sidecar-config
```

#### Pattern 3: Gateway (Deployment)

```yaml
# Best for: Central aggregation, cross-cluster routing
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otel-collector-gateway
  namespace: observability
spec:
  replicas: 3
  selector:
    matchLabels:
      app: otel-collector-gateway
  template:
    metadata:
      labels:
        app: otel-collector-gateway
    spec:
      containers:
      - name: collector
        image: otel/opentelemetry-collector-contrib:latest
        args: ["--config=/etc/otel/config.yaml"]
        ports:
        - containerPort: 4317
        - containerPort: 4318
        resources:
          requests:
            cpu: 500m
            memory: 1Gi
          limits:
            cpu: 2
            memory: 4Gi
        livenessProbe:
          httpGet:
            path: /
            port: 13133
        readinessProbe:
          httpGet:
            path: /
            port: 13133
---
apiVersion: v1
kind: Service
metadata:
  name: otel-collector
  namespace: observability
spec:
  type: ClusterIP
  selector:
    app: otel-collector-gateway
  ports:
  - name: otlp-grpc
    port: 4317
    targetPort: 4317
  - name: otlp-http
    port: 4318
    targetPort: 4318
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: otel-collector-gateway
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: otel-collector-gateway
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

### 7.3 Collector Configuration

#### Complete Production Configuration

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
        max_recv_msg_size_mib: 16
      http:
        endpoint: 0.0.0.0:4318

  # Collect container logs
  filelog:
    include: [/var/log/pods/*/*/*.log]
    include_file_path: true
    operators:
      - type: router
        id: get-format
        routes:
          - output: parser-docker
            expr: 'body matches "^\\{"'
          - output: parser-crio
            expr: 'body matches "^[^ Z]+ "'
      - type: json_parser
        id: parser-docker
        output: extract-metadata
      - type: regex_parser
        id: parser-crio
        regex: '^(?P<time>[^ Z]+) (?P<stream>stdout|stderr) (?P<logtag>[^ ]*) ?(?P<log>.*)$'
        output: extract-metadata
      - type: move
        id: extract-metadata
        from: attributes["log.file.path"]
        to: resource["log.file.path"]

  # Kubernetes cluster metrics
  k8s_cluster:
    collection_interval: 30s
    node_conditions_to_report: [Ready, MemoryPressure, DiskPressure]

  # Prometheus metrics scraping
  prometheus:
    config:
      scrape_configs:
        - job_name: 'kubernetes-pods'
          kubernetes_sd_configs:
            - role: pod
          relabel_configs:
            - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
              action: keep
              regex: true

processors:
  # Batching for efficiency
  batch:
    timeout: 1s
    send_batch_size: 1024
    send_batch_max_size: 2048

  # Memory limiter to prevent OOM
  memory_limiter:
    check_interval: 1s
    limit_mib: 1800
    spike_limit_mib: 500

  # Add Kubernetes metadata
  k8sattributes:
    auth_type: serviceAccount
    passthrough: false
    extract:
      metadata:
        - k8s.namespace.name
        - k8s.deployment.name
        - k8s.pod.name
        - k8s.node.name
        - k8s.container.name
      labels:
        - tag_name: app
          key: app
          from: pod
        - tag_name: version
          key: version
          from: pod
    pod_association:
      - sources:
          - from: resource_attribute
            name: k8s.pod.ip

  # Resource enrichment
  resource:
    attributes:
      - key: deployment.environment
        value: production
        action: upsert
      - key: service.version
        from_attribute: version
        action: upsert

  # Filter out noisy data
  filter:
    error_mode: ignore
    traces:
      span:
        - 'attributes["http.target"] == "/health"'
        - 'attributes["http.target"] == "/ready"'
        - 'attributes["http.target"] == "/metrics"'
    logs:
      log_record:
        - 'severity_number < SEVERITY_NUMBER_WARN'  # Drop below WARN in prod

  # Tail-based sampling
  tail_sampling:
    decision_wait: 10s
    num_traces: 50000
    policies:
      - name: errors
        type: status_code
        status_code: {status_codes: [ERROR]}
      - name: slow-traces
        type: latency
        latency: {threshold_ms: 1000}
      - name: probabilistic
        type: probabilistic
        probabilistic: {sampling_percentage: 10}

  # Transform attributes
  transform:
    trace_statements:
      - context: span
        statements:
          - set(attributes["http.url"], Concat([attributes["http.scheme"], "://", attributes["http.host"], attributes["http.target"]], ""))
    log_statements:
      - context: log
        statements:
          - merge_maps(attributes, ParseJSON(body), "insert") where IsMatch(body, "^\\{")

exporters:
  # OTLP to backend
  otlp:
    endpoint: tempo.observability:4317
    tls:
      insecure: true

  # ClickHouse for long-term storage
  clickhouse:
    endpoint: tcp://clickhouse.observability:9000
    database: otel
    logs_table_name: otel_logs
    traces_table_name: otel_traces
    metrics_table_name: otel_metrics
    ttl_days: 30
    timeout: 10s
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 300s

  # Prometheus remote write
  prometheusremotewrite:
    endpoint: http://prometheus.observability:9090/api/v1/write
    tls:
      insecure: true

  # Debug logging
  debug:
    verbosity: detailed

extensions:
  health_check:
    endpoint: 0.0.0.0:13133

  pprof:
    endpoint: 0.0.0.0:1777

  zpages:
    endpoint: 0.0.0.0:55679

service:
  extensions: [health_check, pprof, zpages]

  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, k8sattributes, resource, filter, tail_sampling, batch]
      exporters: [otlp, clickhouse]

    metrics:
      receivers: [otlp, prometheus, k8s_cluster]
      processors: [memory_limiter, k8sattributes, resource, batch]
      exporters: [prometheusremotewrite, clickhouse]

    logs:
      receivers: [otlp, filelog]
      processors: [memory_limiter, k8sattributes, resource, transform, filter, batch]
      exporters: [clickhouse]

  telemetry:
    logs:
      level: info
    metrics:
      address: 0.0.0.0:8888
```

### 7.4 Key Processors

| Processor | Purpose | When to Use |
|-----------|---------|-------------|
| **batch** | Group data for efficiency | Always |
| **memory_limiter** | Prevent OOM | Always in production |
| **k8sattributes** | Add K8s metadata | Kubernetes environments |
| **resource** | Add/modify resource attributes | Add env, version info |
| **filter** | Drop unwanted data | Remove health checks, low-severity logs |
| **tail_sampling** | Sample complete traces | High-volume production |
| **transform** | Modify data with OTTL | Complex transformations |
| **probabilistic_sampler** | Head-based sampling | Simple sampling needs |

---

## 8. Correlation & Data Model

### 8.1 Correlation Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    SIGNAL CORRELATION                                    │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                         REQUEST FLOW                             │   │
│  │                                                                  │   │
│  │   User ──▶ Gateway ──▶ Service A ──▶ Service B ──▶ Database    │   │
│  │            │            │             │             │           │   │
│  │            ▼            ▼             ▼             ▼           │   │
│  │         ┌────────────────────────────────────────────────────┐ │   │
│  │         │              TRACE                                 │ │   │
│  │         │  trace_id: abc123                                  │ │   │
│  │         │  ┌──────────────────────────────────────────────┐ │ │   │
│  │         │  │ Span: gateway    (span_id: 001)              │ │ │   │
│  │         │  │  └─ Span: service-a (span_id: 002)           │ │ │   │
│  │         │  │       └─ Span: service-b (span_id: 003)      │ │ │   │
│  │         │  │            └─ Span: db-query (span_id: 004)  │ │ │   │
│  │         │  └──────────────────────────────────────────────┘ │ │   │
│  │         └────────────────────────────────────────────────────┘ │   │
│  │                                                                 │   │
│  │         ┌────────────────────────────────────────────────────┐ │   │
│  │         │              LOGS (correlated by trace_id)         │ │   │
│  │         │                                                    │ │   │
│  │         │  [gateway]   trace_id=abc123 "Request received"   │ │   │
│  │         │  [service-a] trace_id=abc123 "Processing order"   │ │   │
│  │         │  [service-b] trace_id=abc123 "Inventory check"    │ │   │
│  │         │  [service-a] trace_id=abc123 "Order completed"    │ │   │
│  │         └────────────────────────────────────────────────────┘ │   │
│  │                                                                 │   │
│  │         ┌────────────────────────────────────────────────────┐ │   │
│  │         │              METRICS (correlated by labels)        │ │   │
│  │         │                                                    │ │   │
│  │         │  http_request_duration{service="gateway"} 45ms    │ │   │
│  │         │  http_request_duration{service="service-a"} 30ms  │ │   │
│  │         │  db_query_duration{service="service-b"} 15ms      │ │   │
│  │         └────────────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

### 8.2 Trace Context Propagation

#### W3C Trace Context Headers

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    W3C TRACE CONTEXT                                     │
│                                                                         │
│  HTTP Headers:                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │ traceparent: 00-<trace_id>-<parent_span_id>-<flags>              │  │
│  │              00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01│  │
│  │                  │                         │                 │      │  │
│  │                  │                         │                 │      │  │
│  │            32 hex chars              16 hex chars         flags    │  │
│  │            (16 bytes)                (8 bytes)            (sampled)│  │
│  │                                                                     │  │
│  │ tracestate: vendor1=value1,vendor2=value2                          │  │
│  │             (vendor-specific trace data)                           │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  Propagation Flow:                                                      │
│  ┌──────────┐        ┌──────────┐        ┌──────────┐                 │
│  │ Service A│──HTTP──│ Service B│──HTTP──│ Service C│                 │
│  │          │headers │          │headers │          │                 │
│  │ trace_id │──────▶│ trace_id │──────▶│ trace_id │                 │
│  │ span_001 │        │ span_002 │        │ span_003 │                 │
│  │ (parent) │        │ (child)  │        │ (child)  │                 │
│  └──────────┘        └──────────┘        └──────────┘                 │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Supported Propagators

| Propagator | Headers | Use Case |
|------------|---------|----------|
| **W3C TraceContext** | traceparent, tracestate | Standard (recommended) |
| **B3** | X-B3-TraceId, X-B3-SpanId | Zipkin compatibility |
| **B3 Multi** | X-B3-* (multiple headers) | Legacy Zipkin |
| **Jaeger** | uber-trace-id | Jaeger systems |
| **AWS X-Ray** | X-Amzn-Trace-Id | AWS environments |
| **OT Trace** | ot-tracer-* | OpenTracing legacy |

### 8.3 Log Correlation

#### Automatic Log Correlation by Language

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    LOG CORRELATION                                       │
│                                                                         │
│  Java (MDC):                                                            │
│  ─────────────                                                          │
│  // Automatic with OTEL_INSTRUMENTATION_LOGBACK_MDC_ENABLED=true       │
│  logger.info("Processing order");                                       │
│  // Output: {"trace_id":"abc123","span_id":"def456","msg":"Processing"}│
│                                                                         │
│  Python:                                                                │
│  ────────                                                               │
│  # Automatic with OTEL_PYTHON_LOG_CORRELATION=true                     │
│  logging.info("Processing order")                                       │
│  # Output: trace_id=abc123 span_id=def456 Processing order             │
│                                                                         │
│  Node.js (Winston):                                                     │
│  ──────────────────                                                     │
│  // Requires explicit integration                                       │
│  const { trace } = require('@opentelemetry/api');                      │
│  const span = trace.getActiveSpan();                                   │
│  logger.info('Processing order', {                                     │
│    trace_id: span?.spanContext().traceId,                              │
│    span_id: span?.spanContext().spanId                                 │
│  });                                                                    │
│                                                                         │
│  .NET (ILogger):                                                        │
│  ───────────────                                                        │
│  // Automatic with OTEL_DOTNET_AUTO_LOGS_INCLUDE_FORMATTED_MESSAGE=true│
│  _logger.LogInformation("Processing order");                           │
│  // trace_id and span_id automatically added to scope                  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Log Format Recommendations

```json
{
  "timestamp": "2024-01-15T10:30:00.123Z",
  "severity": "INFO",
  "body": "Order processed successfully",
  "trace_id": "0af7651916cd43dd8448eb211c80319c",
  "span_id": "b7ad6b7169203331",
  "resource": {
    "service.name": "order-service",
    "service.version": "1.2.3",
    "deployment.environment": "production",
    "k8s.namespace.name": "commerce",
    "k8s.pod.name": "order-service-abc123"
  },
  "attributes": {
    "order.id": "ORD-12345",
    "customer.id": "CUST-789"
  }
}
```

### 8.4 Data Model (OTLP)

#### Resource Attributes (Common)

```yaml
resource:
  attributes:
    # Service identification
    service.name: "order-service"
    service.version: "1.2.3"
    service.namespace: "commerce"

    # Deployment context
    deployment.environment: "production"

    # Kubernetes metadata
    k8s.cluster.name: "prod-us-east-1"
    k8s.namespace.name: "commerce"
    k8s.deployment.name: "order-service"
    k8s.pod.name: "order-service-7d8f9c-abc12"
    k8s.node.name: "ip-10-0-1-123"
    k8s.container.name: "order-service"

    # Host information
    host.name: "ip-10-0-1-123"
    host.type: "m5.large"

    # Cloud provider
    cloud.provider: "aws"
    cloud.region: "us-east-1"
    cloud.availability_zone: "us-east-1a"
```

#### Span Attributes (Trace)

```yaml
span:
  trace_id: "0af7651916cd43dd8448eb211c80319c"
  span_id: "b7ad6b7169203331"
  parent_span_id: "a1b2c3d4e5f67890"
  name: "POST /api/orders"
  kind: SERVER
  start_time: "2024-01-15T10:30:00.000Z"
  end_time: "2024-01-15T10:30:00.045Z"
  status:
    code: OK

  attributes:
    # HTTP semantic conventions
    http.method: "POST"
    http.url: "https://api.example.com/api/orders"
    http.status_code: 201
    http.route: "/api/orders"
    http.request_content_length: 1234
    http.response_content_length: 567

    # User context
    enduser.id: "user-12345"

    # Custom attributes
    order.id: "ORD-67890"
    order.total: 99.99

  events:
    - name: "Order validated"
      timestamp: "2024-01-15T10:30:00.010Z"
      attributes:
        validation.result: "success"

    - name: "Payment processed"
      timestamp: "2024-01-15T10:30:00.030Z"
      attributes:
        payment.method: "credit_card"
```

### 8.5 Correlation Queries

#### ClickHouse: Find Logs for a Trace

```sql
-- Find all logs for a specific trace
SELECT
    Timestamp,
    ServiceName,
    SeverityText,
    Body,
    SpanId
FROM otel_logs
WHERE TraceId = '0af7651916cd43dd8448eb211c80319c'
ORDER BY Timestamp ASC;

-- Find traces with errors and their logs
SELECT
    t.TraceId,
    t.ServiceName,
    t.SpanName,
    t.Duration / 1000000 as DurationMs,
    l.Body as ErrorLog
FROM otel_traces t
LEFT JOIN otel_logs l ON t.TraceId = l.TraceId AND l.SeverityText = 'ERROR'
WHERE t.StatusCode = 'STATUS_CODE_ERROR'
  AND t.Timestamp > now() - INTERVAL 1 HOUR
ORDER BY t.Timestamp DESC
LIMIT 100;

-- Correlate metrics with traces (using exemplars)
SELECT
    m.MetricName,
    m.Value,
    m.Exemplars.TraceId as ExemplarTraceId,
    t.SpanName,
    t.Duration / 1000000 as DurationMs
FROM otel_metrics m
ARRAY JOIN Exemplars
LEFT JOIN otel_traces t ON Exemplars.TraceId = t.TraceId
WHERE m.MetricName = 'http_server_duration'
  AND m.Value > 1000  -- > 1 second
  AND m.Timestamp > now() - INTERVAL 1 HOUR;
```

#### Grafana: Trace-Log Correlation

```
# In Grafana, configure derived fields in Loki data source:
# This creates a link from logs to traces

Derived Fields:
  - Name: TraceID
    Regex: trace_id=(\w+)
    URL: ${__value.raw}
    Data source: Tempo

# Query logs and jump to trace:
{namespace="production"} |= "error" | json | trace_id != ""
```

### 8.6 Metric Exemplars

Exemplars connect metrics to traces by attaching trace_id to metric data points.

```yaml
# Collector config to enable exemplars
processors:
  transform:
    metric_statements:
      - context: datapoint
        statements:
          # Add exemplar with trace context
          - set(exemplars, [{"trace_id": trace_id(), "span_id": span_id(), "value": value}])

# In application (Java example):
# Exemplars are automatically recorded when traces are active
DoubleHistogram histogram = meter.histogramBuilder("http_request_duration")
    .setUnit("ms")
    .build();

// When recording with active span, exemplar is automatically attached
histogram.record(responseTime);  // trace_id linked automatically
```

---

## 9. Lifecycle Management

### 9.1 Implementation Phases

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    IMPLEMENTATION ROADMAP                                │
│                                                                         │
│  PHASE 1: FOUNDATION                                                    │
│  ───────────────────                                                    │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Week 1-2: Infrastructure Setup                                   │   │
│  │ • Deploy OTel Collector (Gateway + DaemonSet)                   │   │
│  │ • Configure backend storage (ClickHouse/Tempo)                  │   │
│  │ • Set up Kubernetes RBAC and service accounts                   │   │
│  │ • Install OTel Operator                                          │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                              │                                          │
│                              ▼                                          │
│  PHASE 2: PILOT                                                         │
│  ─────────────                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Week 3-4: First Application                                      │   │
│  │ • Select pilot application (low-risk, representative)           │   │
│  │ • Apply auto-instrumentation                                    │   │
│  │ • Validate traces, metrics, logs                                │   │
│  │ • Configure sampling and filtering                              │   │
│  │ • Create initial dashboards                                     │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                              │                                          │
│                              ▼                                          │
│  PHASE 3: EXPANSION                                                     │
│  ────────────────                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Week 5-8: Broader Rollout                                        │   │
│  │ • Instrument all services by language                           │   │
│  │ • Configure correlation across services                         │   │
│  │ • Set up alerts and SLOs                                        │   │
│  │ • Document runbooks                                             │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                              │                                          │
│                              ▼                                          │
│  PHASE 4: OPTIMIZATION                                                  │
│  ─────────────────────                                                  │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Week 9-12: Production Hardening                                  │   │
│  │ • Implement tail-based sampling                                 │   │
│  │ • Optimize resource usage                                       │   │
│  │ • Set up OpAMP for central management                           │   │
│  │ • Performance testing and tuning                                │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 9.2 Team Responsibilities

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    TEAM RESPONSIBILITIES                                 │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ PLATFORM TEAM                                                    │   │
│  │ (Observability / Infrastructure)                                 │   │
│  │                                                                  │   │
│  │ Responsibilities:                                                │   │
│  │ • Deploy and manage OTel Collectors                             │   │
│  │ • Maintain agent images and versions                            │   │
│  │ • Configure sampling and filtering policies                     │   │
│  │ • Manage backend storage (ClickHouse, Prometheus)               │   │
│  │ • Provide instrumentation guidelines                            │   │
│  │ • OpAMP server management                                       │   │
│  │ • Cost optimization                                             │   │
│  │                                                                  │   │
│  │ Skills: Kubernetes, OpenTelemetry, Observability, Go           │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ APPLICATION TEAMS                                                │   │
│  │ (Development)                                                    │   │
│  │                                                                  │   │
│  │ Responsibilities:                                                │   │
│  │ • Add instrumentation annotations to deployments                │   │
│  │ • Configure service-specific attributes                         │   │
│  │ • Add custom spans for business logic (optional)                │   │
│  │ • Review and act on telemetry data                              │   │
│  │ • Ensure log correlation is enabled                             │   │
│  │                                                                  │   │
│  │ Skills: Application language, basic OTel concepts              │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ SRE TEAM                                                         │   │
│  │ (Site Reliability Engineering)                                   │   │
│  │                                                                  │   │
│  │ Responsibilities:                                                │   │
│  │ • Create and manage dashboards                                  │   │
│  │ • Configure alerts and SLOs                                     │   │
│  │ • Incident response using telemetry                             │   │
│  │ • Capacity planning based on metrics                            │   │
│  │ • Performance analysis                                          │   │
│  │                                                                  │   │
│  │ Skills: Grafana, PromQL, incident management                   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ SECURITY TEAM                                                    │   │
│  │                                                                  │   │
│  │ Responsibilities:                                                │   │
│  │ • Review data retention policies                                │   │
│  │ • Ensure PII is not captured                                    │   │
│  │ • Approve network policies                                      │   │
│  │ • Audit access controls                                         │   │
│  │                                                                  │   │
│  │ Skills: Security, compliance, data privacy                     │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

### 9.3 Onboarding Checklist

#### For Platform Team

```markdown
## Platform Team Checklist

### Infrastructure
- [ ] OTel Collector Gateway deployed (HA mode)
- [ ] OTel Collector DaemonSet deployed
- [ ] Backend storage configured
- [ ] OTel Operator installed
- [ ] Instrumentation CRD created
- [ ] RBAC configured
- [ ] Network policies applied
- [ ] Resource quotas set

### Configuration
- [ ] Base collector config in ConfigMap
- [ ] Sampling policies defined
- [ ] Filter rules for health checks
- [ ] Resource attributes configured
- [ ] Export endpoints verified
- [ ] TLS certificates configured

### Documentation
- [ ] Architecture diagram updated
- [ ] Onboarding guide for app teams
- [ ] Runbook for common issues
- [ ] Escalation procedures defined
```

#### For Application Teams

```markdown
## Application Team Checklist

### Pre-Deployment
- [ ] Identify application language/runtime
- [ ] Review auto-instrumentation docs
- [ ] Plan rollout (canary → production)

### Deployment
- [ ] Add instrumentation annotation to deployment
- [ ] Set OTEL_SERVICE_NAME environment variable
- [ ] Configure OTEL_RESOURCE_ATTRIBUTES
- [ ] Enable log correlation (language-specific)
- [ ] Test in staging environment

### Validation
- [ ] Verify traces appear in backend
- [ ] Verify metrics are collected
- [ ] Verify logs have trace context
- [ ] Check resource attributes are correct
- [ ] Validate distributed traces across services

### Post-Deployment
- [ ] Create service dashboard
- [ ] Configure alerts
- [ ] Document custom attributes
- [ ] Share knowledge with team
```

### 9.4 Versioning Strategy

```yaml
# Version matrix for OTel components
components:
  otel-collector:
    current: "0.96.0"
    min_supported: "0.90.0"
    upgrade_policy: "monthly"

  java-agent:
    current: "2.10.0"
    min_supported: "2.0.0"
    upgrade_policy: "quarterly"

  python-auto:
    current: "0.45b0"
    min_supported: "0.40b0"
    upgrade_policy: "quarterly"

  dotnet-auto:
    current: "1.7.0"
    min_supported: "1.5.0"
    upgrade_policy: "quarterly"

  nodejs-auto:
    current: "0.49.0"
    min_supported: "0.45.0"
    upgrade_policy: "quarterly"

upgrade_process:
  1. Test in staging environment
  2. Update version in central image registry
  3. Rolling update via OpAMP (collectors)
  4. Restart deployments (language agents)
  5. Validate telemetry continuity
  6. Announce to teams
```

### 9.5 Cost Management

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    COST OPTIMIZATION                                     │
│                                                                         │
│  Data Volume Estimation:                                                │
│  ───────────────────────                                                │
│  Traces: ~1KB per span × 1000 spans/sec = 86 GB/day                    │
│  Metrics: ~100 bytes per point × 10000 points/sec = 86 GB/day          │
│  Logs: ~500 bytes per line × 5000 lines/sec = 216 GB/day               │
│                                                                         │
│  Cost Reduction Strategies:                                             │
│  ──────────────────────────                                             │
│                                                                         │
│  1. SAMPLING                                                            │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Head-based: 10% → 90% reduction                                 │   │
│  │ Tail-based: Keep errors + slow → 80% reduction                  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  2. FILTERING                                                           │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ • Drop health checks (10-30% of traffic)                        │   │
│  │ • Drop debug logs in production                                 │   │
│  │ • Drop internal service-to-service traces                       │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  3. AGGREGATION                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ • Aggregate metrics at collector level                          │   │
│  │ • Use delta temporality for counters                            │   │
│  │ • Reduce metric cardinality (limit label values)                │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  4. RETENTION                                                           │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ • Hot storage: 7 days (SSD)                                     │   │
│  │ • Warm storage: 30 days (HDD)                                   │   │
│  │ • Cold storage: 90 days (Object Storage)                        │   │
│  │ • Archive: 1 year (compressed)                                  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 10. Deployment Patterns

### 10.1 Complete Kubernetes Deployment

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: observability
---
# rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: otel-collector
  namespace: observability
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: otel-collector
rules:
- apiGroups: [""]
  resources: ["pods", "namespaces", "nodes", "endpoints"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "daemonsets", "statefulsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: otel-collector
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: otel-collector
subjects:
- kind: ServiceAccount
  name: otel-collector
  namespace: observability
---
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: otel-collector-config
  namespace: observability
data:
  config.yaml: |
    receivers:
      otlp:
        protocols:
          grpc:
            endpoint: 0.0.0.0:4317
          http:
            endpoint: 0.0.0.0:4318

    processors:
      batch:
        timeout: 1s
        send_batch_size: 1024

      memory_limiter:
        check_interval: 1s
        limit_mib: 1800

      k8sattributes:
        auth_type: serviceAccount
        extract:
          metadata:
            - k8s.namespace.name
            - k8s.deployment.name
            - k8s.pod.name
            - k8s.node.name

      resource:
        attributes:
          - key: deployment.environment
            value: production
            action: upsert

    exporters:
      otlp:
        endpoint: clickhouse.observability:4317
        tls:
          insecure: true

      debug:
        verbosity: basic

    extensions:
      health_check:
        endpoint: 0.0.0.0:13133

    service:
      extensions: [health_check]
      pipelines:
        traces:
          receivers: [otlp]
          processors: [memory_limiter, k8sattributes, resource, batch]
          exporters: [otlp]
        metrics:
          receivers: [otlp]
          processors: [memory_limiter, k8sattributes, resource, batch]
          exporters: [otlp]
        logs:
          receivers: [otlp]
          processors: [memory_limiter, k8sattributes, resource, batch]
          exporters: [otlp]
---
# gateway-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otel-collector-gateway
  namespace: observability
  labels:
    app: otel-collector-gateway
spec:
  replicas: 3
  selector:
    matchLabels:
      app: otel-collector-gateway
  template:
    metadata:
      labels:
        app: otel-collector-gateway
    spec:
      serviceAccountName: otel-collector
      containers:
      - name: collector
        image: otel/opentelemetry-collector-contrib:0.96.0
        args: ["--config=/etc/otel/config.yaml"]
        ports:
        - name: otlp-grpc
          containerPort: 4317
        - name: otlp-http
          containerPort: 4318
        - name: health
          containerPort: 13133
        volumeMounts:
        - name: config
          mountPath: /etc/otel
        resources:
          requests:
            cpu: 500m
            memory: 1Gi
          limits:
            cpu: 2
            memory: 4Gi
        livenessProbe:
          httpGet:
            path: /
            port: 13133
          initialDelaySeconds: 10
        readinessProbe:
          httpGet:
            path: /
            port: 13133
          initialDelaySeconds: 5
      volumes:
      - name: config
        configMap:
          name: otel-collector-config
---
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: otel-collector
  namespace: observability
spec:
  type: ClusterIP
  selector:
    app: otel-collector-gateway
  ports:
  - name: otlp-grpc
    port: 4317
    targetPort: 4317
  - name: otlp-http
    port: 4318
    targetPort: 4318
---
# instrumentation.yaml (OTel Operator CRD)
apiVersion: opentelemetry.io/v1alpha1
kind: Instrumentation
metadata:
  name: auto-instrumentation
  namespace: observability
spec:
  exporter:
    endpoint: http://otel-collector.observability:4317
  propagators:
    - tracecontext
    - baggage
  sampler:
    type: parentbased_traceidratio
    argument: "0.1"
  java:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-java:latest
  python:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-python:latest
    env:
      - name: OTEL_PYTHON_LOG_CORRELATION
        value: "true"
  nodejs:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-nodejs:latest
  dotnet:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-dotnet:latest
```

### 10.2 Sample Application Deployment

```yaml
# Example: Instrumenting a Java application
apiVersion: apps/v1
kind: Deployment
metadata:
  name: order-service
  namespace: commerce
spec:
  replicas: 3
  selector:
    matchLabels:
      app: order-service
  template:
    metadata:
      labels:
        app: order-service
        version: v1.2.3
      annotations:
        # This triggers automatic instrumentation
        instrumentation.opentelemetry.io/inject-java: "observability/auto-instrumentation"
    spec:
      containers:
      - name: order-service
        image: myregistry/order-service:1.2.3
        ports:
        - containerPort: 8080
        env:
        - name: OTEL_SERVICE_NAME
          value: "order-service"
        - name: OTEL_RESOURCE_ATTRIBUTES
          value: "service.version=1.2.3,team=commerce"
        resources:
          requests:
            cpu: 200m
            memory: 512Mi
          limits:
            cpu: 1
            memory: 1Gi
```

---

## 11. Operational Runbook

### 11.1 Troubleshooting Guide

#### Issue: No Traces Appearing

```bash
# 1. Check if collector is running
kubectl get pods -n observability -l app=otel-collector-gateway

# 2. Check collector logs
kubectl logs -n observability -l app=otel-collector-gateway --tail=100

# 3. Verify collector is receiving data
kubectl port-forward -n observability svc/otel-collector 8888:8888
curl http://localhost:8888/metrics | grep otelcol_receiver_accepted_spans

# 4. Check application instrumentation
kubectl describe pod <app-pod> | grep -A5 "Environment"
# Should see OTEL_* variables

# 5. Check init container completed (for operator injection)
kubectl get pod <app-pod> -o jsonpath='{.status.initContainerStatuses}'

# 6. Test connectivity from app to collector
kubectl exec -it <app-pod> -- curl -v http://otel-collector.observability:4317
```

#### Issue: High Latency in Collector

```bash
# 1. Check memory usage
kubectl top pods -n observability -l app=otel-collector-gateway

# 2. Check batch processor queue
curl http://localhost:8888/metrics | grep otelcol_processor_batch_batch_send_size

# 3. Check exporter queue
curl http://localhost:8888/metrics | grep otelcol_exporter_queue_size

# 4. Solutions:
# - Increase batch size
# - Add more collector replicas
# - Enable compression
# - Increase memory limits
```

#### Issue: Missing Log Correlation

```bash
# For Java
kubectl exec -it <pod> -- env | grep OTEL_INSTRUMENTATION
# Ensure: OTEL_INSTRUMENTATION_LOGBACK_MDC_ENABLED=true

# For Python
kubectl exec -it <pod> -- env | grep OTEL_PYTHON
# Ensure: OTEL_PYTHON_LOG_CORRELATION=true

# For .NET
kubectl exec -it <pod> -- env | grep OTEL_DOTNET
# Ensure: OTEL_DOTNET_AUTO_LOGS_INCLUDE_FORMATTED_MESSAGE=true

# Verify in logs
kubectl logs <pod> | head -20
# Should see trace_id and span_id in log output
```

### 11.2 Health Monitoring

```yaml
# Prometheus alerts for collector health
groups:
- name: otel-collector-alerts
  rules:
  - alert: OTelCollectorDown
    expr: up{job="otel-collector"} == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "OTel Collector is down"

  - alert: OTelCollectorHighMemory
    expr: process_resident_memory_bytes{job="otel-collector"} > 3e9
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "OTel Collector memory usage above 3GB"

  - alert: OTelCollectorExportFailure
    expr: rate(otelcol_exporter_send_failed_spans[5m]) > 0
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "OTel Collector failing to export spans"

  - alert: OTelCollectorQueueFull
    expr: otelcol_exporter_queue_size / otelcol_exporter_queue_capacity > 0.8
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "OTel Collector export queue near capacity"

  - alert: OTelCollectorDroppedData
    expr: rate(otelcol_processor_dropped_spans[5m]) > 0
    for: 1m
    labels:
      severity: warning
    annotations:
      summary: "OTel Collector dropping spans"
```

### 11.3 Maintenance Procedures

#### Collector Upgrade

```bash
# 1. Update image version in deployment
kubectl set image deployment/otel-collector-gateway \
  collector=otel/opentelemetry-collector-contrib:0.97.0 \
  -n observability

# 2. Monitor rollout
kubectl rollout status deployment/otel-collector-gateway -n observability

# 3. Verify new version
kubectl get pods -n observability -l app=otel-collector-gateway \
  -o jsonpath='{.items[*].spec.containers[*].image}'

# 4. Check for errors
kubectl logs -n observability -l app=otel-collector-gateway --tail=50 | grep -i error

# 5. Rollback if needed
kubectl rollout undo deployment/otel-collector-gateway -n observability
```

#### Agent Upgrade (via OTel Operator)

```bash
# 1. Update Instrumentation CRD
kubectl patch instrumentation auto-instrumentation -n observability --type=merge -p '
{
  "spec": {
    "java": {
      "image": "ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-java:2.11.0"
    }
  }
}'

# 2. Restart application pods to pick up new agent
kubectl rollout restart deployment/order-service -n commerce

# 3. Verify new agent version in pod logs
kubectl logs <new-pod> | grep -i "opentelemetry"
```

### 11.4 Emergency Procedures

#### Disable Instrumentation (Emergency)

```bash
# Remove instrumentation annotation from deployment
kubectl patch deployment order-service -n commerce --type=json -p='
[{"op": "remove", "path": "/spec/template/metadata/annotations/instrumentation.opentelemetry.io~1inject-java"}]'

# Or scale down collector to stop collection
kubectl scale deployment otel-collector-gateway -n observability --replicas=0
```

#### Reduce Data Volume Quickly

```bash
# Apply aggressive sampling via ConfigMap update
kubectl edit configmap otel-collector-config -n observability

# Add or modify:
processors:
  probabilistic_sampler:
    sampling_percentage: 1  # 1% sampling

# Restart collector
kubectl rollout restart deployment/otel-collector-gateway -n observability
```

---

## 12. Best Practices

### 12.1 Instrumentation Best Practices

#### Naming Conventions

```yaml
# Service Naming
service.name: <team>-<component>-<type>
# Examples:
# - commerce-order-api
# - payments-processor-worker
# - auth-gateway-proxy

# Span Naming
span.name: <VERB> <resource>
# Examples:
# - GET /api/orders/{id}
# - process_payment
# - SELECT users

# Metric Naming (Prometheus convention)
<namespace>_<subsystem>_<name>_<unit>
# Examples:
# - http_server_request_duration_seconds
# - db_connection_pool_size_total
# - order_processing_errors_total
```

#### Attribute Guidelines

```yaml
# DO: Use semantic conventions
attributes:
  http.method: "POST"
  http.url: "https://api.example.com/orders"
  http.status_code: 201
  db.system: "postgresql"
  db.statement: "SELECT * FROM users WHERE id = ?"

# DON'T: Create custom attributes when semantic conventions exist
attributes:
  method: "POST"           # Wrong: use http.method
  statusCode: 201          # Wrong: use http.status_code
  query: "SELECT..."       # Wrong: use db.statement

# DO: Add business context
attributes:
  order.id: "ORD-12345"
  customer.tier: "premium"
  feature.flag: "new-checkout"

# DON'T: Include PII or sensitive data
attributes:
  user.email: "john@example.com"    # PII - Don't include
  user.credit_card: "4111..."       # Sensitive - Never include
  auth.token: "Bearer eyJ..."       # Secret - Never include
```

#### Cardinality Management

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    CARDINALITY BEST PRACTICES                           │
│                                                                         │
│  HIGH CARDINALITY (Avoid in Metrics)                                   │
│  ────────────────────────────────────                                   │
│  ❌ user.id (millions of values)                                       │
│  ❌ request.id (unique per request)                                    │
│  ❌ timestamp as label                                                 │
│  ❌ full URL path with IDs (/users/12345)                              │
│                                                                         │
│  LOW CARDINALITY (Safe for Metrics)                                    │
│  ──────────────────────────────────                                     │
│  ✅ http.method (GET, POST, PUT, DELETE)                               │
│  ✅ http.status_code (200, 404, 500)                                   │
│  ✅ service.name (bounded set)                                         │
│  ✅ environment (dev, staging, prod)                                   │
│  ✅ region (us-east-1, eu-west-1)                                      │
│                                                                         │
│  URL Path Normalization:                                                │
│  ───────────────────────                                                │
│  /users/12345/orders/67890  →  /users/{userId}/orders/{orderId}        │
│  /api/v1/products/abc123    →  /api/v1/products/{productId}            │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 12.2 Sampling Best Practices

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    SAMPLING STRATEGIES                                   │
│                                                                         │
│  DEVELOPMENT                                                            │
│  ───────────                                                            │
│  Sample Rate: 100%                                                      │
│  Reason: Full visibility for debugging                                  │
│                                                                         │
│  STAGING                                                                │
│  ───────                                                                │
│  Sample Rate: 50-100%                                                   │
│  Reason: Catch issues before production                                 │
│                                                                         │
│  PRODUCTION                                                             │
│  ──────────                                                             │
│  Strategy: Tail-based sampling                                          │
│  • 100% of errors                                                       │
│  • 100% of slow traces (>P99 latency)                                  │
│  • 5-10% of normal traces                                              │
│  • 100% of traces with specific attributes (debug=true)                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Tail-Based Sampling Configuration

```yaml
processors:
  tail_sampling:
    decision_wait: 10s
    num_traces: 100000
    expected_new_traces_per_sec: 1000
    policies:
      # Always keep errors
      - name: errors-policy
        type: status_code
        status_code:
          status_codes: [ERROR]

      # Keep slow traces
      - name: latency-policy
        type: latency
        latency:
          threshold_ms: 1000

      # Keep traces with specific attributes
      - name: debug-policy
        type: string_attribute
        string_attribute:
          key: debug
          values: ["true"]

      # Sample remaining traces
      - name: probabilistic-policy
        type: probabilistic
        probabilistic:
          sampling_percentage: 10

      # Composite: Apply multiple policies
      - name: composite-policy
        type: composite
        composite:
          max_total_spans_per_second: 1000
          policy_order: [errors-policy, latency-policy, probabilistic-policy]
          rate_allocation:
            - policy: errors-policy
              percent: 50
            - policy: latency-policy
              percent: 30
            - policy: probabilistic-policy
              percent: 20
```

### 12.3 Performance Best Practices

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    PERFORMANCE OPTIMIZATION                              │
│                                                                         │
│  1. BATCHING                                                            │
│  ───────────                                                            │
│  • Batch size: 512-2048 (balance latency vs throughput)                │
│  • Timeout: 1-5 seconds                                                │
│  • Compress payloads (gzip for OTLP)                                   │
│                                                                         │
│  2. CONNECTION MANAGEMENT                                               │
│  ────────────────────────                                               │
│  • Use gRPC over HTTP (more efficient)                                 │
│  • Enable keep-alive connections                                       │
│  • Use connection pooling                                              │
│                                                                         │
│  3. RESOURCE LIMITS                                                     │
│  ──────────────────                                                     │
│  Collector sizing (per 10K spans/sec):                                 │
│  • CPU: 0.5-1 core                                                     │
│  • Memory: 512MB-1GB                                                   │
│  • Set memory_limiter processor                                        │
│                                                                         │
│  4. ASYNC EXPORT                                                        │
│  ─────────────                                                          │
│  • Never block application threads                                     │
│  • Use async exporters in SDKs                                         │
│  • Configure appropriate queue sizes                                   │
│                                                                         │
│  5. SPAN LIMITS                                                         │
│  ────────────                                                           │
│  • Max attributes per span: 128                                        │
│  • Max events per span: 128                                            │
│  • Max attribute value length: 1024 chars                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 12.4 Security Best Practices

```yaml
# 1. TLS EVERYWHERE
exporters:
  otlp:
    endpoint: backend.example.com:4317
    tls:
      cert_file: /certs/client.crt
      key_file: /certs/client.key
      ca_file: /certs/ca.crt

# 2. AUTHENTICATION
extensions:
  bearertoken:
    token: ${env:OTEL_AUTH_TOKEN}

exporters:
  otlp:
    auth:
      authenticator: bearertoken

# 3. DATA SCRUBBING
processors:
  attributes:
    actions:
      # Remove sensitive headers
      - key: http.request.header.authorization
        action: delete
      - key: http.request.header.cookie
        action: delete

      # Hash PII
      - key: user.email
        action: hash

      # Redact patterns
      - key: db.statement
        pattern: "password='[^']*'"
        replacement: "password='***'"
        action: update

# 4. NETWORK POLICIES (Kubernetes)
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: otel-collector-policy
spec:
  podSelector:
    matchLabels:
      app: otel-collector
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              otel-enabled: "true"
      ports:
        - port: 4317
        - port: 4318
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              name: observability
      ports:
        - port: 4317
```

### 12.5 Reliability Best Practices

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    RELIABILITY PATTERNS                                  │
│                                                                         │
│  1. HIGH AVAILABILITY                                                   │
│  ────────────────────                                                   │
│  • Run 3+ collector replicas                                           │
│  • Use load balancer for distribution                                  │
│  • Deploy across availability zones                                    │
│                                                                         │
│  2. BACKPRESSURE HANDLING                                              │
│  ────────────────────────                                               │
│  • Configure queue limits                                              │
│  • Implement retry with exponential backoff                            │
│  • Use persistent queue for critical data                              │
│                                                                         │
│  3. GRACEFUL DEGRADATION                                               │
│  ───────────────────────                                                │
│  • Telemetry failure should never break app                            │
│  • Set timeouts on SDK operations                                      │
│  • Use circuit breaker pattern                                         │
│                                                                         │
│  4. DATA DURABILITY                                                     │
│  ─────────────────                                                      │
│  • Enable persistent queue in collector                                │
│  • Use Kafka as buffer for critical pipelines                          │
│  • Configure retry policies                                            │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Persistent Queue Configuration

```yaml
exporters:
  otlp:
    endpoint: backend:4317
    sending_queue:
      enabled: true
      num_consumers: 10
      queue_size: 10000
      storage: file_storage/otlp
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 300s

extensions:
  file_storage/otlp:
    directory: /var/lib/otel/queue
    timeout: 10s
    compaction:
      on_start: true
      directory: /var/lib/otel/queue
```

### 12.6 Observability Anti-Patterns

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    ANTI-PATTERNS TO AVOID                               │
│                                                                         │
│  ❌ OVER-INSTRUMENTATION                                               │
│  ─────────────────────────                                              │
│  • Creating spans for every function call                              │
│  • Logging at DEBUG level in production                                │
│  • Recording metrics for every request attribute                       │
│  Fix: Instrument at service boundaries, use sampling                   │
│                                                                         │
│  ❌ MISSING CONTEXT                                                     │
│  ──────────────────                                                     │
│  • Logs without trace_id                                               │
│  • Spans without service.name                                          │
│  • Metrics without environment labels                                  │
│  Fix: Always include correlation IDs and resource attributes           │
│                                                                         │
│  ❌ UNBOUNDED CARDINALITY                                              │
│  ────────────────────────                                               │
│  • User ID as metric label                                             │
│  • Full URL path in metrics                                            │
│  • Timestamp in attribute                                              │
│  Fix: Use bounded values, normalize paths                              │
│                                                                         │
│  ❌ SYNCHRONOUS EXPORT                                                  │
│  ─────────────────────                                                  │
│  • Blocking app on telemetry export                                    │
│  • No timeout on export operations                                     │
│  Fix: Use async export, set timeouts                                   │
│                                                                         │
│  ❌ NO SAMPLING STRATEGY                                                │
│  ───────────────────────                                                │
│  • 100% sampling in production                                         │
│  • No priority for errors                                              │
│  Fix: Implement tail-based sampling                                    │
│                                                                         │
│  ❌ IGNORING COSTS                                                      │
│  ─────────────────                                                      │
│  • Not monitoring data volume                                          │
│  • No retention policies                                               │
│  Fix: Monitor ingestion rates, set TTLs                                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 13. Exporters & Backend Comparison

### 13.1 Exporter Types Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    EXPORTER CATEGORIES                                   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ PROTOCOL EXPORTERS (Format Converters)                          │   │
│  │                                                                  │   │
│  │  OTLP ──────────────▶ Native OpenTelemetry protocol            │   │
│  │  Jaeger ────────────▶ Jaeger native format                     │   │
│  │  Zipkin ────────────▶ Zipkin JSON/Protobuf                     │   │
│  │  Prometheus ────────▶ Prometheus exposition format              │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ DATABASE EXPORTERS (Direct Storage)                             │   │
│  │                                                                  │   │
│  │  ClickHouse ────────▶ Column-oriented OLAP                     │   │
│  │  Elasticsearch ─────▶ Search-optimized storage                 │   │
│  │  PostgreSQL ────────▶ Relational storage                       │   │
│  │  Cassandra ─────────▶ Wide-column store                        │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ PLATFORM EXPORTERS (Vendor Integration)                         │   │
│  │                                                                  │   │
│  │  Datadog ───────────▶ Datadog platform                         │   │
│  │  New Relic ─────────▶ New Relic platform                       │   │
│  │  Splunk ────────────▶ Splunk Observability                     │   │
│  │  Dynatrace ─────────▶ Dynatrace platform                       │   │
│  │  AWS X-Ray ─────────▶ AWS native tracing                       │   │
│  │  Google Cloud ──────▶ Cloud Trace/Monitoring                   │   │
│  │  Azure Monitor ─────▶ Azure Application Insights               │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ STREAMING EXPORTERS (Message Queue)                             │   │
│  │                                                                  │   │
│  │  Kafka ─────────────▶ Apache Kafka                             │   │
│  │  Pulsar ────────────▶ Apache Pulsar                            │   │
│  │  AWS Kinesis ───────▶ AWS Kinesis Data Streams                 │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 13.2 Backend Comparison Matrix

#### Traces Backends

| Backend | Type | Best For | Scalability | Query Speed | Cost | Maturity |
|---------|------|----------|-------------|-------------|------|----------|
| **Jaeger** | OSS | K8s native, simple setup | Medium | Fast | Free | Stable |
| **Tempo** | OSS | Grafana stack, cost-effective | High | Medium | Free | Stable |
| **Zipkin** | OSS | Simple traces, legacy | Low | Fast | Free | Stable |
| **ClickHouse** | OSS | Analytics, long retention | Very High | Very Fast | Free | Stable |
| **Elasticsearch** | OSS/Paid | Full-text search | High | Fast | Medium | Stable |
| **Datadog APM** | SaaS | Full platform, enterprise | Very High | Fast | High | Stable |
| **New Relic** | SaaS | Full platform, AI | Very High | Fast | High | Stable |
| **Dynatrace** | SaaS | Auto-discovery, AI | Very High | Fast | Very High | Stable |
| **Honeycomb** | SaaS | High-cardinality queries | High | Very Fast | Medium | Stable |
| **Lightstep** | SaaS | Trace analysis | High | Fast | Medium | Stable |
| **AWS X-Ray** | Cloud | AWS native | High | Medium | Medium | Stable |
| **Google Cloud Trace** | Cloud | GCP native | High | Fast | Medium | Stable |
| **Azure Monitor** | Cloud | Azure native | High | Medium | Medium | Stable |

#### Metrics Backends

| Backend | Type | Best For | Scalability | Query Speed | Cost | Maturity |
|---------|------|----------|-------------|-------------|------|----------|
| **Prometheus** | OSS | K8s metrics, alerting | Medium | Fast | Free | Stable |
| **Thanos** | OSS | Multi-cluster Prometheus | High | Fast | Free | Stable |
| **Cortex** | OSS | Multi-tenant Prometheus | Very High | Fast | Free | Stable |
| **Mimir** | OSS | High-scale Prometheus | Very High | Very Fast | Free | Stable |
| **VictoriaMetrics** | OSS | High performance | Very High | Very Fast | Free | Stable |
| **InfluxDB** | OSS/Paid | Time series, IoT | High | Fast | Medium | Stable |
| **ClickHouse** | OSS | Analytics, SQL queries | Very High | Very Fast | Free | Stable |
| **Datadog Metrics** | SaaS | Full platform | Very High | Fast | High | Stable |
| **CloudWatch** | Cloud | AWS native | High | Medium | Medium | Stable |

#### Logs Backends

| Backend | Type | Best For | Scalability | Query Speed | Cost | Maturity |
|---------|------|----------|-------------|-------------|------|----------|
| **Loki** | OSS | Grafana stack, labels | High | Medium | Free | Stable |
| **Elasticsearch** | OSS/Paid | Full-text search | High | Fast | Medium | Stable |
| **ClickHouse** | OSS | Analytics, SQL | Very High | Very Fast | Free | Stable |
| **OpenSearch** | OSS | Elasticsearch fork | High | Fast | Free | Stable |
| **Splunk** | SaaS/On-prem | Enterprise search | Very High | Very Fast | Very High | Stable |
| **Datadog Logs** | SaaS | Full platform | Very High | Fast | High | Stable |
| **CloudWatch Logs** | Cloud | AWS native | High | Medium | Medium | Stable |

### 13.3 Detailed Backend Analysis

#### Jaeger

```
┌─────────────────────────────────────────────────────────────────────────┐
│ JAEGER                                                                  │
├─────────────────────────────────────────────────────────────────────────┤
│ Type: Open Source | License: Apache 2.0 | CNCF: Graduated             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ PROS ✅                              CONS ❌                            │
│ ─────────                            ────────                           │
│ • Native K8s integration             • Limited long-term storage       │
│ • Simple deployment                  • No built-in metrics             │
│ • Good UI for trace analysis         • Scaling requires tuning         │
│ • OTLP native support               • No log correlation UI            │
│ • Active community                   • Basic alerting                  │
│ • Multiple storage backends          • UI less polished than SaaS     │
│                                                                         │
│ BEST FOR:                                                              │
│ • Small to medium deployments                                          │
│ • Teams starting with distributed tracing                              │
│ • Kubernetes-native environments                                       │
│                                                                         │
│ STORAGE OPTIONS:                                                       │
│ • Cassandra (production)                                               │
│ • Elasticsearch (production)                                           │
│ • Badger (single node)                                                 │
│ • Memory (development)                                                 │
│                                                                         │
│ EXPORTER CONFIG:                                                       │
│ ─────────────────                                                      │
│ exporters:                                                             │
│   jaeger:                                                              │
│     endpoint: jaeger-collector:14250                                   │
│     tls:                                                               │
│       insecure: true                                                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Grafana Tempo

```
┌─────────────────────────────────────────────────────────────────────────┐
│ GRAFANA TEMPO                                                           │
├─────────────────────────────────────────────────────────────────────────┤
│ Type: Open Source | License: AGPLv3 | Vendor: Grafana Labs            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ PROS ✅                              CONS ❌                            │
│ ─────────                            ────────                           │
│ • Very cost-effective storage        • Requires trace ID for query     │
│ • Object storage backend (S3)        • No full-text search             │
│ • Grafana native integration         • Newer, less battle-tested       │
│ • Trace to logs/metrics linking      • Limited standalone UI           │
│ • TraceQL query language            • Search requires extra setup      │
│ • Scales horizontally               • Complex multi-tenant setup       │
│                                                                         │
│ BEST FOR:                                                              │
│ • Grafana stack users                                                  │
│ • Cost-conscious organizations                                         │
│ • High-volume trace storage                                            │
│                                                                         │
│ ARCHITECTURE:                                                          │
│ • Distributor → Ingester → Compactor → Querier                        │
│ • Backend: S3, GCS, Azure Blob, MinIO                                 │
│                                                                         │
│ EXPORTER CONFIG:                                                       │
│ ─────────────────                                                      │
│ exporters:                                                             │
│   otlp:                                                                │
│     endpoint: tempo:4317                                               │
│     tls:                                                               │
│       insecure: true                                                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### ClickHouse

```
┌─────────────────────────────────────────────────────────────────────────┐
│ CLICKHOUSE                                                              │
├─────────────────────────────────────────────────────────────────────────┤
│ Type: Open Source | License: Apache 2.0 | Vendor: ClickHouse Inc      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ PROS ✅                              CONS ❌                            │
│ ─────────                            ────────                           │
│ • Extremely fast queries             • Operational complexity          │
│ • Excellent compression (10:1)       • Requires SQL knowledge          │
│ • All signals in one DB              • No native trace UI              │
│ • SQL query interface               • Cluster management overhead      │
│ • Cost-effective at scale            • Write amplification             │
│ • Real-time analytics               • Learning curve                   │
│ • Native OTEL exporter              • Memory hungry for joins          │
│                                                                         │
│ BEST FOR:                                                              │
│ • High-volume, long-retention needs                                    │
│ • Custom analytics/dashboards                                          │
│ • Organizations with SQL expertise                                     │
│ • Unified observability storage                                        │
│                                                                         │
│ SCHEMA (OTEL Native):                                                  │
│ • otel_traces - Trace/span data                                       │
│ • otel_logs - Log records                                             │
│ • otel_metrics_* - Gauge, Sum, Histogram, Summary                     │
│                                                                         │
│ EXPORTER CONFIG:                                                       │
│ ─────────────────                                                      │
│ exporters:                                                             │
│   clickhouse:                                                          │
│     endpoint: tcp://clickhouse:9000?dial_timeout=10s                  │
│     database: otel                                                     │
│     ttl: 72h                                                           │
│     logs_table_name: otel_logs                                        │
│     traces_table_name: otel_traces                                    │
│     metrics_table_name: otel_metrics                                  │
│     timeout: 5s                                                        │
│     retry_on_failure:                                                  │
│       enabled: true                                                    │
│       initial_interval: 5s                                            │
│       max_interval: 30s                                               │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Prometheus / Mimir / VictoriaMetrics

```
┌─────────────────────────────────────────────────────────────────────────┐
│ PROMETHEUS ECOSYSTEM                                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ PROMETHEUS (Core)                                                       │
│ ─────────────────                                                       │
│ PROS: Simple, battle-tested, huge ecosystem, PromQL                   │
│ CONS: Single-node limits, no native HA, pull-based only               │
│ Best for: Small/medium deployments, learning                          │
│                                                                         │
│ MIMIR (Grafana)                                                        │
│ ───────────────                                                         │
│ PROS: Unlimited scale, multi-tenant, S3 backend, Prometheus compatible│
│ CONS: Complex operations, AGPLv3 license, resource intensive          │
│ Best for: Large-scale, multi-team, Grafana stack                      │
│                                                                         │
│ VICTORIAMETRICS                                                        │
│ ────────────────                                                        │
│ PROS: Very fast, efficient storage, simple cluster, PromQL++          │
│ CONS: Smaller community, some enterprise features paid                │
│ Best for: High-performance needs, cost optimization                   │
│                                                                         │
│ THANOS                                                                 │
│ ──────                                                                  │
│ PROS: Extends existing Prometheus, global view, object storage        │
│ CONS: Complex sidecar architecture, eventual consistency              │
│ Best for: Existing Prometheus users needing scale                     │
│                                                                         │
│ EXPORTER CONFIG:                                                       │
│ ─────────────────                                                      │
│ exporters:                                                             │
│   prometheusremotewrite:                                               │
│     endpoint: http://mimir:9009/api/v1/push                           │
│     tls:                                                               │
│       insecure: true                                                   │
│     headers:                                                           │
│       X-Scope-OrgID: tenant-1                                         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Grafana Loki

```
┌─────────────────────────────────────────────────────────────────────────┐
│ GRAFANA LOKI                                                            │
├─────────────────────────────────────────────────────────────────────────┤
│ Type: Open Source | License: AGPLv3 | Vendor: Grafana Labs            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ PROS ✅                              CONS ❌                            │
│ ─────────                            ────────                           │
│ • Very cost-effective                • Limited full-text search        │
│ • Label-based (like Prometheus)      • Requires good labeling          │
│ • Grafana native integration         • Complex at scale                │
│ • LogQL query language              • Not for high-cardinality         │
│ • Object storage backend            • Slower than Elasticsearch        │
│ • Easy correlation with metrics     • Learning curve for LogQL         │
│                                                                         │
│ BEST FOR:                                                              │
│ • Grafana stack users                                                  │
│ • Cost-conscious logging                                               │
│ • Kubernetes logging                                                   │
│                                                                         │
│ EXPORTER CONFIG:                                                       │
│ ─────────────────                                                      │
│ exporters:                                                             │
│   loki:                                                                │
│     endpoint: http://loki:3100/loki/api/v1/push                       │
│     labels:                                                            │
│       attributes:                                                      │
│         service.name: "service"                                       │
│         k8s.namespace.name: "namespace"                               │
│       resource:                                                        │
│         deployment.environment: "env"                                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 13.4 SaaS Vendor Comparison

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    SAAS VENDOR COMPARISON                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│                    Datadog    New Relic   Dynatrace   Splunk   Honeycomb│
│ ──────────────────────────────────────────────────────────────────────│
│ Traces              ✅         ✅          ✅         ✅        ✅     │
│ Metrics             ✅         ✅          ✅         ✅        ✅     │
│ Logs                ✅         ✅          ✅         ✅        ⚠️     │
│ OTLP Native         ✅         ✅          ✅         ✅        ✅     │
│ Auto-Discovery      ⚠️         ⚠️          ✅         ⚠️        ❌     │
│ AI/ML Analysis      ✅         ✅          ✅         ⚠️        ⚠️     │
│ Custom Dashboards   ✅         ✅          ✅         ✅        ✅     │
│ Alerting            ✅         ✅          ✅         ✅        ✅     │
│ High Cardinality    ⚠️         ⚠️          ⚠️         ⚠️        ✅     │
│ Pricing Model       Per host   Per GB      Per host   Per GB   Per event│
│                                                                         │
│ TYPICAL COST (100 hosts, 1TB/month):                                   │
│ Datadog:      $15,000-30,000/month                                     │
│ New Relic:    $10,000-20,000/month                                     │
│ Dynatrace:    $20,000-40,000/month                                     │
│ Splunk:       $15,000-30,000/month                                     │
│ Honeycomb:    $5,000-15,000/month                                      │
│                                                                         │
│ WHEN TO USE:                                                           │
│ ─────────────                                                          │
│ Datadog:      Full platform, strong APM, large enterprise              │
│ New Relic:    Migration from legacy, good free tier                    │
│ Dynatrace:    Automatic discovery, complex enterprise                  │
│ Splunk:       Existing Splunk users, security focus                    │
│ Honeycomb:    High-cardinality debugging, modern approach              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 13.5 Cloud Provider Native Options

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    CLOUD PROVIDER COMPARISON                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ AWS                                                                     │
│ ───                                                                     │
│ X-Ray:           Distributed tracing (tight AWS integration)           │
│ CloudWatch:      Metrics, logs, dashboards                             │
│ OpenSearch:      Log analytics (managed Elasticsearch)                 │
│ Managed Grafana: Visualization layer                                   │
│ Managed Prometheus: Metrics storage                                    │
│                                                                         │
│ PROS: Native integration, pay-per-use, managed service                │
│ CONS: Vendor lock-in, can be expensive at scale, limited flexibility  │
│                                                                         │
│ GCP                                                                     │
│ ───                                                                     │
│ Cloud Trace:     Distributed tracing                                   │
│ Cloud Monitoring: Metrics                                              │
│ Cloud Logging:   Log management                                        │
│ Managed Prometheus: GMP                                                │
│                                                                         │
│ PROS: Strong Kubernetes integration (GKE), good ML capabilities       │
│ CONS: Less mature than AWS, limited customization                     │
│                                                                         │
│ Azure                                                                   │
│ ─────                                                                   │
│ Application Insights: Full APM                                         │
│ Azure Monitor:   Metrics and logs                                      │
│ Log Analytics:   Query engine                                          │
│                                                                         │
│ PROS: Strong .NET integration, single pane of glass                   │
│ CONS: Complex pricing, learning curve                                 │
│                                                                         │
│ EXPORTER CONFIGS:                                                       │
│ ─────────────────                                                      │
│ # AWS X-Ray                                                            │
│ exporters:                                                             │
│   awsxray:                                                             │
│     region: us-east-1                                                  │
│                                                                         │
│ # GCP Cloud Trace                                                      │
│ exporters:                                                             │
│   googlecloud:                                                         │
│     project: my-project                                                │
│                                                                         │
│ # Azure Monitor                                                        │
│ exporters:                                                             │
│   azuremonitor:                                                        │
│     connection_string: ${APPLICATIONINSIGHTS_CONNECTION_STRING}       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 13.6 Recommended Stack by Use Case

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    RECOMMENDED STACKS                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ STARTUP / SMALL TEAM (< 50 services)                                   │
│ ────────────────────────────────────                                    │
│ Budget: $0-500/month                                                   │
│                                                                         │
│ Traces:  Jaeger (single-node) or Tempo                                │
│ Metrics: Prometheus                                                    │
│ Logs:    Loki                                                          │
│ UI:      Grafana                                                       │
│                                                                         │
│ ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│ MEDIUM COMPANY (50-500 services)                                       │
│ ────────────────────────────────                                        │
│ Budget: $2,000-10,000/month                                            │
│                                                                         │
│ Option A (OSS):                                                        │
│   Traces:  Tempo or Jaeger with Elasticsearch                         │
│   Metrics: Mimir or VictoriaMetrics                                   │
│   Logs:    Loki or OpenSearch                                         │
│   UI:      Grafana                                                     │
│                                                                         │
│ Option B (Hybrid):                                                     │
│   All:     Grafana Cloud (generous free tier + paid)                  │
│                                                                         │
│ ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│ ENTERPRISE (500+ services)                                             │
│ ──────────────────────────                                              │
│ Budget: $20,000+/month                                                 │
│                                                                         │
│ Option A (Full OSS):                                                   │
│   All Signals: ClickHouse (unified storage)                           │
│   Metrics:     Mimir (Prometheus compatible)                          │
│   UI:          Grafana Enterprise                                      │
│                                                                         │
│ Option B (SaaS):                                                       │
│   Datadog, New Relic, or Dynatrace                                    │
│   (Choose based on existing tooling and team expertise)               │
│                                                                         │
│ Option C (Cloud Native):                                               │
│   AWS:   X-Ray + CloudWatch + OpenSearch                              │
│   GCP:   Cloud Trace + Cloud Monitoring + Cloud Logging               │
│   Azure: Application Insights + Azure Monitor                          │
│                                                                         │
│ ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│ HIGH-CARDINALITY / DEBUGGING FOCUS                                     │
│ ──────────────────────────────────                                      │
│                                                                         │
│   Traces:  Honeycomb (best for exploration)                           │
│   Metrics: ClickHouse or VictoriaMetrics                              │
│   Logs:    ClickHouse                                                  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 13.7 Multi-Backend Configuration

```yaml
# Send to multiple backends for redundancy or migration
exporters:
  # Primary: ClickHouse for long-term storage
  clickhouse:
    endpoint: tcp://clickhouse:9000
    database: otel
    ttl: 720h  # 30 days

  # Secondary: Tempo for Grafana integration
  otlp/tempo:
    endpoint: tempo:4317
    tls:
      insecure: true

  # Tertiary: Datadog for APM features
  datadog:
    api:
      key: ${DATADOG_API_KEY}
      site: datadoghq.com

  # Metrics to Prometheus
  prometheusremotewrite:
    endpoint: http://mimir:9009/api/v1/push

  # Logs to Loki
  loki:
    endpoint: http://loki:3100/loki/api/v1/push

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch, resource]
      exporters: [clickhouse, otlp/tempo, datadog]  # Fan-out

    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [clickhouse, prometheusremotewrite]

    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [clickhouse, loki]
```

---

## 14. AI/ML Observability & AIOps

### 14.1 The Rise of GenAI Observability

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    GENAI OBSERVABILITY LANDSCAPE (2025-2026)            │
│                                                                         │
│  Traditional Apps              vs            GenAI/LLM Apps             │
│  ─────────────────                          ────────────────             │
│  • Deterministic                            • Stochastic (non-determin.)│
│  • Predictable latency                      • Variable latency          │
│  • Fixed cost per request                   • Token-based pricing       │
│  • Small payloads                           • Multi-KB prompts          │
│  • Binary success/failure                   • Quality is subjective     │
│                                                                         │
│  NEW TELEMETRY REQUIREMENTS:                                            │
│  ───────────────────────────                                            │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ • Prompt/completion content (multi-KB)                          │   │
│  │ • Token usage (input/output/total)                              │   │
│  │ • Model parameters (temperature, top_p, etc.)                   │   │
│  │ • Tool/function calls                                           │   │
│  │ • Agent reasoning chains                                        │   │
│  │ • Retrieval context (RAG)                                       │   │
│  │ • Response quality metrics                                      │   │
│  │ • Cost per request                                              │   │
│  │ • Hallucination detection                                       │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 14.2 OpenTelemetry GenAI Semantic Conventions

OpenTelemetry has introduced **GenAI Semantic Conventions** (v1.37+) to standardize LLM observability.

#### Supported Signals

| Signal | Purpose | Status |
|--------|---------|--------|
| **Spans** | Model calls, agent invocations | Experimental |
| **Metrics** | Token usage, latency, cost | Experimental |
| **Events** | Prompt/completion content | Experimental |
| **Agent Spans** | Multi-agent workflows | Draft |

#### GenAI Span Attributes

```yaml
# Standard GenAI attributes (OTel Semantic Conventions)
gen_ai.system: "openai"                    # Provider (openai, anthropic, bedrock)
gen_ai.operation.name: "chat"              # Operation type
gen_ai.request.model: "gpt-4-turbo"        # Model requested
gen_ai.response.model: "gpt-4-turbo-2024"  # Model actually used
gen_ai.request.temperature: 0.7            # Temperature setting
gen_ai.request.top_p: 0.9                  # Top-p setting
gen_ai.request.max_tokens: 1000            # Max tokens requested

# Token usage
gen_ai.usage.input_tokens: 150             # Prompt tokens
gen_ai.usage.output_tokens: 500            # Completion tokens
gen_ai.usage.total_tokens: 650             # Total tokens

# Response metadata
gen_ai.response.finish_reason: "stop"      # Why generation stopped
gen_ai.response.id: "chatcmpl-abc123"      # Provider response ID

# Agent-specific (new in 2025)
gen_ai.agent.name: "research-agent"        # Agent identifier
gen_ai.agent.description: "Researches topics"
gen_ai.tool.name: "web_search"             # Tool being called
gen_ai.tool.call.id: "call_abc123"         # Tool call identifier
```

#### GenAI Metrics

```yaml
# Key metrics defined by OTel GenAI conventions
metrics:
  # Token usage
  gen_ai.client.token.usage:
    type: histogram
    unit: "{token}"
    attributes: [gen_ai.system, gen_ai.operation.name, gen_ai.token.type]

  # Request duration
  gen_ai.client.operation.duration:
    type: histogram
    unit: "s"
    attributes: [gen_ai.system, gen_ai.operation.name, gen_ai.request.model]

  # Time to first token (streaming)
  gen_ai.client.time_to_first_token:
    type: histogram
    unit: "s"

  # Time per output token (decode performance)
  gen_ai.client.time_per_output_token:
    type: histogram
    unit: "s"
```

#### GenAI Events (Prompt/Completion Logging)

```yaml
# Events capture large prompt/completion content
events:
  - name: gen_ai.content.prompt
    attributes:
      gen_ai.prompt: "User prompt text..."

  - name: gen_ai.content.completion
    attributes:
      gen_ai.completion: "Model response text..."

  - name: gen_ai.tool.message
    attributes:
      gen_ai.tool.name: "calculator"
      gen_ai.tool.call.arguments: '{"expression": "2+2"}'
      gen_ai.tool.call.result: "4"
```

### 14.3 OpenLLMetry - LLM Observability Framework

[OpenLLMetry](https://github.com/traceloop/openllmetry) extends OpenTelemetry for GenAI workloads.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    OPENLLMETRY ARCHITECTURE                             │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    YOUR LLM APPLICATION                          │   │
│  │                                                                  │   │
│  │  from traceloop.sdk import Traceloop                            │   │
│  │  Traceloop.init()  # Zero-code instrumentation                  │   │
│  │                                                                  │   │
│  │  # Automatic instrumentation for:                               │   │
│  │  # • OpenAI, Anthropic, Cohere, AWS Bedrock                    │   │
│  │  # • LangChain, LlamaIndex, Haystack                           │   │
│  │  # • ChromaDB, Pinecone, Weaviate (vector DBs)                 │   │
│  │  # • Transformers, vLLM                                        │   │
│  └──────────────────────────────┬──────────────────────────────────┘   │
│                                 │                                       │
│                                 │ OTLP                                  │
│                                 ▼                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    OPENLLMETRY HUB                               │   │
│  │                  (LLM Gateway + Telemetry)                       │   │
│  │                                                                  │   │
│  │  • Centralized LLM traffic routing                             │   │
│  │  • Standardized OTel spans                                      │   │
│  │  • Cost tracking across providers                              │   │
│  │  • Caching and rate limiting                                   │   │
│  └──────────────────────────────┬──────────────────────────────────┘   │
│                                 │                                       │
│                                 ▼                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    OBSERVABILITY BACKENDS                        │   │
│  │                                                                  │   │
│  │  • Traceloop (native)    • Datadog    • Grafana/Tempo          │   │
│  │  • Honeycomb             • New Relic  • Any OTLP backend       │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Installation & Usage

```python
# Install
pip install traceloop-sdk

# Initialize (zero-code)
from traceloop.sdk import Traceloop

Traceloop.init(
    app_name="my-llm-app",
    api_endpoint="http://otel-collector:4317",  # Or any OTLP endpoint
    disable_batch=False
)

# Your existing code - automatically instrumented
from openai import OpenAI
client = OpenAI()

response = client.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "Hello!"}]
)
# Spans automatically created with token usage, latency, etc.
```

#### Supported Frameworks

| Framework | Auto-Instrumented | Notes |
|-----------|-------------------|-------|
| **OpenAI** | ✅ | Chat, completions, embeddings |
| **Anthropic** | ✅ | Claude models |
| **AWS Bedrock** | ✅ | All Bedrock models |
| **Azure OpenAI** | ✅ | Full support |
| **Cohere** | ✅ | Chat, embeddings |
| **LangChain** | ✅ | Chains, agents, tools |
| **LlamaIndex** | ✅ | Queries, indices |
| **Haystack** | ✅ | Pipelines |
| **ChromaDB** | ✅ | Vector operations |
| **Pinecone** | ✅ | Vector operations |
| **Weaviate** | ✅ | Vector operations |

### 14.4 AI Agent Observability (2025 Standard)

With agentic AI becoming mainstream, OpenTelemetry is developing conventions for multi-agent systems.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    AI AGENT TRACE STRUCTURE                             │
│                                                                         │
│  User Request: "Research competitors and create a report"              │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Span: invoke_agent orchestrator                                  │   │
│  │ agent.name: "orchestrator"                                      │   │
│  │ agent.description: "Coordinates research workflow"              │   │
│  │                                                                  │   │
│  │  ┌───────────────────────────────────────────────────────────┐  │   │
│  │  │ Span: invoke_agent research-agent                         │  │   │
│  │  │ agent.name: "research-agent"                              │  │   │
│  │  │                                                           │  │   │
│  │  │  ┌─────────────────────────────────────────────────────┐ │  │   │
│  │  │  │ Span: chat gpt-4                                    │ │  │   │
│  │  │  │ gen_ai.request.model: "gpt-4"                       │ │  │   │
│  │  │  │ gen_ai.usage.total_tokens: 1500                     │ │  │   │
│  │  │  └─────────────────────────────────────────────────────┘ │  │   │
│  │  │                                                           │  │   │
│  │  │  ┌─────────────────────────────────────────────────────┐ │  │   │
│  │  │  │ Span: tool_call web_search                          │ │  │   │
│  │  │  │ gen_ai.tool.name: "web_search"                      │ │  │   │
│  │  │  │ gen_ai.tool.call.arguments: {"query": "..."}        │ │  │   │
│  │  │  └─────────────────────────────────────────────────────┘ │  │   │
│  │  └───────────────────────────────────────────────────────────┘  │   │
│  │                                                                  │   │
│  │  ┌───────────────────────────────────────────────────────────┐  │   │
│  │  │ Span: invoke_agent writer-agent                           │  │   │
│  │  │ agent.name: "writer-agent"                                │  │   │
│  │  │                                                           │  │   │
│  │  │  ┌─────────────────────────────────────────────────────┐ │  │   │
│  │  │  │ Span: chat claude-3-opus                            │ │  │   │
│  │  │  │ gen_ai.request.model: "claude-3-opus"               │ │  │   │
│  │  │  │ gen_ai.usage.total_tokens: 3000                     │ │  │   │
│  │  │  └─────────────────────────────────────────────────────┘ │  │   │
│  │  └───────────────────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Total: 4500 tokens, 3 LLM calls, 1 tool call, 2 agents               │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Agent Semantic Conventions (Draft)

```yaml
# Agent invocation span
span:
  name: "invoke_agent research-agent"
  kind: INTERNAL
  attributes:
    gen_ai.operation.name: "invoke_agent"
    gen_ai.agent.name: "research-agent"
    gen_ai.agent.description: "Researches topics using web search"
    gen_ai.agent.id: "agent-uuid-123"

    # Task context
    gen_ai.task.id: "task-uuid-456"
    gen_ai.task.description: "Find competitor information"

    # Memory/context
    gen_ai.memory.type: "conversation"
    gen_ai.memory.size: 5000  # tokens in context

    # Team/orchestration
    gen_ai.team.name: "research-team"
    gen_ai.orchestrator: "orchestrator-agent"
```

### 14.5 AIOps & AI-Driven Observability

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    AIOPS MATURITY MODEL (2025)                          │
│                                                                         │
│  Current State (Industry Survey):                                       │
│  ─────────────────────────────────                                      │
│  ┌───────────────────────────────────────────────────────────────┐     │
│  │ 4%  │ Fully operationalized AI across IT                      │     │
│  ├───────────────────────────────────────────────────────────────┤     │
│  │ 12% │ Automated root cause analysis & remediation             │     │
│  ├───────────────────────────────────────────────────────────────┤     │
│  │ 13% │ Anomaly detection & incident response                   │     │
│  ├───────────────────────────────────────────────────────────────┤     │
│  │ 49% │ Pilots and experiments in limited environments          │     │
│  ├───────────────────────────────────────────────────────────────┤     │
│  │ 22% │ Haven't started                                         │     │
│  └───────────────────────────────────────────────────────────────┘     │
│                                                                         │
│  AIOps Capabilities Evolution:                                         │
│  ─────────────────────────────                                          │
│                                                                         │
│  Level 1: REACTIVE                                                     │
│  ├─ Alert aggregation & deduplication                                  │
│  ├─ Basic correlation                                                  │
│  └─ Dashboard automation                                               │
│                                                                         │
│  Level 2: PREDICTIVE                                                   │
│  ├─ Anomaly detection (ML-based)                                       │
│  ├─ Capacity forecasting                                               │
│  ├─ Pattern recognition                                                │
│  └─ Intelligent alerting (reduce noise by 70-90%)                     │
│                                                                         │
│  Level 3: AUTONOMOUS (2025-2026 goal)                                  │
│  ├─ Automated root cause analysis                                      │
│  ├─ Self-healing actions                                               │
│  ├─ Proactive remediation                                              │
│  └─ Natural language queries ("Why is checkout slow?")                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### AIOps Use Cases with OpenTelemetry

```yaml
# 1. Anomaly Detection
aiops:
  anomaly_detection:
    signals: [metrics, traces]
    algorithms:
      - statistical: [z-score, mad, iqr]
      - ml: [isolation_forest, lstm, prophet]
    features:
      - latency_p99
      - error_rate
      - throughput
      - saturation

# 2. Root Cause Analysis
aiops:
  rca:
    input:
      - correlated traces
      - service dependencies
      - recent changes (deployments)
      - log patterns
    output:
      - probable_cause: "Database connection pool exhausted"
      - confidence: 0.85
      - evidence:
          - "db.pool.active increased 300%"
          - "db.query.duration p99 spike"
          - "Deployment: config change to pool_size"

# 3. Intelligent Alerting
aiops:
  smart_alerts:
    features:
      - alert_correlation: "Group related alerts"
      - noise_reduction: "Suppress during deployments"
      - dynamic_thresholds: "Adjust for time-of-day patterns"
      - blast_radius: "Estimate user impact"
```

### 14.6 LLM Observability Tools Comparison

| Tool | Type | OTel Native | Best For | Pricing |
|------|------|-------------|----------|---------|
| **OpenLLMetry/Traceloop** | OSS | ✅ | Open source LLM tracing | Free / Enterprise |
| **Arize Phoenix** | OSS | ✅ | LLM evaluation & debugging | Free |
| **LangSmith** | SaaS | ❌ | LangChain users | Freemium |
| **Weights & Biases** | SaaS | ⚠️ | ML experiment tracking | Freemium |
| **Helicone** | SaaS | ⚠️ | OpenAI proxy + analytics | Freemium |
| **PromptLayer** | SaaS | ❌ | Prompt management | Freemium |
| **Datadog LLM Obs** | SaaS | ✅ | Enterprise, full platform | $$$ |
| **Honeycomb** | SaaS | ✅ | High-cardinality debugging | $$ |
| **Langfuse** | OSS | ⚠️ | Self-hosted LLM tracing | Free |

### 14.7 Implementing GenAI Observability

#### Collector Configuration for LLM Telemetry

```yaml
# otel-collector-config.yaml for GenAI
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
        max_recv_msg_size_mib: 64  # Larger for prompts/completions

processors:
  batch:
    timeout: 1s
    send_batch_size: 512

  # Redact sensitive content from prompts
  transform:
    log_statements:
      - context: log
        statements:
          # Redact PII patterns in prompts
          - replace_pattern(attributes["gen_ai.prompt"], "\\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Z|a-z]{2,}\\b", "[EMAIL]")
          - replace_pattern(attributes["gen_ai.prompt"], "\\b\\d{3}-\\d{2}-\\d{4}\\b", "[SSN]")

  # Add cost calculation
  transform/cost:
    metric_statements:
      - context: datapoint
        statements:
          # Estimate cost based on model and tokens
          - set(attributes["estimated_cost_usd"],
              attributes["gen_ai.usage.total_tokens"] * 0.00003)
            where attributes["gen_ai.request.model"] == "gpt-4"

  # Filter high-volume, low-value LLM spans
  filter:
    spans:
      exclude:
        match_type: strict
        attributes:
          - key: gen_ai.operation.name
            value: "embeddings"  # Often very high volume

exporters:
  otlp:
    endpoint: backend:4317

  # Separate exporter for LLM traces (different retention)
  clickhouse/llm:
    endpoint: tcp://clickhouse:9000
    database: llm_traces
    ttl: 168h  # 7 days (LLM traces are large)

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch, transform]
      exporters: [otlp, clickhouse/llm]
```

#### Python Example with Manual Instrumentation

```python
from opentelemetry import trace
from opentelemetry.semconv.ai import GenAiAttributes

tracer = trace.get_tracer(__name__)

def call_llm(prompt: str, model: str = "gpt-4"):
    with tracer.start_as_current_span(
        f"chat {model}",
        kind=trace.SpanKind.CLIENT
    ) as span:
        # Set GenAI attributes
        span.set_attribute(GenAiAttributes.GEN_AI_SYSTEM, "openai")
        span.set_attribute(GenAiAttributes.GEN_AI_OPERATION_NAME, "chat")
        span.set_attribute(GenAiAttributes.GEN_AI_REQUEST_MODEL, model)
        span.set_attribute(GenAiAttributes.GEN_AI_REQUEST_TEMPERATURE, 0.7)

        # Make the actual call
        response = openai_client.chat.completions.create(
            model=model,
            messages=[{"role": "user", "content": prompt}]
        )

        # Record response attributes
        span.set_attribute(GenAiAttributes.GEN_AI_RESPONSE_MODEL, response.model)
        span.set_attribute(GenAiAttributes.GEN_AI_USAGE_INPUT_TOKENS,
                          response.usage.prompt_tokens)
        span.set_attribute(GenAiAttributes.GEN_AI_USAGE_OUTPUT_TOKENS,
                          response.usage.completion_tokens)
        span.set_attribute(GenAiAttributes.GEN_AI_RESPONSE_FINISH_REASON,
                          response.choices[0].finish_reason)

        # Log prompt/completion as events (optional, large payloads)
        span.add_event("gen_ai.content.prompt", {"gen_ai.prompt": prompt})
        span.add_event("gen_ai.content.completion",
                      {"gen_ai.completion": response.choices[0].message.content})

        return response
```

### 14.8 Key Trends for 2025-2026

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    OBSERVABILITY TRENDS 2025-2026                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. OPENTELEMETRY DOMINANCE                                            │
│     • Production adoption: 6% (2025) → 11% (2026)                      │
│     • De facto standard for telemetry                                  │
│     • Profiling signal added (2024), stabilizing (2025)                │
│                                                                         │
│  2. GENAI/LLM OBSERVABILITY                                            │
│     • GenAI semantic conventions maturing                              │
│     • Token-based cost tracking essential                              │
│     • Agent observability standards emerging                           │
│     • Multi-modal (text, image, audio) telemetry                      │
│                                                                         │
│  3. AGENTIC AI IN OBSERVABILITY                                        │
│     • Natural language queries ("Why is service X slow?")              │
│     • Automated root cause analysis                                    │
│     • Self-healing infrastructure (controlled settings)                │
│     • AI-assisted incident response                                    │
│                                                                         │
│  4. UNIFIED PLATFORMS                                                   │
│     • 84% pursuing unified observability                               │
│     • Traces + Metrics + Logs + Profiles in one place                 │
│     • Correlation across all signals                                   │
│                                                                         │
│  5. SHIFT-LEFT OBSERVABILITY                                           │
│     • Observability-Driven Development (ODD)                           │
│     • Pre-production testing with telemetry                            │
│     • SLOs defined before deployment                                   │
│                                                                         │
│  6. EDGE & IOT OBSERVABILITY                                           │
│     • Managing telemetry from millions of edge nodes                   │
│     • Latency-aware collection                                         │
│     • Resource-constrained environments                                │
│                                                                         │
│  7. COST OPTIMIZATION                                                   │
│     • Intelligent sampling (keep what matters)                         │
│     • Data tiering (hot/warm/cold)                                     │
│     • FinOps for observability                                         │
│                                                                         │
│  8. SECURITY OBSERVABILITY                                              │
│     • Runtime threat detection                                         │
│     • Compliance monitoring                                            │
│     • Security signal integration                                      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 15. Metrics-Traces Correlation (Exemplars)

### 15.1 The Correlation Challenge

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    THE THREE PILLARS PROBLEM                            │
│                                                                         │
│  Traditional Approach (Disconnected):                                   │
│  ─────────────────────────────────────                                  │
│                                                                         │
│  METRICS              TRACES                LOGS                        │
│  ────────             ──────                ────                        │
│  "P99 latency         "Trace abc123         "Error in                  │
│   spiked to 5s"        took 5.2s"            payment service"          │
│       │                    │                     │                      │
│       │                    │                     │                      │
│       ▼                    ▼                     ▼                      │
│  ┌─────────┐          ┌─────────┐          ┌─────────┐                 │
│  │ Which   │          │ Is this │          │ Which   │                 │
│  │ request?│          │ related?│          │ trace?  │                 │
│  └─────────┘          └─────────┘          └─────────┘                 │
│                                                                         │
│  ═══════════════════════════════════════════════════════════════════   │
│                                                                         │
│  OpenTelemetry Approach (Correlated):                                   │
│  ─────────────────────────────────────                                  │
│                                                                         │
│  METRICS ─────────── EXEMPLARS ─────────── TRACES                      │
│     │                    │                    │                         │
│     │                    │                    │                         │
│     └────────────────────┼────────────────────┘                         │
│                          │                                              │
│                          ▼                                              │
│                    ┌───────────┐                                        │
│                    │ trace_id  │                                        │
│                    │ span_id   │                                        │
│                    └───────────┘                                        │
│                          │                                              │
│                          ▼                                              │
│  LOGS ──────────────────────────────────────────────────               │
│  (Also contain trace_id, span_id)                                      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 15.2 What are Exemplars?

**Exemplars** are sample data points attached to metrics that include trace context, allowing you to jump from a metric directly to a representative trace.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    EXEMPLAR STRUCTURE                                    │
│                                                                         │
│  Histogram Metric: http_request_duration_seconds                       │
│  ───────────────────────────────────────────────                        │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Bucket: le="0.5"  count=1000                                    │   │
│  │ Bucket: le="1.0"  count=1500                                    │   │
│  │ Bucket: le="2.0"  count=1800                                    │   │
│  │ Bucket: le="5.0"  count=1850  ◄─── EXEMPLAR ATTACHED           │   │
│  │                                     │                           │   │
│  │                         ┌───────────┴───────────┐               │   │
│  │                         │ Exemplar:             │               │   │
│  │                         │   value: 4.2s         │               │   │
│  │                         │   timestamp: ...      │               │   │
│  │                         │   trace_id: abc123    │◄── Link to   │   │
│  │                         │   span_id: def456     │    trace!    │   │
│  │                         │   labels:             │               │   │
│  │                         │     user_id: "42"     │               │   │
│  │                         └───────────────────────┘               │   │
│  │ Bucket: le="+Inf" count=1850                                    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  USE CASE:                                                             │
│  "P99 latency is 4.5s" → Click exemplar → See exact slow trace        │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 15.3 Enabling Exemplars

#### Java (Auto-Instrumentation)

```bash
# Exemplars are enabled by default in Java agent
# Just ensure traces and metrics go to same collector

OTEL_METRICS_EXPORTER=otlp
OTEL_TRACES_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4317
```

#### Python

```python
from opentelemetry import metrics, trace
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
from opentelemetry.sdk.metrics.view import ExplicitBucketHistogramAggregation, View

# Configure histogram with exemplars
view = View(
    instrument_name="http.server.duration",
    aggregation=ExplicitBucketHistogramAggregation(
        boundaries=[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
    )
)

# Exemplars automatically captured when recording within a span context
meter = metrics.get_meter(__name__)
histogram = meter.create_histogram("http.server.duration", unit="s")

# When this runs inside a traced request, exemplar is attached
with tracer.start_as_current_span("handle_request"):
    # ... handle request ...
    histogram.record(response_time)  # Exemplar with trace_id attached!
```

#### .NET

```csharp
// Exemplars enabled by default in .NET 8+
// Configure in Program.cs
builder.Services.AddOpenTelemetry()
    .WithMetrics(metrics => metrics
        .AddAspNetCoreInstrumentation()
        .AddOtlpExporter(options =>
        {
            options.ExportProcessorType = ExportProcessorType.Simple;
            options.Protocol = OtlpExportProtocol.Grpc;
        })
    );
```

#### Collector Configuration

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

exporters:
  # Prometheus with exemplars
  prometheus:
    endpoint: 0.0.0.0:8889
    enable_open_metrics: true  # Required for exemplars!

  # OTLP preserves exemplars natively
  otlp:
    endpoint: tempo:4317

service:
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [prometheus, otlp]
```

### 15.4 Querying Correlated Data

#### Prometheus + Grafana

```promql
# Query histogram with exemplars
histogram_quantile(0.99,
  sum(rate(http_request_duration_seconds_bucket[5m])) by (le, service)
)

# In Grafana:
# 1. Enable "Exemplars" toggle in query options
# 2. Configure Tempo as trace datasource
# 3. Click on exemplar dots to jump to trace
```

#### ClickHouse

```sql
-- Find metrics with their exemplar traces
SELECT
    m.MetricName,
    m.Value,
    m.Timestamp,
    arrayJoin(m.Exemplars).TraceId as ExemplarTraceId,
    arrayJoin(m.Exemplars).SpanId as ExemplarSpanId,
    arrayJoin(m.Exemplars).Value as ExemplarValue
FROM otel_metrics m
WHERE m.MetricName = 'http_server_duration'
  AND m.Timestamp > now() - INTERVAL 1 HOUR
  AND length(m.Exemplars) > 0;

-- Join with traces to get full context
SELECT
    m.MetricName,
    m.Value as MetricValue,
    t.TraceId,
    t.SpanName,
    t.Duration / 1000000 as DurationMs,
    t.StatusCode
FROM otel_metrics m
ARRAY JOIN m.Exemplars as e
JOIN otel_traces t ON e.TraceId = t.TraceId
WHERE m.MetricName = 'http_server_duration'
  AND m.Value > 1000  -- Slow requests
ORDER BY m.Timestamp DESC
LIMIT 100;
```

### 15.5 Correlation Methods Summary

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    CORRELATION METHODS                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  METHOD 1: EXEMPLARS (Metrics → Traces)                                │
│  ──────────────────────────────────────                                 │
│  • Attach trace_id/span_id to metric data points                       │
│  • Best for: "This P99 spike - show me an example trace"               │
│  • Support: Prometheus, OTLP, Grafana                                  │
│                                                                         │
│  METHOD 2: TRACE CONTEXT IN LOGS (Logs → Traces)                       │
│  ───────────────────────────────────────────────                        │
│  • Include trace_id/span_id in every log line                          │
│  • Best for: "Show me logs for this trace"                             │
│  • Support: All languages via MDC/context                              │
│                                                                         │
│  METHOD 3: RESOURCE ATTRIBUTES (All Signals)                           │
│  ───────────────────────────────────────────                            │
│  • Same service.name, k8s.pod.name across all signals                  │
│  • Best for: "Show me everything for this service"                     │
│  • Support: Universal in OTel                                          │
│                                                                         │
│  METHOD 4: SPAN EVENTS (Traces → Logs)                                 │
│  ─────────────────────────────────────                                  │
│  • Embed log-like events directly in spans                             │
│  • Best for: Keeping logs with their trace context                     │
│  • Support: All OTel SDKs                                              │
│                                                                         │
│  METHOD 5: BAGGAGE (Cross-Service Context)                             │
│  ─────────────────────────────────────────                              │
│  • Propagate custom key-values across service boundaries               │
│  • Best for: Business context (user_id, tenant_id)                     │
│  • Support: W3C Baggage standard                                       │
│                                                                         │
│  METHOD 6: SPAN LINKS (Trace → Trace)                                  │
│  ────────────────────────────────────                                   │
│  • Connect related but not parent-child traces                         │
│  • Best for: Async workflows, batch processing                         │
│  • Support: All OTel SDKs                                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 15.6 Baggage for Business Context

```python
from opentelemetry import baggage
from opentelemetry.propagate import set_global_textmap
from opentelemetry.propagators.composite import CompositePropagator
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator
from opentelemetry.baggage.propagation import W3CBaggagePropagator

# Enable baggage propagation
set_global_textmap(CompositePropagator([
    TraceContextTextMapPropagator(),
    W3CBaggagePropagator()
]))

# Set baggage (propagates to all downstream services)
ctx = baggage.set_baggage("user.id", "user-12345")
ctx = baggage.set_baggage("tenant.id", "tenant-abc", context=ctx)
ctx = baggage.set_baggage("feature.flags", "new-checkout,dark-mode", context=ctx)

# In downstream service, read baggage
user_id = baggage.get_baggage("user.id")
tenant_id = baggage.get_baggage("tenant.id")

# Add to spans as attributes
span.set_attribute("user.id", user_id)
span.set_attribute("tenant.id", tenant_id)
```

### 15.7 Span Links for Async Correlation

```python
from opentelemetry import trace

tracer = trace.get_tracer(__name__)

# Producer: Create message and capture span context
with tracer.start_as_current_span("produce_message") as producer_span:
    message = {
        "data": "...",
        "trace_context": {
            "trace_id": producer_span.get_span_context().trace_id,
            "span_id": producer_span.get_span_context().span_id
        }
    }
    queue.send(message)

# Consumer: Link back to producer span
def consume_message(message):
    # Create link to producer span
    producer_context = trace.SpanContext(
        trace_id=message["trace_context"]["trace_id"],
        span_id=message["trace_context"]["span_id"],
        is_remote=True,
        trace_flags=trace.TraceFlags(0x01)
    )
    link = trace.Link(producer_context)

    # Start new trace but link to producer
    with tracer.start_as_current_span(
        "consume_message",
        links=[link]  # Link to producer!
    ) as consumer_span:
        process(message)
```

---

## 16. Glossary & Executive Summary

### 16.1 OpenTelemetry Terminology

#### Core Concepts

| Term | Definition | Analogy |
|------|------------|---------|
| **Telemetry** | Data about system behavior (traces, metrics, logs) | A car's dashboard and diagnostic data |
| **Instrumentation** | Code that generates telemetry | Installing sensors in the car |
| **Signal** | A type of telemetry (trace, metric, log, profile) | Different dashboard gauges |
| **OTLP** | OpenTelemetry Protocol - standard data format | Universal language for telemetry |
| **Collector** | Service that receives, processes, and exports telemetry | Mail sorting facility |
| **Exporter** | Component that sends data to backends | Delivery truck |
| **Receiver** | Component that ingests data | Loading dock |
| **Processor** | Component that transforms data | Packaging/sorting |

#### Tracing Terms

| Term | Definition | Example |
|------|------------|---------|
| **Trace** | End-to-end journey of a request across services | A package's journey from warehouse to doorstep |
| **Span** | Single operation within a trace | One leg of the journey (warehouse → truck) |
| **Trace ID** | Unique identifier for entire trace (128-bit) | Tracking number |
| **Span ID** | Unique identifier for a span (64-bit) | Leg-specific ID |
| **Parent Span** | The span that initiated current span | Previous stop in journey |
| **Root Span** | First span in a trace (no parent) | Origin warehouse |
| **Span Kind** | Type of span (CLIENT, SERVER, PRODUCER, CONSUMER, INTERNAL) | Type of transportation |
| **Context Propagation** | Passing trace context between services | Passing tracking number between carriers |
| **Distributed Trace** | Trace spanning multiple services | Multi-carrier shipment |

#### Metrics Terms

| Term | Definition | Example |
|------|------------|---------|
| **Metric** | Numerical measurement over time | CPU usage, request count |
| **Counter** | Monotonically increasing value | Total requests served |
| **Gauge** | Value that can go up or down | Current memory usage |
| **Histogram** | Distribution of values | Request latency distribution |
| **Exemplar** | Sample trace linked to metric data point | "This 5s request" |
| **Cardinality** | Number of unique label combinations | Low (good) vs High (expensive) |
| **Temporality** | How values accumulate (cumulative/delta) | Running total vs per-interval |

#### Logs Terms

| Term | Definition | Example |
|------|------------|---------|
| **Log Record** | Single log entry with attributes | `{"level": "ERROR", "message": "..."}` |
| **Severity** | Log level (TRACE, DEBUG, INFO, WARN, ERROR, FATAL) | Importance indicator |
| **Log Correlation** | Linking logs to traces via trace_id | Finding logs for a specific request |
| **Structured Logging** | Logs as key-value pairs (JSON) | Machine-parseable format |

#### Resource & Attributes

| Term | Definition | Example |
|------|------------|---------|
| **Resource** | Entity producing telemetry | Service, host, container |
| **Resource Attributes** | Metadata about the resource | `service.name`, `k8s.pod.name` |
| **Span Attributes** | Metadata about the operation | `http.method`, `db.statement` |
| **Semantic Conventions** | Standard attribute names | `http.status_code` not `statusCode` |

### 16.2 Architecture Patterns

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    KEY ARCHITECTURE PATTERNS                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  PATTERN: SIDECAR                                                       │
│  ────────────────────                                                   │
│  ┌─────────────────────────────┐                                       │
│  │ Pod                         │                                       │
│  │ ┌─────────┐  ┌───────────┐ │                                       │
│  │ │   App   │──│ Collector │─┼──▶ Backend                            │
│  │ └─────────┘  └───────────┘ │                                       │
│  └─────────────────────────────┘                                       │
│  Use: High isolation, per-pod config                                   │
│                                                                         │
│  PATTERN: DAEMONSET (AGENT)                                            │
│  ──────────────────────────────                                         │
│  ┌──────────────────────────────────────────┐                          │
│  │ Node                                      │                          │
│  │ ┌─────┐ ┌─────┐ ┌─────┐                 │                          │
│  │ │App 1│ │App 2│ │App 3│                 │                          │
│  │ └──┬──┘ └──┬──┘ └──┬──┘                 │                          │
│  │    └───────┼───────┘                     │                          │
│  │            ▼                             │                          │
│  │    ┌───────────────┐                     │                          │
│  │    │   Collector   │─────────────────────┼──▶ Backend              │
│  │    │  (DaemonSet)  │                     │                          │
│  │    └───────────────┘                     │                          │
│  └──────────────────────────────────────────┘                          │
│  Use: Node-level metrics, efficient resource use                       │
│                                                                         │
│  PATTERN: GATEWAY                                                       │
│  ────────────────────                                                   │
│  ┌─────┐ ┌─────┐ ┌─────┐                                              │
│  │App 1│ │App 2│ │App 3│                                              │
│  └──┬──┘ └──┬──┘ └──┬──┘                                              │
│     └───────┼───────┘                                                  │
│             ▼                                                          │
│     ┌───────────────┐                                                  │
│     │    Gateway    │                                                  │
│     │   Collector   │──▶ Backend A                                    │
│     │   (HA, LB)    │──▶ Backend B                                    │
│     └───────────────┘                                                  │
│  Use: Central aggregation, multi-backend routing                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 16.3 Sampling Strategies Explained

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    SAMPLING STRATEGIES                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  HEAD-BASED SAMPLING                                                    │
│  ───────────────────                                                    │
│  Decision made at START of trace                                       │
│                                                                         │
│  Request ──▶ [Sample? 10%] ──▶ Yes: Trace entire request              │
│                           └──▶ No: Don't trace                         │
│                                                                         │
│  PROS: Simple, low overhead                                            │
│  CONS: May miss important traces (errors discovered later)             │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  TAIL-BASED SAMPLING                                                    │
│  ───────────────────                                                    │
│  Decision made at END of trace (in collector)                          │
│                                                                         │
│  Request ──▶ [Collect ALL spans] ──▶ [Analyze complete trace]          │
│                                              │                          │
│                                              ▼                          │
│                                    ┌─────────────────┐                 │
│                                    │ Keep if:        │                 │
│                                    │ • Has error     │                 │
│                                    │ • Slow (>1s)    │                 │
│                                    │ • 10% random    │                 │
│                                    └─────────────────┘                 │
│                                                                         │
│  PROS: Keeps important traces (errors, slow)                           │
│  CONS: Higher resource usage, needs collector buffering                │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  ADAPTIVE SAMPLING                                                      │
│  ─────────────────                                                      │
│  Adjusts rate based on traffic volume                                  │
│                                                                         │
│  Low traffic (100 req/s)  ──▶ 100% sampling                           │
│  High traffic (10K req/s) ──▶ 1% sampling                             │
│                                                                         │
│  PROS: Cost-effective at scale, good coverage during incidents         │
│  CONS: Complex configuration                                           │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 16.4 Executive Summary for Leadership

#### What is OpenTelemetry?

> **OpenTelemetry (OTel)** is an open-source, vendor-neutral standard for collecting application telemetry (traces, metrics, logs). It's backed by the CNCF and supported by all major cloud providers and observability vendors.

#### Why Does It Matter?

| Benefit | Business Impact |
|---------|-----------------|
| **Vendor Independence** | No lock-in, negotiate better contracts, switch backends freely |
| **Reduced Costs** | One instrumentation works everywhere, no duplicate agents |
| **Faster Debugging** | Correlated data = faster MTTR = less downtime |
| **Standard Skills** | Engineers learn one system, applicable across all projects |
| **Future-Proof** | Industry standard with growing adoption (6% → 11% in production) |

#### Key Metrics for Management

| Metric | What It Measures | Target |
|--------|------------------|--------|
| **MTTD** | Mean Time to Detect issues | < 5 minutes |
| **MTTR** | Mean Time to Resolve | < 30 minutes |
| **Service Coverage** | % of services instrumented | > 95% |
| **Trace Success Rate** | % of requests with complete traces | > 99% |
| **Data Volume** | GB/day of telemetry | Monitor for cost |
| **Sampling Efficiency** | % of important traces captured | > 99% of errors |

#### Cost Considerations

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    COST FACTORS                                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  DATA VOLUME DRIVERS:                                                   │
│  • Number of services                                                   │
│  • Requests per second                                                  │
│  • Sampling rate                                                        │
│  • Attribute richness                                                   │
│  • Retention period                                                     │
│                                                                         │
│  COST OPTIMIZATION LEVERS:                                              │
│  ┌─────────────────────┬──────────────────────┬────────────────────┐   │
│  │ Lever               │ Typical Reduction    │ Trade-off          │   │
│  ├─────────────────────┼──────────────────────┼────────────────────┤   │
│  │ Tail-based sampling │ 80-95%               │ Collector overhead │   │
│  │ Filter health checks│ 10-30%               │ None               │   │
│  │ Reduce retention    │ 50-80%               │ Less history       │   │
│  │ Compress data       │ 30-50%               │ CPU overhead       │   │
│  │ Drop debug logs     │ 40-60%               │ Less detail        │   │
│  └─────────────────────┴──────────────────────┴────────────────────┘   │
│                                                                         │
│  ROI CALCULATION:                                                       │
│  • Average incident cost: $5,000 - $500,000+                           │
│  • MTTR reduction: 30-50% with good observability                      │
│  • Developer productivity: 10-20% improvement                          │
│  • Typical payback: 3-6 months                                         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 16.5 Common Acronyms

| Acronym | Full Form | Meaning |
|---------|-----------|---------|
| **OTel** | OpenTelemetry | The observability framework |
| **OTLP** | OpenTelemetry Protocol | Standard data transmission protocol |
| **APM** | Application Performance Monitoring | Monitoring application health |
| **RUM** | Real User Monitoring | Monitoring from browser/mobile |
| **SLO** | Service Level Objective | Target reliability (99.9%) |
| **SLI** | Service Level Indicator | Metric measuring SLO |
| **SLA** | Service Level Agreement | Contractual commitment |
| **MTTD** | Mean Time to Detect | How fast issues are discovered |
| **MTTR** | Mean Time to Resolve | How fast issues are fixed |
| **P50/P95/P99** | Percentiles | 50th/95th/99th percentile of latency |
| **RED** | Rate, Errors, Duration | Key metrics for services |
| **USE** | Utilization, Saturation, Errors | Key metrics for resources |
| **W3C** | World Wide Web Consortium | Standards body (Trace Context) |
| **CNCF** | Cloud Native Computing Foundation | OTel's parent organization |
| **OpAMP** | Open Agent Management Protocol | Remote agent management |
| **eBPF** | Extended Berkeley Packet Filter | Kernel-level instrumentation |

### 16.6 The Four Golden Signals

Google SRE's recommended metrics for any service:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    FOUR GOLDEN SIGNALS                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. LATENCY                                                             │
│     ─────────                                                           │
│     Time to service a request                                          │
│     Key: Distinguish successful vs failed request latency              │
│     Metrics: P50, P95, P99 response times                             │
│                                                                         │
│  2. TRAFFIC                                                             │
│     ─────────                                                           │
│     Demand on the system                                               │
│     Key: Requests per second, concurrent users                         │
│     Metrics: HTTP requests/sec, transactions/sec                       │
│                                                                         │
│  3. ERRORS                                                              │
│     ─────────                                                           │
│     Rate of failed requests                                            │
│     Key: Explicit (500s) and implicit (wrong content, slow)            │
│     Metrics: Error rate %, error count by type                         │
│                                                                         │
│  4. SATURATION                                                          │
│     ─────────────                                                       │
│     How "full" the service is                                          │
│     Key: Utilization of constrained resources                          │
│     Metrics: CPU %, memory %, queue depth, thread pool                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 16.7 RED vs USE Methods

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    RED vs USE METHODS                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  RED METHOD (For Services)                   USE METHOD (For Resources) │
│  ─────────────────────────                   ─────────────────────────  │
│                                                                         │
│  R - Request Rate                            U - Utilization            │
│      (requests/second)                           (% time busy)          │
│                                                                         │
│  E - Error Rate                              S - Saturation             │
│      (failed requests/second)                    (queue depth)          │
│                                                                         │
│  D - Duration                                E - Errors                 │
│      (latency distribution)                      (error count)          │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  WHEN TO USE:                                                          │
│                                                                         │
│  RED: API gateways, microservices, web servers                        │
│       "How is my service performing for users?"                        │
│                                                                         │
│  USE: CPUs, memory, disks, network, thread pools                      │
│       "Are my resources healthy and sufficient?"                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 17. Where Things Go Wrong - Failure Modes & Pitfalls

### 17.1 Common Instrumentation Failures

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    INSTRUMENTATION FAILURE MODES                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  FAILURE 1: MISSING TRACES (Silent Gaps)                               │
│  ───────────────────────────────────────                                │
│                                                                         │
│  Symptom: Traces start at Service B, not Service A                     │
│                                                                         │
│  ┌─────────┐     ┌─────────┐     ┌─────────┐                          │
│  │Service A│────▶│ Gateway │────▶│Service B│                          │
│  │ (No SDK)│     │(No prop)│     │(Traced) │                          │
│  └─────────┘     └─────────┘     └─────────┘                          │
│       ❌              ❌              ✅                                │
│                                                                         │
│  Causes:                                                               │
│  • Service not instrumented                                            │
│  • Proxy/gateway stripping trace headers                               │
│  • Custom HTTP client not instrumented                                 │
│  • Async messaging without context propagation                         │
│                                                                         │
│  Fix:                                                                  │
│  • Verify ALL services have instrumentation                            │
│  • Check proxy configs (nginx, envoy, AWS ALB)                        │
│  • Ensure message queues propagate trace context                       │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  FAILURE 2: BROKEN TRACE CONTEXT                                       │
│  ───────────────────────────────                                        │
│                                                                         │
│  Symptom: Multiple disconnected traces for single request              │
│                                                                         │
│  Expected: A ──▶ B ──▶ C (one trace)                                  │
│  Actual:   A (trace1)  B (trace2)  C (trace3)                         │
│                                                                         │
│  Causes:                                                               │
│  • New trace started instead of continuing                             │
│  • Thread context lost in async code                                   │
│  • Incompatible propagator (B3 vs W3C)                                │
│  • Manual HTTP calls without injecting headers                         │
│                                                                         │
│  Fix:                                                                  │
│  • Use consistent propagators across all services                      │
│  • Properly pass context in async/reactive code                        │
│  • Verify headers: traceparent, tracestate present                    │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  FAILURE 3: AGENT NOT LOADING                                          │
│  ────────────────────────────                                           │
│                                                                         │
│  Symptom: No telemetry despite configuration                           │
│                                                                         │
│  Java:                                                                 │
│  • Wrong JAVA_TOOL_OPTIONS syntax                                      │
│  • Agent JAR not found in container                                    │
│  • JVM version incompatibility                                         │
│                                                                         │
│  Python:                                                               │
│  • opentelemetry-instrument not in PATH                                │
│  • Missing auto-instrumentation packages                               │
│  • Conflicting package versions                                        │
│                                                                         │
│  .NET:                                                                 │
│  • CORECLR_PROFILER_PATH wrong for architecture                        │
│  • Missing startup hooks                                               │
│  • .NET version mismatch                                               │
│                                                                         │
│  Debug:                                                                │
│  $ kubectl exec -it pod -- env | grep OTEL                            │
│  $ kubectl logs pod | grep -i opentelemetry                           │
│  $ kubectl exec -it pod -- ls -la /otel/                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 17.2 Collector Failure Modes

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    COLLECTOR FAILURES                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  FAILURE 1: OOM (Out of Memory)                                        │
│  ──────────────────────────────                                         │
│                                                                         │
│  Symptom: Collector pods restarting, OOMKilled                         │
│                                                                         │
│  Timeline:                                                             │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │ Traffic spike → Queue grows → Memory exhausted → OOM → Restart │   │
│  │                                   ↓                            │   │
│  │                            DATA LOSS!                          │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Prevention:                                                           │
│  processors:                                                           │
│    memory_limiter:                                                     │
│      check_interval: 1s                                                │
│      limit_mib: 1800        # Hard limit                              │
│      spike_limit_mib: 500   # Burst allowance                         │
│      # When limit hit, collector applies backpressure                 │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  FAILURE 2: EXPORT FAILURES (Backend Down)                             │
│  ─────────────────────────────────────────                              │
│                                                                         │
│  Symptom: Exporter errors in logs, data not reaching backend          │
│                                                                         │
│  Error: "error exporting: connection refused"                          │
│  Error: "context deadline exceeded"                                    │
│  Error: "429 Too Many Requests"                                        │
│                                                                         │
│  Prevention:                                                           │
│  exporters:                                                            │
│    otlp:                                                               │
│      endpoint: backend:4317                                            │
│      retry_on_failure:                                                 │
│        enabled: true                                                   │
│        initial_interval: 5s                                            │
│        max_interval: 30s                                               │
│        max_elapsed_time: 300s                                          │
│      sending_queue:                                                    │
│        enabled: true                                                   │
│        num_consumers: 10                                               │
│        queue_size: 5000                                                │
│        storage: file_storage  # Persist across restarts!              │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  FAILURE 3: DATA LOSS DURING ROLLOUT                                   │
│  ───────────────────────────────────                                    │
│                                                                         │
│  Symptom: Gap in telemetry during collector upgrade                    │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │ Timeline:                                                        │ │
│  │ ─────────────────────────────────────────────────────────────── │ │
│  │ |...data...|  GAP  |...data...|                                 │ │
│  │             ↑      ↑                                            │ │
│  │          Old pod  New pod                                       │ │
│  │          killed   ready                                         │ │
│  └──────────────────────────────────────────────────────────────────┘ │
│                                                                         │
│  Prevention:                                                           │
│  • Use persistent queue (survives restarts)                           │
│  • Configure PodDisruptionBudget                                       │
│  • Rolling update with maxUnavailable: 0                              │
│  • Pre-stop hook to drain queue                                        │
│                                                                         │
│  spec:                                                                 │
│    containers:                                                         │
│    - lifecycle:                                                        │
│        preStop:                                                        │
│          exec:                                                         │
│            command: ["/bin/sh", "-c", "sleep 30"]  # Drain time       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 17.3 Cardinality Explosions

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    CARDINALITY EXPLOSION                                 │
│                    (The Silent Killer)                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  WHAT HAPPENS:                                                          │
│  ─────────────                                                          │
│                                                                         │
│  Day 1:   http_requests{path="/api/users"} → 10 time series           │
│  Day 30:  http_requests{path="/api/users/12345"} → 1M time series!    │
│                                                                         │
│  Result:                                                               │
│  • Prometheus OOM                                                      │
│  • Query timeouts                                                      │
│  • Storage costs explode                                               │
│  • Dashboards unusable                                                 │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  COMMON CARDINALITY BOMBS:                                             │
│  ─────────────────────────                                              │
│                                                                         │
│  ❌ user_id as label         → Millions of values                      │
│  ❌ request_id as label      → Infinite (unique per request)           │
│  ❌ Full URL path            → /users/123, /users/456, ...             │
│  ❌ timestamp as label       → New series every second                 │
│  ❌ pod_ip as label          → Changes on every restart                │
│  ❌ error_message as label   → Thousands of unique messages            │
│  ❌ session_id as label      → Millions of sessions                    │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  DETECTION:                                                            │
│  ──────────                                                             │
│                                                                         │
│  # Prometheus: Find high cardinality metrics                          │
│  topk(10, count by (__name__)({__name__=~".+"}))                      │
│                                                                         │
│  # Find labels with most values                                        │
│  count(http_requests_total) by (path)                                 │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  PREVENTION:                                                           │
│  ───────────                                                            │
│                                                                         │
│  1. Path normalization in collector:                                   │
│     processors:                                                        │
│       transform:                                                       │
│         metric_statements:                                             │
│           - context: datapoint                                         │
│             statements:                                                │
│               - replace_pattern(attributes["http.route"],             │
│                   "/users/[0-9]+", "/users/{id}")                     │
│               - replace_pattern(attributes["http.route"],             │
│                   "/orders/[0-9]+", "/orders/{id}")                   │
│                                                                         │
│  2. Drop high-cardinality attributes:                                  │
│     processors:                                                        │
│       attributes:                                                      │
│         actions:                                                       │
│           - key: user.id                                               │
│             action: delete                                             │
│           - key: request.id                                            │
│             action: delete                                             │
│                                                                         │
│  3. Use histograms instead of per-request metrics                     │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 17.4 Clock Skew & Time Issues

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    CLOCK SKEW PROBLEMS                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  SYMPTOM: Traces look impossible                                       │
│  ────────────────────────────────                                       │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Service A (clock: 10:00:00)                                     │   │
│  │    └──▶ calls Service B (clock: 09:59:50)                       │   │
│  │              │                                                   │   │
│  │              ▼                                                   │   │
│  │         Child span appears to START BEFORE parent!              │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Causes:                                                               │
│  • VMs without NTP synchronization                                    │
│  • Containers with wrong timezone                                     │
│  • Clock drift over time                                              │
│  • Virtualization clock issues                                        │
│                                                                         │
│  Impact:                                                               │
│  • Trace visualization broken                                         │
│  • Latency calculations wrong (negative duration!)                    │
│  • Log ordering incorrect                                             │
│  • Alert timing off                                                   │
│                                                                         │
│  Prevention:                                                           │
│  • Enable NTP on all hosts: chronyd or ntpd                          │
│  • Kubernetes: Use hostPath for /etc/localtime                        │
│  • Monitor clock skew: node_timex_offset_seconds                     │
│  • Alert if skew > 100ms                                             │
│                                                                         │
│  # Check clock sync                                                    │
│  $ chronyc tracking                                                   │
│  $ timedatectl status                                                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 18. Production War Stories & Lessons Learned

### 18.1 Real Incident Patterns

```
┌─────────────────────────────────────────────────────────────────────────┐
│  WAR STORY 1: THE MISSING 5% OF TRACES                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Situation:                                                            │
│  • 95% of requests had complete traces                                 │
│  • 5% had orphan spans (no root)                                       │
│  • Debugging took 2 weeks                                              │
│                                                                         │
│  Root Cause:                                                           │
│  • AWS Application Load Balancer was STRIPPING traceparent header     │
│  • Only happened when request hit a specific target group             │
│  • ALB rule was doing a redirect that dropped headers                 │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Client ──▶ ALB ──▶ [Redirect Rule] ──▶ Service                  │   │
│  │               traceparent: abc123       traceparent: (missing)  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Fix:                                                                  │
│  • Modified ALB redirect to preserve headers                          │
│  • Added monitoring for orphan span rate                              │
│                                                                         │
│  Lesson:                                                               │
│  • Always verify trace context through EVERY hop                      │
│  • Monitor orphan span percentage as KPI                              │
│  • Test trace propagation in staging before production                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│  WAR STORY 2: OBSERVABILITY CAUSING THE OUTAGE                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Situation:                                                            │
│  • Black Friday traffic spike (10x normal)                            │
│  • Application started timing out                                      │
│  • Cause: The observability system itself!                            │
│                                                                         │
│  What Happened:                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Traffic 10x → Telemetry 10x → Collector overwhelmed →           │   │
│  │ Backpressure to SDK → SDK blocking app threads → Timeouts!      │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Contributing Factors:                                                 │
│  • SDK configured with synchronous export                             │
│  • No sampling in place (100% of traces)                              │
│  • Collector under-provisioned                                         │
│  • No circuit breaker for telemetry                                   │
│                                                                         │
│  Fix:                                                                  │
│  • Switched to async export with bounded queue                        │
│  • Implemented adaptive sampling                                       │
│  • Added collector HPA (auto-scaling)                                 │
│  • SDK timeout reduced to 1s (fail fast)                              │
│                                                                         │
│  Configuration that saved us:                                          │
│  # SDK config                                                          │
│  OTEL_BSP_SCHEDULE_DELAY=1000       # Batch every 1s                  │
│  OTEL_BSP_MAX_QUEUE_SIZE=2048       # Bounded queue                   │
│  OTEL_BSP_EXPORT_TIMEOUT=1000       # Fail fast                       │
│  OTEL_TRACES_SAMPLER=parentbased_traceidratio                         │
│  OTEL_TRACES_SAMPLER_ARG=0.1        # 10% in emergency                │
│                                                                         │
│  Lesson:                                                               │
│  • Telemetry should NEVER take down the app                           │
│  • Load test your observability stack                                 │
│  • Have emergency sampling toggle ready                               │
│  • Monitor the monitors!                                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│  WAR STORY 3: THE $50,000 CARDINALITY BILL                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Situation:                                                            │
│  • Monthly observability bill jumped from $5K to $55K                 │
│  • No obvious increase in traffic                                      │
│  • Finance team very unhappy                                          │
│                                                                         │
│  Investigation:                                                        │
│  • One team added a "helpful" attribute: request_id                   │
│  • Every request = new unique metric series                           │
│  • 1M requests/day = 1M new time series/day                          │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ http_requests{service="checkout", request_id="abc123"} 1        │   │
│  │ http_requests{service="checkout", request_id="abc124"} 1        │   │
│  │ http_requests{service="checkout", request_id="abc125"} 1        │   │
│  │ ... (millions more)                                              │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Fix:                                                                  │
│  • Removed request_id from metrics (kept in traces)                   │
│  • Added cardinality limits in collector                              │
│  • Implemented attribute allow-list                                    │
│                                                                         │
│  Prevention Config:                                                    │
│  processors:                                                           │
│    filter:                                                             │
│      metrics:                                                          │
│        datapoint:                                                      │
│          - 'attributes["request_id"] != nil'  # Drop these!          │
│                                                                         │
│    # Or use transform to remove specific attributes                   │
│    attributes:                                                         │
│      actions:                                                          │
│        - key: request_id                                               │
│          action: delete                                                │
│        - key: session_id                                               │
│          action: delete                                                │
│                                                                         │
│  Lesson:                                                               │
│  • Review all new attributes before production                        │
│  • Set up cardinality alerts                                          │
│  • Enforce attribute allow-lists                                       │
│  • Educate teams on metrics vs traces (high cardinality → traces)     │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│  WAR STORY 4: SAMPLING DROPPED THE CRITICAL ERROR                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Situation:                                                            │
│  • Payment processing failing for some users                          │
│  • Error rate looked normal (0.1%)                                    │
│  • But NO error traces in the system!                                 │
│                                                                         │
│  Root Cause:                                                           │
│  • Head-based sampling at 10%                                         │
│  • Error happened in 0.1% of requests                                 │
│  • 10% of 0.1% = 0.01% = almost no error traces captured             │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ 1000 requests → 100 sampled → 1 error (not sampled) → NO TRACE  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Fix:                                                                  │
│  • Switched to tail-based sampling                                    │
│  • 100% capture of errors                                             │
│  • 100% capture of slow requests                                      │
│  • 5% of normal requests                                              │
│                                                                         │
│  processors:                                                           │
│    tail_sampling:                                                      │
│      policies:                                                         │
│        - name: always-sample-errors                                    │
│          type: status_code                                             │
│          status_code:                                                  │
│            status_codes: [ERROR]                                       │
│        - name: always-sample-slow                                      │
│          type: latency                                                 │
│          latency:                                                      │
│            threshold_ms: 2000                                          │
│        - name: probabilistic                                           │
│          type: probabilistic                                           │
│          probabilistic:                                                │
│            sampling_percentage: 5                                      │
│                                                                         │
│  Lesson:                                                               │
│  • Head-based sampling loses rare but important events                │
│  • Tail-based sampling is worth the extra resources                   │
│  • Always capture 100% of errors and slow requests                    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 18.2 Debugging Distributed Systems

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    DEBUGGING METHODOLOGY                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  THE "FIVE WHYS" WITH TELEMETRY                                        │
│  ────────────────────────────────                                       │
│                                                                         │
│  Alert: "Checkout latency P99 > 5s"                                    │
│                                                                         │
│  1. WHY is checkout slow?                                              │
│     → Trace shows: payment-service span is 4.8s                        │
│                                                                         │
│  2. WHY is payment-service slow?                                       │
│     → Trace shows: database span is 4.7s                               │
│                                                                         │
│  3. WHY is database slow?                                              │
│     → Metrics show: connection pool saturation 100%                    │
│                                                                         │
│  4. WHY is connection pool saturated?                                  │
│     → Metrics show: active connections spike at same time             │
│     → Correlated with: deployment of new feature                       │
│                                                                         │
│  5. WHY did new feature cause connection spike?                        │
│     → Code review: N+1 query problem in new feature                    │
│     → Each user request = 100 DB queries                               │
│                                                                         │
│  ROOT CAUSE: N+1 query in new feature                                  │
│  FIX: Batch queries, rollback feature                                  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 18.3 Critical Metrics to Alert On

```yaml
# Observability System Health Alerts
groups:
- name: observability-health
  rules:
  # Collector health
  - alert: OTelCollectorDown
    expr: up{job="otel-collector"} == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "OTel Collector is down - data loss occurring"

  - alert: OTelCollectorHighMemory
    expr: |
      container_memory_usage_bytes{container="otel-collector"} /
      container_spec_memory_limit_bytes{container="otel-collector"} > 0.85
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Collector memory > 85%, risk of OOM"

  - alert: OTelExporterFailures
    expr: rate(otelcol_exporter_send_failed_spans_total[5m]) > 0
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "Collector failing to export data"

  - alert: OTelReceiverRefused
    expr: rate(otelcol_receiver_refused_spans_total[5m]) > 100
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "Collector refusing spans - backpressure active"

  - alert: OTelQueueNearCapacity
    expr: |
      otelcol_exporter_queue_size / otelcol_exporter_queue_capacity > 0.8
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Export queue > 80% full"

  # Data quality alerts
  - alert: HighOrphanSpanRate
    expr: |
      sum(rate(spans_without_parent_total[5m])) /
      sum(rate(spans_total[5m])) > 0.05
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "> 5% orphan spans - trace context broken somewhere"

  - alert: HighCardinalityMetric
    expr: |
      count by(__name__)(
        count by(__name__, service)(
          {__name__=~"http_.*|grpc_.*"}
        )
      ) > 10000
    for: 1h
    labels:
      severity: warning
    annotations:
      summary: "Metric cardinality explosion detected"

  - alert: NoTracesFromService
    expr: |
      absent(rate(spans_total{service="critical-service"}[5m]))
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "No traces from critical-service - instrumentation broken?"

  # Cost alerts
  - alert: TelemetryVolumeSpike
    expr: |
      sum(rate(otelcol_receiver_accepted_spans_total[1h])) >
      sum(rate(otelcol_receiver_accepted_spans_total[1h] offset 1d)) * 2
    for: 30m
    labels:
      severity: warning
    annotations:
      summary: "Telemetry volume 2x higher than yesterday"
```

---

## 19. Handling Traffic Spikes & Capacity Planning

### 19.1 Spike Patterns

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    COMMON SPIKE PATTERNS                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  PATTERN 1: PREDICTABLE SPIKES (Events)                                │
│  ──────────────────────────────────────                                 │
│                                                                         │
│  Examples: Black Friday, Product launches, Marketing campaigns         │
│                                                                         │
│  Telemetry Impact:                                                     │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │     Normal        Event Day                                     │   │
│  │     ───────       ─────────                                     │   │
│  │     1K spans/s    50K spans/s  (50x)                           │   │
│  │     10K metrics   50K metrics   (5x cardinality risk!)         │   │
│  │     5K logs/s     100K logs/s  (20x)                           │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Preparation:                                                          │
│  • Pre-scale collectors (3x-5x normal)                                │
│  • Reduce sampling rate (50% → 10%)                                   │
│  • Increase batch sizes                                               │
│  • Pre-warm backend storage                                           │
│  • Have runbook ready for emergency sampling                          │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  PATTERN 2: UNPREDICTABLE SPIKES (Viral/Incident)                      │
│  ────────────────────────────────────────────────                       │
│                                                                         │
│  Examples: Viral content, DDoS, Cascade failures                       │
│                                                                         │
│  Challenge: No time to prepare                                         │
│                                                                         │
│  Automatic Protection:                                                 │
│  processors:                                                           │
│    # 1. Memory protection                                              │
│    memory_limiter:                                                     │
│      limit_mib: 2000                                                   │
│      spike_limit_mib: 500                                              │
│                                                                         │
│    # 2. Adaptive sampling                                              │
│    probabilistic_sampler:                                              │
│      sampling_percentage: ${SAMPLING_RATE:-10}  # Can change live     │
│                                                                         │
│    # 3. Rate limiting                                                  │
│    rate_limiter:                                                       │
│      spans_per_second: 10000  # Hard cap                              │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  PATTERN 3: RETRY STORMS                                               │
│  ───────────────────────                                                │
│                                                                         │
│  Scenario: Backend slow → Apps retry → 10x traffic → Backend slower   │
│                                                                         │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │ Normal: App ──▶ Backend (1 request)                            │   │
│  │                                                                 │   │
│  │ Storm:  App ──▶ Backend (timeout)                              │   │
│  │              ──▶ Backend (retry 1)                             │   │
│  │              ──▶ Backend (retry 2)                             │   │
│  │              ──▶ Backend (retry 3)                             │   │
│  │         = 4x telemetry for 1 user request!                     │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Detection:                                                            │
│  • Alert on retry span ratio                                          │
│  • Monitor span count per trace (should be stable)                    │
│                                                                         │
│  Prevention:                                                           │
│  • Circuit breakers in apps                                           │
│  • Exponential backoff                                                │
│  • Sample retries at lower rate                                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 19.2 Capacity Planning

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    CAPACITY PLANNING FORMULAS                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  COLLECTOR SIZING                                                       │
│  ─────────────────                                                      │
│                                                                         │
│  Base formula:                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │ Collectors = (Spans/sec × Avg Span Size × Safety Factor)        │ │
│  │              ─────────────────────────────────────────────       │ │
│  │              Collector Throughput                                │ │
│  └──────────────────────────────────────────────────────────────────┘ │
│                                                                         │
│  Example:                                                              │
│  • 50,000 spans/second                                                │
│  • Average span size: 2KB                                             │
│  • Safety factor: 2x                                                  │
│  • Collector throughput: 20,000 spans/sec per instance               │
│                                                                         │
│  Collectors = (50,000 × 2) / 20,000 = 5 instances                     │
│                                                                         │
│  Resource per collector:                                              │
│  • CPU: 1-2 cores per 10K spans/sec                                   │
│  • Memory: 1-2 GB per 10K spans/sec                                   │
│  • Network: ~100 Mbps per 10K spans/sec                               │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  STORAGE SIZING                                                        │
│  ──────────────                                                         │
│                                                                         │
│  Daily storage formula:                                                │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │ Storage/day = Spans/sec × 86400 × Avg Size × (1 - Compression)  │ │
│  └──────────────────────────────────────────────────────────────────┘ │
│                                                                         │
│  Example:                                                              │
│  • 10,000 spans/second                                                │
│  • Avg span size: 1KB                                                 │
│  • Compression ratio: 10:1 (ClickHouse typical)                       │
│  • Retention: 30 days                                                 │
│                                                                         │
│  Raw:   10,000 × 86,400 × 1KB = 864 GB/day                           │
│  Compressed: 864 GB / 10 = 86 GB/day                                  │
│  30-day retention: 86 × 30 = 2.6 TB                                   │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  QUICK REFERENCE TABLE                                                 │
│  ─────────────────────                                                  │
│                                                                         │
│  Spans/sec │ Collectors │ Daily Storage │ Monthly Cost (est.)         │
│  ──────────┼────────────┼───────────────┼────────────────────          │
│  1,000     │ 1          │ 8 GB          │ $50-100                      │
│  10,000    │ 2          │ 80 GB         │ $200-500                     │
│  50,000    │ 5          │ 400 GB        │ $1,000-2,500                 │
│  100,000   │ 10         │ 800 GB        │ $2,000-5,000                 │
│  500,000   │ 25+        │ 4 TB          │ $10,000-25,000               │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 19.3 Auto-Scaling Configuration

```yaml
# Horizontal Pod Autoscaler for Collector
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: otel-collector-hpa
  namespace: observability
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: otel-collector-gateway
  minReplicas: 3
  maxReplicas: 20
  metrics:
  # Scale on CPU
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70

  # Scale on memory
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 75

  # Scale on queue size (custom metric)
  - type: Pods
    pods:
      metric:
        name: otelcol_exporter_queue_size
      target:
        type: AverageValue
        averageValue: "5000"

  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 100  # Can double
        periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300  # Wait 5 min before scaling down
      policies:
      - type: Percent
        value: 25
        periodSeconds: 60
---
# Pod Disruption Budget
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: otel-collector-pdb
  namespace: observability
spec:
  minAvailable: 2  # Always keep 2 running
  selector:
    matchLabels:
      app: otel-collector-gateway
```

### 19.4 Emergency Runbook

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    EMERGENCY RESPONSE RUNBOOK                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  SCENARIO: TELEMETRY STORM (Data volume spike)                         │
│  ─────────────────────────────────────────────                          │
│                                                                         │
│  STEP 1: ASSESS (1 minute)                                             │
│  ─────────────────────────                                              │
│  $ kubectl top pods -n observability                                   │
│  $ kubectl logs -n observability -l app=otel-collector --tail=50      │
│  $ curl collector:8888/metrics | grep otelcol_receiver                │
│                                                                         │
│  STEP 2: IMMEDIATE RELIEF (2 minutes)                                  │
│  ────────────────────────────────────                                   │
│  # Reduce sampling to 1%                                               │
│  $ kubectl set env deployment/otel-collector \                         │
│      SAMPLING_PERCENTAGE=1 -n observability                            │
│                                                                         │
│  # Or scale up collectors                                              │
│  $ kubectl scale deployment otel-collector-gateway \                   │
│      --replicas=10 -n observability                                    │
│                                                                         │
│  STEP 3: DROP NON-CRITICAL DATA (if needed)                           │
│  ──────────────────────────────────────────                             │
│  # Apply emergency filter config                                       │
│  $ kubectl apply -f emergency-filter-config.yaml                       │
│                                                                         │
│  # emergency-filter-config.yaml                                        │
│  processors:                                                           │
│    filter:                                                             │
│      spans:                                                            │
│        exclude:                                                        │
│          match_type: regexp                                            │
│          services:                                                     │
│            - "non-critical-.*"                                         │
│      logs:                                                             │
│        exclude:                                                        │
│          severity_texts: ["DEBUG", "INFO"]                            │
│                                                                         │
│  STEP 4: MONITOR RECOVERY (ongoing)                                    │
│  ──────────────────────────────────                                     │
│  $ watch kubectl top pods -n observability                            │
│  $ watch 'curl -s collector:8888/metrics | grep queue_size'           │
│                                                                         │
│  STEP 5: POST-INCIDENT                                                 │
│  ────────────────────────                                               │
│  • Identify root cause of spike                                        │
│  • Update capacity planning                                            │
│  • Review sampling policies                                            │
│  • Update runbook if needed                                            │
│                                                                         │
│  ─────────────────────────────────────────────────────────────────────  │
│                                                                         │
│  SCENARIO: COLLECTOR DOWN (No telemetry)                               │
│  ───────────────────────────────────────                                │
│                                                                         │
│  STEP 1: Check collector status                                        │
│  $ kubectl get pods -n observability -l app=otel-collector            │
│  $ kubectl describe pod <collector-pod> -n observability              │
│                                                                         │
│  STEP 2: Check for common issues                                       │
│  • OOMKilled → Increase memory limits                                 │
│  • CrashLoopBackOff → Check config validity                           │
│  • ImagePullBackOff → Check image registry                            │
│                                                                         │
│  STEP 3: Restart with safe config                                      │
│  $ kubectl apply -f safe-minimal-config.yaml                          │
│  $ kubectl rollout restart deployment/otel-collector                  │
│                                                                         │
│  STEP 4: Verify data flow                                              │
│  $ kubectl port-forward svc/otel-collector 8888:8888                  │
│  $ curl localhost:8888/metrics | grep accepted_spans                  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 20. Advanced Patterns & Expert Tips

### 20.1 Multi-Cluster Observability

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    MULTI-CLUSTER ARCHITECTURE                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────┐    ┌─────────────────────┐                   │
│  │ Cluster A (US-East) │    │ Cluster B (EU-West) │                   │
│  │                     │    │                     │                   │
│  │ ┌─────────────────┐ │    │ ┌─────────────────┐ │                   │
│  │ │ Local Collector │ │    │ │ Local Collector │ │                   │
│  │ │  (DaemonSet)    │ │    │ │  (DaemonSet)    │ │                   │
│  │ └────────┬────────┘ │    │ └────────┬────────┘ │                   │
│  │          │          │    │          │          │                   │
│  │ ┌────────▼────────┐ │    │ ┌────────▼────────┐ │                   │
│  │ │ Gateway         │ │    │ │ Gateway         │ │                   │
│  │ │ Collector       │ │    │ │ Collector       │ │                   │
│  │ └────────┬────────┘ │    │ └────────┬────────┘ │                   │
│  └──────────┼──────────┘    └──────────┼──────────┘                   │
│             │                          │                               │
│             └────────────┬─────────────┘                               │
│                          │                                             │
│             ┌────────────▼────────────┐                               │
│             │   Central Gateway       │                               │
│             │   (Global Aggregation)  │                               │
│             └────────────┬────────────┘                               │
│                          │                                             │
│             ┌────────────▼────────────┐                               │
│             │   Central Backend       │                               │
│             │   (ClickHouse/Tempo)    │                               │
│             └─────────────────────────┘                               │
│                                                                         │
│  KEY CONSIDERATIONS:                                                   │
│  • Add cluster.name attribute at each gateway                         │
│  • Regional pre-aggregation to reduce cross-region traffic            │
│  • Consistent service.name across clusters                            │
│  • Time synchronization critical across regions                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 20.2 Trace Context Across Async Boundaries

```python
# Pattern: Propagating trace context through message queues

# PRODUCER
from opentelemetry import trace
from opentelemetry.propagate import inject

def send_message(queue, payload):
    tracer = trace.get_tracer(__name__)

    with tracer.start_as_current_span("send_to_queue") as span:
        # Inject trace context into message headers
        headers = {}
        inject(headers)  # Adds traceparent, tracestate

        message = {
            "payload": payload,
            "headers": headers  # Include trace context!
        }
        queue.send(message)

# CONSUMER
from opentelemetry.propagate import extract

def process_message(message):
    tracer = trace.get_tracer(__name__)

    # Extract trace context from message
    ctx = extract(message["headers"])

    # Continue the trace (not start new one!)
    with tracer.start_as_current_span(
        "process_message",
        context=ctx,  # Continue parent trace
        kind=trace.SpanKind.CONSUMER
    ) as span:
        do_work(message["payload"])
```

### 20.3 Custom Business Metrics

```python
# Adding business context to observability

from opentelemetry import metrics, trace

meter = metrics.get_meter(__name__)
tracer = trace.get_tracer(__name__)

# Business metrics (not just technical)
order_total = meter.create_histogram(
    "business.order.total",
    unit="USD",
    description="Order total amount"
)

order_items = meter.create_histogram(
    "business.order.items",
    unit="{items}",
    description="Items per order"
)

conversion_counter = meter.create_counter(
    "business.checkout.conversions",
    description="Successful checkout conversions"
)

def process_order(order):
    with tracer.start_as_current_span("process_order") as span:
        # Add business attributes to span
        span.set_attribute("order.id", order.id)
        span.set_attribute("order.total", order.total)
        span.set_attribute("customer.tier", order.customer.tier)
        span.set_attribute("order.is_first_purchase", order.is_first_purchase)

        # Record business metrics
        order_total.record(
            order.total,
            {"customer.tier": order.customer.tier, "region": order.region}
        )
        order_items.record(
            len(order.items),
            {"category": order.primary_category}
        )

        if order.completed:
            conversion_counter.add(
                1,
                {"source": order.traffic_source, "device": order.device_type}
            )
```

### 20.4 Testing Observability

```python
# Unit testing your instrumentation

import unittest
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

class TestOrderInstrumentation(unittest.TestCase):
    def setUp(self):
        # Use in-memory exporter for testing
        self.exporter = InMemorySpanExporter()
        provider = TracerProvider()
        provider.add_span_processor(
            SimpleSpanProcessor(self.exporter)
        )
        trace.set_tracer_provider(provider)

    def test_order_span_has_required_attributes(self):
        # Call the instrumented function
        process_order(mock_order)

        # Get captured spans
        spans = self.exporter.get_finished_spans()

        # Verify span exists
        self.assertEqual(len(spans), 1)
        span = spans[0]

        # Verify required attributes
        self.assertEqual(span.name, "process_order")
        self.assertIn("order.id", span.attributes)
        self.assertIn("order.total", span.attributes)
        self.assertIn("customer.tier", span.attributes)

    def test_error_span_has_exception(self):
        # Call function that raises
        with self.assertRaises(PaymentError):
            process_payment(invalid_card)

        spans = self.exporter.get_finished_spans()
        span = spans[0]

        # Verify error recorded
        self.assertEqual(span.status.status_code, StatusCode.ERROR)
        self.assertIn("exception.type", span.events[0].attributes)

    def tearDown(self):
        self.exporter.clear()
```

### 20.5 Feature Flags & Observability

```yaml
# Dynamic observability configuration via feature flags

# ConfigMap with feature flags
apiVersion: v1
kind: ConfigMap
metadata:
  name: otel-feature-flags
data:
  # Can be toggled without restart via OpAMP or config reload
  ENABLE_DEBUG_LOGGING: "false"
  SAMPLING_RATE: "10"
  ENABLE_PROFILING: "false"
  ENABLE_DETAILED_DB_TRACES: "false"
  TRACE_ALL_ERRORS: "true"
  CAPTURE_REQUEST_BODY: "false"  # PII risk, normally off
  HIGH_CARDINALITY_MODE: "false"  # For debugging only
```

```python
# Application code respecting feature flags
import os

def should_capture_request_body():
    return os.getenv("CAPTURE_REQUEST_BODY", "false").lower() == "true"

def get_sampling_rate():
    return float(os.getenv("SAMPLING_RATE", "10")) / 100

@app.middleware("http")
async def observability_middleware(request, call_next):
    with tracer.start_as_current_span("http_request") as span:
        span.set_attribute("http.method", request.method)
        span.set_attribute("http.url", str(request.url))

        # Conditionally capture body (controlled by feature flag)
        if should_capture_request_body():
            body = await request.body()
            span.set_attribute("http.request.body", body[:1000])  # Truncate

        response = await call_next(request)
        return response
```

### 20.6 Observability Maturity Model

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    OBSERVABILITY MATURITY MODEL                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  LEVEL 1: REACTIVE (Most organizations start here)                     │
│  ─────────────────────────────────────────────────                      │
│  ✓ Basic metrics (CPU, memory, disk)                                   │
│  ✓ Application logs (unstructured)                                     │
│  ✓ Alerting on symptoms (high CPU, errors)                            │
│  ✗ No distributed tracing                                              │
│  ✗ No correlation between signals                                      │
│  MTTR: Hours to days                                                   │
│                                                                         │
│  LEVEL 2: PROACTIVE                                                     │
│  ─────────────────────                                                  │
│  ✓ Structured logging with context                                     │
│  ✓ Basic distributed tracing                                           │
│  ✓ Service-level metrics (RED/USE)                                    │
│  ✓ Dashboards per service                                              │
│  ✗ Limited correlation                                                 │
│  ✗ Manual root cause analysis                                          │
│  MTTR: Minutes to hours                                                │
│                                                                         │
│  LEVEL 3: CORRELATED                                                    │
│  ───────────────────────                                                │
│  ✓ Full distributed tracing                                            │
│  ✓ Logs correlated with traces                                         │
│  ✓ Metrics with exemplars                                              │
│  ✓ Service dependency maps                                             │
│  ✓ SLOs and error budgets                                              │
│  ✗ Limited predictive capabilities                                     │
│  MTTR: Minutes                                                         │
│                                                                         │
│  LEVEL 4: PREDICTIVE                                                    │
│  ───────────────────────                                                │
│  ✓ Anomaly detection (ML-based)                                        │
│  ✓ Capacity forecasting                                                │
│  ✓ Automated root cause analysis                                       │
│  ✓ Change correlation                                                  │
│  ✓ Proactive alerting (before users impacted)                         │
│  MTTR: Seconds to minutes                                              │
│                                                                         │
│  LEVEL 5: AUTONOMOUS (Future state)                                    │
│  ──────────────────────────────────                                     │
│  ✓ Self-healing systems                                                │
│  ✓ Automated remediation                                               │
│  ✓ AI-driven insights                                                  │
│  ✓ Natural language queries                                            │
│  ✓ Continuous optimization                                             │
│  MTTR: Automatic resolution                                            │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 20.7 Expert Checklist

```markdown
## Production Readiness Checklist

### Instrumentation
- [ ] All services instrumented (>95% coverage)
- [ ] Consistent service.name across all signals
- [ ] Trace context propagates through all hops (including proxies, queues)
- [ ] Log correlation enabled (trace_id in all logs)
- [ ] Error spans properly marked with exceptions
- [ ] Sensitive data scrubbed (PII, secrets)

### Sampling
- [ ] Tail-based sampling configured
- [ ] 100% of errors captured
- [ ] 100% of slow requests captured (>P99)
- [ ] Sampling rate appropriate for volume
- [ ] Emergency sampling toggle ready

### Collectors
- [ ] HA deployment (3+ replicas)
- [ ] Memory limiter configured
- [ ] Persistent queue enabled
- [ ] Auto-scaling configured
- [ ] Resource limits set appropriately
- [ ] Health checks configured

### Backend
- [ ] Retention policies defined
- [ ] Backup strategy in place
- [ ] Capacity for 2x current volume
- [ ] Query performance tested
- [ ] Cost monitoring enabled

### Alerting
- [ ] Collector health alerts
- [ ] Data pipeline alerts (gaps, delays)
- [ ] Cardinality alerts
- [ ] Cost threshold alerts
- [ ] SLO-based alerts (not just symptoms)

### Documentation
- [ ] Architecture diagram current
- [ ] Runbooks for common issues
- [ ] On-call procedures documented
- [ ] Team training completed
- [ ] Attribute conventions documented

### Testing
- [ ] Instrumentation unit tests
- [ ] Load testing of observability stack
- [ ] Chaos testing (collector failure)
- [ ] DR testing (backend recovery)
```

---

## Appendix A: Quick Reference

### Environment Variables (All Languages)

| Variable | Description | Example |
|----------|-------------|---------|
| `OTEL_SERVICE_NAME` | Service identifier | `order-service` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Collector endpoint | `http://collector:4317` |
| `OTEL_TRACES_EXPORTER` | Trace exporter | `otlp` |
| `OTEL_METRICS_EXPORTER` | Metrics exporter | `otlp` |
| `OTEL_LOGS_EXPORTER` | Logs exporter | `otlp` |
| `OTEL_RESOURCE_ATTRIBUTES` | Additional attributes | `env=prod,version=1.0` |
| `OTEL_TRACES_SAMPLER` | Sampler type | `parentbased_traceidratio` |
| `OTEL_TRACES_SAMPLER_ARG` | Sampler argument | `0.1` |

### Useful Commands

```bash
# Check collector metrics
kubectl port-forward -n observability svc/otel-collector 8888:8888
curl localhost:8888/metrics | grep otelcol

# View collector config
kubectl get configmap otel-collector-config -n observability -o yaml

# Check instrumentation status
kubectl get instrumentation -A

# View injected environment variables
kubectl exec -it <pod> -- env | grep OTEL

# Test OTLP connectivity
grpcurl -plaintext otel-collector.observability:4317 list
```

---

## Appendix B: References

### Official OpenTelemetry Documentation

- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)
- [OTLP Specification](https://opentelemetry.io/docs/specs/otlp/)
- [OTel Collector Contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib)
- [OTel Operator](https://github.com/open-telemetry/opentelemetry-operator)
- [OpAMP Specification](https://github.com/open-telemetry/opamp-spec)

### Language-Specific Instrumentation

- [Java Auto-Instrumentation](https://github.com/open-telemetry/opentelemetry-java-instrumentation)
- [Python Auto-Instrumentation](https://opentelemetry.io/docs/languages/python/automatic/)
- [.NET Auto-Instrumentation](https://github.com/open-telemetry/opentelemetry-dotnet-instrumentation)
- [Node.js Auto-Instrumentation](https://github.com/open-telemetry/opentelemetry-js-contrib)
- [Go Auto-Instrumentation (eBPF)](https://github.com/open-telemetry/opentelemetry-go-instrumentation)

### GenAI & LLM Observability

- [OpenTelemetry GenAI Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/)
- [GenAI Agent Spans Conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/)
- [GenAI Metrics Conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-metrics/)
- [LLM Observability Introduction - OpenTelemetry Blog](https://opentelemetry.io/blog/2024/llm-observability/)
- [OpenTelemetry for Generative AI - OpenTelemetry Blog](https://opentelemetry.io/blog/2024/otel-generative-ai/)
- [AI Agent Observability Standards - OpenTelemetry Blog](https://opentelemetry.io/blog/2025/ai-agent-observability/)
- [OpenLLMetry Project](https://github.com/traceloop/openllmetry)
- [Arize Phoenix](https://github.com/Arize-ai/phoenix)
- [Datadog LLM Observability with OTel GenAI Conventions](https://www.datadoghq.com/blog/llm-otel-semantic-convention/)
- [OpenTelemetry for GenAI - Dotan Horovits](https://horovits.medium.com/opentelemetry-for-genai-and-the-openllmetry-project-81b9cea6a771)
- [AI Engineer's Guide to LLM Observability](https://agenta.ai/blog/the-ai-engineer-s-guide-to-llm-observability-with-opentelemetry)
- [Best LLM Observability Tools 2025](https://www.firecrawl.dev/blog/best-llm-observability-tools)

### eBPF Instrumentation

- [Grafana Beyla](https://github.com/grafana/beyla)
- [Odigos - Automatic Instrumentation](https://github.com/odigos-io/odigos)
- [Pixie - Kubernetes Observability](https://px.dev/)
- [Cilium/Hubble - Network Observability](https://cilium.io/)

### AIOps & Industry Trends

- [Can OpenTelemetry Save Observability in 2026? - The New Stack](https://thenewstack.io/can-opentelemetry-save-observability-in-2026/)
- [Observability in 2025: OpenTelemetry and AI - The New Stack](https://thenewstack.io/observability-in-2025-opentelemetry-and-ai-to-fill-in-gaps/)
- [Observability Trends 2026 - IBM](https://www.ibm.com/think/insights/observability-trends)
- [Top 8 Observability Trends 2026 - TechTarget](https://www.techtarget.com/searchitoperations/feature/Top-observability-trends-to-watch)
- [5 Observability & AI Trends for 2026 - LogicMonitor](https://www.logicmonitor.com/blog/observability-ai-trends-2026)
- [Building Autonomous Infrastructure with AIOps - Futuriom](https://www.futuriom.com/articles/news/building-autonomous-infrastructure-with-observability-and-aiops/2025/12)
- [Future of Observability Trends 2025 - Leapcell](https://leapcell.medium.com/the-future-of-observability-trends-shaping-2025-427fc9d0cd34)
- [Observability in 2025 - Hydrolix](https://hydrolix.io/blog/observability-in-2025/)
- [Datadog AI Innovation](https://www.datadoghq.com/blog/datadog-ai-innovation/)
- [Best AI Observability Tools 2025 - Monte Carlo](https://www.montecarlodata.com/blog-best-ai-observability-tools/)

### Backend & Storage

- [ClickHouse](https://clickhouse.com/)
- [Grafana Tempo](https://grafana.com/oss/tempo/)
- [Grafana Mimir](https://grafana.com/oss/mimir/)
- [Grafana Loki](https://grafana.com/oss/loki/)
- [Jaeger](https://www.jaegertracing.io/)
- [Prometheus](https://prometheus.io/)
- [VictoriaMetrics](https://victoriametrics.com/)

### Vendor Documentation

- [Datadog OpenTelemetry](https://docs.datadoghq.com/opentelemetry/)
- [New Relic OpenTelemetry](https://docs.newrelic.com/docs/opentelemetry/get-started/opentelemetry-get-started-intro/)
- [Dynatrace OpenTelemetry](https://www.dynatrace.com/support/help/extend-dynatrace/opentelemetry)
- [Honeycomb OpenTelemetry](https://docs.honeycomb.io/send-data/opentelemetry/)
- [Splunk OpenTelemetry](https://docs.splunk.com/observability/en/gdi/opentelemetry/opentelemetry.html)
- [AWS X-Ray OpenTelemetry](https://docs.aws.amazon.com/xray/latest/devguide/xray-instrumenting-your-app.html)
- [Google Cloud Trace OpenTelemetry](https://cloud.google.com/trace/docs/setup)
- [Azure Monitor OpenTelemetry](https://learn.microsoft.com/en-us/azure/azure-monitor/app/opentelemetry-overview)

### GitHub Repositories

- [OpenTelemetry Semantic Conventions](https://github.com/open-telemetry/semantic-conventions)
- [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector)
- [OpenTelemetry Collector Contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib)
- [BindPlane OP](https://github.com/observIQ/bindplane-op)
- [Signoz](https://github.com/SigNoz/signoz)
- [Uptrace](https://github.com/uptrace/uptrace)

### Additional Resources

- [CNCF OpenTelemetry Project](https://www.cncf.io/projects/opentelemetry/)
- [OpenTelemetry Community](https://opentelemetry.io/community/)
- [OTel End User Working Group](https://opentelemetry.io/community/end-user/)
- [W3C Trace Context Specification](https://www.w3.org/TR/trace-context/)
- [W3C Baggage Specification](https://www.w3.org/TR/baggage/)

---

**Document Version:** 1.0
**Last Updated:** February 2025

**Author:** Madhukar Beema, Distinguished Engineer
**Contact:** mbeema@gmail.com
**Maintainer:** Observability Consultant

