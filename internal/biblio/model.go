package biblio

import "time"

type Entry struct {
	ItemCode string
	BiblioID string
	Title    string
	// ImageFilename is the cover reference the dashboards use to build the cover
	// URL. With Koha it is the biblionumber, set ONLY when a local cover image
	// exists (the OPAC answers a 1x1 gif for biblios without one). Empty means
	// "no cover": the dashboard shows its placeholder.
	ImageFilename string
	LastSynced    time.Time
}
