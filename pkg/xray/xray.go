package xray

import (
	"context"
	"errors"
	"fmt"

	_ "github.com/xtls/xray-core/main/distro/all" // register all features

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/xtls/xray-core/core"
)

const (
	XrayAPITag     = "api"
	XraySSProxyTag = "ss_proxy"

	SyncTime = 60
)

func StartXrayServer(ctx context.Context, cfg *config.Config) (*core.Instance, error) {
	coreCfg, err := cfg.XRayConfig.Build()
	if err != nil {
		return nil, err
	}
	server, err := core.New(coreCfg)
	if err != nil {
		return nil, err
	}
	if err := server.Start(); err != nil {
		return nil, err
	}
	return server, nil
}

func StartSyncTask(ctx context.Context, cfg *config.Config) error {
	// find api port and server, hard code api Tag to `api`
	var grpcEndPoint string
	for _, inbound := range cfg.XRayConfig.InboundConfigs {
		if inbound.Tag == XrayAPITag {
			grpcEndPoint = fmt.Sprintf("%s:%d", inbound.ListenOn.String(), inbound.PortList.Range[0].From)
			break
		}
	}
	if grpcEndPoint == "" {
		return errors.New("[xray] can't find api port")
	}
	logger.Infof("[xray] api port: %s", grpcEndPoint)

	up, err := NewUserPool(ctx, grpcEndPoint)
	if err != nil {
		return err
	}
	up.StartSyncUserTask(ctx, cfg.SyncTrafficEndPoint)
	return nil
}
