package main

import (
	"log"

	"github.com/alexbacchin/ssm-session-client/cmd"
	"github.com/alexbacchin/ssm-session-client/config"
	"go.uber.org/zap"
)

func main() {
	logger, err := config.CreateLogger()
	if err != nil {
		log.Fatalf("failed to create logger: %v", err)
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync() // flushes buffer, if any
	cmd.Execute()
}
