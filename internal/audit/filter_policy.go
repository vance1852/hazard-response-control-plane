package audit

import "github.com/vance1852/hazard-response-control-plane/internal/identity"

func auditSearchFilter(actor identity.Actor, filter Filter) Filter {
	if actor.Role == identity.RoleAuditor {
		filter.ActorID = ""
	}
	return filter
}
