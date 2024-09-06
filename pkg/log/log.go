package log

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	doOnce      sync.Once
	globalInitd bool

	globalWebSocketSyncher *WebSocketLogSyncher
)

func initLogger(logLevel string, replaceGlobal bool) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		return nil, err
	}

	consoleEncoder := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		TimeKey:     "ts",
		LevelKey:    "level",
		MessageKey:  "msg",
		NameKey:     "name",
		EncodeLevel: zapcore.LowercaseColorLevelEncoder,
		EncodeTime:  zapcore.RFC3339TimeEncoder,
		EncodeName:  zapcore.FullNameEncoder,
	})
	stdoutCore := zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level)

	jsonEncoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})

	globalWebSocketSyncher = NewWebSocketLogSyncher()
	wsCore := zapcore.NewCore(jsonEncoder, globalWebSocketSyncher, level)

	// 合并两个 core
	core := zapcore.NewTee(stdoutCore, wsCore)

	l := zap.New(core)
	if replaceGlobal {
		zap.ReplaceGlobals(l)
	}
	return l, nil
}

func InitGlobalLogger(logLevel string) error {
	var err error
	doOnce.Do(func() {
		_, err = initLogger(logLevel, true)
		globalInitd = true
	})
	return err
}

func MustNewLogger(logLevel string) *zap.Logger {
	l, err := initLogger(logLevel, false)
	if err != nil {
		panic(err)
	}
	return l
}
