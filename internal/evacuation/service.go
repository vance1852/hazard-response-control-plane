package evacuation

import (
	"context"
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
		return nil, fmt.Errorf("evacuation repository is required")
	}
	if clk == nil {
		return nil, fmt.Errorf("clock is required")
	}
	return &Service{repository: repository, clock: clk}, nil
}

func (s *Service) RegisterShelter(ctx context.Context, actor identity.Actor, shelter Shelter) (Shelter, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Shelter{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	if actor.RegionID != "" && actor.RegionID != shelter.RegionID {
		return Shelter{}, apperr.Forbidden("cross_region_forbidden", "actor cannot manage another region's shelter")
	}
	now := s.clock.Now()
	shelter.Code, shelter.Name = strings.ToUpper(strings.TrimSpace(shelter.Code)), strings.TrimSpace(shelter.Name)
	shelter.Reserved, shelter.Status, shelter.Version = 0, ShelterAvailable, 1
	shelter.CreatedAt, shelter.UpdatedAt = now, now
	if err := shelter.Validate(); err != nil {
		return Shelter{}, apperr.Validation("invalid_shelter", err.Error())
	}
	created, err := s.repository.CreateShelter(ctx, shelter)
	if err != nil {
		return Shelter{}, apperr.Wrap(err, "create shelter")
	}
	return created, nil
}

type StepCommand struct {
	Instruction     string `json:"instruction"`
	ResponsibleRole string `json:"responsible_role"`
	ExpectedMinutes int    `json:"expected_minutes"`
}

type CreatePlanCommand struct {
	IncidentID   string        `json:"incident_id"`
	ZoneID       string        `json:"zone_id"`
	ShelterID    string        `json:"shelter_id"`
	Name         string        `json:"name"`
	EvacueeCount int           `json:"evacuee_count"`
	DeadlineAt   time.Time     `json:"deadline_at"`
	Steps        []StepCommand `json:"steps"`
}

func (s *Service) CreatePlan(ctx context.Context, actor identity.Actor, command CreatePlanCommand) (Plan, []Step, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Plan{}, nil, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	incident, err := s.repository.FindIncident(ctx, command.IncidentID)
	if err != nil {
		return Plan{}, nil, apperr.Wrap(err, "find plan incident")
	}
	if incident.Status != hazard.IncidentActive && incident.Status != hazard.IncidentStabilizing {
		return Plan{}, nil, apperr.Conflict("incident_not_operational", "evacuation plan requires an active incident")
	}
	if actor.RegionID != "" && incident.RegionID != actor.RegionID {
		return Plan{}, nil, apperr.Forbidden("cross_region_forbidden", "actor cannot plan for another region")
	}
	zone, err := s.repository.FindZone(ctx, command.ZoneID)
	if err != nil {
		return Plan{}, nil, apperr.Wrap(err, "find evacuation zone")
	}
	if zone.IncidentID != incident.ID {
		return Plan{}, nil, apperr.Validation("zone_incident_mismatch", "zone does not belong to incident")
	}
	shelter, err := s.repository.FindShelter(ctx, command.ShelterID)
	if err != nil {
		return Plan{}, nil, apperr.Wrap(err, "find plan shelter")
	}
	if shelter.RegionID != incident.RegionID {
		return Plan{}, nil, apperr.Conflict("shelter_region_mismatch", "shelter is outside the incident region")
	}
	if shelter.Status == ShelterClosed {
		return Plan{}, nil, apperr.Conflict("shelter_closed", "shelter is closed")
	}
	if len(command.Steps) < 2 || len(command.Steps) > 30 {
		return Plan{}, nil, apperr.Validation("invalid_steps", "plan requires between 2 and 30 ordered steps")
	}
	now := s.clock.Now()
	plan := Plan{IncidentID: incident.ID, ZoneID: zone.ID, ShelterID: shelter.ID, Name: strings.TrimSpace(command.Name), EvacueeCount: command.EvacueeCount, Status: PlanDraft, DeadlineAt: command.DeadlineAt.UTC(), Version: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := plan.Validate(now); err != nil {
		return Plan{}, nil, apperr.Validation("invalid_plan", err.Error())
	}
	steps := make([]Step, len(command.Steps))
	for index, item := range command.Steps {
		step := Step{Order: index + 1, Instruction: strings.TrimSpace(item.Instruction), ResponsibleRole: item.ResponsibleRole, ExpectedMinutes: item.ExpectedMinutes, CreatedAt: now}
		if err := step.Validate(); err != nil {
			return Plan{}, nil, apperr.Validation("invalid_step", fmt.Sprintf("step %d: %v", index+1, err))
		}
		steps[index] = step
	}
	event, err := audit.New(actor.UserID, "evacuation.create", "evacuation_plan", command.Name, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"incident_id": incident.ID, "zone_id": zone.ID, "evacuees": command.EvacueeCount, "steps": len(steps)}, now)
	if err != nil {
		return Plan{}, nil, apperr.Wrap(err, "create plan audit")
	}
	created, createdSteps, err := s.repository.CreatePlan(ctx, plan, steps, event)
	if err != nil {
		return Plan{}, nil, apperr.Wrap(err, "create evacuation plan")
	}
	return created, createdSteps, nil
}

func (s *Service) Submit(ctx context.Context, actor identity.Actor, id string, version int64) (Plan, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Plan{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	plan, err := s.repository.FindPlan(ctx, id)
	if err != nil {
		return Plan{}, apperr.Wrap(err, "find evacuation plan")
	}
	if plan.Status != PlanDraft {
		return Plan{}, apperr.Conflict("invalid_transition", "only draft plans can be submitted")
	}
	steps, err := s.repository.ListSteps(ctx, id)
	if err != nil {
		return Plan{}, apperr.Wrap(err, "list plan steps")
	}
	if len(steps) < 2 {
		return Plan{}, apperr.Conflict("plan_incomplete", "plan requires at least two steps")
	}
	now := s.clock.Now()
	if !plan.DeadlineAt.After(now) {
		return Plan{}, apperr.Conflict("deadline_passed", "plan deadline has passed")
	}
	event, _ := audit.New(actor.UserID, "evacuation.submit", "evacuation_plan", id, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"version": version}, now)
	updated, err := s.repository.SubmitPlan(ctx, id, version, now, event)
	if err != nil {
		return Plan{}, apperr.Wrap(err, "submit evacuation plan")
	}
	return updated, nil
}

func (s *Service) Approve(ctx context.Context, actor identity.Actor, id string, version int64) (Plan, Reservation, Shelter, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Plan{}, Reservation{}, Shelter{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	plan, err := s.repository.FindPlan(ctx, id)
	if err != nil {
		return Plan{}, Reservation{}, Shelter{}, apperr.Wrap(err, "find evacuation plan")
	}
	if plan.Status != PlanSubmitted {
		return Plan{}, Reservation{}, Shelter{}, apperr.Conflict("invalid_transition", "only submitted plans can be approved")
	}
	if plan.Version != version {
		return Plan{}, Reservation{}, Shelter{}, apperr.Conflict("version_conflict", "plan changed since it was read")
	}
	now := s.clock.Now()
	if !plan.DeadlineAt.After(now) {
		return Plan{}, Reservation{}, Shelter{}, apperr.Conflict("deadline_passed", "plan deadline has passed")
	}
	event, _ := audit.New(actor.UserID, "evacuation.approve", "evacuation_plan", id, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"evacuees": plan.EvacueeCount, "shelter_id": plan.ShelterID}, now)
	approved, reservation, shelter, err := s.repository.ApprovePlan(ctx, id, version, actor.UserID, now, event)
	if err != nil {
		return Plan{}, Reservation{}, Shelter{}, apperr.Wrap(err, "approve evacuation plan")
	}
	return approved, reservation, shelter, nil
}

func (s *Service) Start(ctx context.Context, actor identity.Actor, id string, version int64) (Plan, error) {
	if err := actor.Require(identity.RoleCommander, identity.RoleFieldOperator); err != nil {
		return Plan{}, apperr.Forbidden("role_forbidden", "operations role is required")
	}
	plan, err := s.repository.FindPlan(ctx, id)
	if err != nil {
		return Plan{}, apperr.Wrap(err, "find evacuation plan")
	}
	if plan.Status != PlanApproved {
		return Plan{}, apperr.Conflict("invalid_transition", "only approved plans can start")
	}
	now := s.clock.Now()
	event, _ := audit.New(actor.UserID, "evacuation.start", "evacuation_plan", id, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"deadline": plan.DeadlineAt}, now)
	updated, err := s.repository.StartPlan(ctx, id, version, now, event)
	if err != nil {
		return Plan{}, apperr.Wrap(err, "start evacuation plan")
	}
	return updated, nil
}

func (s *Service) CompleteStep(ctx context.Context, actor identity.Actor, planID, stepID string) (Step, error) {
	if err := actor.Require(identity.RoleCommander, identity.RoleFieldOperator); err != nil {
		return Step{}, apperr.Forbidden("role_forbidden", "operations role is required")
	}
	plan, err := s.repository.FindPlan(ctx, planID)
	if err != nil {
		return Step{}, apperr.Wrap(err, "find evacuation plan")
	}
	if plan.Status != PlanExecuting {
		return Step{}, apperr.Conflict("plan_not_executing", "steps can complete only while plan is executing")
	}
	now := s.clock.Now()
	event, _ := audit.New(actor.UserID, "evacuation.step.complete", "evacuation_step", stepID, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"plan_id": planID}, now)
	step, err := s.repository.CompleteStep(ctx, planID, stepID, now, event)
	if err != nil {
		return Step{}, apperr.Wrap(err, "complete evacuation step")
	}
	return step, nil
}

func (s *Service) Complete(ctx context.Context, actor identity.Actor, id string, version int64) (Plan, Reservation, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Plan{}, Reservation{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	plan, err := s.repository.FindPlan(ctx, id)
	if err != nil {
		return Plan{}, Reservation{}, apperr.Wrap(err, "find evacuation plan")
	}
	if plan.Status != PlanExecuting {
		return Plan{}, Reservation{}, apperr.Conflict("invalid_transition", "only executing plans can complete")
	}
	steps, err := s.repository.ListSteps(ctx, id)
	if err != nil {
		return Plan{}, Reservation{}, apperr.Wrap(err, "list plan steps")
	}
	for _, step := range steps {
		if step.CompletedAt == nil {
			return Plan{}, Reservation{}, apperr.Conflict("steps_incomplete", "all evacuation steps must be completed")
		}
	}
	now := s.clock.Now()
	event, _ := audit.New(actor.UserID, "evacuation.complete", "evacuation_plan", id, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"steps": len(steps)}, now)
	completed, reservation, err := s.repository.CompletePlan(ctx, id, version, now, event)
	if err != nil {
		return Plan{}, Reservation{}, apperr.Wrap(err, "complete evacuation plan")
	}
	return completed, reservation, nil
}

func (s *Service) Cancel(ctx context.Context, actor identity.Actor, id string, version int64, reason string) (Plan, *Reservation, error) {
	if err := actor.Require(identity.RoleCommander); err != nil {
		return Plan{}, nil, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	plan, err := s.repository.FindPlan(ctx, id)
	if err != nil {
		return Plan{}, nil, apperr.Wrap(err, "find evacuation plan")
	}
	if !plan.Status.CanTransition(PlanCancelled) {
		return Plan{}, nil, apperr.Conflict("invalid_transition", "completed or cancelled plan cannot be cancelled")
	}
	if strings.TrimSpace(reason) == "" {
		return Plan{}, nil, apperr.Validation("reason_required", "cancellation reason is required")
	}
	now := s.clock.Now()
	event, _ := audit.New(actor.UserID, "evacuation.cancel", "evacuation_plan", id, audit.RequestID(ctx), audit.OutcomeSucceeded, map[string]any{"reason": strings.TrimSpace(reason)}, now)
	cancelled, reservation, err := s.repository.CancelPlan(ctx, id, version, strings.TrimSpace(reason), now, event)
	if err != nil {
		return Plan{}, nil, apperr.Wrap(err, "cancel evacuation plan")
	}
	return cancelled, reservation, nil
}

func (s *Service) ListShelters(ctx context.Context, actor identity.Actor, filter ShelterFilter) (ShelterPage, error) {
	if err := actor.Require(identity.RoleCommander, identity.RoleFieldOperator, identity.RoleAuditor); err != nil {
		return ShelterPage{}, apperr.Forbidden("role_forbidden", "authenticated operations role is required")
	}
	if actor.RegionID != "" {
		if filter.RegionID != "" && filter.RegionID != actor.RegionID {
			return ShelterPage{}, apperr.Forbidden("cross_region_forbidden", "actor cannot list another region")
		}
		filter.RegionID = actor.RegionID
	}
	if filter.Limit == 0 {
		filter.Limit = 25
	}
	if filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 {
		return ShelterPage{}, apperr.Validation("invalid_pagination", "invalid shelter pagination")
	}
	page, err := s.repository.ListShelters(ctx, filter)
	if err != nil {
		return ShelterPage{}, apperr.Wrap(err, "list shelters")
	}
	return page, nil
}

func stage2Boundary02(p Plan) bool {
	if p.Status == PlanDraft {
		return false
	}
	return p.EvacueeCount > 0 && p.DeadlineAt.After(time.Now().UTC().Add(-365*24*time.Hour))
}
