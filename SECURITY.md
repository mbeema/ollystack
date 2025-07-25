# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

### How to Report

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to: **security@ollystack.io**

Include the following information:
- Type of vulnerability
- Full paths of source file(s) related to the vulnerability
- Location of the affected source code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit it

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Fix Timeline**: Depends on severity (see below)

### Severity Levels

| Severity | Response Time | Example |
|----------|---------------|---------|
| Critical | 24-48 hours   | Remote code execution, data breach |
| High     | 7 days        | Authentication bypass, SQL injection |
| Medium   | 30 days       | XSS, CSRF, information disclosure |
| Low      | 90 days       | Minor issues, hardening |

## Security Best Practices

When deploying OllyStack, follow these security recommendations:

### Authentication & Authorization

- Enable authentication in production (`auth.enabled: true`)
- Use strong JWT secrets (minimum 256 bits)
- Implement RBAC for multi-tenant deployments
- Rotate API keys regularly

### Network Security

- Use TLS for all connections
- Deploy behind a reverse proxy (nginx, Traefik)
- Restrict network access to ClickHouse and Redis
- Use Kubernetes NetworkPolicies in K8s deployments

### Secrets Management

- Never commit secrets to version control
- Use environment variables or secret managers
- Rotate credentials regularly
- Use separate credentials for each environment

### ClickHouse Security

- Change default passwords
- Create dedicated users with minimal privileges
- Enable TLS for ClickHouse connections
- Restrict query capabilities as needed

### Monitoring

- Enable audit logging
- Monitor for unusual query patterns
- Set up alerts for authentication failures
- Review access logs regularly

## Security Features

OllyStack includes these security features:

- **CORS Configuration**: Configurable allowed origins
- **Rate Limiting**: Prevent abuse
- **Input Validation**: Request size limits, query validation
- **Security Headers**: X-Frame-Options, CSP, etc.
- **TLS Support**: Optional TLS termination

## Acknowledgments

We appreciate security researchers who help keep OllyStack secure. Responsible disclosure will be acknowledged in our security advisories.

## Contact

For security concerns: security@ollystack.io
For general questions: [GitHub Discussions](https://github.com/ollystack/ollystack/discussions)
