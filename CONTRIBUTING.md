# Contributing to OllyStack

Thank you for your interest in contributing to OllyStack! This document provides guidelines and information for contributors.

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## How to Contribute

### Reporting Issues

- Check if the issue already exists in [GitHub Issues](https://github.com/ollystack/ollystack/issues)
- Use the issue templates when available
- Provide detailed reproduction steps
- Include relevant logs and environment information

### Feature Requests

- Open a GitHub issue with the "feature request" label
- Describe the problem you're trying to solve
- Explain your proposed solution
- Consider if it aligns with OllyStack's philosophy: "Build where we differentiate, integrate where commoditized"

### Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Set up your development environment:**
   ```bash
   git clone https://github.com/YOUR_USERNAME/ollystack.git
   cd ollystack
   make dev-up
   ```
3. **Make your changes** following our coding standards
4. **Write tests** for any new functionality
5. **Update documentation** if needed
6. **Submit a pull request** with a clear description

## Development Setup

### Prerequisites

- Docker & Docker Compose
- Go 1.22+ (for Go services)
- Rust 1.75+ (for stream processor)
- Node.js 20+ (for web UI)
- Make

### Running Locally

```bash
# Start all services
make dev-up

# Run tests
make test

# Build all components
make build
```

### Project Structure

```
ollystack/
├── api-server/          # Go - REST/GraphQL API
├── web-ui/              # TypeScript/React - Dashboard
├── stream-processor/    # Rust - Real-time processing
├── ingestion-gateway/   # Go - OTLP receiver
├── ai-engine/           # Python - ML/AI features
├── collector/           # Go - OTel Collector extensions
├── deploy/              # Docker, Kubernetes, Terraform
└── docs/                # Documentation
```

## Coding Standards

### Go

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` and `golint`
- Write table-driven tests
- Handle errors explicitly

### Rust

- Follow the [Rust Style Guide](https://doc.rust-lang.org/1.0.0/style/README.html)
- Use `cargo fmt` and `cargo clippy`
- Write unit tests and integration tests

### TypeScript/React

- Follow the existing code style
- Use TypeScript strict mode
- Write component tests with React Testing Library

### Python

- Follow PEP 8
- Use type hints
- Write pytest tests

## Key Areas for Contribution

We especially welcome contributions in these areas:

1. **Custom OTel Collector Processors** - Enrichment, sampling, transformation
2. **Stream Processor Analytics** - Anomaly detection algorithms, correlation
3. **AI/ML Models** - Improved anomaly detection, NLQ understanding
4. **Web UI Components** - Visualization, dashboard widgets
5. **Documentation** - Tutorials, examples, translations
6. **Testing** - Unit tests, integration tests, benchmarks

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Examples:
- `feat(api): add trace correlation endpoint`
- `fix(ui): resolve service map rendering issue`
- `docs: update deployment guide`

## Pull Request Process

1. Ensure all tests pass
2. Update relevant documentation
3. Add entry to CHANGELOG.md (if applicable)
4. Request review from maintainers
5. Address review feedback
6. Squash commits if requested

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

## Questions?

- Open a [GitHub Discussion](https://github.com/ollystack/ollystack/discussions)
- Join our [Discord](https://discord.gg/ollystack)

Thank you for contributing to OllyStack!
