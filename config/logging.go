package config

import (
	"os"
	"path"
	"runtime"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logConfig zap.Config

func CreateLogger() *zap.Logger {
	logConfig = zap.Config{
		Level: zap.NewAtomicLevelAt(zapcore.InfoLevel),
	}
	stdout := zapcore.AddSync(os.Stdout)

	file := zapcore.AddSync(&lumberjack.Logger{
		Filename:   path.Join(getLogFolder(), "app.log"),
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     1, // days
	})

	productionCfg := zap.NewProductionEncoderConfig()
	productionCfg.TimeKey = "timestamp"
	productionCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	developmentCfg := zap.NewDevelopmentEncoderConfig()
	developmentCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder

	consoleEncoder := zapcore.NewConsoleEncoder(developmentCfg)
	fileEncoder := zapcore.NewJSONEncoder(productionCfg)

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, stdout, logConfig.Level),
		zapcore.NewCore(fileEncoder, file, logConfig.Level),
	)

	return zap.New(core)
}

func SetLogLevel(logLevel string) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		panic("invalid log level: " + logLevel)
	}
	logConfig.Level.SetLevel(level)
}

func getLogFolder() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic("unable to determine user home directory")
	}

	switch os := runtime.GOOS; os {
	case "windows":
		return homeDir + "\\AppData\\Local\\ssm-session-client\\logs"
	case "darwin":
		return homeDir + "/Library/Logs/ssm-session-client"
	default: // Linux and other Unix-like systems
		return homeDir + "/.ssm-session-client/logs"
	}
}

// initLogging initializes the logger with the appropriate configuration
