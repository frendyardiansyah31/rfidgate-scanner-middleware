package decision

import "github.com/frendyardiansyah31/rfid-middleware/internal/cache"

func ToCacheStatus(d int) cache.Status {
	switch d {
	case DecisionAllow:
		return cache.StatusLoaned
	case DecisionAlarm:
		return cache.StatusNotLoaned
	default:
		return cache.StatusUnknown
	}
}
