package dispatch

import (
	"context"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
)

type Repository interface {
	CreateUnit(context.Context, Unit) (Unit, error)
	FindUnit(context.Context, string) (Unit, error)
	FindIncident(context.Context, string) (hazard.Incident, error)
	FindZone(context.Context, string) (hazard.Zone, error)
	CreateRequest(context.Context, Request, audit.Event) (Request, error)
	FindRequest(context.Context, string) (Request, error)
	ApproveRequest(context.Context, string, int64, string, time.Time, audit.Event) (Request, error)
	AllocateUnit(context.Context, string, int64, string, int64, string, time.Time, audit.Event) (Request, Deployment, Unit, error)
	FindDeployment(context.Context, string) (Deployment, error)
	TransitionDeployment(context.Context, string, int64, DeploymentStatus, string, time.Time, audit.Event) (Deployment, *Unit, error)
	ListRequests(context.Context, RequestFilter) (RequestPage, error)
}
