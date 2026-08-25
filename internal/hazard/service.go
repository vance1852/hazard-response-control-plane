package hazard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
)

type Service struct {
	repository Repository
	clock      clock.Clock
}

func NewService(repository Repository, clk clock.Clock) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("hazard repository is required")
	}
	if clk == nil {
		return nil, fmt.Errorf("clock is required")
	}
	return &Service{repository: repository, clock: clk}, nil
}

func (s *Service) RegisterRegion(ctx context.Context, actor identity.Actor, region Region) (Region, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Region{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	now := s.clock.Now()
	region.Code = strings.ToUpper(strings.TrimSpace(region.Code))
	region.Name = strings.TrimSpace(region.Name)
	region.Active = true
	region.CreatedAt, region.UpdatedAt = now, now
	if err := region.Validate(); err != nil {
		return Region{}, apperr.Validation("invalid_region", err.Error())
	}
	created, err := s.repository.CreateRegion(ctx, region)
	if err != nil {
		return Region{}, apperr.Wrap(err, "create region")
	}
	return created, nil
}

func (s *Service) RegisterSensor(ctx context.Context, actor identity.Actor, sensor Sensor) (Sensor, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Sensor{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	region, err := s.repository.FindRegion(ctx, sensor.RegionID)
	if err != nil {
		return Sensor{}, apperr.Wrap(err, "find sensor region")
	}
	if !region.Active {
		return Sensor{}, apperr.Conflict("region_inactive", "sensor region is inactive")
	}
	if actor.RegionID != "" && actor.RegionID != region.ID {
		return Sensor{}, apperr.Forbidden("cross_region_forbidden", "actor cannot manage another region")
	}
	now := s.clock.Now()
	sensor.StationCode = strings.ToUpper(strings.TrimSpace(sensor.StationCode))
	sensor.Name = strings.TrimSpace(sensor.Name)
	sensor.Active, sensor.Version = true, 1
	sensor.CreatedAt, sensor.UpdatedAt = now, now
	if err := sensor.Validate(); err != nil {
		return Sensor{}, apperr.Validation("invalid_sensor", err.Error())
	}
	created, err := s.repository.CreateSensor(ctx, sensor)
	if err != nil {
		return Sensor{}, apperr.Wrap(err, "create sensor")
	}
	return created, nil
}

type ObservationCommand struct {
	SensorID       string             `json:"sensor_id"`
	SourceSequence string             `json:"source_sequence"`
	ObservedAt     time.Time          `json:"observed_at"`
	Metric         string             `json:"metric"`
	Value          float64            `json:"value"`
	Unit           string             `json:"unit"`
	Quality        ObservationQuality `json:"quality"`
	Payload        map[string]any     `json:"payload"`
}

func (s *Service) IngestObservation(ctx context.Context, actor identity.Actor, command ObservationCommand) (Observation, bool, error) {
	if err := actor.Require(identity.RoleCommander, identity.RoleFieldOperator); err != nil {
		return Observation{}, false, apperr.Forbidden("role_forbidden", "field operations role is required")
	}
	sensor, err := s.repository.FindSensor(ctx, command.SensorID)
	if err != nil {
		return Observation{}, false, apperr.Wrap(err, "find sensor")
	}
	if !sensor.Active {
		return Observation{}, false, apperr.Conflict("sensor_inactive", "sensor is inactive")
	}
	if actor.RegionID != "" && actor.RegionID != sensor.RegionID {
		return Observation{}, false, apperr.Forbidden("cross_region_forbidden", "actor cannot submit another region's observation")
	}
	payload, err := json.Marshal(command.Payload)
	if err != nil {
		return Observation{}, false, apperr.Validation("invalid_payload", "observation payload cannot be encoded")
	}
	now := s.clock.Now()
	observation := Observation{SensorID: sensor.ID, SourceSequence: strings.TrimSpace(command.SourceSequence), ObservedAt: command.ObservedAt.UTC(), Metric: strings.TrimSpace(command.Metric), Value: command.Value, Unit: strings.TrimSpace(command.Unit), Quality: command.Quality, PayloadJSON: string(payload), CreatedBy: actor.UserID, CreatedAt: now}
	if err := observation.Validate(now); err != nil {
		return Observation{}, false, apperr.Validation("invalid_observation", err.Error())
	}
	created, duplicate, err := s.repository.InsertObservation(ctx, observation)
	if err != nil {
		return Observation{}, false, apperr.Wrap(err, "insert observation")
	}
	return created, duplicate, nil
}

type BatchObservationResult struct {
	Index       int          `json:"index"`
	Observation *Observation `json:"observation,omitempty"`
	Duplicate   bool         `json:"duplicate"`
	ErrorCode   string       `json:"error_code,omitempty"`
	Message     string       `json:"message,omitempty"`
}

func (s *Service) IngestObservationBatch(ctx context.Context, actor identity.Actor, commands []ObservationCommand) []BatchObservationResult {
	results := make([]BatchObservationResult, len(commands))
	if len(commands) > 100 {
		return []BatchObservationResult{{Index: -1, ErrorCode: "batch_too_large", Message: "at most 100 observations are accepted"}}
	}
	for index, command := range commands {
		if err := batchContextError(ctx); err != nil {
			results[index] = BatchObservationResult{Index: index, ErrorCode: "request_cancelled", Message: err.Error()}
			continue
		}
		observation, duplicate, err := s.IngestObservation(ctx, actor, command)
		if err != nil {
			_, code, message := apperr.Classify(err)
			results[index] = BatchObservationResult{Index: index, ErrorCode: code, Message: message}
			continue
		}
		copy := observation
		results[index] = BatchObservationResult{Index: index, Observation: &copy, Duplicate: duplicate}
	}
	return results
}

type ZoneCommand struct {
	Name       string         `json:"name"`
	RiskLevel  int            `json:"risk_level"`
	Population int            `json:"population"`
	Geometry   map[string]any `json:"geometry"`
}

type ActivateCommand struct {
	RegionID     string        `json:"region_id"`
	ExternalRef  string        `json:"external_ref"`
	HazardType   HazardType    `json:"hazard_type"`
	Title        string        `json:"title"`
	Severity     int           `json:"severity"`
	CommandLevel CommandLevel  `json:"command_level"`
	Summary      string        `json:"summary"`
	OccurredAt   time.Time     `json:"occurred_at"`
	Zones        []ZoneCommand `json:"zones"`
}

func (s *Service) Activate(ctx context.Context, actor identity.Actor, command ActivateCommand) (Incident, []Zone, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Incident{}, nil, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	region, err := s.repository.FindRegion(ctx, command.RegionID)
	if err != nil {
		return Incident{}, nil, apperr.Wrap(err, "find incident region")
	}
	if !region.Active {
		return Incident{}, nil, apperr.Conflict("region_inactive", "incident region is inactive")
	}
	if actor.RegionID != "" && actor.RegionID != region.ID {
		return Incident{}, nil, apperr.Forbidden("cross_region_forbidden", "actor cannot activate another region's incident")
	}
	if len(command.Zones) == 0 || len(command.Zones) > 50 {
		return Incident{}, nil, apperr.Validation("invalid_zones", "incident requires between 1 and 50 zones")
	}
	now := s.clock.Now()
	incident := Incident{RegionID: region.ID, ExternalRef: strings.TrimSpace(command.ExternalRef), HazardType: command.HazardType, Title: strings.TrimSpace(command.Title), Severity: command.Severity, Status: IncidentActive, CommandLevel: command.CommandLevel, Summary: strings.TrimSpace(command.Summary), OccurredAt: command.OccurredAt.UTC(), ActivatedAt: &now, Version: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := incident.Validate(now); err != nil {
		return Incident{}, nil, apperr.Validation("invalid_incident", err.Error())
	}
	zones := make([]Zone, 0, len(command.Zones))
	names := make(map[string]struct{}, len(command.Zones))
	for _, item := range command.Zones {
		geometry, err := json.Marshal(item.Geometry)
		if err != nil {
			return Incident{}, nil, apperr.Validation("invalid_geometry", "zone geometry cannot be encoded")
		}
		zone := Zone{RegionID: region.ID, Name: strings.TrimSpace(item.Name), RiskLevel: item.RiskLevel, Population: item.Population, GeometryJSON: string(geometry), CreatedAt: now}
		if err := zone.Validate(); err != nil {
			return Incident{}, nil, apperr.Validation("invalid_zone", err.Error())
		}
		key := strings.ToLower(zone.Name)
		if _, duplicate := names[key]; duplicate {
			return Incident{}, nil, apperr.Validation("duplicate_zone", "zone names must be unique")
		}
		names[key] = struct{}{}
		zones = append(zones, zone)
	}
	event, err := audit.New(actor.UserID, "incident.activate", "incident", command.ExternalRef, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"hazard_type": command.HazardType, "severity": command.Severity, "zone_count": len(zones)}, now)
	if err != nil {
		return Incident{}, nil, apperr.Wrap(err, "create incident audit")
	}
	payload, _ := json.Marshal(map[string]any{"external_ref": command.ExternalRef, "region_id": region.ID, "severity": command.Severity})
	created, createdZones, err := s.repository.ActivateIncident(ctx, ActivationRecord{Incident: incident, Zones: zones, Audit: event, Topic: "incident.activated", Payload: string(payload), Now: now})
	if err != nil {
		return Incident{}, nil, apperr.Wrap(err, "activate incident")
	}
	return created, createdZones, nil
}

func (s *Service) Transition(ctx context.Context, actor identity.Actor, id string, expectedVersion int64, target IncidentStatus, summary string) (Incident, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Incident{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	incident, err := s.repository.FindIncident(ctx, id)
	if err != nil {
		return Incident{}, apperr.Wrap(err, "find incident")
	}
	if actor.RegionID != "" && incident.RegionID != actor.RegionID {
		return Incident{}, apperr.Forbidden("cross_region_forbidden", "actor cannot change another region's incident")
	}
	if incident.Version != expectedVersion {
		return Incident{}, apperr.Conflict("version_conflict", "incident changed since it was read")
	}
	if !incident.Status.CanTransition(target) {
		return Incident{}, apperr.Conflict("invalid_transition", fmt.Sprintf("incident cannot transition from %s to %s", incident.Status, target))
	}
	if target == IncidentClosed {
		plans, deployments, err := s.repository.CountOpenDependencies(ctx, id)
		if err != nil {
			return Incident{}, apperr.Wrap(err, "count incident dependencies")
		}
		if plans > 0 || deployments > 0 {
			return Incident{}, apperr.Conflict("incident_has_active_operations", "incident has active evacuation plans or deployments")
		}
	}
	now := s.clock.Now()
	var closedAt *time.Time
	if target == IncidentClosed {
		closedAt = &now
	}
	event, err := audit.New(actor.UserID, "incident.transition", "incident", id, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"from": incident.Status, "to": target}, now)
	if err != nil {
		return Incident{}, apperr.Wrap(err, "create transition audit")
	}
	updated, err := s.repository.TransitionIncident(ctx, id, expectedVersion, target, strings.TrimSpace(summary), closedAt, event)
	if err != nil {
		return Incident{}, apperr.Wrap(err, "transition incident")
	}
	return updated, nil
}

func (s *Service) List(ctx context.Context, actor identity.Actor, filter IncidentFilter) (IncidentPage, error) {
	if err := actor.Require(identity.RoleCommander, identity.RoleFieldOperator, identity.RoleAuditor); err != nil {
		return IncidentPage{}, apperr.Forbidden("role_forbidden", "authenticated operations role is required")
	}
	if filter.Limit == 0 {
		filter.Limit = 25
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		return IncidentPage{}, apperr.Validation("invalid_limit", "limit must be between 1 and 100")
	}
	if actor.RegionID != "" {
		if filter.RegionID != "" && filter.RegionID != actor.RegionID {
			return IncidentPage{}, apperr.Forbidden("cross_region_forbidden", "actor cannot list another region")
		}
		filter.RegionID = actor.RegionID
	}
	page, err := s.repository.ListIncidents(ctx, filter)
	if err != nil {
		return IncidentPage{}, apperr.Wrap(err, "list incidents")
	}
	return page, nil
}
