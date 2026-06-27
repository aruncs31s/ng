package ng

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

func GetLogger() *zap.Logger {
	if Log == nil {
		InitLogger()
	}
	return Log
}

func InitLogger() {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.ConsoleSeparator = " | "

	var cores []zapcore.Core

	consoleConfig := zap.NewDevelopmentEncoderConfig()
	consoleConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	cores = append(cores, zapcore.NewCore(
		zapcore.NewConsoleEncoder(consoleConfig),
		zapcore.AddSync(os.Stdout),
		zap.DebugLevel,
	))

	core := zapcore.NewTee(cores...)

	Log = zap.New(
		core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
}

func SyncLogger() {
	if Log != nil {
		_ = Log.Sync()
	}
}
