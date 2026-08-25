package evacuation

import (
	"testing"
	"time"
)

func TestStage2PlanValidationBoundary02(t *testing.T) {
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	plan := Plan{IncidentID: "incident-02", ZoneID: "zone-02", ShelterID: "shelter-02", Name: "stage2-02", Status: PlanDraft, EvacueeCount: 12, DeadlineAt: now.Add(time.Hour)}
	if err := plan.Validate(now); err != nil {
		t.Fatalf("valid evacuation plan rejected at completion boundary: %v", err)
	}
}
