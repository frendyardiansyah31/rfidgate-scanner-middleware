package koha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
)

type rec struct {
	RFIDNorm string `json:"rfid_norm"`
	ItemCode string `json:"item_code"`
	Status   string `json:"status"`
}

// statusServer serves `all` (sorted by rfid_norm) in pages of `page`, honouring
// the `after` keyset cursor like the real plugin. It records the queries it saw.
func statusServer(t *testing.T, all []rec, page int, hits *int32, lastQuery *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if r.URL.Path != "/rfid/status" || r.Method != http.MethodGet {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if lastQuery != nil {
			*lastQuery = r.URL.RawQuery
		}
		after := r.URL.Query().Get("after")
		var rows []rec
		for _, x := range all {
			if x.RFIDNorm > after {
				rows = append(rows, x)
			}
		}
		more := len(rows) > page
		if more {
			rows = rows[:page]
		}
		next := ""
		if more {
			next = rows[len(rows)-1].RFIDNorm
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"server_time": "2026-11-11 22:00:00",
			"since":       r.URL.Query().Get("since"),
			"total_rfid":  len(all),
			"count":       len(rows),
			"has_more":    more,
			"next_after":  next,
			"records":     rows,
		})
	}))
}

func sample(n int) []rec {
	out := make([]rec, n)
	for i := range out {
		st := "NOT_BORROWED"
		if i%2 == 0 {
			st = "BORROWED"
		}
		out[i] = rec{RFIDNorm: fmt.Sprintf("T%04d", i), ItemCode: fmt.Sprintf("B%04d", i), Status: st}
	}
	return out
}

func TestFetchStatus_FollowsCursorAcrossPages(t *testing.T) {
	var hits int32
	srv := statusServer(t, sample(7), 3, &hits, nil) // 3 pages: 3+3+1
	defer srv.Close()

	res, err := NewRemoteLookup(srv.URL, "k").FetchStatus(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 7 || res.TotalRFID != 7 || res.ServerTime != "2026-11-11 22:00:00" {
		t.Fatalf("got %d records total=%d time=%q", len(res.Records), res.TotalRFID, res.ServerTime)
	}
	if hits != 3 {
		t.Fatalf("want 3 page requests, got %d", hits)
	}
	if r := res.Records[0]; r.RFIDNorm != "T0000" || r.ItemCode != "B0000" || r.Status != cache.StatusLoaned {
		t.Fatalf("first record mapped wrong: %+v", r)
	}
	if r := res.Records[1]; r.Status != cache.StatusNotLoaned {
		t.Fatalf("NOT_BORROWED must map to StatusNotLoaned, got %+v", r)
	}
}

func TestFetchStatus_SendsSinceOnlyWhenGiven(t *testing.T) {
	var hits int32
	var q string
	srv := statusServer(t, sample(1), 10, &hits, &q)
	defer srv.Close()
	c := NewRemoteLookup(srv.URL, "k")

	if _, err := c.FetchStatus(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if want := "limit=5000"; q != want {
		t.Fatalf("full listing query = %q, want %q", q, want)
	}

	if _, err := c.FetchStatus(context.Background(), "2026-11-11 21:45:00"); err != nil {
		t.Fatal(err)
	}
	if want := "limit=5000&since=2026-11-11+21%3A45%3A00"; q != want {
		t.Fatalf("delta query = %q, want %q", q, want)
	}
}

func TestFetchStatus_Errors(t *testing.T) {
	t.Run("wrong api key -> error, no partial result", func(t *testing.T) {
		var hits int32
		srv := statusServer(t, sample(2), 10, &hits, nil)
		defer srv.Close()
		res, err := NewRemoteLookup(srv.URL, "wrong").FetchStatus(context.Background(), "")
		if err == nil || len(res.Records) != 0 {
			t.Fatalf("want error and no records, got err=%v records=%d", err, len(res.Records))
		}
	})

	t.Run("unknown status string -> error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"server_time":"x","total_rfid":1,"has_more":false,"records":[{"rfid_norm":"A","item_code":"B","status":"WEIRD"}]}`))
		}))
		defer srv.Close()
		if _, err := NewRemoteLookup(srv.URL, "k").FetchStatus(context.Background(), ""); err == nil {
			t.Fatal("want error for unknown status")
		}
	})

	t.Run("cursor that never advances -> error, not an endless loop", func(t *testing.T) {
		var hits int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			_, _ = w.Write([]byte(`{"server_time":"x","total_rfid":9,"has_more":true,"next_after":"SAME","records":[]}`))
		}))
		defer srv.Close()
		_, err := NewRemoteLookup(srv.URL, "k").FetchStatus(context.Background(), "")
		if err == nil {
			t.Fatal("want error for stuck cursor")
		}
		if hits > 3 {
			t.Fatalf("looped %d times on a stuck cursor", hits)
		}
	})

	t.Run("http 500 -> error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		if _, err := NewRemoteLookup(srv.URL, "k").FetchStatus(context.Background(), ""); err == nil {
			t.Fatal("want error on 500")
		}
	})
}

// --- /rfid/biblio --------------------------------------------------------

func TestFetchBiblio_FollowsCursor_AndMapsFields(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/rfid/biblio" || r.Header.Get("X-API-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("after") == "" {
			_, _ = w.Write([]byte(`{"server_time":"2026-11-11 23:00:00","total":3,"has_more":true,"next_after":"2",
				"records":[{"item_code":"B1","biblio_id":"10","title":"One","has_cover":true},
				           {"item_code":"B2","biblio_id":"11","title":"Two","has_cover":false}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"server_time":"ignored","total":3,"has_more":false,"next_after":"",
			"records":[{"item_code":"B3","biblio_id":"12","title":"Three","has_cover":true}]}`))
	}))
	defer srv.Close()

	res, err := NewRemoteLookup(srv.URL, "k").FetchBiblio(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if hits != 2 || len(res.Records) != 3 || res.Total != 3 || res.ServerTime != "2026-11-11 23:00:00" {
		t.Fatalf("hits=%d records=%d total=%d time=%q", hits, len(res.Records), res.Total, res.ServerTime)
	}
	if r := res.Records[0]; r.ItemCode != "B1" || r.BiblioID != "10" || r.Title != "One" || !r.HasCover {
		t.Fatalf("record mapped wrong: %+v", r)
	}
	if res.Records[1].HasCover {
		t.Fatal("has_cover=false must stay false")
	}
}

func TestFetchBiblio_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := NewRemoteLookup(srv.URL, "k").FetchBiblio(context.Background(), ""); err == nil {
		t.Fatal("want error on http 500")
	}

	stuck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"total":9,"has_more":true,"next_after":"SAME","records":[]}`))
	}))
	defer stuck.Close()
	if _, err := NewRemoteLookup(stuck.URL, "k").FetchBiblio(context.Background(), ""); err == nil {
		t.Fatal("want error for a cursor that never advances")
	}
}
