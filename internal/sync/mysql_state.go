package sync

import (
	"sync/atomic"

	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

func (s *MySQLSlimsSync) markDown(err error) {
	if atomic.CompareAndSwapUint32(&s.mysqlDown, 0, 1) {
		s.log.Warn("mysql connection lost, entering retry mode",
			logger.F("error", err),
		)
	}
}

func (s *MySQLSlimsSync) markUp() {
	if atomic.CompareAndSwapUint32(&s.mysqlDown, 1, 0) {
		s.log.Info("mysql reconnected, sync resumed")
	}
}
