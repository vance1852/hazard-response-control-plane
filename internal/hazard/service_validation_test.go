package hazard

import (
	"context"
	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
	"testing"
	"time"
)

type validationRepo struct{}

func (validationRepo) CreateRegion(context.Context, Region) (Region, error) { return Region{}, nil }
func (validationRepo) FindRegion(context.Context, string) (Region, error) {
	return Region{ID: "r", Active: true}, nil
}
func (validationRepo) CreateSensor(context.Context, Sensor) (Sensor, error) { return Sensor{}, nil }
func (validationRepo) FindSensor(context.Context, string) (Sensor, error)   { return Sensor{}, nil }
func (validationRepo) InsertObservation(context.Context, Observation) (Observation, bool, error) {
	return Observation{}, false, nil
}
func (validationRepo) FindObservationBySequence(context.Context, string, string) (Observation, error) {
	return Observation{}, nil
}
func (validationRepo) ActivateIncident(context.Context, ActivationRecord) (Incident, []Zone, error) {
	return Incident{}, nil, nil
}
func (validationRepo) FindIncident(context.Context, string) (Incident, error)    { return Incident{}, nil }
func (validationRepo) ListIncidentZones(context.Context, string) ([]Zone, error) { return nil, nil }
func (validationRepo) TransitionIncident(context.Context, string, int64, IncidentStatus, string, *time.Time, audit.Event) (Incident, error) {
	return Incident{}, nil
}
func (validationRepo) CountOpenDependencies(context.Context, string) (int, int, error) {
	return 0, 0, nil
}
func (validationRepo) ListIncidents(context.Context, IncidentFilter) (IncidentPage, error) {
	return IncidentPage{}, nil
}

type inactiveValidationRepo struct{ validationRepo }

func (inactiveValidationRepo) FindRegion(context.Context, string) (Region, error) {
	return Region{ID: "r", Active: false}, nil
}
func TestRegisterSensorRejectsInactiveRegion(t *testing.T) {
	svc, _ := NewService(inactiveValidationRepo{}, clock.NewManual(time.Now()))
	_, err := svc.RegisterSensor(context.Background(), identity.Actor{UserID: "u", Role: identity.RoleCommander}, Sensor{RegionID: "r", StationCode: "s", Name: "S", Kind: SensorRainfall, Latitude: 1, Longitude: 1})
	if !apperr.IsKind(err, apperr.KindConflict) {
		t.Fatalf("error=%v", err)
	}
}
func TestListRequiresAuthenticatedRole(t *testing.T) {
	svc, _ := NewService(validationRepo{}, clock.NewManual(time.Now()))
	_, err := svc.List(context.Background(), identity.Actor{}, IncidentFilter{})
	if !apperr.IsKind(err, apperr.KindForbidden) {
		t.Fatalf("error=%v", err)
	}
}

func TestRegisterRegionRequiresCommander(t *testing.T) {
	service, _ := NewService(validationRepo{}, clock.NewManual(time.Now()))
	_, err := service.RegisterRegion(context.Background(), identity.Actor{UserID: "operator", Role: identity.RoleFieldOperator}, Region{Code: "CQ", Name: "Central", Timezone: "Asia/Shanghai"})
	if !apperr.IsKind(err, apperr.KindForbidden) {
		t.Fatalf("error = %v", err)
	}
}

func TestActorRegionCannotCrossBoundary(t *testing.T) {
	service, _ := NewService(validationRepo{}, clock.NewManual(time.Now()))
	_, err := service.RegisterSensor(context.Background(), identity.Actor{UserID: "commander", RegionID: "other", Role: identity.RoleCommander}, Sensor{RegionID: "r", StationCode: "S", Name: "Sensor", Kind: SensorRainfall, Latitude: 1, Longitude: 1})
	if !apperr.IsKind(err, apperr.KindForbidden) {
		t.Fatalf("error = %v", err)
	}
}

func TestHazardTypeValidation(t *testing.T) {
	if !Earthquake.Valid() || !Rainstorm.Valid() || !Landslide.Valid() { t.Fatal("supported hazard type rejected") }
	if HazardType("volcano").Valid() { t.Fatal("unsupported hazard type accepted") }
}
