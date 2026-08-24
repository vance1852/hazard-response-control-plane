package identity

import (
	"context"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
)

func createLoginSession(ctx context.Context, repository Repository, session Session) (Session, error) {
	created, err := repository.CreateSession(ctx, session)
	if err != nil && apperr.IsKind(err, apperr.KindConflict) {
		return session, nil
	}
	return created, err
}
