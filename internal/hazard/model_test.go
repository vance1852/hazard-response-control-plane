package hazard

import (
	"testing"
	"time"
)

func TestIncidentTransitionsModelLifecycle(t *testing.T) {
	valid := []struct{ from, to IncidentStatus }{{IncidentMonitoring, IncidentActive}, {IncidentActive, IncidentStabilizing}, {IncidentStabilizing, IncidentClosed}, {IncidentActive, IncidentCancelled}}
	for _, item := range valid {
		if !item.from.CanTransition(item.to) {
			t.Fatalf("%s -> %s should be valid", item.from, item.to)
		}
	}
	invalid := []struct{ from, to IncidentStatus }{{IncidentMonitoring, IncidentClosed}, {IncidentClosed, IncidentActive}, {IncidentCancelled, IncidentMonitoring}}
	for _, item := range invalid {
		if item.from.CanTransition(item.to) {
			t.Fatalf("%s -> %s should be invalid", item.from, item.to)
		}
	}
}

func TestIncidentValidationRejectsFutureAndInvalidSeverity(t *testing.T) {
	now := time.Now().UTC()
	incident := Incident{RegionID: "r", ExternalRef: "x", HazardType: Earthquake, Title: "quake", Severity: 6, CommandLevel: CommandJoint, OccurredAt: now}
	if err := incident.Validate(now); err == nil {
		t.Fatal("invalid severity accepted")
	}
	incident.Severity = 3
	incident.OccurredAt = now.Add(10 * time.Minute)
	if err := incident.Validate(now); err == nil {
		t.Fatal("future occurrence accepted")
	}
}

func TestZoneValidationRequiresObjectGeometry(t *testing.T) {
	zone := Zone{Name: "west", RiskLevel: 3, Population: 10, GeometryJSON: "[]"}
	if err := zone.Validate(); err == nil {
		t.Fatal("array geometry accepted")
	}
	zone.GeometryJSON = "{}"
	if err := zone.Validate(); err != nil {
		t.Fatalf("valid geometry rejected: %v", err)
	}
}

func TestObservationValidationAllowsSmallClockSkew(t *testing.T) {
	now := time.Now().UTC()
	observation := Observation{SensorID: "s", SourceSequence: "1", ObservedAt: now.Add(4 * time.Minute), Metric: "mm", Unit: "mm", Quality: QualityProvisional, PayloadJSON: "{}"}
	if err := observation.Validate(now); err != nil {
		t.Fatalf("small clock skew rejected: %v", err)
	}
	observation.ObservedAt = now.Add(6 * time.Minute)
	if err := observation.Validate(now); err == nil {
		t.Fatal("large clock skew accepted")
	}
}
