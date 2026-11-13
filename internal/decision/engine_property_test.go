package decision

import (
	"strings"
	"context"
	"testing"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func TestDecisionEngineProperties(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)

	// Generator: semua status yang valid
	statusGen := gen.OneConstOf(cache.StatusLoaned, cache.StatusNotLoaned, cache.StatusUnknown)
	// Generator: random RFID string
	rfidGen := gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && !strings.Contains(s, "SN") })

	properties.Property("Decision engine returns correct decision for all statuses", prop.ForAll(
		func(rfid string, s cache.Status) bool {
			// Setup fake cache
			cacheRepo := newFakeCache()
			engine := NewEngine(cacheRepo, nil, 1*time.Second)

			// Simulasi cache sudah ada
			cacheRepo.Set(rfid, s)

			decisionCode := engine.CheckRFID(context.Background(), rfid)

			// 1️⃣ Cek decision
			switch s {
			case cache.StatusLoaned:
				if decisionCode.Decision != DecisionAllow {
					t.Logf("Loaned status -> expected DecisionAllow, got %d", decisionCode.Decision)
					return false
				}
			case cache.StatusNotLoaned, cache.StatusUnknown:
				if decisionCode.Decision != DecisionAlarm {
					t.Logf("Status %v -> expected DecisionAlarm, got %d", s, decisionCode.Decision)
					return false
				}
			}

			// rfid_status adalah ground truth dari Koha — CheckRFID tidak boleh menulis balik
			statusCached, ok := cacheRepo.Get(rfid)
			if !ok || statusCached != s {
				t.Logf("Cache must stay untouched: got %v, expected unchanged %v", statusCached, s)
				return false
			}

			return true
		},
		rfidGen,
		statusGen,
	))

	properties.TestingRun(t)
}
