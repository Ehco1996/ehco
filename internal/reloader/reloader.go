package reloader

type Reloader interface {
	Reload(force bool) error
}
