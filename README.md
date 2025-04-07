# SSM Session Client CLI

This project is a fork of [ssm-session-client](https://github.com/mmmorris1975/ssm-session-client) with added CLI functionality to connect to AWS SSM sessions. The goal is to provide a single executable for SSM Session functionality, especially useful in environments where AWS CLI execution is restricted. Such as:

- Microsoft AppLocker
- AirLock
- Manage Engine

The main goal of this project is to enable SSM Client in complex environments where AWS Services endpoints (PrivateLink) are accessible from private networks via VPN or Direct Connect.

When the SSM `StartSession` is called, the API will always return the [StreamUrl](https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_StartSession.html#API_StartSession_ResponseSyntax) with the regional SSM Messages endpoint. Even if when a SSM Messages endpoint PrivateLink is reachable in a private network, the only options to use it for session streams are HTTPS proxy or [DNS RPZ](https://dnsrpz.info/). For this reason, this app has a flag to set the SSM Messages endpoint, then it will replace the StreamUrl with your SSM Messages endpoint.

**Note**: [Windows SSH Client](https://learn.microsoft.com/en-us/windows/terminal/tutorials/ssh#access-windows-ssh-client-and-ssh-server) is not installed by default.

## Requirements

1. Download the executable from [Releases](https://github.com/alexbacchin/ssm-session-client/releases) and copy it to the target operating system.
2. (Optional) Install the [Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html). It is recommended and will be used by default if installed.

## Configuration

First, follow the standard [configure AWS SDK and Tools](https://docs.aws.amazon.com/sdkref/latest/guide/creds-config-files.html) to provide AWS credentials.

The utility can be configured via:

1. Configuration file: Default is `$HOME/.ssm-session-client.yaml`. It will also search in:
   a. Current folder
   b. User's home folder
   c. Application folder
2. Environment Variables with the `SCC_` prefix
3. Command Line parameters

These are the configuration options:

| Description                | App Config/Flag       | App Env Variable         | AWS SDK Variable               |
| :------------------------- | :-------------------: | :----------------------: | :-----------------------------:|
| Config File Path           | config                | SCC_CONFIG               | n/a                            |
| Log Level                  | log-level             | SCC_LOG_LEVEL            | n/a                            |
| AWS SDK profile name       | aws-profile           | SCC_AWS_PROFILE          | AWS_PROFILE                    |
| AWS SDK region name        | aws-region            | SCC_AWS_REGION           | AWS_REGION or AWS_DEFAULT_REGION |
| STS Endpoint               | sts-endpoint          | SCC_STS_ENDPOINT         | AWS_ENDPOINT_URL_STS           |
| EC2 Endpoint               | ec2-endpoint          | SCC_EC2_ENDPOINT         | AWS_ENDPOINT_URL_EC2           |
| SSM Endpoint               | ssm-endpoint          | SCC_SSM_ENDPOINT         | AWS_ENDPOINT_URL_SSM           |
| SSM Messages Endpoint      | ssmmessages-endpoint  | SCC_SSMMESSAGES_ENDPOINT | n/a                            |
| Proxy URL                  | proxy-url             | SCC_PROXY_URL            | HTTPS_PROXY                    |
| SSM Session Plugin         | ssm-session-plugin    | SCC_SSM_SESSION_PLUGIN   | n/a                            |

### Remarks

- The `proxy-url` flag is only applicable to services where custom endpoints are not set.
- The `ssmmessages-endpoint` flag is used to perform the WSS connection during an SSM Session by replacing the StreamUrl with the SSM Messages endpoint.
- The `ssm-session-plugin` flag specifies whether to use the AWS Session Manager plugin for the session.

### Logging

Logging is generated on the console and log file at:

- Windows: `%USERPROFILE%\AppData\Local\ssm-session-client\logs`
- MACOS: `$HOME/Library/Logs/ssm-session-client`
- Linux and other Unix-like systems: `$HOME/.ssm-session-client/logs`

Log files are rotated daily or when size reaches 10MB and the last 3 log files are kept

### Sample config file

```yaml
ec2-endpoint: vpce-059c3b85db66f8165-mzb6o9nb.ec2.us-west-2.vpce.amazonaws.com
ssm-endpoint: vpce-06ef6f173680a1306-bt58rzff.ssm.us-west-2.vpce.amazonaws.com
ssmmessages-endpoint: vpce-0e5e5b0c558a14bf2-r3p6zkdm.ssmmessages.us-west-2.vpce.amazonaws.com
sts-endpoint: vpce-0877b4abeb479ee06-arkdktlc.sts.us-west-2.vpce.amazonaws.com
aws-profile: sandbox
proxy-url: http://myproxy:3128
log-level: warn
```

## Supported modes

## Shell

Shell-level access to an instance can be obtained using the `shell` command. This command requires an AWS SDK profile and a string to identify the target instance.

**Note**: If you have enabled KMS encryption for Sessions, you must use the AWS Session Manager plugin.

```shell
$ssm-session-client shell i-0bdb4f892de4bb54c --config=config.yaml
```

IAM: [Sample IAM policies for Session Manager](https://docs.aws.amazon.com/systems-manager/latest/userguide/getting-started-restrict-access-quickstart.html)

## SSH

SSH over SSM integration can be used via the `ssh` command. Ensure the target instance has SSH authentication configured before connecting. This feature is meant to be used in SSH configuration files according to the [AWS documentation](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started-enable-ssh-connections.html).

You need to configure the ProxyCommand `$HOME/.ssh/config` (Linux/macOS) or `%USERPROFILE%\.ssh\config` (Windows).

```shell
# SSH over Session Manager
Host i-*
  ProxyCommand ssm-session-client ssh %r@%h --ssm-session-plugin=true --config=config.yaml
```

Then to connect:

```shell
$ssh ec2-user@i-0bdb4f892de4bb54c
```

IAM: [Controlling user permissions for SSH connections through Session Manager](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started-enable-ssh-connections.html)

## SSH with Instance Connect (Linux targets only)

SSH over SSM with [EC2 Instance Connect](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/connect-linux-inst-eic.html) can be used via the `instance-connect` command. This configuration is similar to the SSH setup above, but SSH authentication configuration is not required. Authentication is managed by the IAM action `ec2-instance-connect:SendSSHPublicKey`.

In this mode, the app will attempt to use default public SSH keys (`id_ed25519.pub` and `id_rsa.pub`) for temporary SSH authentication. Alternatively, you can specify a custom public key file using the `ssh-public-key-file` flag.

**Note**: EC2 Instance Connect endpoints are not available via AWS PrivateLink. Internet access or an Internet proxy is required to use this mode.

```shell
# SSH over Session Manager with EC2 Instance Connect and default SSH keys
Host i-*
  ProxyCommand ssm-session-client instance-connect %r@%h --ssm-session-plugin=true --config=config.yaml
```

```shell
# SSH over Session Manager with EC2 Instance Connect and custom SSH keys
Host i-*
  IdentityFile ~/.ssh/custom
  ProxyCommand ssm-session-client instance-connect %r@%h --ssm-session-plugin=true --config=config.yaml --ssh-public-key-file=~/.ssh/custom.pub
```

Then to connect:

```shell
$ssh ec2-user@i-0bdb4f892de4bb54c
```

IAM:

- [Controlling user permissions for SSH connections through Session Manager](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started-enable-ssh-connections.html)
- [Grant IAM permissions for EC2 Instance Connect](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-connect-configure-IAM-role.html)

## Port Forwarding

Port Forwarding via SSM allows you to securely create tunnels between your instances deployed in private subnets without needing to start the SSH service on the server, open the SSH port in the security group, or use a bastion host. It can be used via the `port-forwarding` command. If a local port is not provided, SSH will assign a random local port.

```shell
# SSH Port Forwarding from local port 8888 to instance port 443
$ssm-session-client port-forwarding i-0bdb4f892de4bb54c:443 8888 --config=config.yaml
```

## Target Lookup

The target can be an instance ID, hostname or even IP address. The app uses a few functions to resolve the target.

## Building from source

To build this Go project, ensure you have Go installed on your system. You can download and install it from the [official Go website](https://golang.org/dl/).

1. Clone the repository:

```shell
git clone https://github.com/alexbacchin/ssm-session-client.git
cd ssm-session-client
```

2. Build the project for different operating systems:

For Linux:

```shell
GOOS=linux GOARCH=amd64 go build -o ssm-session-client-linux main.go
```

For macOS:

```shell
GOOS=darwin GOARCH=amd64 go build -o ssm-session-client-macos main.go
```

For Windows:

```shell
GOOS=windows GOARCH=amd64 go build -o ssm-session-client.exe main.go
```

This will create an executable file named `ssm-session-client` in the current directory.

You can now use the `ssm-session-client` executable as described in the sections above.

## TODO

- EC2 Instance Connect automatically generate disposable SSH key pair for SSH authentication
- Unit testing
- Allow multiplexed connections (multiple, simultaneous streams) with port forwarding
- Robustness (retries/error recovery)
