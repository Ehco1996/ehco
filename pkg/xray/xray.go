package xray

import (
	"context"

	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/xtls/xray-core/core"
)

func StartXrayServer(ctx context.Context, cfg *core.Config) error {
	logger.Infof("[xray] Start xray Server")
	<-ctx.Done()
	return nil
}
