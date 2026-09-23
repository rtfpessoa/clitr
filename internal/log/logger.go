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

// SetLevel changes the minimum level emitted by the shared logger.
func SetLevel(level zapcore.Level) {
	atomicLevel.SetLevel(level)
}

// Debug logs diagnostic details when debug logging is enabled.
func Debug(msg string, fields ...zap.Field) {
	logger.Debug(msg, fields...)
}

// Info logs normal application progress.
func Info(msg string, fields ...zap.Field) {
	logger.Info(msg, fields...)
}

// Warn logs a recoverable problem that needs attention.
func Warn(msg string, fields ...zap.Field) {
	logger.Warn(msg, fields...)
}

// Error logs a failure along with any structured context.
func Error(msg string, fields ...zap.Field) {
	logger.Error(msg, fields...)
}

// Sync flushes buffered log output, ignoring unsupported stream sync errors.
func Sync() {
	// Ignore errors from syncing stderr/stdout (common on some systems)
	// See: https://github.com/uber-go/zap/issues/880
	_ = logger.Sync()
}
