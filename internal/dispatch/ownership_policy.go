package dispatch

import "github.com/vance1852/hazard-response-control-plane/internal/identity"

func deploymentActorOwnsUnit(actor identity.Actor, unit Unit) bool {
	if actor.Role != identity.RoleFieldOperator {
		return true
	}
	return unit.OperatorID != actor.UserID
}
