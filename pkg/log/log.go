package log

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *zap.SugaredLogger
	doOnce sync.Once
)

func initLogger(logLevel string) (*zap.SugaredLogger, error) {
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
	return zap.New(core).Sugar(), nil
}

func InitGlobalLogger(logLevel string) error {
	var err error
	doOnce.Do(func() {
		Logger, err = initLogger(logLevel)
	})
	return err
}

func MustNewInfoLogger(name string) *zap.SugaredLogger {
	l, err := initLogger("info")
	if err != nil {
		panic(err)
	}
	return l.Named(name)
}
