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
)

func initLogger(logLevel string, replaceGlobal bool) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		return nil, err
	}
	writers := []zapcore.WriteSyncer{zapcore.AddSync(os.Stdout)}
	encoder := zapcore.EncoderConfig{
		TimeKey:     "ts",
		LevelKey:    "level",
		MessageKey:  "msg",
		NameKey:     "name",
		EncodeLevel: zapcore.LowercaseColorLevelEncoder,
		EncodeTime:  zapcore.RFC3339TimeEncoder,
		EncodeName:  zapcore.FullNameEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoder),
		zapcore.NewMultiWriteSyncer(writers...),
		level,
	)
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
