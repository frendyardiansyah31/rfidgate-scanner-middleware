// internal/sync/slims_pull.go
package sync

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
)

func (s *MySQLSlimsSync) PullAll(ctx context.Context) ([]RFIDStatus, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			i.label_code AS rfid,
			CASE
				WHEN l.loan_id IS NOT NULL
				     AND l.is_return = 0
				THEN 1
				ELSE 2
			END AS status
		FROM item i
		LEFT JOIN loan l
			ON l.item_code = i.item_code
			AND l.is_return = 0
		WHERE i.label_code IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []RFIDStatus

	for rows.Next() {
		var rfid string
		var statusInt int

		if err := rows.Scan(&rfid, &statusInt); err != nil {
			return nil, err
		}

		var status cache.Status
		switch statusInt {
		case 1:
			status = cache.StatusLoaned
		case 2:
			status = cache.StatusNotLoaned
		default:
			status = cache.StatusUnknown
		}

		result = append(result, RFIDStatus{
			RFID:   rfid,
			Status: status,
		})
	}

	return result, rows.Err()
}
