# SSM Session Client

A self-contained CLI for AWS SSM Session Manager — shell, SSH, port forwarding, and RDP over SSM without bastion hosts, exposed ports, or the AWS CLI plugin.

Built for restricted environments (AppLocker, AirLock, Manage Engine) and complex networks where AWS PrivateLink endpoints are only reachable from private networks via VPN or Direct Connect.

## Features

- **Shell** — Interactive shell sessions over SSM with KMS encryption support and automatic reconnection
- **SSH (ProxyCommand)** — Use as an SSH `ProxyCommand` for seamless `ssh` integration with automatic SSH config setup
- **SSH Direct** — Native Go SSH client over SSM — no external SSH binary required, with TOFU host key verification
- **VSCode Remote SSH** — Drop-in replacement for the SSH executable in VSCode Remote SSH, works on Windows without OpenSSH
- **EC2 Instance Connect** — Ephemeral key authentication via IAM — no long-lived SSH keys to manage
- **Port Forwarding** — Secure TCP tunnels to private instances with stream multiplexing for SSM agents v3.0.196.0+
- **RDP** *(Windows)* — One-command Remote Desktop with optional password retrieval and clipboard integration
- **Flexible Target Resolution** — Connect by instance ID, EC2 tag, private IP, DNS TXT record, or named alias
- **VPC Endpoint Overrides** — Custom endpoints for STS, SSM, SSM Messages, EC2, and KMS (PrivateLink support)
- **AWS SSO / Identity Center** — Automatic device-code browser login with cached token support
- **Proxy Support** — HTTPS proxy for environments behind corporate proxies

## Quick Start

1. Download the latest binary from [Releases](https://github.com/alexbacchin/ssm-session-client/releases)
2. Configure AWS credentials ([guide](https://docs.aws.amazon.com/sdkref/latest/guide/creds-config-files.html))
3. Connect:

```shell
# Shell session
ssm-session-client shell i-0abc1234def56789

# SSH direct (no external SSH client needed)
ssm-session-client ssh-direct ec2-user@i-0abc1234def56789

# Port forwarding
ssm-session-client port-forwarding i-0abc1234def56789:443 8443
```

## Documentation

Full documentation including installation, configuration, session modes, troubleshooting, and contributing guides is available at:

**[https://alexbacchin.github.io/ssm-session-client](https://alexbacchin.github.io/ssm-session-client)**

## Building from Source

```shell
git clone https://github.com/alexbacchin/ssm-session-client.git
cd ssm-session-client
go build -o ssm-session-client main.go
```

Cross-compile for other platforms:

```shell
GOOS=linux   GOARCH=amd64 go build -o ssm-session-client-linux main.go
GOOS=darwin  GOARCH=arm64 go build -o ssm-session-client-darwin-arm64 main.go
GOOS=windows GOARCH=amd64 go build -o ssm-session-client.exe main.go
```

## License

See [LICENSE](LICENSE) for details.
