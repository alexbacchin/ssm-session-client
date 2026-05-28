# Acceptance Tests

End-to-end tests that exercise `ssm-session-client` against real AWS infrastructure.

## Prerequisites

- AWS credentials with permissions to provision EC2 instances, IAM roles, and SSM
- [OpenTofu](https://opentofu.org/) or Terraform
- Go 1.21+
- `jq` (used by Makefile targets)

## Directory Layout

```
test/
  acceptance/   Go test files (build tag: acceptance)
  infra/        Terraform/OpenTofu configuration for test infrastructure
  scripts/      Makefile for lifecycle management
```

## Quick Start

All commands are run from `test/scripts/`.

```bash
cd test/scripts

# Full lifecycle: provision infrastructure, run tests, tear down
make acceptance AWS_REGION=ap-southeast-2
```

## Makefile Targets

Run `make help` to see all targets:

| Target | Description |
|---|---|
| `acceptance` | Full lifecycle: provision, wait for SSM, test, destroy (destroy always runs) |
| `acceptance-prepare` | Provision test infrastructure with Terraform/OpenTofu |
| `acceptance-wait-ssm` | Wait for SSM agent to come online on provisioned instances |
| `acceptance-run-local` | Run acceptance tests (requires infrastructure to be provisioned) |
| `acceptance-run-ci` | Run acceptance tests in CI (same as local, relies on OIDC credentials) |
| `acceptance-destroy` | Tear down all test infrastructure |
| `acceptance-backend-init` | Reconfigure the Terraform backend (state bucket) |
| `acceptance-sso-interactive` | Run interactive SSO login test (requires browser approval) |

## Running Tests

### Full Lifecycle

Provisions infrastructure, runs all tests, then destroys infrastructure (even on failure):

```bash
make acceptance AWS_REGION=ap-southeast-2
```

### Step-by-Step

Useful when iterating on tests without reprovisioning each time:

```bash
# 1. Provision infrastructure (once)
make acceptance-prepare AWS_REGION=ap-southeast-2

# 2. Wait for SSM agent
make acceptance-wait-ssm

# 3. Run tests (repeat as needed)
make acceptance-run-local

# 4. Tear down when done
make acceptance-destroy
```

### Running a Specific Test

Pass `ACCEPTANCE_ARGS` to filter by test name:

```bash
# Single test
make acceptance-run-local ACCEPTANCE_ARGS='-run TestSSHCompatBasic'

# Pattern match
make acceptance-run-local ACCEPTANCE_ARGS='-run TestSSHCompat'

# Multiple specific tests
make acceptance-run-local ACCEPTANCE_ARGS='-run "TestSSHCompatBasic|TestSSHDirectInstanceConnect"'
```

Or run `go test` directly from the repository root:

```bash
go test ./test/acceptance/... -tags acceptance -v -timeout 20m -count=1 -race -run TestSSHCompatBasic
```

### SSO Tests

The interactive SSO test requires a human to approve the login in a browser:

```bash
make acceptance-sso-interactive SSO_PROFILE=my-sso-profile SSO_TEST_TIMEOUT=10
```

### Windows / RDP Tests

RDP tests require `create_windows_instance=true` during provisioning and must run on a Windows host:

```bash
make acceptance-prepare TF_EXTRA_VARS='-var="create_windows_instance=true"'
make acceptance-run-local ACCEPTANCE_ARGS='-run "RDP"'
```

## Available Tests

### Shell Sessions

| Test | Description |
|---|---|
| `TestShellByInstanceID` | Shell session using instance ID |
| `TestShellByTag` | Shell session using tag-based target resolution |
| `TestShellByAlias` | Shell session using a configured alias |
| `TestShellByPrivateIP` | Shell session using private IP address |
| `TestShellByDNSTXT` | Shell session using DNS TXT record resolution |

### SSH Direct

| Test | Description |
|---|---|
| `TestSSHDirectInstanceConnect` | SSH direct with EC2 Instance Connect ephemeral key |
| `TestSSHDirectCustomPort` | SSH direct specifying a custom port |

### SSH Compat (OpenSSH-compatible mode)

| Test | Description |
|---|---|
| `TestSSHCompatVersionFlag` | `-V` flag returns OpenSSH version string |
| `TestSSHCompatBasic` | Minimal SSH compat: `-T user@host echo marker` |
| `TestSSHCompatVSCodeStyle` | VSCode Remote SSH flag pattern: `-T -D <port> -o ...` |
| `TestSSHCompatWithSSHConfig` | SSH compat reading from an SSH config file (`-F`) |
| `TestSSHCompatWithIdentityFile` | SSH compat with explicit identity file (`-i`) |
| `TestSSHCompatWithLoginUser` | SSH compat with `-l user` flag |
| `TestSSHCompatVSCodeStylePipedStdin` | VSCode install pattern: script piped via stdin |
| `TestSSHCompatTOFUNewHost` | Trust-On-First-Use host key prompt and acceptance |
| `TestSSHCompatCompoundFlags` | Compound boolean flags like `-Tv` |

### SSH Proxy

| Test | Description |
|---|---|
| `TestSSHProxyByInstanceID` | SSH proxy mode using system `ssh` binary |

### EC2 Instance Connect

| Test | Description |
|---|---|
| `TestInstanceConnectPushKey` | Push an ephemeral key via EC2 Instance Connect API |
| `TestInstanceConnectThenSSHDirect` | Push key then SSH direct session |

### Port Forwarding

| Test | Description |
|---|---|
| `TestPortForwardingToSSHPort` | Port forward to SSH port and verify TCP connectivity |
| `TestPortForwardingMultipleConnections` | Multiple concurrent connections through a forwarded port |
| `TestPortForwardingToRDPPort` | Port forward to RDP port (requires Windows instance) |

### RDP (Windows only)

| Test | Description |
|---|---|
| `TestRDPTunnelEstablished` | RDP tunnel starts and accepts TCP connections |
| `TestRDPGetPassword` | Retrieve EC2 Windows password and establish RDP tunnel |

### Target Resolution

| Test | Description |
|---|---|
| `TestResolveByTag` | Resolve target by EC2 tag |
| `TestResolveByIP` | Resolve target by private IP |
| `TestResolveMultipleInstances` | Error when multiple instances match |
| `TestResolveNotFound` | Error when no instances match |

### SSO Authentication

| Test | Description |
|---|---|
| `TestSSOLoginWithCachedToken` | SSO login using a cached token |
| `TestSSOLoginInteractive` | Interactive SSO login (requires browser, human approval) |

### Error Handling

| Test | Description |
|---|---|
| `TestInvalidTarget` | Graceful error for an invalid target |
| `TestMissingRegion` | Graceful error when region is not configured |
| `TestSessionTermination` | Clean session termination on signal |

## Configuration Variables

| Variable | Default | Description |
|---|---|---|
| `AWS_REGION` | From AWS config or `ap-southeast-2` | AWS region for infrastructure and tests |
| `TF_CMD` | `terraform` or `tofu` (auto-detected) | Terraform/OpenTofu binary |
| `TF_EXTRA_VARS` | (empty) | Additional `-var` flags for Terraform |
| `TF_STATE_BUCKET` | Auto-computed from AWS account ID | S3 bucket for Terraform state |
| `SSM_WAIT_SECS` | `300` | Timeout waiting for SSM agent readiness |
| `SSM_POLL_SECS` | `10` | Poll interval for SSM agent readiness check |
| `SSO_PROFILE` | (required for SSO tests) | AWS SSO profile name |
| `SSO_TEST_TIMEOUT` | `10` | Timeout in minutes for SSO interactive test |
| `ACCEPTANCE_ARGS` | (empty) | Extra args passed to `go test` (e.g. `-run TestName`) |
