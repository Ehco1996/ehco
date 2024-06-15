package relay

import "context"

func (r *Relay) HealthCheck(ctx context.Context) error {
	return r.relayServer.HealthCheck(ctx)
}
