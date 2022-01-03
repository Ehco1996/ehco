package xray

import (
	"context"

	"github.com/xtls/xray-core/core"
)

func StartXrayServer(ctx context.Context, cfg *core.Config) error {
	println(cfg)
	return nil
}
