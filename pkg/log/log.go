package log

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var doOnce sync.Once

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
		EncodeTime:  zapcore.ISO8601TimeEncoder,
		EncodeName:  zapcore.FullNameEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoder),
		zapcore.NewMultiWriteSyncer(writers...),
		level,
	)
	l := zap.New(core)
	zap.ReplaceGlobals(l)
	return l, nil
}

func InitGlobalLogger(logLevel string) error {
	var err error
	doOnce.Do(func() {
		_, err = initLogger(logLevel, true)
	})
	return err
}

func NewLogger(logLevel string) (*zap.Logger, error) {
	return initLogger(logLevel, false)
}
