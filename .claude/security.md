# Security Review - ssm-session-client

**Date**: 2026-02-14
**Reviewer**: Claude Opus 4.6 (AI-assisted)
**Scope**: Full codebase security review

---

## Positive Findings

- Proper AES-256-GCM encryption with random nonces and KMS key management
- SHA-256 payload integrity validation on all agent messages
- SSO cache files written with 0600 permissions (file level)
- Port forwarding listeners bind to `localhost` only
- No hardcoded credentials, no `unsafe` package usage
- Input validation on targets (regex), host:port parsing via `net` package
- `govulncheck` reports 0 directly-called vulnerabilities

---

## Issues Found

### HIGH Severity

#### 1. Missing bounds check in `UnmarshalBinary` - panic on crafted messages

**File**: `datachannel/agent_message.go:85-104`

No bounds checking on `data` length before slicing at fixed offsets (`data[4:36]`, `data[36:40]`, `data[64:80]`, etc.). While `Read()` checks for `agentMsgHeaderLen` (116 bytes), the unmarshal also accesses `data[m.headerLength : m.headerLength+4]` and `data[payloadLenEnd : payloadLenEnd+m.payloadLength]` where `payloadLength` comes from the message itself. A crafted message with a large `payloadLength` would panic with an index out-of-range.

**Remediation**: Add length validation on `data` before accessing fixed offsets and before using attacker-controlled `payloadLength` to slice the buffer. Check that `len(data) >= agentMsgHeaderLen+4` before header parsing, and that `payloadLenEnd + m.payloadLength <= uint32(len(data))` before reading the payload.

---

#### 2. Encryption key material never zeroed from memory

**File**: `datachannel/encryption.go:105-106`, `datachannel/data_channel.go:725-726`

The plaintext data key from `KMS GenerateDataKey` (`output.Plaintext`) is never zeroed after being copied into `encryptKey`/`decryptKey`. The keys on `SsmDataChannel` are never cleared on `Close()` either. This leaves cryptographic key material in heap memory indefinitely.

**Remediation**: After splitting `output.Plaintext` into encrypt/decrypt keys in `GenerateEncryptionKeys()`, zero the plaintext slice. Add a `zeroKeys()` method to `SsmDataChannel` that zeros `encryptKey` and `decryptKey`, and call it from `Close()`. Use a helper like `for i := range key { key[i] = 0 }` to prevent compiler optimization from eliding the zeroing.

---

#### 3. Data races on encryption and session state fields

**File**: `datachannel/data_channel.go`

The fields `encryptionEnabled`, `encryptKey`, `decryptKey`, `pausePub`, and `agentVersion` are accessed from multiple goroutines without synchronization. The `mu` mutex only protects WebSocket writes, not these fields. This can cause data races and potentially send unencrypted data on the wire.

**Remediation**: Protect these with the existing `mu` mutex, use `sync/atomic` for boolean flags, or use `sync.RWMutex` for read-heavy fields. At minimum, `encryptionEnabled` must be synchronized since a race could cause data to be sent unencrypted.

**Note**: Blocked by issue #2 (both touch encryption state in `SsmDataChannel`).

---

#### 4. SSO cache directory created with 0600 (missing execute bit)

**File**: `pkg/sso.go:200`

`os.MkdirAll(dir, 0600)` creates the directory without execute permission. Directories need the execute bit to be traversable (`0700`). Without it, subsequent `os.WriteFile` may fail or the directory may be created with incorrect permissions depending on umask.

**Remediation**: Change `os.MkdirAll(dir, 0600)` to `os.MkdirAll(dir, 0700)`. The file itself should remain 0600.

---

### MEDIUM Severity

#### 5. No enforcement that WebSocket URL scheme is WSS

**File**: `datachannel/data_channel.go:629`

`websocket.DefaultDialer.Dial(url, ...)` doesn't enforce WSS scheme or configure TLS. If the `StreamUrl` from `StartSession` were somehow tampered with (e.g., via endpoint override), the connection could downgrade to unencrypted WS.

**Remediation**: Before calling `websocket.DefaultDialer.Dial()`, parse the URL and verify the scheme is `"wss"`. Reject `"ws"` connections to prevent accidental unencrypted WebSocket communication. Also consider setting explicit TLS configuration on the dialer.

---

#### 6. VPC endpoint and proxy URL inputs not validated

**File**: `pkg/client.go:23,52-61`, `datachannel/data_channel.go:81`

Endpoint flag values are prepended with `"https://"` without validation. A value like `evil.com/path?q=` would produce a valid URL pointing to an attacker-controlled host. The proxy URL scheme isn't validated either - an `http://` proxy would cause AWS SDK requests (containing SigV4 signatures) to traverse an unencrypted proxy. The stream URL override in `StreamEndpointOverride` also replaces the host without validating the replacement value.

**Remediation**: Validate VPC endpoint values are valid hostnames (no path, query, fragments, or whitespace). Validate proxy URL scheme is `http` or `https`. Reject endpoint values containing `/`, `?`, `#`, or whitespace.

---

#### 7. Signal handlers only fire once + `os.Exit` bypasses deferred cleanup

**File**: `ssmclient/shell_posix.go:38-52`, `ssmclient/shell_windows.go:121-138`, `ssmclient/port_forwarding.go:304-316`

Signal handler goroutines use `<-sigCh` (non-looping), so only the first signal is handled. Subsequent SIGWINCH signals are ignored, breaking terminal resize. Additionally, `os.Exit(0)` terminates immediately, skipping all deferred functions including logger sync, connection cleanup, and terminal state restoration.

**Remediation**: Change signal handler goroutines to loop (`for sig := range sigCh`) instead of processing only the first signal. Replace `os.Exit(0)` with a context cancellation approach so deferred cleanup functions execute. If `os.Exit` must be used, ensure all critical cleanup happens before the call.

---

### LOW Severity

#### 8. SSO credentials logged at debug level

**File**: `pkg/sso.go:119,127,170`, `datachannel/data_channel.go:699-713`

Debug-level logging of `SSOLoginInput`, config profiles, and `SSOLoginOutput` could expose tokens and secrets to log files. The `cacheFileData` struct contains `AccessToken` and `ClientSecret`. Additionally, internal error messages are sent to the remote SSM agent in handshake responses, leaking implementation details.

**Remediation**: Implement `String()`/`GoString()` methods that redact sensitive fields (AccessToken, ClientSecret), or selectively log only non-sensitive fields. Sanitize error messages sent in handshake responses.

---

#### 9. `viper.BindPFlag` return values unchecked

**File**: `cmd/root.go:49-61`

All `viper.BindPFlag()` calls silently ignore errors. While failures are rare, they could cause configuration to silently not take effect.

**Remediation**: Collect errors and log/fatal if any binding fails.

---

## Dependency Vulnerabilities

| Module | Vulnerability | Impact |
|--------|--------------|--------|
| `github.com/aws/aws-sdk-go` v1.55.8 (indirect) | GO-2022-0646 (CBC padding oracle in S3 Crypto SDK) | Not directly called by this codebase. Pulled transitively by `session-manager-plugin`. |

---

## Summary Table

| # | Severity | Issue | File |
|---|----------|-------|------|
| 1 | **HIGH** | Missing bounds check in `UnmarshalBinary` | `agent_message.go:85` |
| 2 | **HIGH** | Encryption key material never zeroed | `encryption.go`, `data_channel.go` |
| 3 | **HIGH** | Data races on encryption state fields | `data_channel.go` |
| 4 | **HIGH** | SSO cache dir 0600 needs 0700 | `sso.go:200` |
| 5 | **MEDIUM** | No WSS scheme enforcement on WebSocket | `data_channel.go:629` |
| 6 | **MEDIUM** | VPC endpoint / proxy URL not validated | `client.go:23,52` |
| 7 | **MEDIUM** | Signal handlers fire once + os.Exit skips defers | `shell_posix.go`, `port_forwarding.go` |
| 8 | **LOW** | SSO tokens logged at debug level | `sso.go:119,170` |
| 9 | **LOW** | `viper.BindPFlag` errors ignored | `root.go:49-61` |
