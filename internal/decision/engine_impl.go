package decision

import (
	"context"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
	"github.com/frendyardiansyah31/rfid-middleware/internal/rfidtag"
)

// defaultRemoteTimeout caps the request-time Koha lookup. The gate is
// latency-sensitive, so a slow/broken remote must flip to the local mirror
// fast — no retry loop on this path.
const defaultRemoteTimeout = 400 * time.Millisecond

type EngineImpl struct {
	remote        RemoteLookup
	remoteTimeout time.Duration
	cache         cache.Cache
	history       history.Repository
	timeout       time.Duration
}

// NewEngine keeps the original 4-arg constructor. With no remote wired the
// engine goes straight to the local mirror (memory → SQLite), i.e. the
// behaviour before remote-first lookup existed.
func NewEngine(
	cache cache.Cache,
	history history.Repository,
	timeout time.Duration,
) *EngineImpl {
	return NewEngineWithRemote(nil, cache, history, timeout)
}

// NewEngineWithRemote wires a request-time authoritative lookup (Koha). A nil
// remote falls back to unreachableRemote, so the local mirror is used.
func NewEngineWithRemote(
	remote RemoteLookup,
	cache cache.Cache,
	history history.Repository,
	timeout time.Duration,
) *EngineImpl {
	if remote == nil {
		remote = unreachableRemote{}
	}
	rt := defaultRemoteTimeout
	if timeout > 0 && timeout < rt {
		rt = timeout
	}
	return &EngineImpl{
		remote:        remote,
		remoteTimeout: rt,
		cache:         cache,
		history:       history,
		timeout:       timeout,
	}
}

// CheckRFID resolves a tag in strict order: remote (Koha, authoritative while
// reachable) → in-memory cache → SQLite mirror → unknown. The cache/SQLite tiers
// are only consulted when the remote is unreachable — never when the remote is
// up and simply has no such tag. Unknown tags are allowed and leave no trace.
func (e *EngineImpl) CheckRFID(ctx context.Context, rfid string) Result {
	// --- 1. Remote first. Short, no-retry deadline. ---
	remoteCtx, cancelRemote := context.WithTimeout(ctx, e.remoteTimeout)
	status, found, err := e.remoteLookup(remoteCtx, rfid)
	cancelRemote()

	if err == nil {
		if found {
			return e.decideFromStatus(rfid, status)
		}
		// Remote is reachable and the tag genuinely does not exist there.
		// "remote down" != "tag doesn't exist" — do NOT consult the (possibly
		// stale) local mirror for this case.
		return e.applyUnknown(rfid)
	}

	// --- 2. Remote unreachable / timed out / errored → local mirror. ---

	// Caller's own context is already done → treat as timeout, same as the
	// original engine.
	if ctx.Err() != nil {
		e.applyDecision(rfid, DecisionAlarm)
		return Result{Decision: DecisionAlarm, Reason: ReasonTimeout, Signal: false}
	}

	cacheCtx, cancelCache := context.WithTimeout(ctx, e.timeout)
	defer cancelCache()

	type localResult struct {
		status cache.Status
		ok     bool
	}
	ch := make(chan localResult, 1)

	go func() {
		// A panic here would kill the whole service. Recovering without sending a
		// result lets the select below hit its timeout and answer Alarm, the
		// documented behaviour for "could not look it up in time".
		defer func() { _ = recover() }()
		// The mirror is keyed by the normalised tag (no "SN"), the same rule as
		// the legacy gate and the Koha plugin, so one lookup covers both the
		// prefixed and the plain form of a tag. CompositeCache.Get already does
		// memory → SQLite internally.
		st, ok := e.cache.Get(rfidtag.Normalize(rfid))
		ch <- localResult{status: st, ok: ok}
	}()

	select {
	case <-cacheCtx.Done():
		e.applyDecision(rfid, DecisionAlarm)
		return Result{Decision: DecisionAlarm, Reason: ReasonTimeout, Signal: false}

	case res := <-ch:
		if !res.ok {
			return e.applyUnknown(rfid)
		}
		return e.decideFromStatus(rfid, res.status)
	}
}

// remoteLookup asks the remote ONCE with the raw tag. The Koha RfidGateApi
// plugin normalises the "SN" prefix on both sides (same rule as the legacy gate),
// so a second "SN"+rfid probe would only cost another round-trip. Any error is
// returned as-is so the engine can fall back to the local mirror.
func (e *EngineImpl) remoteLookup(ctx context.Context, rfid string) (cache.Status, bool, error) {
	return e.remote.Status(ctx, rfid)
}

// decideFromStatus turns a resolved loan status into a gate decision. Shared by
// the remote-found and local-found paths so the rule lives in one place.
func (e *EngineImpl) decideFromStatus(rfid string, status cache.Status) Result {
	if status == cache.StatusLoaned {
		e.applyDecision(rfid, DecisionAllow)
		return Result{Decision: DecisionAllow, Reason: ReasonLoanValid, Signal: false}
	}
	e.applyDecision(rfid, DecisionAlarm)
	return Result{Decision: DecisionAlarm, Reason: ReasonNotLoan, Signal: true}
}

// applyUnknown is the "tag not known to us" outcome: Allow (visitor's
// wallet/clothes, not a tagged book), no signal to the gate.
//
// Deliberately leaves NO trace — nothing is written to gate_logs. The legacy gate returned silently for a tag
// that is not a library item; recording it as "loaned" polluted the logs and
// the dashboard's permitted count.
func (e *EngineImpl) applyUnknown(rfid string) Result {
	return Result{Decision: DecisionAllow, Reason: ReasonUnknownRFID, Signal: false}
}

func (e *EngineImpl) applyDecision(rfid string, decision int) {
	// Keputusan TIDAK ditulis balik ke e.cache: rfid_status adalah ground truth dari sync Koha
	// (single source of truth), gate tidak boleh menimpanya. Menulis balik di sini pernah
	// menyebabkan bug: rfid mentah yang di-scan gate membuat entry cache baru yang
	// membajak lookup untuk selamanya, mengoverride data Koha yang sebenarnya lebih baru.

	// Simpan ke gate_logs (best-effort)
	if e.history != nil {
		go func() {
			// A panic in a goroutine is not caught by net/http's per-request
			// recover and would take the whole service down. Logging is
			// best-effort, so swallow it.
			defer func() { _ = recover() }()
			_ = e.history.Insert(&history.HistoryLog{
				RFID:   rfid,
				Status: decision,
			})
		}()
	}
}
