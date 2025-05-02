#!/bin/bash
# OllyStack GitHub Repository Setup Script
# Configures GitHub secrets and environments for CI/CD
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="${PROJECT_NAME:-ollystack}"
AWS_REGION="${AWS_REGION:-us-east-2}"
GITHUB_REPO="${GITHUB_REPO:-}"

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check GitHub CLI
    if ! command -v gh &> /dev/null; then
        log_error "GitHub CLI (gh) is not installed. Install from: https://cli.github.com/"
    fi

    # Check if authenticated
    if ! gh auth status &>/dev/null; then
        log_error "Not authenticated with GitHub. Run: gh auth login"
    fi

    # Check AWS CLI
    if ! command -v aws &> /dev/null; then
        log_error "AWS CLI is not installed."
    fi

    log_success "Prerequisites met"
}

detect_repo() {
    if [ -n "${GITHUB_REPO}" ]; then
        log_info "Using specified repository: ${GITHUB_REPO}"
        return
    fi

    # Try to detect from git remote
    if git remote get-url origin &>/dev/null; then
        local remote=$(git remote get-url origin)

        # Handle SSH format: git@github.com:owner/repo.git
        if [[ "${remote}" =~ git@github\.com:(.+)/(.+)\.git ]]; then
            GITHUB_REPO="${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
        # Handle HTTPS format: https://github.com/owner/repo.git
        elif [[ "${remote}" =~ github\.com/(.+)/(.+)(\.git)?$ ]]; then
            GITHUB_REPO="${BASH_REMATCH[1]}/${BASH_REMATCH[2]%.git}"
        fi
    fi

    if [ -z "${GITHUB_REPO}" ]; then
        log_error "Could not detect GitHub repository. Set GITHUB_REPO environment variable."
    fi

    log_info "Detected repository: ${GITHUB_REPO}"
}

get_aws_info() {
    log_info "Getting AWS account information..."

    if ! aws sts get-caller-identity &>/dev/null; then
        log_warn "AWS credentials not configured. Some secrets will need manual input."
        ACCOUNT_ID=""
        return
    fi

    ACCOUNT_ID=$(aws sts get-caller-identity --query "Account" --output text)
    ECR_REGISTRY="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

    log_success "AWS Account: ${ACCOUNT_ID}"
}

prompt_secret() {
    local name=$1
    local description=$2
    local default=${3:-}

    echo ""
    echo -e "${CYAN}${name}${NC}: ${description}"
    if [ -n "${default}" ]; then
        echo -e "  Default: ${default}"
        read -p "  Enter value (or press Enter for default): " value
        echo "${value:-${default}}"
    else
        read -p "  Enter value: " value
        echo "${value}"
    fi
}

set_github_secret() {
    local name=$1
    local value=$2

    if [ -z "${value}" ]; then
        log_warn "Skipping empty secret: ${name}"
        return
    fi

    log_info "Setting secret: ${name}"
    echo "${value}" | gh secret set "${name}" -R "${GITHUB_REPO}"
}

configure_secrets() {
    log_info "Configuring GitHub repository secrets..."

    echo ""
    echo "==========================================="
    echo "  Configure AWS Credentials"
    echo "==========================================="
    echo ""
    echo "Enter your AWS credentials for CI/CD."
    echo "These will be stored as encrypted GitHub secrets."
    echo ""

    # AWS credentials
    local aws_access_key_id=$(prompt_secret "AWS_ACCESS_KEY_ID" "AWS Access Key ID")
    local aws_secret_access_key=$(prompt_secret "AWS_SECRET_ACCESS_KEY" "AWS Secret Access Key")
    local aws_region=$(prompt_secret "AWS_REGION" "AWS Region" "${AWS_REGION}")

    # Derived values
    local aws_account_id="${ACCOUNT_ID}"
    if [ -z "${aws_account_id}" ]; then
        aws_account_id=$(prompt_secret "AWS_ACCOUNT_ID" "AWS Account ID")
    fi

    local ecr_registry="${aws_account_id}.dkr.ecr.${aws_region}.amazonaws.com"

    # Terraform state
    local tf_state_bucket=$(prompt_secret "TF_STATE_BUCKET" "Terraform state S3 bucket" "${PROJECT_NAME}-terraform-state-${aws_region}")
    local tf_lock_table=$(prompt_secret "TF_LOCK_TABLE" "Terraform lock DynamoDB table" "${PROJECT_NAME}-terraform-lock")

    echo ""
    echo "==========================================="
    echo "  Optional Integrations"
    echo "==========================================="
    echo ""

    local slack_webhook=$(prompt_secret "SLACK_WEBHOOK_URL" "Slack webhook URL for notifications (optional)")
    local codecov_token=$(prompt_secret "CODECOV_TOKEN" "Codecov token for coverage reports (optional)")

    # Set secrets
    echo ""
    log_info "Setting GitHub secrets..."

    set_github_secret "AWS_ACCESS_KEY_ID" "${aws_access_key_id}"
    set_github_secret "AWS_SECRET_ACCESS_KEY" "${aws_secret_access_key}"
    set_github_secret "AWS_REGION" "${aws_region}"
    set_github_secret "AWS_ACCOUNT_ID" "${aws_account_id}"
    set_github_secret "ECR_REGISTRY" "${ecr_registry}"
    set_github_secret "TF_STATE_BUCKET" "${tf_state_bucket}"
    set_github_secret "TF_LOCK_TABLE" "${tf_lock_table}"

    if [ -n "${slack_webhook}" ]; then
        set_github_secret "SLACK_WEBHOOK_URL" "${slack_webhook}"
    fi

    if [ -n "${codecov_token}" ]; then
        set_github_secret "CODECOV_TOKEN" "${codecov_token}"
    fi

    log_success "Secrets configured"
}

configure_environments() {
    log_info "Configuring GitHub environments..."

    echo ""
    echo "Creating deployment environments..."
    echo ""

    # Create dev environment
    log_info "Creating 'dev' environment..."
    gh api -X PUT "repos/${GITHUB_REPO}/environments/dev" \
        --silent || log_warn "Could not create dev environment (may already exist)"

    # Create prod environment with protection
    log_info "Creating 'prod' environment with protection rules..."
    gh api -X PUT "repos/${GITHUB_REPO}/environments/prod" \
        -f "wait_timer=5" \
        --silent || log_warn "Could not create prod environment (may already exist)"

    log_success "Environments configured"

    echo ""
    echo -e "${YELLOW}Note:${NC} For production protection rules (required reviewers),"
    echo "please configure manually in GitHub:"
    echo "  Settings > Environments > prod > Required reviewers"
}

configure_branch_protection() {
    log_info "Configuring branch protection for main..."

    # Note: This requires admin access to the repository
    gh api -X PUT "repos/${GITHUB_REPO}/branches/main/protection" \
        -f "required_status_checks[strict]=true" \
        -f "required_status_checks[contexts][]=ci / api-server" \
        -f "required_status_checks[contexts][]=ci / web-ui" \
        -f "enforce_admins=false" \
        -f "required_pull_request_reviews[required_approving_review_count]=1" \
        -f "allow_force_pushes=false" \
        -f "allow_deletions=false" \
        --silent 2>/dev/null || log_warn "Could not configure branch protection (may require admin access)"

    log_success "Branch protection configured"
}

verify_setup() {
    log_info "Verifying GitHub configuration..."

    echo ""
    echo "Current secrets (names only):"
    gh secret list -R "${GITHUB_REPO}"

    echo ""
    echo "Current environments:"
    gh api "repos/${GITHUB_REPO}/environments" --jq '.environments[].name' 2>/dev/null || echo "  (none or not accessible)"
}

print_summary() {
    echo ""
    echo "==========================================="
    echo "  GitHub Setup Complete!"
    echo "==========================================="
    echo ""
    echo "Repository: ${GITHUB_REPO}"
    echo ""
    echo "Configured secrets:"
    echo "  - AWS_ACCESS_KEY_ID"
    echo "  - AWS_SECRET_ACCESS_KEY"
    echo "  - AWS_REGION"
    echo "  - AWS_ACCOUNT_ID"
    echo "  - ECR_REGISTRY"
    echo "  - TF_STATE_BUCKET"
    echo "  - TF_LOCK_TABLE"
    echo ""
    echo "Configured environments:"
    echo "  - dev"
    echo "  - prod (with wait timer)"
    echo ""
    echo "Next steps:"
    echo "  1. Push code to trigger CI/CD pipeline"
    echo "  2. Create a tag (v1.0.0) to trigger release"
    echo ""
    echo "Manual configuration needed:"
    echo "  - Add required reviewers to prod environment"
    echo "  - Configure CODEOWNERS file if needed"
    echo ""
}

show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Configure GitHub repository for OllyStack CI/CD"
    echo ""
    echo "Options:"
    echo "  -h, --help     Show this help message"
    echo "  --secrets-only Configure secrets only (skip environments)"
    echo "  --verify       Verify current configuration"
    echo ""
    echo "Environment variables:"
    echo "  GITHUB_REPO    Repository in format owner/repo"
    echo "  AWS_REGION     AWS region (default: us-east-2)"
    echo "  PROJECT_NAME   Project name (default: ollystack)"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Interactive setup"
    echo "  GITHUB_REPO=myorg/ollystack $0        # Specify repo"
    echo "  $0 --verify                          # Check current config"
    echo ""
}

# Main execution
main() {
    case "${1:-}" in
        -h|--help)
            show_help
            exit 0
            ;;
        --verify)
            check_prerequisites
            detect_repo
            verify_setup
            exit 0
            ;;
        --secrets-only)
            SECRETS_ONLY=true
            ;;
    esac

    echo ""
    echo "==========================================="
    echo "  OllyStack GitHub Setup"
    echo "==========================================="
    echo ""

    check_prerequisites
    detect_repo
    get_aws_info
    configure_secrets

    if [ "${SECRETS_ONLY:-false}" != "true" ]; then
        configure_environments
        # configure_branch_protection  # Uncomment if you have admin access
    fi

    verify_setup
    print_summary
}

main "$@"
