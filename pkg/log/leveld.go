package log

import (
	"github.com/hashicorp/go-retryablehttp"
	"go.uber.org/zap"
)

// ZapLeveledLogger wrapper for zap.logger for use with hashicorp's go-retryable LeveledLogger
// port from https://github.com/hashicorp/go-retryablehttp/issues/182#issuecomment-1758011585
type ZapLeveledLogger struct {
	Logger *zap.SugaredLogger
}

// New creates a ZapLeveledLogger with a zap.logger that satisfies standard library log.Logger interface.
func NewZapLeveledLogger(name string) retryablehttp.LeveledLogger {
	if globalInitd {
		return &ZapLeveledLogger{Logger: zap.S().Named(name)}
	}
	logger, _ := initLogger("info", false)
	return &ZapLeveledLogger{Logger: logger.Sugar().Named(name)}
}

func (l *ZapLeveledLogger) Error(msg string, keysAndValues ...interface{}) {
	l.Logger.Errorw(msg, keysAndValues...)
}

func (l *ZapLeveledLogger) Info(msg string, keysAndValues ...interface{}) {
	l.Logger.Infow(msg, keysAndValues...)
}

func (l *ZapLeveledLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.Logger.Debugw(msg, keysAndValues...)
}

func (l *ZapLeveledLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.Logger.Warnw(msg, keysAndValues...)
}
