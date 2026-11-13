package sync

import (
	"context"
	"database/sql"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

// MySQLRemoteLookup answers request-time RFID status queries straight from the
// SLiMS MySQL database. It satisfies decision.RemoteLookup.
//
// The loan-status rule is intentionally identical to the bulk delta pull in
// mysql_sync.go PullDelta: a tag is "loaned" (0) when its most recent loan row
// has is_lent = 1 AND is_return = 0; otherwise "available" (1). Keep the two in
// sync if the SLiMS loan model ever changes.
type MySQLRemoteLookup struct {
	db  *sql.DB
	log logger.Logger
}

func NewMySQLRemoteLookup(db *sql.DB, log logger.Logger) *MySQLRemoteLookup {
	return &MySQLRemoteLookup{db: db, log: log}
}

const remoteStatusQuery = `
	SELECT
		CASE WHEN l.is_lent = 1 AND l.is_return = 0 THEN 0 ELSE 1 END AS status
	FROM item i
	LEFT JOIN loan l
		ON l.item_code = i.item_code
		AND l.last_update = (
			SELECT MAX(l2.last_update) FROM loan l2 WHERE l2.item_code = i.item_code
		)
	WHERE i.label_code = ?
	LIMIT 1
`

// Status returns:
//
//	(status, true,  nil)  → tag found in SLiMS
//	(_,      false, nil)  → SLiMS reachable, tag not present
//	(_,      false, err)  → query failed / DB unreachable / context deadline
func (m *MySQLRemoteLookup) Status(ctx context.Context, rfid string) (cache.Status, bool, error) {
	var status int
	err := m.db.QueryRowContext(ctx, remoteStatusQuery, rfid).Scan(&status)
	if err == sql.ErrNoRows {
		return cache.StatusUnknown, false, nil
	}
	if err != nil {
		return cache.StatusUnknown, false, err
	}
	// status is 0 (Loaned) or 1 (Available); cache.Status uses the same codes.
	return cache.Status(status), true, nil
}
