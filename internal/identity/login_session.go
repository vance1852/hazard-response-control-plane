package identity

import (
	"context"
)

// createLoginSession persists the session and propagates every persistence
// failure. A conflict (for example a token_digest collision under database
// contention) means the row for this session was NOT inserted, so the caller
// must not issue a bearer token against it; aborting lets the client retry
// login and obtain a fresh, durable session.
func createLoginSession(ctx context.Context, repository Repository, session Session) (Session, error) {
	created, err := repository.CreateSession(ctx, session)
	if err != nil {
		return Session{}, err
	}
	return created, nil
}
