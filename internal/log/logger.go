package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger
var atomicLevel zap.AtomicLevel

func init() {
	atomicLevel = zap.NewAtomicLevelAt(zapcore.InfoLevel)

	config := zap.NewProductionConfig()
	config.Encoding = "console"
	config.EncoderConfig.TimeKey = ""
	config.EncoderConfig.CallerKey = ""
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	config.Level = atomicLevel

	var err error
	logger, err = config.Build()
	if err != nil {
		// Fallback to nop logger if build fails
		logger = zap.NewNop()
	}
}

func SetLevel(level zapcore.Level) {
	atomicLevel.SetLevel(level)
}

func Debug(msg string, fields ...zap.Field) {
	logger.Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	logger.Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	logger.Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	logger.Error(msg, fields...)
}

func Sync() {
	// Ignore errors from syncing stderr/stdout (common on some systems)
	// See: https://github.com/uber-go/zap/issues/880
	_ = logger.Sync()
}
