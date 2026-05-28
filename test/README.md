# Test Suite

This directory contains all test-related code and infrastructure for `ssm-session-client`.

## Structure

```
test/
├── README.md                    # This file
├── ACCEPTANCE_TESTS.md          # Detailed acceptance test setup and configuration
├── INTEGRATION_TESTS.md         # Integration test documentation
├── acceptance/                  # Acceptance test files (build tag: acceptance)
│   ├── main_test.go             # Test setup, infrastructure outputs, helpers
│   ├── *_acceptance_test.go     # Individual feature tests
│   └── ...
├── infra/                       # Terraform/OpenTofu infrastructure code
│   ├── main.tf                  # EC2 instances, security groups
│   ├── iam.tf                   # IAM roles and policies
│   ├── variables.tf             # Configurable variables
│   ├── outputs.tf               # Infrastructure outputs (outputs.json)
│   └── ...
└── scripts/                     # Test orchestration
    ├── Makefile                 # Test lifecycle management
    └── setup-github-oidc.sh     # GitHub OIDC setup (one-time)
```

## Quick Start

From the **repository root**, run tests:

```bash
# Unit tests
make test

# Full acceptance test cycle (provision → test → destroy)
make acceptance AWS_REGION=ap-southeast-2

# See all available targets
make help
```

See `ACCEPTANCE_TESTS.md` for detailed acceptance test setup.

## Test Types

### Unit Tests

Located in `cmd/`, `session/`, `ssmclient/`, `datachannel/`, `config/` packages.

Run with: `make test`

### Acceptance Tests

End-to-end tests against real AWS infrastructure.

- **Location:** `test/acceptance/` (build tag: `//go:build acceptance`)
- **Run with:** `make acceptance-run-local` (requires infrastructure provisioned)
- **Setup:** `make acceptance-prepare AWS_REGION=...`
- **Cleanup:** `make acceptance-destroy`

See `ACCEPTANCE_TESTS.md` for detailed configuration options.

## Infrastructure

Test infrastructure is defined in `test/infra/` using Terraform/OpenTofu.

- **Default instance:** Amazon Linux 2023 (t3.micro)
- **Optional:** Windows Server 2022, Route53 DNS, KMS key, VPC endpoints

Configure with variables in `infra/variables.tf` or pass via `TF_EXTRA_VARS`.

See `ACCEPTANCE_TESTS.md` for options.

## For Developers

### Adding Tests

1. Create file in `test/acceptance/` with `//go:build acceptance` tag
2. Import helpers from `acceptance` package (not `acceptance_test`):
   ```go
   func TestMyFeature(t *testing.T) {
       i := infra(t)
       waitForSSMReady(t, i.InstanceID)
       // ...
   }
   ```
3. Run with: `make acceptance-run-local -run TestMyFeature`

### Helpers Available

From `test/acceptance/main_test.go`:

- `infra(t)` — Get infrastructure outputs
- `waitForSSMReady(t, instanceID)` — Wait for SSM agent
- `registerSessionLeakCheck(t, instanceID)` — Verify no leaked sessions
- `terminateAllSessions(t, instanceID)` — Clean up sessions
- `runCmd(t, args...)` — Run ssm-session-client command
- `freePort(t)` — Allocate an unused port

## Documentation

- `ACCEPTANCE_TESTS.md` — Full acceptance test guide with examples
- `INTEGRATION_TESTS.md` — Integration test information
- `../../TESTING.md` — High-level testing guide (from repo root)
- `../../CLAUDE.md` — Project architecture and conventions
