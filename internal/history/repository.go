package history

type Repository interface {
	Insert(log *HistoryLog) error
	GetRecent(limit int) ([]HistoryLog, error)
}
