package sync

type HealthAware interface {
	IsOnline() bool
}
