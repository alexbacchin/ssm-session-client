# SSH Direct: Native SSH Client over SSM

## Overview

Integrate a native SSH client directly into `ssm-session-client`, allowing users to SSH into EC2 instances via SSM **without** needing an external SSH client or ProxyCommand configuration.

```
ssm-session-client ssh-direct [user@]target[:port]
```

### Current Flow (requires external SSH)
```
User → ssh command → ProxyCommand (ssm-session-client ssh) → SSM WebSocket → SSM Agent → sshd
```

### Proposed Flow (self-contained)
```
User → ssm-session-client ssh-direct → [embedded SSH client] → SSM WebSocket → SSM Agent → sshd
```

---

## Feasibility: CONFIRMED

- `golang.org/x/crypto/ssh` is already an indirect dependency (v0.48.0)
- `ssh.NewClientConn()` accepts any `net.Conn` — transport-agnostic by design
- The existing `port_mux.go` already proves the `net.Pipe()` bridge pattern works over SSM
- Full SSH feature set available: PTY, shell, window resize, agent forwarding, SFTP subsystem
- All major auth methods supported: public key, SSH agent, password, keyboard-interactive

---

## Architecture

```
┌──────────────────────────────────────────────────────┐
│  CLI Layer                                           │
│  cmd/ssm_ssh_direct.go                               │
│    └── cobra command: ssh-direct [user@]target[:port]│
└──────────────┬───────────────────────────────────────┘
               │
┌──────────────▼───────────────────────────────────────┐
│  Business Logic                                      │
│  pkg/ssm_ssh_direct.go                               │
│    └── StartSSHDirectSession(target, options)         │
│        - Parse target, resolve instance ID            │
│        - Build AWS config, open SSM data channel      │
│        - Create SSH client over data channel          │
│        - Launch interactive shell or exec command     │
└──────────────┬───────────────────────────────────────┘
               │
┌──────────────▼───────────────────────────────────────┐
│  SSH Direct Client                                   │
│  ssmclient/ssh_direct.go                             │
│    ├── SSHDirectSession(cfg, opts) error              │
│    ├── ssmConn (net.Conn wrapper for SsmDataChannel)  │
│    └── SSH auth, PTY, shell, window resize            │
└──────────────┬───────────────────────────────────────┘
               │
┌──────────────▼───────────────────────────────────────┐
│  Existing Infrastructure (no changes needed)         │
│  datachannel/data_channel.go                         │
│    └── SsmDataChannel (WebSocket + SSM protocol)     │
└──────────────────────────────────────────────────────┘
```

---

## Implementation Plan

### Phase 1: Core `net.Conn` Wrapper

**File: `ssmclient/ssm_conn.go`**

Create a `net.Conn` implementation that wraps `SsmDataChannel`, allowing `golang.org/x/crypto/ssh` to use it as a transport.

```go
type ssmConn struct {
    channel   *datachannel.SsmDataChannel
    localAddr net.Addr
    remoteAddr net.Addr
}
```

**Key design decisions:**
- Use `net.Pipe()` bridge pattern (proven in `port_mux.go`) rather than a custom `net.Conn` wrapper. This is simpler and avoids needing to handle the SSM message protocol in `Read()` — the existing `WriteTo()`/`ReadFrom()` methods already handle message decoding/encoding correctly.
- Two goroutines bridge: `io.Copy(pipeConn, ssmDataChannel)` and `io.Copy(ssmDataChannel, pipeConn)`
- The SSH library uses `localConn` (the other end of the pipe) as its `net.Conn`
- Deadline methods can be no-ops — SSH manages its own timeouts via goroutines/channels

### Phase 2: SSH Authentication

**File: `ssmclient/ssh_direct.go`**

Support multiple authentication methods (tried in order):

1. **SSH Agent** (if `SSH_AUTH_SOCK` is set) — uses existing keys from ssh-agent
2. **Private key files** — auto-discover `~/.ssh/id_ed25519`, `~/.ssh/id_rsa`, or user-specified key
3. **Password** — interactive prompt (keyboard-interactive fallback)

```go
func buildSSHAuthMethods(keyFile string) []ssh.AuthMethod {
    var methods []ssh.AuthMethod

    // 1. Try SSH agent
    if agent, err := connectSSHAgent(); err == nil {
        methods = append(methods, ssh.PublicKeysCallback(agent.Signers))
    }

    // 2. Try key files
    if signer, err := loadSSHKey(keyFile); err == nil {
        methods = append(methods, ssh.PublicKeys(signer))
    }

    // 3. Password prompt fallback
    methods = append(methods, ssh.PasswordCallback(promptPassword))

    return methods
}
```

### Phase 3: Interactive Shell with PTY

**File: `ssmclient/ssh_direct.go`**

After establishing the SSH connection, request a PTY and start an interactive shell:

```go
func runInteractiveSSHSession(client *ssh.Client, termType string) error {
    session, err := client.NewSession()

    // Request PTY with current terminal size
    w, h := getTerminalSize()
    session.RequestPty(termType, h, w, ssh.TerminalModes{...})

    // Connect stdio
    session.Stdin = os.Stdin
    session.Stdout = os.Stdout
    session.Stderr = os.Stderr

    // Set terminal to raw mode (reuse existing shell_posix.go logic)
    // Handle SIGWINCH for terminal resize → session.WindowChange()

    session.Shell()
    session.Wait()
}
```

**Terminal handling:** Reuse the existing platform-specific terminal code from `ssmclient/shell_posix.go` and `ssmclient/shell_windows.go` for raw mode and signal handling.

### Phase 4: Host Key Verification

**Strategy (layered):**

1. **Check `~/.ssh/known_hosts`** using `golang.org/x/crypto/ssh/knownhosts` — look up by instance ID and/or IP
2. **Trust-on-first-use (TOFU)** — if not found, display the key fingerprint and prompt the user to accept (interactive mode) or reject (non-interactive)
3. **`--no-host-key-check` flag** — skip verification entirely (with warning). Reasonable since SSM transport is already IAM-authenticated and TLS-encrypted
4. **Save accepted keys** — optionally append to `~/.ssh/known_hosts` after TOFU acceptance

### Phase 5: CLI Command

**File: `cmd/ssm_ssh_direct.go`**

```go
var ssmSshDirectCmd = &cobra.Command{
    Use:   "ssh-direct [user@]target[:port]",
    Short: "SSH directly to an EC2 instance via SSM",
    Long:  `Start a direct SSH session to an EC2 instance through AWS SSM Session Manager.
Unlike the 'ssh' command which acts as a proxy for an external SSH client,
ssh-direct provides a fully integrated SSH experience with no external dependencies.`,
    Args: cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        pkg.InitializeClient()
        pkg.StartSSHDirectSession(args[0])
    },
}
```

**Additional flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--ssh-key` | Path to SSH private key file | Auto-discover |
| `--no-host-key-check` | Skip host key verification | `false` |
| `--exec` | Execute command instead of interactive shell | (none) |
| `--ssh-port` | Remote SSH port | `22` |

### Phase 6: Command Execution Mode

Support non-interactive command execution:

```
ssm-session-client ssh-direct ec2-user@i-0123456789 --exec "uptime"
ssm-session-client ssh-direct ec2-user@i-0123456789 --exec "cat /etc/os-release"
```

When `--exec` is specified, run the command via `session.Run(cmd)` instead of `session.Shell()`, print output, and exit with the remote command's exit code.

---

## File Summary

| File | Action | Description |
|------|--------|-------------|
| `ssmclient/ssm_conn.go` | **New** | `net.Conn` wrapper using `net.Pipe()` bridge over `SsmDataChannel` |
| `ssmclient/ssh_direct.go` | **New** | Core SSH direct session: auth, PTY, shell, resize, command exec |
| `pkg/ssm_ssh_direct.go` | **New** | Business logic: parse target, build config, orchestrate session |
| `cmd/ssm_ssh_direct.go` | **New** | Cobra CLI command definition and flags |
| `config/flags.go` | **Edit** | Add `SSHKeyFile`, `NoHostKeyCheck`, `SSHExecCommand` fields |
| `config/util.go` | **Edit** | Add `FindSSHPrivateKey()` helper (similar to existing `FindSSHPublicKey()`) |
| `go.mod` | **Auto** | `golang.org/x/crypto/ssh` promoted from indirect to direct |

---

## Integration with EC2 Instance Connect

The existing `instance-connect` command can be enhanced to use `ssh-direct` mode internally:

```
ssm-session-client instance-connect --direct [target]
```

This would:
1. Push ephemeral public key via EC2 Instance Connect API (existing code)
2. Use `ssh-direct` to connect with the corresponding private key (new code)
3. No external SSH client needed at all

This is a **future enhancement** — not part of the initial implementation.

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `net.Pipe()` deadlock during SSH handshake | SSH connection fails | Bridge goroutines run concurrently, providing buffering. Same pattern works for smux in `port_mux.go`. |
| Double encryption (SSH + KMS) | Performance overhead | Functionally correct. KMS encryption is optional and depends on SSM config. |
| Host key unknown for new instances | UX friction | TOFU with interactive prompt + `--no-host-key-check` flag |
| SSH agent not available | Auth fails | Fallback chain: agent → key file → password prompt |
| Large file transfers over SSH/SCP | May be slow due to SSM overhead | Port forwarding + SFTP is better for bulk transfers. Document this. |

---

## Testing Strategy

1. **Unit tests** for `ssmConn` wrapper (mock `SsmDataChannel`)
2. **Unit tests** for SSH auth method builder
3. **Unit tests** for host key callback logic
4. **Integration test** with real EC2 instance: interactive shell
5. **Integration test** with real EC2 instance: command execution
6. **Cross-platform** build verification (Linux, macOS, Windows)

---

## Dependencies

- `golang.org/x/crypto/ssh` — already indirect dep (v0.48.0), promote to direct
- `golang.org/x/crypto/ssh/agent` — SSH agent support
- `golang.org/x/crypto/ssh/knownhosts` — host key verification
- No new external dependencies required
