package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
)

// touchAuthenticatedSession records that an authenticated session is still in use.
//
// A genuine optimistic concurrency conflict (the session version changed, or the
// session was revoked/expired between read and write) is returned as a conflict
// so the caller can tolerate it: another request simply won the refresh race and
// the session remains valid. Any other failure is an infrastructure error
// (database unavailable, driver fault) and is returned unchanged so the caller
// classifies it as unavailable/internal and rejects access rather than proceeding
// without a confirmed session write. Returning every error as a conflict would
// mask infrastructure failures and let protected requests continue when the
// server cannot confirm the refresh.
func touchAuthenticatedSession(ctx context.Context, repository Repository, session Session, now time.Time) error {
	if err := repository.TouchSession(ctx, session.ID, session.Version, now); err != nil {
		if apperr.IsKind(err, apperr.KindConflict) {
			return apperr.Conflict("session_changed", fmt.Sprintf("session refresh conflicted: %v", err))
		}
		return err
	}
	return nil
}
