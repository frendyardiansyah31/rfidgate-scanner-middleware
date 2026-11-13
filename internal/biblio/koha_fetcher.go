package biblio

import (
	"context"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/koha"
)

// Batch is one listing from a delta-capable source.
type Batch struct {
	Entries    []Entry
	ServerTime string // the source's own clock, reused as the next `since`
	Total      int    // how many entries the source says it holds (for audit)
}

// DeltaFetcher is a source that can list only what changed. An empty `since`
// means a FULL listing.
type DeltaFetcher interface {
	FetchSince(ctx context.Context, since string) (Batch, error)
}

// kohaSource is the part of the Koha client this adapter needs.
type kohaSource interface {
	FetchBiblio(ctx context.Context, since string) (koha.BiblioResult, error)
}

// KohaFetcher adapts the Koha RfidGateApi /rfid/biblio listing to DeltaFetcher.
type KohaFetcher struct{ src kohaSource }

func NewKohaFetcher(src kohaSource) *KohaFetcher { return &KohaFetcher{src: src} }

func (f *KohaFetcher) FetchSince(ctx context.Context, since string) (Batch, error) {
	res, err := f.src.FetchBiblio(ctx, since)
	if err != nil {
		return Batch{}, err
	}
	now := time.Now()
	out := Batch{ServerTime: res.ServerTime, Total: res.Total}
	for _, r := range res.Records {
		if r.ItemCode == "" {
			continue // cannot be looked up from a scan
		}
		e := Entry{ItemCode: r.ItemCode, BiblioID: r.BiblioID, Title: r.Title, LastSynced: now}
		if r.HasCover {
			e.ImageFilename = r.BiblioID // cover reference = biblionumber, only when a cover exists
		}
		out.Entries = append(out.Entries, e)
	}
	return out, nil
}
