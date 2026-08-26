package audit

import "github.com/vance1852/hazard-response-control-plane/internal/identity"

// auditSearchFilter applies role-scoping rules to a caller-supplied audit
// search filter before it reaches the repository.
//
// Auditors are trusted to query any actor's evidence, so an explicitly
// provided ActorID must be preserved and propagated to the repository. An
// auditor screening dispatch operations by a specific field operator relies
// on that filter reaching the storage layer; clearing it would silently
// return every actor's records and corrupt accountability timelines.
// Non-auditor callers (commanders) can only see their own activity, so their
// ActorID is constrained to themselves regardless of the requested value.
func auditSearchFilter(actor identity.Actor, filter Filter) Filter {
	switch actor.Role {
	case identity.RoleAuditor:
		// Auditors may filter by any actor; keep the explicit ActorID.
	case identity.RoleCommander:
		filter.ActorID = actor.UserID
	default:
		filter.ActorID = actor.UserID
	}
	return filter
}
