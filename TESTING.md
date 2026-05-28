# Testing Guide

This guide explains how to run tests for `ssm-session-client` from the repository root.

## Quick Start

All testing can be done from the repository root using `make` targets:

```bash
# Run unit tests
make test

# Run all acceptance tests (requires AWS credentials and infrastructure setup)
make acceptance AWS_REGION=ap-southeast-2

# Build the binary
make build
```

## Unit Tests

Run unit tests with race detector:

```bash
make test
```

This runs all tests in the `cmd/`, `session/`, `ssmclient/`, `datachannel/`, and `config/` packages with the race detector enabled.

## Acceptance Tests

Acceptance tests exercise the CLI against real AWS infrastructure. They require:

- AWS credentials with permissions to:
  - Provision EC2 instances
  - Create IAM roles
  - Use AWS Systems Manager (SSM)
  - Manage Route53 DNS (optional)
  - Create KMS keys (optional)
- [OpenTofu](https://opentofu.org/) or Terraform
- Go 1.21+
- `jq` (used by Makefile targets)

### Full Lifecycle

Run the complete acceptance test cycle (provision infrastructure → run tests → destroy infrastructure):

```bash
make acceptance AWS_REGION=ap-southeast-2
```

This is the recommended way to run acceptance tests. The infrastructure is automatically destroyed after tests complete, even if tests fail.

### Manual Steps

If you want to manage infrastructure separately:

```bash
# 1. Provision test infrastructure
make acceptance-prepare AWS_REGION=ap-southeast-2

# 2. Wait for SSM agent to come online
make acceptance-wait-ssm

# 3. Run tests multiple times without reprovisioning
make acceptance-run-local
make acceptance-run-local
```

### Run Specific Tests

Run specific acceptance tests by passing additional arguments:

```bash
make acceptance-run-local ACCEPTANCE_ARGS="-run TestPortForwardingToSSHPort"
```

## Coverage Report

Generate a coverage report:

```bash
make cover
```

This creates `coverage.html` showing which code paths are covered by tests.

## Code Quality

Format code:

```bash
make fmt
```

Run linter:

```bash
make lint
```

## Directory Structure

```
├── Makefile                    # Root Makefile — convenient test targets
├── TESTING.md                  # This file
├── test/
│   ├── README.md               # Test suite overview
│   ├── ACCEPTANCE_TESTS.md     # Detailed acceptance test documentation
│   ├── INTEGRATION_TESTS.md    # Integration test documentation
│   ├── acceptance/             # Acceptance test files (build tag: acceptance)
│   ├── infra/                  # Terraform/OpenTofu infrastructure code
│   └── scripts/
│       └── Makefile            # Test orchestration Makefile
└── ...
```

## Advanced Targets

### Interactive SSO Testing

Test AWS SSO login interactively (requires a human to approve in a browser):

```bash
make acceptance-sso-interactive SSO_PROFILE=my-sso-profile
```

### Terraform Backend Setup

Reconfigure the Terraform state bucket (one-time setup):

```bash
make acceptance-backend-init AWS_REGION=ap-southeast-2
```

### CI Mode

Run acceptance tests in CI (uses OIDC credentials instead of credentials files):

```bash
make acceptance-run-ci
```

## Troubleshooting

### "SSO_PROFILE is required"

When running `make acceptance-sso-interactive`, you must provide an AWS SSO profile name:

```bash
make acceptance-sso-interactive SSO_PROFILE=my-profile
```

### Infrastructure Left Behind

If the acceptance test lifecycle is interrupted (e.g., manual kill), infrastructure may remain:

```bash
# Clean up manually
make acceptance-destroy AWS_REGION=ap-southeast-2
```

### Terraform State Issues

If Terraform state becomes corrupted, reinitialize the backend:

```bash
make acceptance-backend-init AWS_REGION=ap-southeast-2
```

## For Developers

### Adding New Tests

1. Create test file in `test/acceptance/` with `//go:build acceptance` tag
2. Import from `acceptance` package (not `acceptance_test`)
3. Use helpers from `main_test.go`: `infra()`, `waitForSSMReady()`, `registerSessionLeakCheck()`, etc.
4. Run: `make acceptance-run-local -run TestYourNewTest`

### Test Infrastructure

The acceptance test infrastructure (EC2 instance, IAM roles, security groups) is defined in `test/infra/`:

- `main.tf` — Primary test instance, security groups, IAM setup
- `iam.tf` — IAM roles and policies
- `outputs.tf` — Exports instance ID, region, etc. to `outputs.json`
- `variables.tf` — Configurable options

Common variables:
- `region` — AWS region
- `create_dns_record` — Enable Route53 TXT record for DNS resolver tests
- `create_kms_key` — Enable KMS key for encryption tests
- `create_windows_instance` — Enable Windows Server instance for RDP tests

## More Information

- `test/README.md` — Overview of test directory structure
- `test/ACCEPTANCE_TESTS.md` — Detailed acceptance test setup and configuration
- `test/INTEGRATION_TESTS.md` — Integration test documentation
- `test/acceptance/main_test.go` — Acceptance test helpers and setup
