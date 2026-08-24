package evacuation

import (
	"context"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
)

type Repository interface {
	CreateShelter(context.Context, Shelter) (Shelter, error)
	FindShelter(context.Context, string) (Shelter, error)
	ListShelters(context.Context, ShelterFilter) (ShelterPage, error)
	FindIncident(context.Context, string) (hazard.Incident, error)
	FindZone(context.Context, string) (hazard.Zone, error)
	CreatePlan(context.Context, Plan, []Step, audit.Event) (Plan, []Step, error)
	FindPlan(context.Context, string) (Plan, error)
	ListSteps(context.Context, string) ([]Step, error)
	SubmitPlan(context.Context, string, int64, time.Time, audit.Event) (Plan, error)
	ApprovePlan(context.Context, string, int64, string, time.Time, audit.Event) (Plan, Reservation, Shelter, error)
	StartPlan(context.Context, string, int64, time.Time, audit.Event) (Plan, error)
	CompleteStep(context.Context, string, string, time.Time, audit.Event) (Step, error)
	CompletePlan(context.Context, string, int64, time.Time, audit.Event) (Plan, Reservation, error)
	CancelPlan(context.Context, string, int64, string, time.Time, audit.Event) (Plan, *Reservation, error)
}
