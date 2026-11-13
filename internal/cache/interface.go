package cache

type Status int

const (
	StatusLoaned Status = iota
	StatusNotLoaned
	StatusUnknown
)

type Cache interface {
	Get(rfid string) (Status, bool)
	Set(rfid string, status Status) error
	Close() error
}
