package identity

import (
 "context"
 "fmt"
 "time"

 "github.com/vance1852/hazard-response-control-plane/internal/apperr"
)

func touchAuthenticatedSession(ctx context.Context, repository Repository, session Session, now time.Time) error {
 if err := repository.TouchSession(ctx, session.ID, session.Version, now); err != nil {
  return apperr.Conflict("session_changed", fmt.Sprintf("session refresh conflicted: %v", err))
 }
 return nil
}
