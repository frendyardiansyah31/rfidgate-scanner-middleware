package history

// Enricher attaches metadata (item_code, biblio_id, title, image_filename) to a HistoryLog.
type Enricher interface {
	Enrich(log HistoryLog) HistoryLog
}

type EnrichingRepository struct {
	Repository
	enricher Enricher
}

func NewEnrichingRepository(repo Repository, enricher Enricher) Repository {
	return &EnrichingRepository{Repository: repo, enricher: enricher}
}

func (r *EnrichingRepository) Insert(log *HistoryLog) error {
	*log = r.enricher.Enrich(*log)
	return r.Repository.Insert(log)
}
