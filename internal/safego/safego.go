// Package safego keeps a panic in a background goroutine from taking the whole
// service down. net/http recovers panics per request, but a panic in any other
// goroutine (a sync loop, the queue worker) terminates the process, and with it
// the gate's decisions.
package safego

import (
	"fmt"
	"runtime/debug"

	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

// Recover must be deferred directly: `defer safego.Recover(log, "mirror.sync", nil)`.
// It logs the panic with its stack. If errp is not nil the panic is also stored
// there as an error, so a caller that returns an error reports it normally.
func Recover(log logger.Logger, where string, errp *error) {
	r := recover()
	if r == nil {
		return
	}
	if log != nil {
		log.Error("panic recovered",
			logger.F("where", where),
			logger.F("panic", fmt.Sprint(r)),
			logger.F("stack", string(debug.Stack())),
		)
	}
	if errp != nil {
		*errp = fmt.Errorf("panic in %s: %v", where, r)
	}
}
