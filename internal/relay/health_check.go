package relay

import (
	"context"
	"fmt"

	"github.com/Ehco1996/ehco/internal/glue"
)

var _ glue.HealthChecker = (*Server)(nil)

func (r *Server) HealthCheck(ctx context.Context, relayID string) (int64, error) {
	rs, ok := r.relayM.Load(relayID)
	if !ok {
		return 0, fmt.Errorf("label for relay: %s not found,can not health check", relayID)
	}
	inner, _ := rs.(*Relay)
	return inner.relayServer.HealthCheck(ctx)
}
