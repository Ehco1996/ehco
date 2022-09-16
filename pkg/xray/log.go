package xray

import (
	"sync"

	"github.com/Ehco1996/ehco/pkg/log"
	"go.uber.org/zap"
)

var (
	l      *zap.SugaredLogger
	doOnce sync.Once
)

func initXrayLogger() {
	doOnce.Do(func() {
		l = log.Logger.Named("xray")
	})
}
