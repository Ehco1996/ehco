package xray

import (
	"github.com/Ehco1996/ehco/pkg/log"
	"go.uber.org/zap"
)

var (
	L *zap.SugaredLogger
)

func init() {
	L = log.InfoLogger.Named("xray")
}
