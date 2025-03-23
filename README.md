# SSM Session Client CLI

This is a fork from [ssm-session-client](https://github.com/mmmorris1975/ssm-session-client) with some CLI and additional functionality to connect to AWS SSM sessions.  The goal of this utility is to provide a single executable to provide SSM Session functionality. This is an alternative for restricted environments where the AWS CLI execution is restricted.

The main goal for this project is to enable SSM Client on complex environments where AWS Services endpoints (PrivateLink) are reachable from private networks via VPN or Direct Connects.

When the SSM `StartSession` is called, the API will return the [StreamUrl](https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_StartSession.html#API_StartSession_ResponseSyntax) with the regional SSM Messages endpoint. Even if SSM Messages endpoint is reachable in a private network, the only options to use it are HTTPS proxy or [DNS RPZ](https://dnsrpz.info/). For this reason, this app has a flag to set the SSM Messages endpoint, then it will replace the StreamUrl with your SSM Messages endpoint.

## Requirements

1) Copy the executable from [Releases](https://github.com/alexbacchin/ssm-session-client/releases) to the target operating system
2) (Optional) Install [Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html) recommended. When installed it will be used by default.

## Configuration

First, follow the standard [configure AWS SDK and Tools](https://docs.aws.amazon.com/sdkref/latest/guide/creds-config-files.html) to provide AWS credentials.

The utility configuration can set via:

1. Configuration file. Default `$HOME/.ssm-session-client.yaml` or will be searched at the following:
  a. current folder
  b. users home folder
  c. applicaiton folder
2. Environment Variables with `SCC_` prefix
3. Command Line paramenters

These are the configuration options:

| Description       | App Config/Flag | App Env Variable | AWS SDK Variable |
| :---------------- | :------: | ----: | ----: |
| Config File Path |   config   |  SCC_CONFIG | n/a |
| AWS SDK profile name  |   aws-profile   | SCC_AWS_PROFILE | AWS_PROFILE |
| AWS SDK region name  |   aws-region   | SCC_AWS_REGION| AWS_REGION or AWS_DEFAULT_REGION |
| STS Endpoint        |   sts-endpoint   | SCC_STS_ENDPOINT | AWS_ENDPOINT_URL_STS |
| EC2 Endpoint        |   ec2-endpoint   | SCC_EC2_ENDPOINT | AWS_ENDPOINT_URL_EC2 |
| SSM Endpoint        |   ssm-endpoint   | SCC_SSM_ENDPOINT | AWS_ENDPOINT_URL_SSM |
| SSM Messages Endpoint        |   ssmmessages-endpoint   | SCC_SSMMESSAGES_ENDPOINT | n/a |
| Proxy URL        |   proxy-url   | SCC_PROXY_URL | HTTPS_PROXY |
| SSM Sessoion Plugin       |   ssm-session-plugin   | SCC_SSM_SESSION_PLUGIN  | n/a |

### Remarks

- The app `proxy-url` flag only applies to services where the custom endpoints are not set.
- The app `ssmmessages-endpoint` flag, it will be used to perform the WSS connection during an SSM Session by replacing the StreamUrl with the SSM Messages endpoint.
- The app `ssm-session-plugin` flag,

### Sample config file

```yaml
ec2-endpoint: vpce-059c3b85db66f8165-mzb6o9nb.ec2.us-west-2.vpce.amazonaws.com
ssm-endpoint: vpce-06ef6f173680a1306-bt58rzff.ssm.us-west-2.vpce.amazonaws.com
ssmmessages-endpoint: vpce-0e5e5b0c558a14bf2-r3p6zkdm.ssmmessages.us-west-2.vpce.amazonaws.com
sts-endpoint: vpce-0877b4abeb479ee06-arkdktlc.sts.us-west-2.vpce.amazonaws.com
aws-profile: sandbox
proxy-url: http://myproxy:3128
```

## Supported modes

[Windows SSH Client](https://learn.microsoft.com/en-us/windows/terminal/tutorials/ssh#access-windows-ssh-client-and-ssh-server) is not installed by default.

## Shell

Shell-level access to an instance can be obtained using the `shell` command.  This command takes
an AWS SDK and a string to identify the target to connect with.

Note: If you have enabled KMS encryption for Sessions, then must use AWS Session Manager plugin.

```shell
$ssm-session-client shell i-0bdb4f892de4bb54c --config=config.yaml
```

IAM: [Sample IAM policies for Session Manager](https://docs.aws.amazon.com/systems-manager/latest/userguide/getting-started-restrict-access-quickstart.html)

## SSH

SSH over SSM integration can be used via the `ssh` command. Ensure the target instance has SSH authenticaiton configured before connecting. This feature is meant to be used in SSH configuration files according to the [AWS documentation](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started-enable-ssh-connections.html).

First you need to configure `$HOME/.ssh/config` or `%USERPROFILE%\.ssh\config` (Windows)

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

SSH over SSM with [EC2 Instance Connect](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/connect-linux-inst-eic.html) can be used via the `instance-connect` command. The configuration is the same as SSH above, however SSH authentication configuration is not required. The authentication is allowed by IAM action `ec2-instance-connect:SendSSHPublicKey`

In this mode the app will try to use default public SSH keys: `id_ed25519.pub` and `id_rsa.pub`. for the temporary SSH authentication. Alternativelly a `ssh-public-key-file` flag can be set

**Note**: EC2 Instance connect endpoints are not available via AWS Private Link. Internet access or Internet Proxy will be required to use this mode.

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

Port Forwarding via SSM allows you to securely create tunnels between your instances deployed in private subnets, without the need to start the SSH service on the server, to open the SSH port in the security group or the need to use a bastion host.
It can be used via the `port-forwarding` command. If local port is not provided SSH will assign a random local port

```shell
#SSH Port Forwarding from port local port 8888 to instance port 443
$ssm-session-client port-forwarding i-0bdb4f892de4bb54c:443 8888 --config=config.yaml
```

## Target Lookup

The target can be an instance ID, hostname or even IP address. The app uses a few functions to resolve the target.

## TODO

- EC2 Instance Connect automatically generate disposable SSH key pair for SSH authentication
- Testing functions
- Allow multiplexed connections (multiple, simultaneous streams) with port forwarding
- Robustness (retries/error recovery)
