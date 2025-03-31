package main

import (
	"github.com/alexbacchin/ssm-session-client/cmd"
	"github.com/alexbacchin/ssm-session-client/config"
	"go.uber.org/zap"
)

func main() {
	logger := config.CreateLogger()
	zap.ReplaceGlobals(logger)
	defer logger.Sync() // flushes buffer, if any

	cmd.Execute()
}
