package koha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
)

// RemoteLookup answers request-time RFID status queries via the Koha
// RfidGateApi plugin (POST {base_url}/rfid/check). It satisfies
// decision.RemoteLookup.
//
// base_url is the plugin namespace root, e.g.
// http://localhost:8001/api/v1/contrib/rfid_gate
//
// Auth is X-API-Key only. Koha core intercepts any "Authorization: Bearer"
// header as OAuth2 before plugin routes run, so Bearer must never be sent.
type RemoteLookup struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewRemoteLookup(baseURL, apiKey string) *RemoteLookup {
	return &RemoteLookup{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		// Engine already bounds each call with its own remote deadline; this
		// is only a safety net for callers that pass a context without one.
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

type checkResponse struct {
	Status  string `json:"status"`
	RFIDTag string `json:"rfid_tag"`
	Message string `json:"message"`
}

// Status returns:
//
//	(status, true,  nil)  → BORROWED / NOT_BORROWED
//	(_,      false, nil)  → ITEM_NOT_FOUND
//	(_,      false, err)  → transport error, non-200, auth failure, unknown status
func (k *RemoteLookup) Status(ctx context.Context, rfid string) (cache.Status, bool, error) {
	body, err := json.Marshal(map[string]string{"rfid_tag": rfid})
	if err != nil {
		return cache.StatusUnknown, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.baseURL+"/rfid/check", bytes.NewReader(body))
	if err != nil {
		return cache.StatusUnknown, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", k.apiKey)

	resp, err := k.client.Do(req)
	if err != nil {
		return cache.StatusUnknown, false, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return cache.StatusUnknown, false, err
	}

	var out checkResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return cache.StatusUnknown, false, fmt.Errorf("koha rfid/check: http %d, invalid json: %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK {
		return cache.StatusUnknown, false, fmt.Errorf("koha rfid/check: http %d %s: %s", resp.StatusCode, out.Status, out.Message)
	}

	switch out.Status {
	case "BORROWED":
		return cache.StatusLoaned, true, nil
	case "NOT_BORROWED":
		return cache.StatusNotLoaned, true, nil
	case "ITEM_NOT_FOUND":
		return cache.StatusUnknown, false, nil
	default:
		return cache.StatusUnknown, false, fmt.Errorf("koha rfid/check: unknown status %q", out.Status)
	}
}
