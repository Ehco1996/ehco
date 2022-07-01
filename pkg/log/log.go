package log

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *zap.SugaredLogger

	// auto init a info level logger
	InfoLogger *zap.SugaredLogger
)

func init() {
	InfoLogger, _ = initLogger(zapcore.InfoLevel.String())
}

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

func InitLogger(logLevel string) error {
	l, err := initLogger(logLevel)
	if err != nil {
		return err
	}
	Logger = l
	return nil
}
