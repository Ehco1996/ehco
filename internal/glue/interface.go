package glue

import (
	"context"
)

type Reloader interface {
	Reload(force bool) error
}

type HealthChecker interface {
	// get relay by ID and check the connection health
	HealthCheck(ctx context.Context, RelayID string) (int64, error)
}
