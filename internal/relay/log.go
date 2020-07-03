package relay

import "go.uber.org/zap"

var Logger *zap.SugaredLogger

func init() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	Logger = logger.Sugar()
	Logger.Info("Init zap logger")
}
