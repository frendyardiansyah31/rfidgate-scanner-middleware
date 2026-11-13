package repository

import (
	"context"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/domain"
)

type RFIDRepository interface {
	UpsertStatus(ctx context.Context, snap domain.RFIDSnapshot) error
	UpsertItem(ctx context.Context, snap domain.RFIDSnapshot) error
	WithTx(ctx context.Context, fn func(repo RFIDRepository) error) error
	LastUpdated(ctx context.Context) (time.Time, error)
}
