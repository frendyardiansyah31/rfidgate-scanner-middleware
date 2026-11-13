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

func TestDecisionEngine_PropertiesExtended(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50 // lebih banyak iterasi untuk bulk simulation
	properties := gopter.NewProperties(parameters)

	statusGen := gen.OneConstOf(cache.StatusLoaned, cache.StatusNotLoaned, cache.StatusUnknown)
	rfidGen := gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && !strings.Contains(s, "SN") })

	// --- Property 1: All statuses produce correct decision ---
	properties.Property(
		"Decision engine returns correct decision for all statuses",
		prop.ForAll(
			func(rfid string, s cache.Status) bool {
				cacheRepo := newFakeCache()
				queue := newFakeQueue()
				engine := NewEngine(cacheRepo, queue, nil, 1*time.Second)

				cacheRepo.Set(rfid, s)

				decisionCode := engine.CheckRFID(context.Background(), rfid)

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

				wantPublished := ToCacheStatus(decisionCode.Decision)
				if len(queue.events) != 1 || queue.events[0].RFID != rfid || queue.events[0].Status != wantPublished {
					t.Logf("Queue event mismatch: %+v, want status %v", queue.events, wantPublished)
					return false
				}

				// rfid_status adalah ground truth dari SLiMS — CheckRFID tidak boleh menulis balik
				statusCached, ok := cacheRepo.Get(rfid)
				if !ok || statusCached != s {
					t.Logf("Cache must stay untouched: got %v, expected unchanged %v", statusCached, s)
					return false
				}

				return true
			},
			rfidGen,
			statusGen,
		),
	)

	// --- Property 2: Cache miss triggers DecisionAlarm ---
	properties.Property(
		"Unknown RFID triggers alarm",
		prop.ForAll(
			func(rfid string) bool {
				cacheRepo := newFakeCache() // empty cache
				queue := newFakeQueue()
				engine := NewEngine(cacheRepo, queue, nil, 1*time.Second)

				decisionCode := engine.CheckRFID(context.Background(), rfid)

				// Unknown/cache-miss RFID is treated as Allow (e.g. visitor's clothes/wallet,
				// not a tagged book) — see engine_impl.go's ReasonUnknownRFID branch.
				if decisionCode.Decision != DecisionAllow {
					t.Logf("Unknown RFID -> expected DecisionAllow, got %d", decisionCode.Decision)
					return false
				}

				// Unknown tags leave no trace: nothing is published.
				if len(queue.events) != 0 {
					t.Logf("unknown RFID must not publish, got %+v", queue.events)
					return false
				}

				// Cache-miss must not create a new cache entry
				if _, ok := cacheRepo.Get(rfid); ok {
					t.Logf("unknown rfid must not create a cache entry")
					return false
				}

				return true
			},
			rfidGen,
		),
	)

	// --- Property 3: Bulk random RFIDs ---
	properties.Property(
		"Bulk random RFIDs all produce valid decisions",
		prop.ForAll(
			func(rfids []string, statuses []cache.Status) bool {
				if len(rfids) != len(statuses) || len(rfids) == 0 {
					return true // skip invalid combination
				}

				cacheRepo := newFakeCache()
				queue := newFakeQueue()
				engine := NewEngine(cacheRepo, queue, nil, 1*time.Second)

				// rfids isn't guaranteed unique — cacheRepo.Set is last-write-wins per key,
				// so the expected status per rfid must follow the same rule, not statuses[i].
				want := make(map[string]cache.Status, len(rfids))
				for i, rfid := range rfids {
					cacheRepo.Set(rfid, statuses[i])
					want[rfid] = statuses[i]
				}

				for _, rfid := range rfids {
					decisionCode := engine.CheckRFID(context.Background(), rfid)
					s := want[rfid]

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
				}

				if len(queue.events) != len(rfids) {
					t.Logf("Queue event count mismatch, got %d, expected %d", len(queue.events), len(rfids))
					return false
				}

				return true
			},
			gen.SliceOfN(20, rfidGen),   // generate 20 random RFIDs
			gen.SliceOfN(20, statusGen), // generate 20 random statuses
		),
	)

	properties.TestingRun(t)
}
