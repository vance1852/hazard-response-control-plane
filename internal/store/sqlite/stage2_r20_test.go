package sqlite

import (
 "context"
 "testing"

 "github.com/vance1852/hazard-response-control-plane/internal/audit"
 "github.com/vance1852/hazard-response-control-plane/internal/hazard"
)

func TestStage2ActivationRollsBackWhenOutboxPersistenceFails(t *testing.T) {
 store := openTestStore(t)
 ctx := context.Background()
 now := testClock().Now()
 seedUser(t, store, now, "user-r20")
 region, err := store.CreateRegion(ctx, hazard.Region{Code:"R20",Name:"Rain Basin",Timezone:"UTC",Active:true,CreatedAt:now,UpdatedAt:now})
 if err != nil { t.Fatal(err) }
 if _, err = store.DB().Exec(`CREATE TRIGGER reject_r20_outbox BEFORE INSERT ON outbox_events BEGIN SELECT RAISE(ABORT, 'outbox unavailable'); END;`); err != nil { t.Fatal(err) }
 event, err := audit.New("user-r20", "incident.activate", "incident", "pending-r20", "request-r20", audit.OutcomeSucceeded, map[string]any{"source":"rain gauges"}, now)
 if err != nil { t.Fatal(err) }
 record := hazard.ActivationRecord{Incident:hazard.Incident{RegionID:region.ID,ExternalRef:"activation-r20",HazardType:hazard.Rainstorm,Title:"River rise",Severity:4,Status:hazard.IncidentActive,CommandLevel:hazard.CommandRegional,Summary:"evacuation warning",OccurredAt:now,ActivatedAt:&now,Version:1,CreatedBy:"user-r20",CreatedAt:now,UpdatedAt:now},Zones:[]hazard.Zone{{RegionID:region.ID,Name:"Lower basin",RiskLevel:4,Population:240,GeometryJSON:"{}",CreatedAt:now}},Audit:event,Topic:"incident.activated",Payload:"{\"external_ref\":\"activation-r20\"}",Now:now}
 if _, _, err = store.ActivateIncident(ctx, record); err == nil { t.Fatal("activation succeeded while outbox persistence failed") }
 var incidents, zones, audits int
 if err = store.DB().QueryRow(`SELECT COUNT(*) FROM incidents WHERE external_ref='activation-r20'`).Scan(&incidents); err != nil { t.Fatal(err) }
 if err = store.DB().QueryRow(`SELECT COUNT(*) FROM incident_zones WHERE name='Lower basin'`).Scan(&zones); err != nil { t.Fatal(err) }
 if err = store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE request_id='request-r20'`).Scan(&audits); err != nil { t.Fatal(err) }
 if incidents != 0 || zones != 0 || audits != 0 { t.Fatalf("failed activation leaked incident=%d zones=%d audits=%d", incidents, zones, audits) }
 if _, err = store.DB().Exec(`DROP TRIGGER reject_r20_outbox`); err != nil { t.Fatal(err) }
 incident, createdZones, err := store.ActivateIncident(ctx, record)
 if err != nil { t.Fatalf("activation retry after outbox recovery: %v", err) }
 var outbox int
 if err = store.DB().QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE aggregate_id=?`, incident.ID).Scan(&outbox); err != nil { t.Fatal(err) }
 if len(createdZones) != 1 || outbox != 1 { t.Fatalf("retry state: zones=%d outbox=%d", len(createdZones), outbox) }
}
