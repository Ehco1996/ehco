package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.SugaredLogger

func init() {
	writers := []zapcore.WriteSyncer{zapcore.AddSync(os.Stdout)}
	encoder := zapcore.EncoderConfig{
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.FullCallerEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoder),
		zapcore.NewMultiWriteSyncer(writers...),
		zapcore.InfoLevel,
	)
	Logger = zap.New(core).Sugar()
}

func Info(args ...interface{}) {
	Logger.Info(args...)
}

func Fatal(args ...interface{}) {
	Logger.Fatal(args...)
}

func Infof(template string, args ...interface{}) {
	Logger.Infof(template, args...)
}

func Fatalf(template string, args ...interface{}) {
	Logger.Fatalf(template, args...)
}

func Errorf(template string, args ...interface{}) {
	Logger.Errorf(template, args...)
}
