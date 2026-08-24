package evacuation

import (
	"testing"
	"time"
)

func TestPlanTransitionsRequireOrderedOperations(t *testing.T) {
	if !PlanDraft.CanTransition(PlanSubmitted) || !PlanSubmitted.CanTransition(PlanApproved) || !PlanApproved.CanTransition(PlanExecuting) || !PlanExecuting.CanTransition(PlanCompleted) {
		t.Fatal("valid plan lifecycle rejected")
	}
	if PlanDraft.CanTransition(PlanCompleted) || PlanCompleted.CanTransition(PlanCancelled) {
		t.Fatal("invalid plan lifecycle accepted")
	}
}

func TestShelterAvailableCapacity(t *testing.T) {
	shelter := Shelter{Capacity: 200, Reserved: 75}
	if shelter.AvailableCapacity() != 125 {
		t.Fatalf("available capacity = %d", shelter.AvailableCapacity())
	}
	shelter.Reserved = 201
	if err := shelter.Validate(); err == nil {
		t.Fatal("over-reserved shelter accepted")
	}
}

func TestPlanValidationRequiresFutureDeadline(t *testing.T) {
	now := time.Now().UTC()
	plan := Plan{IncidentID: "i", ZoneID: "z", ShelterID: "s", Name: "evac", EvacueeCount: 10, DeadlineAt: now.Add(-time.Minute)}
	if err := plan.Validate(now); err == nil {
		t.Fatal("past deadline accepted")
	}
}

func TestStepValidationBoundsDuration(t *testing.T) {
	step := Step{Order: 1, Instruction: "notify", ResponsibleRole: "field_operator", ExpectedMinutes: 0}
	if err := step.Validate(); err == nil {
		t.Fatal("zero duration accepted")
	}
	step.ExpectedMinutes = 30
	if err := step.Validate(); err != nil {
		t.Fatalf("valid step rejected: %v", err)
	}
}
