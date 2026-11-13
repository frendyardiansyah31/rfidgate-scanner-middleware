package decision

import (
	"context"
	"errors"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
)

// RemoteLookup is the request-time authoritative status source (SLiMS).
//
// The three return shapes are distinct and every implementation MUST honour
// them — error is never a stand-in for "not found":
//
//	found=true,  err=nil  → remote reachable, tag exists
//	found=false, err=nil  → remote reachable, tag does NOT exist
//	found=false, err!=nil  → remote lookup failed / unreachable / timed out
type RemoteLookup interface {
	Status(ctx context.Context, rfid string) (status cache.Status, found bool, err error)
}

// ErrRemoteUnavailable is what the default no-op lookup returns when no remote
// is wired (SLiMS disabled or MySQL not initialised). The engine treats it like
// any other remote error and falls through to the local mirror.
var ErrRemoteUnavailable = errors.New("remote lookup unavailable")

// unreachableRemote is the zero-dependency default so callers that build the
// engine without a remote (all existing tests, standalone mode) behave exactly
// as before: straight to the memory/SQLite mirror.
type unreachableRemote struct{}

func (unreachableRemote) Status(context.Context, string) (cache.Status, bool, error) {
	return cache.StatusUnknown, false, ErrRemoteUnavailable
}
