package koha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
)

func fakeKoha(t *testing.T, code int, status string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rfid/check" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "k" {
			t.Errorf("X-API-Key = %q, want %q", got, "k")
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization header must not be sent to Koha")
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["rfid_tag"] != "TAG1" {
			t.Errorf("rfid_tag = %q", body["rfid_tag"])
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": status, "rfid_tag": body["rfid_tag"]})
	}))
}

func TestStatus(t *testing.T) {
	cases := []struct {
		name       string
		code       int
		status     string
		wantStatus cache.Status
		wantFound  bool
		wantErr    bool
	}{
		{"borrowed", 200, "BORROWED", cache.StatusLoaned, true, false},
		{"not borrowed", 200, "NOT_BORROWED", cache.StatusNotLoaned, true, false},
		{"not found", 200, "ITEM_NOT_FOUND", cache.StatusUnknown, false, false},
		{"unauthorized", 401, "UNAUTHORIZED", cache.StatusUnknown, false, true},
		{"server error", 500, "INTERNAL_ERROR", cache.StatusUnknown, false, true},
		{"unknown status", 200, "WEIRD", cache.StatusUnknown, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := fakeKoha(t, tc.code, tc.status)
			defer srv.Close()

			st, found, err := NewRemoteLookup(srv.URL+"/", "k").Status(context.Background(), "TAG1")
			if (err != nil) != tc.wantErr || found != tc.wantFound || st != tc.wantStatus {
				t.Fatalf("got (%v, %v, %v), want (%v, %v, err=%v)", st, found, err, tc.wantStatus, tc.wantFound, tc.wantErr)
			}
		})
	}
}

func TestStatusUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, found, err := NewRemoteLookup(url, "k").Status(context.Background(), "TAG1")
	if err == nil || found {
		t.Fatalf("want transport error, got found=%v err=%v", found, err)
	}
}
