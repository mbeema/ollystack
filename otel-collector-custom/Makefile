.PHONY: build install-ocb generate test clean docker-build docker-run

# Install OTel Collector Builder
install-ocb:
	go install go.opentelemetry.io/collector/cmd/builder@v0.96.0

# Generate/build the custom collector
build: install-ocb
	builder --config=builder-config.yaml

# Run tests for the correlation processor
test:
	cd ../otel-processor-correlation && go test -v ./...

# Clean build artifacts
clean:
	rm -rf dist/

# Build Docker image
docker-build:
	docker build -t ollystack-collector:latest -f Dockerfile ..

# Run collector locally with Docker
docker-run:
	docker run --rm -it \
		-p 4317:4317 \
		-p 4318:4318 \
		-p 8888:8888 \
		-p 13133:13133 \
		-e CLICKHOUSE_HOST=host.docker.internal \
		-e PROMETHEUS_HOST=host.docker.internal \
		ollystack-collector:latest

# Run collector locally (after build)
run:
	./dist/ollystack-collector --config=config.yaml

# Validate configuration
validate:
	./dist/ollystack-collector validate --config=config.yaml
