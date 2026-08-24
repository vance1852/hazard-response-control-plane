package audit

import (
	"context"
	"fmt"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
)

type Service struct{ repository Repository }

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("audit repository is required")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) Search(ctx context.Context, actor identity.Actor, filter Filter) (Page, error) {
	if err := actor.Require(identity.RoleCommander, identity.RoleAuditor); err != nil {
		return Page{}, apperr.Forbidden("role_forbidden", "commander or auditor role is required")
	}
	page, err := s.repository.Search(auditQueryContext(ctx), filter)
	if err != nil {
		return Page{}, apperr.Wrap(err, "search audit events")
	}
	return page, nil
}
