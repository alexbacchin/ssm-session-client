module github.com/alexbacchin/ssm-session-client

go 1.25.0

require (
	github.com/aws/aws-sdk-go-v2 v1.42.0
	github.com/aws/aws-sdk-go-v2/config v1.32.25
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.308.0
	github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect v1.33.2
	github.com/aws/aws-sdk-go-v2/service/ssm v1.69.3
	github.com/aws/aws-sdk-go-v2/service/sts v1.43.3
	github.com/aws/session-manager-plugin v0.0.0-20260317185859-1fc63e3f5a01
	github.com/aws/smithy-go v1.27.3
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/spf13/cobra v1.10.2
	github.com/spf13/viper v1.21.0
	golang.org/x/crypto v0.53.0
	golang.org/x/net v0.56.0
	golang.org/x/sys v0.46.0
	golang.org/x/term v0.44.0
)

require (
	github.com/aws/aws-sdk-go-v2/credentials v1.19.24
	github.com/aws/aws-sdk-go-v2/service/kms v1.53.4
	github.com/aws/aws-sdk-go-v2/service/sso v1.31.3
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.36.6
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/xtaci/smux v1.5.57
	go.uber.org/zap v1.28.0
	gopkg.in/ini.v1 v1.67.3
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/aws/aws-sdk-go v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.29 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.29 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.29 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.29 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/pelletier/go-toml/v2 v2.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/twinj/uuid v1.0.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/twinj/uuid => github.com/twinj/uuid v0.0.0-20151029044442-89173bcdda19 // indirect
