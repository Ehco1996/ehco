package reloader

import "context"

type Reloader interface {
	Reload() error
	WatchAndReload(ctx context.Context)
	TriggerReload()
}
