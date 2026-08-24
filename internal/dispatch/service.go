package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
)

type Service struct {
	repository Repository
	clock      clock.Clock
}

func NewService(repository Repository, clk clock.Clock) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("dispatch repository is required")
	}
	if clk == nil {
		return nil, fmt.Errorf("clock is required")
	}
	return &Service{repository: repository, clock: clk}, nil
}

func (s *Service) RegisterUnit(ctx context.Context, actor identity.Actor, unit Unit, capabilities []string) (Unit, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Unit{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	if actor.RegionID != "" && unit.RegionID != actor.RegionID {
		return Unit{}, apperr.Forbidden("cross_region_forbidden", "actor cannot manage another region's unit")
	}
	if len(capabilities) == 0 || len(capabilities) > 30 {
		return Unit{}, apperr.Validation("invalid_capabilities", "unit requires between 1 and 30 capabilities")
	}
	set := make(map[string]struct{}, len(capabilities))
	clean := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.ToLower(strings.TrimSpace(capability))
		if capability == "" {
			return Unit{}, apperr.Validation("invalid_capability", "capability cannot be empty")
		}
		if _, ok := set[capability]; ok {
			continue
		}
		set[capability] = struct{}{}
		clean = append(clean, capability)
	}
	payload, _ := json.Marshal(clean)
	now := s.clock.Now()
	unit.CallSign, unit.CapabilitiesJSON = strings.ToUpper(strings.TrimSpace(unit.CallSign)), string(payload)
	unit.Status, unit.Version, unit.CreatedAt, unit.UpdatedAt = UnitAvailable, 1, now, now
	if err := unit.Validate(); err != nil {
		return Unit{}, apperr.Validation("invalid_unit", err.Error())
	}
	created, err := s.repository.CreateUnit(ctx, unit)
	if err != nil {
		return Unit{}, apperr.Wrap(err, "create field unit")
	}
	return created, nil
}

type CreateRequestCommand struct {
	IncidentID         string    `json:"incident_id"`
	ZoneID             string    `json:"zone_id,omitempty"`
	RequestedType      UnitType  `json:"requested_type"`
	RequiredCapability string    `json:"required_capability"`
	Quantity           int       `json:"quantity"`
	Priority           int       `json:"priority"`
	NeededBy           time.Time `json:"needed_by"`
}

func (s *Service) CreateRequest(ctx context.Context, actor identity.Actor, command CreateRequestCommand) (Request, error) {
	if err := actor.Require(identity.RoleCommander, identity.RoleFieldOperator); err != nil {
		return Request{}, apperr.Forbidden("role_forbidden", "operations role is required")
	}
	incident, err := s.repository.FindIncident(ctx, command.IncidentID)
	if err != nil {
		return Request{}, apperr.Wrap(err, "find request incident")
	}
	if incident.Status != hazard.IncidentActive && incident.Status != hazard.IncidentStabilizing {
		return Request{}, apperr.Conflict("incident_not_operational", "resource request requires an active incident")
	}
	if actor.RegionID != "" && incident.RegionID != actor.RegionID {
		return Request{}, apperr.Forbidden("cross_region_forbidden", "actor cannot request resources for another region")
	}
	if command.ZoneID != "" {
		zone, err := s.repository.FindZone(ctx, command.ZoneID)
		if err != nil {
			return Request{}, apperr.Wrap(err, "find request zone")
		}
		if zone.IncidentID != incident.ID {
			return Request{}, apperr.Validation("zone_incident_mismatch", "zone does not belong to incident")
		}
	}
	now := s.clock.Now()
	request := Request{IncidentID: incident.ID, ZoneID: command.ZoneID, RequestedType: command.RequestedType, RequiredCapability: strings.ToLower(strings.TrimSpace(command.RequiredCapability)), Quantity: command.Quantity, Priority: command.Priority, Status: RequestRequested, NeededBy: command.NeededBy.UTC(), Version: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := request.Validate(now); err != nil {
		return Request{}, apperr.Validation("invalid_request", err.Error())
	}
	event, _ := audit.New(actor.UserID, "resource.request", "resource_request", incident.ID, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"type": request.RequestedType, "quantity": request.Quantity, "priority": request.Priority}, now)
	created, err := s.repository.CreateRequest(ctx, request, event)
	if err != nil {
		return Request{}, apperr.Wrap(err, "create resource request")
	}
	return created, nil
}

func (s *Service) Approve(ctx context.Context, actor identity.Actor, id string, version int64) (Request, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Request{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	request, err := s.repository.FindRequest(ctx, id)
	if err != nil {
		return Request{}, apperr.Wrap(err, "find resource request")
	}
	if request.Status != RequestRequested {
		return Request{}, apperr.Conflict("invalid_transition", "only requested resources can be approved")
	}
	now := s.clock.Now()
	if !request.NeededBy.After(now) {
		return Request{}, apperr.Conflict("need_time_passed", "resource request needed-by time has passed")
	}
	event, _ := audit.New(actor.UserID, "resource.approve", "resource_request", id, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"quantity": request.Quantity}, now)
	approved, err := s.repository.ApproveRequest(ctx, id, version, actor.UserID, now, event)
	if err != nil {
		return Request{}, apperr.Wrap(err, "approve resource request")
	}
	return approved, nil
}

func (s *Service) Allocate(ctx context.Context, actor identity.Actor, requestID string, requestVersion int64, unitID string, unitVersion int64) (Request, Deployment, Unit, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Request{}, Deployment{}, Unit{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	request, err := s.repository.FindRequest(ctx, requestID)
	if err != nil {
		return Request{}, Deployment{}, Unit{}, apperr.Wrap(err, "find resource request")
	}
	if request.Status != RequestApproved && request.Status != RequestPartiallyAllocated {
		return Request{}, Deployment{}, Unit{}, apperr.Conflict("request_not_allocatable", "request is not approved for allocation")
	}
	unit, err := s.repository.FindUnit(ctx, unitID)
	if err != nil {
		return Request{}, Deployment{}, Unit{}, apperr.Wrap(err, "find field unit")
	}
	incident, err := s.repository.FindIncident(ctx, request.IncidentID)
	if err != nil {
		return Request{}, Deployment{}, Unit{}, apperr.Wrap(err, "find request incident")
	}
	if unit.RegionID != incident.RegionID {
		return Request{}, Deployment{}, Unit{}, apperr.Conflict("unit_region_mismatch", "unit is outside the incident region")
	}
	if unit.Type != request.RequestedType {
		return Request{}, Deployment{}, Unit{}, apperr.Conflict("unit_type_mismatch", "unit type does not match request")
	}
	var capabilities []string
	if err := json.Unmarshal([]byte(unit.CapabilitiesJSON), &capabilities); err != nil {
		return Request{}, Deployment{}, Unit{}, apperr.Wrap(err, "decode unit capabilities")
	}
	qualified := false
	for _, value := range capabilities {
		if value == request.RequiredCapability {
			qualified = true
			break
		}
	}
	if !qualified {
		return Request{}, Deployment{}, Unit{}, apperr.Conflict("unit_capability_missing", "unit lacks the required capability")
	}
	now := s.clock.Now()
	if !request.NeededBy.After(now) {
		return Request{}, Deployment{}, Unit{}, apperr.Conflict("need_time_passed", "resource request needed-by time has passed")
	}
	event, _ := audit.New(actor.UserID, "resource.allocate", "resource_request", requestID, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"unit_id": unitID}, now)
	updated, deployment, assigned, err := s.repository.AllocateUnit(allocationContext(ctx), requestID, requestVersion, unitID, unitVersion, actor.UserID, now, event)
	if err != nil {
		return Request{}, Deployment{}, Unit{}, apperr.Wrap(err, "allocate field unit")
	}
	return updated, deployment, assigned, nil
}

func (s *Service) TransitionDeployment(ctx context.Context, actor identity.Actor, deploymentID string, version int64, target DeploymentStatus) (Deployment, *Unit, error) {
	deployment, err := s.repository.FindDeployment(ctx, deploymentID)
	if err != nil {
		return Deployment{}, nil, apperr.Wrap(err, "find deployment")
	}
	unit, err := s.repository.FindUnit(ctx, deployment.UnitID)
	if err != nil {
		return Deployment{}, nil, apperr.Wrap(err, "find deployment unit")
	}
	if actor.Role == identity.RoleFieldOperator && unit.OperatorID != actor.UserID {
		return Deployment{}, nil, apperr.Forbidden("deployment_not_owned", "deployment belongs to another operator")
	}
	if err := actor.Require(identity.RoleCommander, identity.RoleFieldOperator); err != nil {
		return Deployment{}, nil, apperr.Forbidden("role_forbidden", "operations role is required")
	}
	if !deployment.Status.CanTransition(target) {
		return Deployment{}, nil, apperr.Conflict("invalid_transition", fmt.Sprintf("deployment cannot transition from %s to %s", deployment.Status, target))
	}
	if (target == DeploymentCancelled || target == DeploymentReleased) && actor.Role != identity.RoleCommander {
		return Deployment{}, nil, apperr.Forbidden("commander_required", "commander role is required to release a deployment")
	}
	now := s.clock.Now()
	event, _ := audit.New(actor.UserID, "deployment.transition", "deployment", deploymentID, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"from": deployment.Status, "to": target}, now)
	updated, released, err := s.repository.TransitionDeployment(ctx, deploymentID, version, target, actor.UserID, now, event)
	if err != nil {
		return Deployment{}, nil, apperr.Wrap(err, "transition deployment")
	}
	return updated, released, nil
}

func (s *Service) ListRequests(ctx context.Context, actor identity.Actor, filter RequestFilter) (RequestPage, error) {
	if err := actor.Require(identity.RoleCommander, identity.RoleFieldOperator, identity.RoleAuditor); err != nil {
		return RequestPage{}, apperr.Forbidden("role_forbidden", "authenticated operations role is required")
	}
	if filter.Limit == 0 {
		filter.Limit = 25
	}
	if filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 {
		return RequestPage{}, apperr.Validation("invalid_pagination", "invalid request pagination")
	}
	page, err := s.repository.ListRequests(ctx, filter)
	if err != nil {
		return RequestPage{}, apperr.Wrap(err, "list resource requests")
	}
	return page, nil
}
