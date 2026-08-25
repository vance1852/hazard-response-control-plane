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

// batchRepo records each observation with a distinct sensor ID and value so
// the batch results can be checked for ownership of the matching command.
type batchRepo struct{ validationRepo }

func (batchRepo) FindSensor(_ context.Context, id string) (Sensor, error) {
	return Sensor{ID: id, RegionID: "r", Active: true}, nil
}
func (batchRepo) InsertObservation(_ context.Context, observation Observation) (Observation, bool, error) {
	observation.ID = "obs-" + observation.SensorID
	return observation, false, nil
}

func TestIngestObservationBatchKeepsEachObservation(t *testing.T) {
	now := time.Now()
	svc, _ := NewService(batchRepo{}, clock.NewManual(now))
	actor := identity.Actor{UserID: "u", Role: identity.RoleFieldOperator}
	commands := []ObservationCommand{
		{SensorID: "sensor-a", SourceSequence: "a-1", ObservedAt: now.Add(-time.Minute), Metric: "rainfall", Value: 1.5, Unit: "mm", Quality: QualityVerified, Payload: map[string]any{"src": "a"}},
		{SensorID: "sensor-b", SourceSequence: "b-1", ObservedAt: now.Add(-time.Minute), Metric: "rainfall", Value: 9.9, Unit: "mm", Quality: QualityVerified, Payload: map[string]any{"src": "b"}},
	}

	results := svc.IngestObservationBatch(context.Background(), actor, commands)
	if len(results) != len(commands) {
		t.Fatalf("result count = %d, want %d", len(results), len(commands))
	}
	for i, r := range results {
		if r.ErrorCode != "" {
			t.Fatalf("result %d errored: %s", i, r.Message)
		}
		if r.Observation == nil {
			t.Fatalf("result %d missing observation", i)
		}
		if got := r.Observation.SensorID; got != commands[i].SensorID {
			t.Fatalf("result %d sensor = %q, want %q", i, got, commands[i].SensorID)
		}
		if got := r.Observation.Value; got != commands[i].Value {
			t.Fatalf("result %d value = %v, want %v", i, got, commands[i].Value)
		}
	}
	if results[0].Observation.SensorID == results[1].Observation.SensorID {
		t.Fatalf("both results alias the same observation %q", results[0].Observation.SensorID)
	}
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
