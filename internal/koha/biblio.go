package koha

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// BiblioRecord is the minimal bibliographic data the gate dashboard needs for one
// RFID-tagged item, as reported by GET /rfid/biblio.
type BiblioRecord struct {
	ItemCode string // item barcode (same as StatusRecord.ItemCode)
	BiblioID string // biblionumber
	Title    string // already cleaned of trailing ISBD punctuation by the plugin
	HasCover bool   // a local cover image exists for this biblio
}

// BiblioResult is everything a (possibly multi-page) biblio listing returned.
type BiblioResult struct {
	Records    []BiblioRecord
	ServerTime string // Koha's clock, read before the query; reuse as the next `since`
	Total      int    // tagged items with a barcode, for audit
}

type biblioPageDTO struct {
	ServerTime string `json:"server_time"`
	Total      int    `json:"total"`
	HasMore    bool   `json:"has_more"`
	NextAfter  string `json:"next_after"`
	Records    []struct {
		ItemCode string `json:"item_code"`
		BiblioID string `json:"biblio_id"`
		Title    string `json:"title"`
		HasCover bool   `json:"has_cover"`
	} `json:"records"`
}

// FetchBiblio lists biblio data from Koha, following the keyset cursor until the
// server reports no more pages. Empty `since` is a FULL listing; otherwise only
// items changed since that Koha-local time ("YYYY-MM-DD HH:MM:SS"). Like
// FetchStatus it fails as a whole and never returns a partial result.
func (k *RemoteLookup) FetchBiblio(ctx context.Context, since string) (BiblioResult, error) {
	var out BiblioResult
	after := ""

	for page := 0; page < maxStatusPages; page++ {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(statusPageSize))
		if since != "" {
			q.Set("since", since)
		}
		if after != "" {
			q.Set("after", after)
		}
		var dto biblioPageDTO
		if err := k.getJSON(ctx, "/rfid/biblio", q, &dto); err != nil {
			return BiblioResult{}, err
		}
		if page == 0 {
			out.ServerTime = dto.ServerTime
			out.Total = dto.Total
		}
		for _, r := range dto.Records {
			out.Records = append(out.Records, BiblioRecord{
				ItemCode: r.ItemCode, BiblioID: r.BiblioID, Title: r.Title, HasCover: r.HasCover,
			})
		}
		if !dto.HasMore {
			return out, nil
		}
		if dto.NextAfter == "" || dto.NextAfter == after {
			return BiblioResult{}, fmt.Errorf("koha rfid/biblio: cursor did not advance (after=%q)", after)
		}
		after = dto.NextAfter
	}
	return BiblioResult{}, fmt.Errorf("koha rfid/biblio: more than %d pages", maxStatusPages)
}
