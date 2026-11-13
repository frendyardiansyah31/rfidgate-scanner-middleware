package koha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
)

// StatusRecord is one RFID-tagged item as reported by GET /rfid/status.
// RFIDNorm is already normalised by the plugin (no "SN"), matching the key the
// local mirror uses.
type StatusRecord struct {
	RFIDNorm string
	ItemCode string
	Status   cache.Status
}

// StatusResult is everything a (possibly multi-page) status listing returned.
type StatusResult struct {
	Records []StatusRecord
	// ServerTime is Koha's own clock read before the first page's query. Send it
	// back (minus an overlap window) as the next `since`.
	ServerTime string
	// TotalRFID is how many tagged items Koha holds, for audit against the mirror.
	TotalRFID int
}

type statusPageDTO struct {
	ServerTime string `json:"server_time"`
	TotalRFID  int    `json:"total_rfid"`
	HasMore    bool   `json:"has_more"`
	NextAfter  string `json:"next_after"`
	Records    []struct {
		RFIDNorm string `json:"rfid_norm"`
		ItemCode string `json:"item_code"`
		Status   string `json:"status"`
	} `json:"records"`
}

const statusPageSize = 5000

// maxStatusPages stops a misbehaving server (a cursor that never advances) from
// looping forever. 200 pages x 5000 = 1M tagged items, far above any real library.
const maxStatusPages = 200

// FetchStatus lists item status from Koha, following the keyset cursor until the
// server reports no more pages. An empty `since` is a FULL listing; otherwise only
// items changed since that Koha-local time ("YYYY-MM-DD HH:MM:SS").
//
// It fails as a whole on any page error, and never returns a partial result, so a
// caller can safely treat success as "this is the complete answer".
func (k *RemoteLookup) FetchStatus(ctx context.Context, since string) (StatusResult, error) {
	var out StatusResult
	after := ""

	for page := 0; page < maxStatusPages; page++ {
		dto, err := k.statusPage(ctx, since, after)
		if err != nil {
			return StatusResult{}, err
		}
		if page == 0 {
			out.ServerTime = dto.ServerTime
			out.TotalRFID = dto.TotalRFID
		}
		for _, r := range dto.Records {
			st, err := parseStatus(r.Status)
			if err != nil {
				return StatusResult{}, err
			}
			out.Records = append(out.Records, StatusRecord{RFIDNorm: r.RFIDNorm, ItemCode: r.ItemCode, Status: st})
		}
		if !dto.HasMore {
			return out, nil
		}
		if dto.NextAfter == "" || dto.NextAfter == after {
			return StatusResult{}, fmt.Errorf("koha rfid/status: cursor did not advance (after=%q)", after)
		}
		after = dto.NextAfter
	}
	return StatusResult{}, fmt.Errorf("koha rfid/status: more than %d pages", maxStatusPages)
}

func parseStatus(s string) (cache.Status, error) {
	switch s {
	case "BORROWED":
		return cache.StatusLoaned, nil
	case "NOT_BORROWED":
		return cache.StatusNotLoaned, nil
	default:
		return cache.StatusUnknown, fmt.Errorf("koha rfid/status: unknown status %q", s)
	}
}

func (k *RemoteLookup) statusPage(ctx context.Context, since, after string) (statusPageDTO, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(statusPageSize))
	if since != "" {
		q.Set("since", since)
	}
	if after != "" {
		q.Set("after", after)
	}
	var dto statusPageDTO
	err := k.getJSON(ctx, "/rfid/status", q, &dto)
	return dto, err
}

// getJSON performs an authenticated GET against the plugin and decodes the JSON
// body into out. Any non-200 is an error; bodies are size-bounded.
func (k *RemoteLookup) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", k.apiKey)

	resp, err := k.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// A 5000-row page is ~0.5 MB; allow generous headroom but still bound it.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("koha %s: http %d", path, resp.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("koha %s: invalid json: %w", path, err)
	}
	return nil
}
