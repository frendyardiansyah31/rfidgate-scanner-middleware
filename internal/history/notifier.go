package history

type Notifier interface {
	Notify(log HistoryLog)
}

type NotifyingRepository struct {
	Repository
	notifier Notifier
}

func NewNotifyingRepository(repo Repository, notifier Notifier) Repository {
	return &NotifyingRepository{Repository: repo, notifier: notifier}
}

func (r *NotifyingRepository) Insert(log *HistoryLog) error {
	if err := r.Repository.Insert(log); err != nil {
		return err
	}
	r.notifier.Notify(*log)
	return nil
}
