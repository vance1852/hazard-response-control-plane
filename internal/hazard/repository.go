package hazard

import (
	"context"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/audit"
)

type ActivationRecord struct {
	Incident Incident
	Zones    []Zone
	Audit    audit.Event
	Topic    string
	Payload  string
	Now      time.Time
}

type Repository interface {
	CreateRegion(context.Context, Region) (Region, error)
	FindRegion(context.Context, string) (Region, error)
	CreateSensor(context.Context, Sensor) (Sensor, error)
	FindSensor(context.Context, string) (Sensor, error)
	InsertObservation(context.Context, Observation) (Observation, bool, error)
	FindObservationBySequence(context.Context, string, string) (Observation, error)
	ActivateIncident(context.Context, ActivationRecord) (Incident, []Zone, error)
	FindIncident(context.Context, string) (Incident, error)
	ListIncidentZones(context.Context, string) ([]Zone, error)
	TransitionIncident(context.Context, string, int64, IncidentStatus, string, *time.Time, audit.Event) (Incident, error)
	CountOpenDependencies(context.Context, string) (plans int, deployments int, err error)
	ListIncidents(context.Context, IncidentFilter) (IncidentPage, error)
}
