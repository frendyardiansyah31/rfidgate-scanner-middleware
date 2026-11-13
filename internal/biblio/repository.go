package biblio

type Repository interface {
	Upsert(entry Entry) error
	Lookup(itemCode string) (Entry, bool, error)
	Count() (int, error)
}
