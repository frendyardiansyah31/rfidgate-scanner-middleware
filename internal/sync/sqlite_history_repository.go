package sync

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
)

type SQLiteHistoryRepository struct {
	store *SQLiteOfflineStore
}

func NewSQLiteHistoryRepository(
	store *SQLiteOfflineStore,
) *SQLiteHistoryRepository {
	return &SQLiteHistoryRepository{store: store}
}

func (r *SQLiteHistoryRepository) Insert(log history.HistoryLog) error {
	return r.store.Insert(context.Background(), log)
}

func (r *SQLiteHistoryRepository) GetRecent(limit int) ([]history.HistoryLog, error) {
	return r.store.GetRecent(limit)
}
