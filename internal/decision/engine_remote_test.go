package decision

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
)

// --- fakes ---------------------------------------------------------------

type fakeRemote struct {
	mu    sync.Mutex
	byKey map[string]cache.Status // key present => found
	err   error                   // if set, every call returns it
	delay time.Duration           // simulate a slow remote (context-aware)
	calls int32
	keys  []string
}

func newFakeRemote() *fakeRemote {
	return &fakeRemote{byKey: make(map[string]cache.Status)}
}

func (f *fakeRemote) Status(ctx context.Context, rfid string) (cache.Status, bool, error) {
	atomic.AddInt32(&f.calls, 1)
	f.mu.Lock()
	f.keys = append(f.keys, rfid)
	f.mu.Unlock()

	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return cache.StatusUnknown, false, ctx.Err()
		}
	}
	if f.err != nil {
		return cache.StatusUnknown, false, f.err
	}
	f.mu.Lock()
	st, ok := f.byKey[rfid]
	f.mu.Unlock()
	return st, ok, nil
}

// spyCache counts Get calls so a test can prove the cache was never consulted.
type spyCache struct {
	data map[string]cache.Status
	gets int32
}

func newSpyCache() *spyCache { return &spyCache{data: make(map[string]cache.Status)} }

func (s *spyCache) Get(rfid string) (cache.Status, bool) {
	atomic.AddInt32(&s.gets, 1)
	v, ok := s.data[rfid]
	return v, ok
}
func (s *spyCache) Set(rfid string, st cache.Status) error { s.data[rfid] = st; return nil }
func (s *spyCache) Close() error                           { return nil }

type flakyRemote struct{ n int32 }

func (f *flakyRemote) Status(_ context.Context, _ string) (cache.Status, bool, error) {
	if atomic.AddInt32(&f.n, 1)%2 == 0 {
		return cache.StatusNotLoaned, true, nil
	}
	return cache.StatusUnknown, false, errors.New("flaky remote")
}

// --- tests: lookup order REMOTE -> MEMORY -> SQLITE ---------------------

// Test 1: remote reachable + found -> use remote, cache must NOT be touched.
func TestCheckRFID_RemoteFound_UsesRemote_SkipsCache(t *testing.T) {
	rem := newFakeRemote()
	rem.byKey["TAG1"] = cache.StatusLoaned
	spy := newSpyCache()
	q := newFakeQueue()

	e := NewEngineWithRemote(rem, spy, q, nil, 1*time.Second)
	got := e.CheckRFID(context.Background(), "TAG1")

	if got.Decision != DecisionAllow || got.Reason != ReasonLoanValid {
		t.Fatalf("want allow/loan-valid from remote, got %+v", got)
	}
	if n := atomic.LoadInt32(&spy.gets); n != 0 {
		t.Fatalf("cache was consulted (%d Get calls) even though remote was authoritative", n)
	}
	if len(q.events) != 1 {
		t.Fatalf("want 1 queue event, got %d", len(q.events))
	}
}

func TestCheckRFID_RemoteFound_NotLoaned_Alarms(t *testing.T) {
	rem := newFakeRemote()
	rem.byKey["TAG2"] = cache.StatusNotLoaned

	e := NewEngineWithRemote(rem, newSpyCache(), newFakeQueue(), nil, time.Second)
	got := e.CheckRFID(context.Background(), "TAG2")

	if got.Decision != DecisionAlarm || !got.Signal || got.Reason != ReasonNotLoan {
		t.Fatalf("want alarm+signal from remote, got %+v", got)
	}
}

// Test 2: remote unreachable + memory hit -> return cached status.
func TestCheckRFID_RemoteError_FallsBackToCache(t *testing.T) {
	rem := newFakeRemote()
	rem.err = errors.New("connection refused")
	c := newFakeCache()
	c.Set("TAG3", cache.StatusLoaned)

	e := NewEngineWithRemote(rem, c, newFakeQueue(), nil, time.Second)
	got := e.CheckRFID(context.Background(), "TAG3")

	if got.Decision != DecisionAllow || got.Reason != ReasonLoanValid {
		t.Fatalf("want allow from cache fallback, got %+v", got)
	}
}

// Test 4: remote unreachable + cache miss (raw and SN) -> unknown.
func TestCheckRFID_RemoteError_CacheMiss_Unknown(t *testing.T) {
	rem := newFakeRemote()
	rem.err = errors.New("i/o timeout")

	e := NewEngineWithRemote(rem, newFakeCache(), newFakeQueue(), nil, time.Second)
	got := e.CheckRFID(context.Background(), "NOPE")

	if got.Decision != DecisionAllow || got.Reason != ReasonUnknownRFID {
		t.Fatalf("want unknown, got %+v", got)
	}
}

// Test 5: remote reachable + NOT found -> unknown, and the (possibly stale)
// local mirror must NOT be consulted.
func TestCheckRFID_RemoteReachableNotFound_NoCacheFallback(t *testing.T) {
	rem := newFakeRemote() // no err, empty byKey => reachable, not found
	spy := newSpyCache()
	spy.Set("GHOST", cache.StatusLoaned)   // stale mirror claims it's fine
	spy.Set("SNGHOST", cache.StatusLoaned) // ...and under the SN key too

	e := NewEngineWithRemote(rem, spy, newFakeQueue(), nil, time.Second)
	got := e.CheckRFID(context.Background(), "GHOST")

	if got.Decision != DecisionAllow || got.Reason != ReasonUnknownRFID {
		t.Fatalf("want unknown (no stale-cache fallback), got %+v", got)
	}
	if n := atomic.LoadInt32(&spy.gets); n != 0 {
		t.Fatalf("stale cache consulted (%d Get calls) on remote-reachable-not-found", n)
	}
	if n := atomic.LoadInt32(&rem.calls); n != 1 {
		t.Fatalf("want exactly 1 remote call (server normalises SN), got %d", n)
	}
}

// Test 6: remote hangs -> fall back within the remote timeout, not after a
// long retry.
func TestCheckRFID_RemoteTimeout_FallsBackFast(t *testing.T) {
	rem := newFakeRemote()
	rem.delay = 500 * time.Millisecond
	c := newFakeCache()
	c.Set("TAG6", cache.StatusNotLoaned)

	e := NewEngineWithRemote(rem, c, newFakeQueue(), nil, time.Second)
	e.remoteTimeout = 30 * time.Millisecond // white-box: tighten for the test

	start := time.Now()
	got := e.CheckRFID(context.Background(), "TAG6")
	elapsed := time.Since(start)

	if got.Decision != DecisionAlarm || got.Reason != ReasonNotLoan {
		t.Fatalf("want cache-derived alarm, got %+v", got)
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("fallback too slow: %v (remote delay 500ms, remote timeout 30ms)", elapsed)
	}
}

// Test 7: the remote is asked ONCE with the raw tag. SN normalisation is the
// server's job (Koha plugin), so the engine must not send an "SN"+rfid probe.
func TestCheckRFID_RemoteSingleCall_NoSNProbe(t *testing.T) {
	rem := newFakeRemote()
	rem.byKey["TAG7"] = cache.StatusLoaned

	e := NewEngineWithRemote(rem, newSpyCache(), newFakeQueue(), nil, time.Second)
	got := e.CheckRFID(context.Background(), "TAG7")

	if got.Decision != DecisionAllow || got.Reason != ReasonLoanValid {
		t.Fatalf("remote lookup failed: %+v", got)
	}
	if n := atomic.LoadInt32(&rem.calls); n != 1 {
		t.Fatalf("want exactly 1 remote call, got %d", n)
	}
	rem.mu.Lock()
	defer rem.mu.Unlock()
	if len(rem.keys) != 1 || rem.keys[0] != "TAG7" {
		t.Fatalf("remote must receive the raw tag only, got %v", rem.keys)
	}
}

// Unknown tags (reachable remote, tag absent) are allowed and leave no trace:
// nothing published, nothing written to history.
type spyHistory struct {
	mu    sync.Mutex
	count int32
	boom  bool
	done  chan struct{}
}

func (s *spyHistory) Insert(*history.HistoryLog) error {
	atomic.AddInt32(&s.count, 1)
	if s.done != nil {
		defer close(s.done)
	}
	if s.boom {
		panic("history down")
	}
	return nil
}
func (s *spyHistory) GetRecent(int) ([]history.HistoryLog, error) { return nil, nil }

func TestCheckRFID_Unknown_LeavesNoTrace(t *testing.T) {
	rem := newFakeRemote() // reachable, empty => tag not found
	q := newFakeQueue()
	h := &spyHistory{}

	e := NewEngineWithRemote(rem, newSpyCache(), q, h, time.Second)
	got := e.CheckRFID(context.Background(), "WALLET")
	time.Sleep(50 * time.Millisecond) // history insert is async; give it a chance to (wrongly) run

	if got.Decision != DecisionAllow || got.Reason != ReasonUnknownRFID || got.Signal {
		t.Fatalf("want allow/unknown/no-signal, got %+v", got)
	}
	if len(q.events) != 0 {
		t.Fatalf("unknown tag must not publish, got %d events", len(q.events))
	}
	if n := atomic.LoadInt32(&h.count); n != 0 {
		t.Fatalf("unknown tag must not write history, got %d inserts", n)
	}
}

// Known tags still leave a trace (history + queue).
func TestCheckRFID_Known_StillRecorded(t *testing.T) {
	rem := newFakeRemote()
	rem.byKey["BOOK"] = cache.StatusNotLoaned
	q := newFakeQueue()
	h := &spyHistory{done: make(chan struct{})}

	e := NewEngineWithRemote(rem, newSpyCache(), q, h, time.Second)
	got := e.CheckRFID(context.Background(), "BOOK")

	if got.Decision != DecisionAlarm {
		t.Fatalf("want alarm, got %+v", got)
	}
	if len(q.events) != 1 {
		t.Fatalf("known tag must publish 1 event, got %d", len(q.events))
	}
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("history insert did not run for a known tag")
	}
}

// A panic inside the async history insert must not crash the process.
func TestCheckRFID_HistoryPanic_IsRecovered(t *testing.T) {
	rem := newFakeRemote()
	rem.byKey["BOOK"] = cache.StatusLoaned
	h := &spyHistory{boom: true, done: make(chan struct{})}

	e := NewEngineWithRemote(rem, newSpyCache(), newFakeQueue(), h, time.Second)
	e.CheckRFID(context.Background(), "BOOK")

	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("history insert did not run")
	}
	time.Sleep(50 * time.Millisecond) // if recover were missing the test binary would have crashed by now
}

// Remote down: the mirror is keyed by the normalised tag, so a scan with or
// without the "SN" prefix reaches the same record with a single lookup.
func TestCheckRFID_LocalMirror_PrefixedAndPlainScan(t *testing.T) {
	for _, scanned := range []string{"TAG8", "SNTAG8"} {
		rem := newFakeRemote()
		rem.err = errors.New("remote down")
		c := newFakeCache()
		c.Set("TAG8", cache.StatusNotLoaned) // normalised key

		e := NewEngineWithRemote(rem, c, newFakeQueue(), nil, time.Second)
		got := e.CheckRFID(context.Background(), scanned)

		if got.Decision != DecisionAlarm || got.Reason != ReasonNotLoan {
			t.Fatalf("scan %q: mirror lookup failed: %+v", scanned, got)
		}
	}
}

// Test 8: concurrent requests against a flapping remote — no race, one event
// per call. Run the suite with -race.
func TestCheckRFID_RemoteFlaky_Concurrent(t *testing.T) {
	rem := &flakyRemote{}
	c := newRaceSafeCache()
	c.Set("R", cache.StatusLoaned)
	c.Set("SNR", cache.StatusLoaned)
	q := newRaceSafeQueue()

	e := NewEngineWithRemote(rem, c, q, nil, time.Second)

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			e.CheckRFID(context.Background(), "R")
		}()
	}
	wg.Wait()

	if q.Count() != n {
		t.Fatalf("want %d events, got %d", n, q.Count())
	}
}
