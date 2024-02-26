# OllyStack Makefile
# Build orchestration for all components

.PHONY: all build clean test lint dev prod help

# Variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go variables
GO := go
GOFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Rust variables
CARGO := cargo
CARGO_FLAGS := --release

# Node variables
NPM := npm
PNPM := pnpm

# Docker variables
DOCKER := docker
DOCKER_COMPOSE := docker-compose
REGISTRY ?= ghcr.io/ollystack

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

#==============================================================================
# Main targets
#==============================================================================

all: build ## Build all components

help: ## Show this help
	@echo "OllyStack Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(BLUE)%-20s$(NC) %s\n", $$1, $$2}'

#==============================================================================
# Development
#==============================================================================

dev: ## Start development environment
	@echo "$(GREEN)Starting development environment...$(NC)"
	$(DOCKER_COMPOSE) -f deploy/docker/docker-compose.dev.yml up -d
	@echo "$(GREEN)Development environment started!$(NC)"
	@echo "  - UI: http://localhost:3000"
	@echo "  - API: http://localhost:8080"
	@echo "  - Collector: localhost:4317 (gRPC), localhost:4318 (HTTP)"

dev-down: ## Stop development environment
	@echo "$(YELLOW)Stopping development environment...$(NC)"
	$(DOCKER_COMPOSE) -f deploy/docker/docker-compose.dev.yml down

dev-logs: ## Show development environment logs
	$(DOCKER_COMPOSE) -f deploy/docker/docker-compose.dev.yml logs -f

#==============================================================================
# Build targets
#==============================================================================

build: build-agents build-collector build-api build-stream-processor build-query-engine build-ui ## Build all components
	@echo "$(GREEN)All components built successfully!$(NC)"

build-agents: build-universal-agent build-ebpf-agent ## Build all agents
	@echo "$(GREEN)Agents built successfully!$(NC)"

build-universal-agent: ## Build Universal Agent (Go)
	@echo "$(BLUE)Building Universal Agent...$(NC)"
	cd agents/universal-agent && $(GO) build $(GOFLAGS) -o ../../bin/ollystack-agent ./cmd/agent

build-ebpf-agent: ## Build eBPF Agent (Rust)
	@echo "$(BLUE)Building eBPF Agent...$(NC)"
	cd agents/ebpf-agent && $(CARGO) build $(CARGO_FLAGS)
	cp agents/ebpf-agent/target/release/ebpf-agent bin/ollystack-ebpf-agent 2>/dev/null || true

build-collector: ## Build Enhanced OTel Collector (Go)
	@echo "$(BLUE)Building Collector...$(NC)"
	cd collector && $(GO) build $(GOFLAGS) -o ../bin/ollystack-collector ./cmd/collector

build-api: ## Build API Server (Go)
	@echo "$(BLUE)Building API Server...$(NC)"
	cd api-server && $(GO) build $(GOFLAGS) -o ../bin/ollystack-api ./cmd/server

build-stream-processor: ## Build Stream Processor (Rust)
	@echo "$(BLUE)Building Stream Processor...$(NC)"
	cd stream-processor && $(CARGO) build $(CARGO_FLAGS)
	cp stream-processor/target/release/stream-processor bin/ollystack-stream-processor 2>/dev/null || true

build-query-engine: ## Build Query Engine (Rust)
	@echo "$(BLUE)Building Query Engine...$(NC)"
	cd query-engine && $(CARGO) build $(CARGO_FLAGS)
	cp query-engine/target/release/query-engine bin/ollystack-query-engine 2>/dev/null || true

build-ui: ## Build Web UI (React)
	@echo "$(BLUE)Building Web UI...$(NC)"
	cd web-ui && $(PNPM) install && $(PNPM) build

build-rum-sdk: ## Build RUM SDK (TypeScript)
	@echo "$(BLUE)Building RUM SDK...$(NC)"
	cd agents/rum-sdk && $(PNPM) install && $(PNPM) build

#==============================================================================
# Docker targets
#==============================================================================

docker: docker-build docker-push ## Build and push all Docker images

docker-build: ## Build all Docker images
	@echo "$(BLUE)Building Docker images...$(NC)"
	$(DOCKER) build -t $(REGISTRY)/universal-agent:$(VERSION) -f agents/universal-agent/Dockerfile agents/universal-agent
	$(DOCKER) build -t $(REGISTRY)/ebpf-agent:$(VERSION) -f agents/ebpf-agent/Dockerfile agents/ebpf-agent
	$(DOCKER) build -t $(REGISTRY)/collector:$(VERSION) -f collector/Dockerfile collector
	$(DOCKER) build -t $(REGISTRY)/api-server:$(VERSION) -f api-server/Dockerfile api-server
	$(DOCKER) build -t $(REGISTRY)/stream-processor:$(VERSION) -f stream-processor/Dockerfile stream-processor
	$(DOCKER) build -t $(REGISTRY)/query-engine:$(VERSION) -f query-engine/Dockerfile query-engine
	$(DOCKER) build -t $(REGISTRY)/web-ui:$(VERSION) -f web-ui/Dockerfile web-ui
	@echo "$(GREEN)Docker images built successfully!$(NC)"

docker-push: ## Push all Docker images
	@echo "$(BLUE)Pushing Docker images...$(NC)"
	$(DOCKER) push $(REGISTRY)/universal-agent:$(VERSION)
	$(DOCKER) push $(REGISTRY)/ebpf-agent:$(VERSION)
	$(DOCKER) push $(REGISTRY)/collector:$(VERSION)
	$(DOCKER) push $(REGISTRY)/api-server:$(VERSION)
	$(DOCKER) push $(REGISTRY)/stream-processor:$(VERSION)
	$(DOCKER) push $(REGISTRY)/query-engine:$(VERSION)
	$(DOCKER) push $(REGISTRY)/web-ui:$(VERSION)
	@echo "$(GREEN)Docker images pushed successfully!$(NC)"

#==============================================================================
# Test targets
#==============================================================================

test: test-go test-rust test-ui ## Run all tests
	@echo "$(GREEN)All tests passed!$(NC)"

test-go: ## Run Go tests
	@echo "$(BLUE)Running Go tests...$(NC)"
	cd agents/universal-agent && $(GO) test -v -race -cover ./...
	cd collector && $(GO) test -v -race -cover ./...
	cd api-server && $(GO) test -v -race -cover ./...

test-rust: ## Run Rust tests
	@echo "$(BLUE)Running Rust tests...$(NC)"
	cd agents/ebpf-agent && $(CARGO) test
	cd stream-processor && $(CARGO) test
	cd query-engine && $(CARGO) test

test-ui: ## Run UI tests
	@echo "$(BLUE)Running UI tests...$(NC)"
	cd web-ui && $(PNPM) test

test-integration: ## Run integration tests
	@echo "$(BLUE)Running integration tests...$(NC)"
	$(DOCKER_COMPOSE) -f deploy/docker/docker-compose.test.yml up --abort-on-container-exit
	$(DOCKER_COMPOSE) -f deploy/docker/docker-compose.test.yml down

test-e2e: ## Run end-to-end tests
	@echo "$(BLUE)Running E2E tests...$(NC)"
	cd tests/e2e && $(GO) test -v ./...

#==============================================================================
# Lint targets
#==============================================================================

lint: lint-go lint-rust lint-ui ## Run all linters
	@echo "$(GREEN)Linting complete!$(NC)"

lint-go: ## Lint Go code
	@echo "$(BLUE)Linting Go code...$(NC)"
	cd agents/universal-agent && golangci-lint run
	cd collector && golangci-lint run
	cd api-server && golangci-lint run

lint-rust: ## Lint Rust code
	@echo "$(BLUE)Linting Rust code...$(NC)"
	cd agents/ebpf-agent && $(CARGO) clippy -- -D warnings
	cd stream-processor && $(CARGO) clippy -- -D warnings
	cd query-engine && $(CARGO) clippy -- -D warnings

lint-ui: ## Lint UI code
	@echo "$(BLUE)Linting UI code...$(NC)"
	cd web-ui && $(PNPM) lint

fmt: fmt-go fmt-rust fmt-ui ## Format all code

fmt-go: ## Format Go code
	cd agents/universal-agent && $(GO) fmt ./...
	cd collector && $(GO) fmt ./...
	cd api-server && $(GO) fmt ./...

fmt-rust: ## Format Rust code
	cd agents/ebpf-agent && $(CARGO) fmt
	cd stream-processor && $(CARGO) fmt
	cd query-engine && $(CARGO) fmt

fmt-ui: ## Format UI code
	cd web-ui && $(PNPM) format

#==============================================================================
# Proto targets
#==============================================================================

proto: ## Generate protobuf code
	@echo "$(BLUE)Generating protobuf code...$(NC)"
	cd proto && buf generate
	@echo "$(GREEN)Protobuf code generated!$(NC)"

proto-lint: ## Lint protobuf files
	cd proto && buf lint

#==============================================================================
# Clean targets
#==============================================================================

clean: ## Clean all build artifacts
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	rm -rf bin/
	rm -rf agents/universal-agent/bin
	rm -rf agents/ebpf-agent/target
	rm -rf stream-processor/target
	rm -rf query-engine/target
	rm -rf web-ui/dist
	rm -rf web-ui/node_modules
	rm -rf agents/rum-sdk/dist
	@echo "$(GREEN)Clean complete!$(NC)"

clean-docker: ## Remove all Docker images
	@echo "$(YELLOW)Removing Docker images...$(NC)"
	$(DOCKER) rmi $(REGISTRY)/universal-agent:$(VERSION) 2>/dev/null || true
	$(DOCKER) rmi $(REGISTRY)/ebpf-agent:$(VERSION) 2>/dev/null || true
	$(DOCKER) rmi $(REGISTRY)/collector:$(VERSION) 2>/dev/null || true
	$(DOCKER) rmi $(REGISTRY)/api-server:$(VERSION) 2>/dev/null || true
	$(DOCKER) rmi $(REGISTRY)/stream-processor:$(VERSION) 2>/dev/null || true
	$(DOCKER) rmi $(REGISTRY)/query-engine:$(VERSION) 2>/dev/null || true
	$(DOCKER) rmi $(REGISTRY)/web-ui:$(VERSION) 2>/dev/null || true

#==============================================================================
# Install targets
#==============================================================================

install-tools: ## Install development tools
	@echo "$(BLUE)Installing development tools...$(NC)"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	cargo install cargo-watch
	npm install -g pnpm
	@echo "$(GREEN)Development tools installed!$(NC)"

#==============================================================================
# Release targets
#==============================================================================

release: ## Create a new release
	@echo "$(BLUE)Creating release $(VERSION)...$(NC)"
	goreleaser release --clean

release-snapshot: ## Create a snapshot release
	goreleaser release --snapshot --clean

#==============================================================================
# Documentation
#==============================================================================

docs: ## Generate documentation
	@echo "$(BLUE)Generating documentation...$(NC)"
	cd docs && mkdocs build

docs-serve: ## Serve documentation locally
	cd docs && mkdocs serve

#==============================================================================
# AWS Deployment (Low-Cost Architecture)
#==============================================================================

# AWS Variables
AWS_REGION ?= us-east-2
ENV ?= dev
CLUSTER_NAME ?= ollystack-$(ENV)

aws-check: ## Verify AWS credentials
	@if [ -z "$$AWS_ACCESS_KEY_ID" ] && ! aws sts get-caller-identity &>/dev/null; then \
		echo "$(RED)Error: AWS credentials not configured$(NC)"; \
		echo "Set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ AWS credentials configured$(NC)"
	@aws sts get-caller-identity --query 'Account' --output text

aws-bootstrap: aws-check ## First-time AWS setup (creates all resources)
	@echo "$(BLUE)╔══════════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(BLUE)║           OllyStack AWS Bootstrap                             ║$(NC)"
	@echo "$(BLUE)╚══════════════════════════════════════════════════════════════╝$(NC)"
	@./scripts/bootstrap-aws.sh $(ENV)

aws-deploy-all: aws-check ## Deploy everything to AWS (infra + app)
	@echo "$(BLUE)╔══════════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(BLUE)║           Deploying OllyStack to AWS                          ║$(NC)"
	@echo "$(BLUE)║           Environment: $(ENV)                                      ║$(NC)"
	@echo "$(BLUE)╚══════════════════════════════════════════════════════════════╝$(NC)"
	@./scripts/deploy-all.sh $(ENV)

aws-infra-init: aws-check ## Initialize Terraform
	@echo "$(BLUE)Initializing Terraform...$(NC)"
	@ACCOUNT=$$(aws sts get-caller-identity --query Account --output text) && \
	cd deploy/aws/terraform && terraform init \
		-backend-config="bucket=ollystack-tf-state-$$ACCOUNT" \
		-backend-config="key=ollystack/$(ENV)/terraform.tfstate" \
		-backend-config="region=$(AWS_REGION)"

aws-infra-plan: aws-infra-init ## Plan infrastructure changes
	@echo "$(BLUE)Planning infrastructure...$(NC)"
	cd deploy/aws/terraform && terraform plan \
		-var-file="environments/$(ENV)/terraform.tfvars" \
		-out=tfplan

aws-infra-apply: ## Apply infrastructure changes
	@echo "$(BLUE)Applying infrastructure...$(NC)"
	cd deploy/aws/terraform && terraform apply tfplan

aws-infra-destroy: ## Destroy infrastructure (DANGEROUS!)
	@echo "$(RED)WARNING: This will destroy all infrastructure!$(NC)"
	@read -p "Type 'destroy-$(ENV)' to confirm: " confirm && [ "$$confirm" = "destroy-$(ENV)" ]
	cd deploy/aws/terraform && terraform destroy \
		-var-file="environments/$(ENV)/terraform.tfvars" \
		-auto-approve

aws-configure-kubectl: aws-check ## Configure kubectl for EKS
	@echo "$(BLUE)Configuring kubectl...$(NC)"
	aws eks update-kubeconfig --region $(AWS_REGION) --name $(CLUSTER_NAME)

aws-deploy-app: aws-configure-kubectl ## Deploy application to EKS
	@echo "$(BLUE)Deploying application...$(NC)"
	kubectl apply -k deploy/kubernetes/overlays/$(ENV)
	kubectl rollout status deployment/api-server -n ollystack --timeout=300s
	@echo "$(GREEN)Application deployed!$(NC)"

aws-deploy-clickhouse: aws-configure-kubectl ## Deploy ClickHouse to EKS
	@echo "$(BLUE)Deploying ClickHouse...$(NC)"
	kubectl apply -f deploy/aws/k8s/clickhouse.yaml
	kubectl wait --for=condition=ready pod -l app=clickhouse -n ollystack --timeout=300s

aws-db-init: aws-configure-kubectl ## Initialize ClickHouse database
	@echo "$(BLUE)Initializing database...$(NC)"
	kubectl port-forward svc/clickhouse 8123:8123 -n ollystack &
	sleep 5
	curl -s "http://localhost:8123/" --data-binary @deploy/scripts/init-clickhouse.sql || true
	pkill -f "port-forward.*clickhouse" || true
	@echo "$(GREEN)Database initialized!$(NC)"

aws-status: aws-configure-kubectl ## Show deployment status
	@echo "$(BLUE)╔══════════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(BLUE)║           OllyStack Status                                    ║$(NC)"
	@echo "$(BLUE)╚══════════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@echo "$(YELLOW)Cluster:$(NC) $(CLUSTER_NAME)"
	@echo "$(YELLOW)Region:$(NC)  $(AWS_REGION)"
	@echo ""
	@echo "$(YELLOW)Pods:$(NC)"
	@kubectl get pods -n ollystack 2>/dev/null || echo "  No pods found"
	@echo ""
	@echo "$(YELLOW)Ingress URL:$(NC)"
	@kubectl get ingress ollystack-ingress -n ollystack -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "  Not available yet"
	@echo ""

aws-logs: aws-configure-kubectl ## View all logs
	kubectl logs -f -l app.kubernetes.io/part-of=ollystack -n ollystack --max-log-requests=10

aws-port-forward: aws-configure-kubectl ## Port forward services locally
	@echo "$(BLUE)Starting port forwards...$(NC)"
	@echo "  Web UI:     http://localhost:3000"
	@echo "  API:        http://localhost:8080"
	@echo "  OTLP:       localhost:4317"
	@echo "  ClickHouse: localhost:8123"
	@echo ""
	@kubectl port-forward svc/web-ui 3000:80 -n ollystack & \
	 kubectl port-forward svc/api-server 8080:8080 -n ollystack & \
	 kubectl port-forward svc/otel-collector 4317:4317 -n ollystack & \
	 kubectl port-forward svc/clickhouse 8123:8123 -n ollystack & \
	 wait

aws-ecr-login: aws-check ## Login to ECR
	@echo "$(BLUE)Logging in to ECR...$(NC)"
	@aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin $$(aws sts get-caller-identity --query Account --output text).dkr.ecr.$(AWS_REGION).amazonaws.com

aws-push-images: aws-ecr-login docker-build ## Build and push images to ECR
	@echo "$(BLUE)Pushing images to ECR...$(NC)"
	@ACCOUNT=$$(aws sts get-caller-identity --query Account --output text) && \
	docker tag $(REGISTRY)/api-server:$(VERSION) $$ACCOUNT.dkr.ecr.$(AWS_REGION).amazonaws.com/ollystack/api-server:$(VERSION) && \
	docker tag $(REGISTRY)/web-ui:$(VERSION) $$ACCOUNT.dkr.ecr.$(AWS_REGION).amazonaws.com/ollystack/web-ui:$(VERSION) && \
	docker push $$ACCOUNT.dkr.ecr.$(AWS_REGION).amazonaws.com/ollystack/api-server:$(VERSION) && \
	docker push $$ACCOUNT.dkr.ecr.$(AWS_REGION).amazonaws.com/ollystack/web-ui:$(VERSION)
	@echo "$(GREEN)Images pushed!$(NC)"

#==============================================================================
# GitHub Actions Setup
#==============================================================================

github-setup: aws-check ## Configure GitHub repository secrets for CI/CD
	@echo "$(BLUE)Setting up GitHub Actions...$(NC)"
	@./scripts/setup-github.sh
	@echo "$(GREEN)GitHub setup complete!$(NC)"
