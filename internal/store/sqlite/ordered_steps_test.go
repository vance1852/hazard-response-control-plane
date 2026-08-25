package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/evacuation"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
)

func seedExecutingPlan(t *testing.T, store *Store, name string, now time.Time) (evacuation.Plan, []evacuation.Step) {
	t.Helper()
	ctx := context.Background()
	region, _ := store.CreateRegion(ctx, hazard.Region{Code: "CQ", Name: "Central", Timezone: "Asia/Shanghai", Active: true, CreatedAt: now, UpdatedAt: now})
	incident, _, _ := store.ActivateIncident(ctx, hazard.ActivationRecord{
		Incident: hazard.Incident{RegionID: region.ID, ExternalRef: name, HazardType: hazard.Rainstorm, Title: "Rain", Severity: 3, Status: hazard.IncidentActive, CommandLevel: hazard.CommandLocal, Summary: "rain", OccurredAt: now, ActivatedAt: &now, Version: 1, CreatedBy: "u", CreatedAt: now, UpdatedAt: now},
		Zones:    []hazard.Zone{{RegionID: region.ID, Name: "z", RiskLevel: 3, Population: 100, GeometryJSON: "{}", CreatedAt: now}},
		Audit:    mustAudit(t, now), Topic: "incident.activated", Payload: "{}", Now: now,
	})
	zone, _ := store.FindZone(ctx, firstZone(t, store, incident.ID))
	shelter, _ := store.CreateShelter(ctx, evacuation.Shelter{RegionID: region.ID, Code: name, Name: "Shelter", Capacity: 100, Status: evacuation.ShelterAvailable, Version: 1, CreatedAt: now, UpdatedAt: now})
	plan, steps, err := store.CreatePlan(ctx,
		evacuation.Plan{IncidentID: incident.ID, ZoneID: zone.ID, ShelterID: shelter.ID, Name: name, EvacueeCount: 10, Status: evacuation.PlanSubmitted, DeadlineAt: now.Add(time.Hour), Version: 1, CreatedBy: "u", CreatedAt: now, UpdatedAt: now},
		[]evacuation.Step{
			{Order: 1, Instruction: "one", ResponsibleRole: "commander", ExpectedMinutes: 10, CreatedAt: now},
			{Order: 2, Instruction: "two", ResponsibleRole: "field_operator", ExpectedMinutes: 10, CreatedAt: now},
			{Order: 3, Instruction: "three", ResponsibleRole: "commander", ExpectedMinutes: 10, CreatedAt: now},
		},
		mustAudit(t, now))
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, _, _, err := store.ApprovePlan(ctx, plan.ID, plan.Version, "u", now, mustAudit(t, now)); err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	if _, err := store.StartPlan(ctx, plan.ID, plan.Version+1, now, mustAudit(t, now)); err != nil {
		t.Fatalf("start plan: %v", err)
	}
	return plan, steps
}

// stepAudit builds the same audit event the service emits for a step completion
// so the persisted audit trail is identifiable by action in assertions.
func stepAudit(t *testing.T, now time.Time) audit.Event {
	t.Helper()
	event, err := audit.New("u", "evacuation.step.complete", "evacuation_step", "step", "request", audit.OutcomeSucceeded, map[string]string{}, now)
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func TestCompleteStepRejectsPredecessorIncomplete(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := testClock().Now()
	seedUser(t, store, now, "u")
	plan, steps := seedExecutingPlan(t, store, "reject", now)

	// Completing step 3 while steps 1 and 2 are open must be rejected.
	if _, err := store.CompleteStep(ctx, plan.ID, steps[2].ID, now, stepAudit(t, now)); err == nil {
		t.Fatal("completing later step before predecessors was accepted")
	} else if !apperr.IsKind(err, apperr.KindConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	// Completing step 2 before step 1 must also be rejected.
	if _, err := store.CompleteStep(ctx, plan.ID, steps[1].ID, now, stepAudit(t, now)); err == nil {
		t.Fatal("completing step 2 before step 1 was accepted")
	}

	// Audit must reflect only the rejected attempts so far: no step completion event was written.
	var stepCompletes int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action = 'evacuation.step.complete'`).Scan(&stepCompletes)
	if stepCompletes != 0 {
		t.Fatalf("audit logged %d step completions despite rejection", stepCompletes)
	}
}

func TestCompleteStepAcceptsOrderedCompletion(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := testClock().Now()
	seedUser(t, store, now, "u")
	plan, steps := seedExecutingPlan(t, store, "accept", now)

	// Completing steps in order must succeed and the audit trail must grow per completion.
	for i, step := range steps {
		completed, err := store.CompleteStep(ctx, plan.ID, step.ID, now, stepAudit(t, now))
		if err != nil {
			t.Fatalf("complete step %d: %v", step.Order, err)
		}
		if completed.CompletedAt == nil || completed.CompletedBy != "u" {
			t.Fatalf("step %d not stamped: %#v", step.Order, completed)
		}
		var auditCount int
		_ = store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action = 'evacuation.step.complete'`).Scan(&auditCount)
		if auditCount != i+1 {
			t.Fatalf("after step %d: audit count=%d want %d", step.Order, auditCount, i+1)
		}
	}

	// Re-completing an already-completed step must be rejected (idempotency guard stays intact).
	if _, err := store.CompleteStep(ctx, plan.ID, steps[0].ID, now, stepAudit(t, now)); err == nil {
		t.Fatal("re-completing finished step was accepted")
	}

	// With all steps complete, the plan can transition to completed.
	if _, _, err := store.CompletePlan(ctx, plan.ID, plan.Version+2, now, mustAudit(t, now)); err != nil {
		t.Fatalf("complete plan: %v", err)
	}
}
