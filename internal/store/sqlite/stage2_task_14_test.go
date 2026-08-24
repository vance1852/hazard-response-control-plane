package sqlite

import (
 "context"
 "testing"
 "time"
 "github.com/vance1852/hazard-response-control-plane/internal/evacuation"
 "github.com/vance1852/hazard-response-control-plane/internal/hazard"
)

func TestStage2StepCompletionRejectsOutOfOrder(t *testing.T) {
 store := openTestStore(t); ctx := context.Background(); now := testClock().Now(); seedUser(t, store, now, "u")
 region, err := store.CreateRegion(ctx, hazard.Region{Code:"OR", Name:"Order Region", Timezone:"UTC", Active:true, CreatedAt:now, UpdatedAt:now}); if err != nil { t.Fatal(err) }
 incident, _, err := store.ActivateIncident(ctx, hazard.ActivationRecord{Incident: hazard.Incident{RegionID:region.ID, ExternalRef:"order-incident", HazardType:hazard.Earthquake, Title:"Order", Severity:3, Status:hazard.IncidentActive, CommandLevel:hazard.CommandLocal, Summary:"order", OccurredAt:now, ActivatedAt:&now, Version:1, CreatedBy:"u", CreatedAt:now, UpdatedAt:now}, Zones:[]hazard.Zone{{RegionID:region.ID, Name:"zone", RiskLevel:2, Population:10, GeometryJSON:"{}", CreatedAt:now}}, Audit:mustAudit(t, now), Topic:"incident.activated", Payload:"{}", Now:now}); if err != nil { t.Fatal(err) }
 var zoneID string; if err:=store.DB().QueryRow(`SELECT id FROM incident_zones WHERE incident_id=?`, incident.ID).Scan(&zoneID); err != nil { t.Fatal(err) }
 shelter, err := store.CreateShelter(ctx, evacuation.Shelter{RegionID:region.ID, Code:"OR-S", Name:"Order Shelter", Capacity:100, Status:evacuation.ShelterAvailable, Version:1, CreatedAt:now, UpdatedAt:now}); if err != nil { t.Fatal(err) }
 plan, steps, err := store.CreatePlan(ctx, evacuation.Plan{IncidentID:incident.ID, ZoneID:zoneID, ShelterID:shelter.ID, Name:"ordered", EvacueeCount:5, Status:evacuation.PlanExecuting, DeadlineAt:now.Add(time.Hour), Version:1, CreatedBy:"u", CreatedAt:now, UpdatedAt:now}, []evacuation.Step{{Order:1, Instruction:"first", ResponsibleRole:"commander", ExpectedMinutes:5, CreatedAt:now},{Order:2, Instruction:"second", ResponsibleRole:"field_operator", ExpectedMinutes:5, CreatedAt:now}}, mustAudit(t, now)); if err != nil { t.Fatal(err) }
 if len(steps)!=2 { t.Fatalf("steps=%d", len(steps)) }
 if _, err := store.CompleteStep(ctx, plan.ID, steps[1].ID, now, mustAudit(t, now)); err == nil { t.Fatal("second evacuation step completed before its predecessor") }
}
