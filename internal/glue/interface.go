package glue

import (
	"context"

	"github.com/Ehco1996/ehco/pkg/lb"
)

type Reloader interface {
	Reload(force bool) error
}

type HealthChecker interface {
	HealthCheck(ctx context.Context, remote *lb.Node) error
}
