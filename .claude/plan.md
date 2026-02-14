# Feature Plan: Windows Shell, Multiplexed Port Forwarding, Testing & Robustness

## Context

The ssm-session-client currently has four gaps:
1. **Windows shell sessions are broken** — `shell_windows.go` has stubs for stdin raw mode and cleanup, so interactive shells don't work from Windows clients.
2. **No Windows CI or structured tests** — builds and tests only run on Ubuntu; no verification of Windows-specific code.
3. **Port forwarding is limited to 1 connection** — `LimitListener(l, 1)` prevents concurrent TCP connections, despite the SSM protocol supporting multiplexing.
4. **No reconnection or backoff** — WebSocket disconnection is fatal; retransmission uses a fixed 500ms interval with no backoff.

---

## Phase 1: Windows Shell Console Mode

**Goal**: Make interactive shell sessions work from Windows clients.

### Files to modify
- `ssmclient/shell_windows.go` — implement `configureStdin()`, update `initialize()`, implement `cleanup()`

### Implementation
1. Add package-level vars `origInMode`, `origOutMode` to save original console modes.
2. Create `configureStdin()`:
   - Get stdin handle via `windows.GetStdHandle(STD_INPUT_HANDLE)`
   - Save original mode via `windows.GetConsoleMode()`
   - Clear `ENABLE_ECHO_INPUT`, `ENABLE_LINE_INPUT`, `ENABLE_PROCESSED_INPUT`
   - Set `ENABLE_VIRTUAL_TERMINAL_INPUT` (0x0200) for escape sequence support
   - Get stdout handle and enable `ENABLE_VIRTUAL_TERMINAL_PROCESSING` for VT output
3. Update `initialize()` to call `configureStdin()` after signal/resize setup.
4. Implement `cleanup()` to restore both stdin and stdout original modes.
5. Update `installSignalHandlers()` to call `cleanup()` before exit.

### Testing
- Build verification: `GOOS=windows go build ./...`
- New file `ssmclient/shell_windows_test.go` with build tag — test error paths and cleanup idempotency

---

## Phase 2: CI + Mock Tests for Windows

**Goal**: Add Windows runner to CI and write mock-based tests for platform-specific code.

### Files to modify
- `.github/workflows/build.yml` — add Windows matrix, add cross-compilation check

### Files to create
- `ssmclient/shell_windows_test.go` — Windows-specific unit tests (build-tagged)

### Implementation
1. Update `build.yml` to use a `matrix.os: [ubuntu-latest, windows-latest]` strategy.
2. Add cross-compilation check step (`GOOS=windows`, `GOOS=darwin`) on Ubuntu runner.
3. Write mock-based unit tests for Windows console code that can run on Windows CI runners.
4. Add integration test documentation in a `testing/` directory describing manual test matrix:
   - Windows client -> Linux target (shell, SSH, port forward)
   - Windows client -> Windows target (shell)
   - Linux/Mac client -> Windows target (shell)

---

## Phase 3: Multiplexed Port Forwarding

**Goal**: Support multiple concurrent TCP connections over a single SSM session using `xtaci/smux`, following the AWS SSM agent's `port_mux.go` pattern.

### Files to modify
- `datachannel/data_channel.go` — bump `ClientVersion` to `"1.2.0"`, store `agentVersion` from handshake, expose `AgentVersion()` method
- `ssmclient/port_forwarding.go` — route to mux vs basic mode based on agent version, remove `LimitListener` from top-level `createListener()`
- `go.mod` — promote `xtaci/smux` from indirect to direct dependency

### Files to create
- `ssmclient/port_mux.go` — multiplexed port forwarding using smux
- `ssmclient/version.go` — `agentVersionGte()` semver comparison utility
- `ssmclient/version_test.go` — table-driven tests for version comparison
- `ssmclient/port_mux_test.go` — tests using net.Pipe + smux server/client pairs

### Implementation

**3.1 — Handshake changes** (`datachannel/data_channel.go`):
- Add `agentVersion string` field to `SsmDataChannel`
- In `processHandshakeRequest()`, store `req.AgentVersion`
- Add `AgentVersion() string` method
- Change `ClientVersion` from `"0.0.1"` to `"1.2.0"` in `buildHandshakeResponse()`

**3.2 — Version utility** (`ssmclient/version.go`):
- `agentVersionGte(agentVersion, minVersion string) bool` — dot-separated integer comparison
- `parseVersion(v string) []int`

**3.3 — Mux port forwarding** (`ssmclient/port_mux.go`):
- `startMuxPortForwarding(ctx, c, listener, agentVersion)`:
  - Create `net.Pipe()` pair — one end for smux.Client, other bridges to WebSocket data channel
  - Configure smux: disable KeepAlive if agent >= 3.1.1511.0
  - Create `smux.Client(localConn, config)`
  - Bridge goroutines: `io.Copy(datachannel, pipeConn)` and `io.Copy(pipeConn, datachannel)`
  - Accept loop: for each TCP conn, `muxSession.OpenStream()`, then bidirectional copy between TCP conn and smux stream

**3.4 — Routing logic** (`ssmclient/port_forwarding.go`):
- In `startPortForwardingSession()`, after handshake: check `agentVersionGte(c.AgentVersion(), "3.0.196.0")`
  - If true: call `startMuxPortForwarding()` with unlimited listener
  - If false: call `startBasicPortForwarding()` with `LimitListener(l, 1)` (existing logic extracted)
- Extract current outer/inner loop into `startBasicPortForwarding()`
- Update `openDataChannel()` to use `AWS-StartPortForwardingSessionToRemoteHost` when `Host != ""`

### Testing
- `version_test.go`: table-driven tests for version comparison edge cases
- `port_mux_test.go`: create local smux server/client pair, verify bidirectional data flow, verify multiple concurrent streams
- Update existing handshake tests to verify new `ClientVersion` value

---

## Phase 4: Robustness — Retries, Reconnection & Health Monitoring

**Goal**: Add exponential backoff, WebSocket ping/pong, and reconnection via `ssm.ResumeSession`.

### Files to modify
- `datachannel/data_channel.go` — backoff in `processOutboundQueue()`, ping/pong loop, reconnection logic, store SSM client
- `config/flags.go` — add `EnableReconnect`, `MaxReconnects` config fields
- `cmd/root.go` — add `--enable-reconnect` and `--max-reconnects` CLI flags

### Files to create
- `datachannel/reconnect_test.go` — tests for reconnection and backoff logic

### Implementation

**4.1 — Exponential backoff** (`datachannel/data_channel.go`):
- In `processOutboundQueue()`: replace fixed 500ms sleep with exponential backoff
  - Start at 500ms, double on each round with unacknowledged messages, cap at 30s
  - Reset to 500ms when outbound queue is empty

**4.2 — Ping/pong health monitoring** (`datachannel/data_channel.go`):
- Add `pingLoop()` goroutine: send WebSocket ping every 30s
- Set pong handler on WebSocket connection
- Start ping loop in `StartSessionFromDataChannelURL()` after dial

**4.3 — Reconnection** (`datachannel/data_channel.go`):
- Add fields: `reconnectEnabled`, `maxReconnects`, `reconnectCount`, `ssmClient *ssm.Client`
- Store `ssm.Client` in `startSession()` for later `ResumeSession` calls
- Add `reconnect()` method:
  - Call `ssm.ResumeSession(sessionId)` to get new StreamUrl/TokenValue
  - Close old WebSocket, dial new one, re-open data channel
  - Exponential backoff between attempts (1s base, 30s max, up to `maxReconnects`)
  - Re-setup ping/pong on new connection
- Update `Read()`: on abnormal WebSocket close (code 1006 / unexpected), call `reconnect()` instead of returning EOF
- Normal close (1000, 1001) remains fatal (intentional termination)

**4.4 — Configuration** (`config/flags.go`, `cmd/root.go`):
- `--enable-reconnect` (default: true) — toggle reconnection behavior
- `--max-reconnects` (default: 5) — maximum reconnection attempts per session

**4.5 — Error messages**:
- Wrap errors in session handlers (shell.go, ssh.go, port_forwarding.go) with user-friendly context
- Log reconnection attempts at Info level

### Testing
- `reconnect_test.go`:
  - Mock SSM client with `ResumeSession` returning success/failure
  - Test max retries exhaustion
  - Test reconnection disabled path
  - Test exponential backoff interval calculation
  - Test ping loop sends WebSocket ping messages (using test WebSocket server)

---

## Phase Ordering & Dependencies

```
Phase 1 (Windows Shell) ──> Phase 2 (CI + Tests)
                                                    ──> Phase 4 (Robustness)
Phase 3 (Mux Port Forwarding) ─────────────────────/
```

- Phases 1 and 3 can run **in parallel** (different files)
- Phase 2 depends on Phase 1
- Phase 4 depends on Phase 3 (both modify `data_channel.go`; reconnection must handle mux state)

## Verification

After all phases:
1. `go build ./...` succeeds on Linux, macOS, and Windows
2. `go test ./... -race` passes on all platforms
3. `GOOS=windows go build ./...` cross-compiles successfully
4. Manual test: Windows client shell to Linux EC2 instance
5. Manual test: port forwarding with multiple concurrent `curl` requests through same tunnel
6. Manual test: disconnect network briefly during active session, verify reconnection
