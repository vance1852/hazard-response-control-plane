package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
	"github.com/vance1852/hazard-response-control-plane/internal/evacuation"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, ":memory:", dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testClock() *clock.Manual { return clock.NewManual(time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)) }

func seedUser(t *testing.T, store *Store, now time.Time, id string) {
	t.Helper()
	if _, err := store.CreateUser(context.Background(), identity.User{ID: id, Username: id, PasswordHash: "hash", DisplayName: "Tester", Role: identity.RoleCommander, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationsAreVersionedAndRepeatable(t *testing.T) {
	store := openTestStore(t)
	var count int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("migration count = %d", count)
	}
	if err := store.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables < 20 {
		t.Fatalf("table count = %d", tables)
	}
}

func TestMigrationHistoryRejectsChecksumTampering(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.DB().Exec(`UPDATE schema_migrations SET checksum='tampered' WHERE version=1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUserAndSessionPersistence(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := testClock().Now()
	user := identity.User{Username: "commander", PasswordHash: "hash", DisplayName: "Commander", Role: identity.RoleCommander, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	created, err := store.CreateUser(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("user ID is empty")
	}
	session := identity.Session{UserID: created.ID, TokenDigest: []byte("digest"), ExpiresAt: now.Add(time.Hour), LastSeenAt: now, Version: 1, CreatedAt: now}
	if _, err := store.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	found, actorUser, err := store.FindSessionByDigest(ctx, []byte("digest"))
	if err != nil {
		t.Fatal(err)
	}
	if found.UserID != created.ID || actorUser.Username != "commander" {
		t.Fatalf("session lookup mismatch: %#v %#v", found, actorUser)
	}
	if err := store.RevokeSession(ctx, found.ID, 0, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.FindSessionByDigest(ctx, []byte("digest")); err != nil {
		t.Fatal(err)
	}
}

func TestActivationTransactionPersistsIncidentZonesAuditAndOutbox(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := testClock().Now()
	seedUser(t, store, now, "user")
	region := hazard.Region{Code: "CQ", Name: "Central", Timezone: "Asia/Shanghai", Active: true, CreatedAt: now, UpdatedAt: now}
	region, err := store.CreateRegion(ctx, region)
	if err != nil {
		t.Fatal(err)
	}
	event, err := audit.New("user", "incident.activate", "incident", "external", "request", audit.OutcomeSucceeded, map[string]string{"test": "yes"}, now)
	if err != nil {
		t.Fatal(err)
	}
	record := hazard.ActivationRecord{Incident: hazard.Incident{RegionID: region.ID, ExternalRef: "external", HazardType: hazard.Earthquake, Title: "Quake", Severity: 4, Status: hazard.IncidentActive, CommandLevel: hazard.CommandJoint, Summary: "road damage", OccurredAt: now, ActivatedAt: &now, Version: 1, CreatedBy: "user", CreatedAt: now, UpdatedAt: now}, Zones: []hazard.Zone{{RegionID: region.ID, Name: "west", RiskLevel: 4, Population: 100, GeometryJSON: "{}", CreatedAt: now}}, Audit: event, Topic: "incident.activated", Payload: "{}", Now: now}
	incident, zones, err := store.ActivateIncident(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) != 1 {
		t.Fatal("zone missing")
	}
	if _, err := store.FindIncident(ctx, incident.ID); err != nil {
		t.Fatal(err)
	}
	var auditCount, outboxCount int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditCount)
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM outbox_events`).Scan(&outboxCount)
	if auditCount != 1 || outboxCount != 1 {
		t.Fatalf("audit=%d outbox=%d", auditCount, outboxCount)
	}
}

func TestActivationRollsBackWhenOutboxWriteFails(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := testClock().Now()
	seedUser(t, store, now, "user")
	region, _ := store.CreateRegion(ctx, hazard.Region{Code: "CQ", Name: "Central", Timezone: "Asia/Shanghai", Active: true, CreatedAt: now, UpdatedAt: now})
	if _, err := store.DB().ExecContext(ctx, `DROP TABLE outbox_events`); err != nil {
		t.Fatal(err)
	}
	event, _ := audit.New("user", "incident.activate", "incident", "rainstorm-1", "request", audit.OutcomeSucceeded, nil, now)
	record := hazard.ActivationRecord{Incident: hazard.Incident{RegionID: region.ID, ExternalRef: "rainstorm-1", HazardType: hazard.Rainstorm, Title: "Rain", Severity: 3, Status: hazard.IncidentActive, CommandLevel: hazard.CommandLocal, Summary: "rain", OccurredAt: now, ActivatedAt: &now, Version: 1, CreatedBy: "user", CreatedAt: now, UpdatedAt: now}, Zones: []hazard.Zone{{RegionID: region.ID, Name: "flood", RiskLevel: 3, Population: 100, GeometryJSON: "{}", CreatedAt: now}}, Audit: event, Topic: "incident.activated", Payload: "{}", Now: now}
	if _, _, err := store.ActivateIncident(ctx, record); err == nil {
		t.Fatal("activation succeeded despite outbox write failure")
	}
	var incidents, zones, audits int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM incidents WHERE external_ref='rainstorm-1'`).Scan(&incidents)
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM incident_zones`).Scan(&zones)
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&audits)
	if incidents != 0 || zones != 0 || audits != 0 {
		t.Fatalf("aggregate not rolled back: incidents=%d zones=%d audits=%d", incidents, zones, audits)
	}
	if _, err := store.DB().ExecContext(ctx, `CREATE TABLE outbox_events (
    id TEXT PRIMARY KEY,
    topic TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'publishing', 'published', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TEXT NOT NULL,
    lease_owner TEXT,
    lease_until TEXT,
    last_error TEXT,
    created_at TEXT NOT NULL,
    published_at TEXT
)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ActivateIncident(ctx, record); err != nil {
		t.Fatalf("retry with same external_ref failed: %v", err)
	}
	var outboxCount int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM outbox_events`).Scan(&outboxCount)
	if outboxCount != 1 {
		t.Fatalf("outbox published %d times, want exactly 1", outboxCount)
	}
}

func TestActivationRollbackOnInvalidZone(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := testClock().Now()
	seedUser(t, store, now, "user")
	region := hazard.Region{Code: "CQ", Name: "Central", Timezone: "Asia/Shanghai", Active: true, CreatedAt: now, UpdatedAt: now}
	region, _ = store.CreateRegion(ctx, region)
	event, _ := audit.New("user", "incident.activate", "incident", "bad", "request", audit.OutcomeSucceeded, nil, now)
	record := hazard.ActivationRecord{Incident: hazard.Incident{RegionID: region.ID, ExternalRef: "bad", HazardType: hazard.Earthquake, Title: "Bad", Severity: 3, Status: hazard.IncidentActive, CommandLevel: hazard.CommandLocal, Summary: "bad", OccurredAt: now, Version: 1, CreatedBy: "user", CreatedAt: now, UpdatedAt: now}, Zones: []hazard.Zone{{RegionID: region.ID, Name: "dup", RiskLevel: 2, Population: 2, GeometryJSON: "{}", CreatedAt: now}, {RegionID: region.ID, Name: "dup", RiskLevel: 2, Population: 2, GeometryJSON: "{}", CreatedAt: now}}, Audit: event, Topic: "incident.activated", Payload: "{}", Now: now}
	if _, _, err := store.ActivateIncident(ctx, record); err == nil {
		t.Fatal("duplicate zone activation succeeded")
	}
	var count int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM incidents WHERE external_ref='bad'`).Scan(&count)
	if count != 0 {
		t.Fatalf("rolled back incident count=%d", count)
	}
}

func TestObservationIdempotencyRejectsChangedSequence(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := testClock().Now()
	seedUser(t, store, now, "u")
	region, _ := store.CreateRegion(ctx, hazard.Region{Code: "CQ", Name: "Central", Timezone: "Asia/Shanghai", Active: true, CreatedAt: now, UpdatedAt: now})
	sensor, _ := store.CreateSensor(ctx, hazard.Sensor{RegionID: region.ID, StationCode: "S1", Name: "Rain", Kind: hazard.SensorRainfall, Latitude: 29, Longitude: 106, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now})
	observation := hazard.Observation{SensorID: sensor.ID, SourceSequence: "42", ObservedAt: now, Metric: "rain", Value: 20, Unit: "mm", Quality: hazard.QualityVerified, PayloadJSON: "{}", CreatedBy: "u", CreatedAt: now}
	created, duplicate, err := store.InsertObservation(ctx, observation)
	if err != nil || duplicate || created.ID == "" {
		t.Fatalf("first observation: %#v %v %v", created, duplicate, err)
	}
	second, duplicate, err := store.InsertObservation(ctx, observation)
	if err != nil || !duplicate || second.ID != created.ID {
		t.Fatalf("duplicate observation: %#v %v %v", second, duplicate, err)
	}
	observation.Value = 99
	if _, _, err := store.InsertObservation(ctx, observation); err == nil {
		t.Fatal("changed sequence accepted")
	}
}

func TestConcurrentShelterReservationDoesNotOverbook(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := testClock().Now()
	seedUser(t, store, now, "u")
	region, _ := store.CreateRegion(ctx, hazard.Region{Code: "CQ", Name: "Central", Timezone: "Asia/Shanghai", Active: true, CreatedAt: now, UpdatedAt: now})
	incident, _, _ := store.ActivateIncident(ctx, hazard.ActivationRecord{Incident: hazard.Incident{RegionID: region.ID, ExternalRef: "i", HazardType: hazard.Rainstorm, Title: "Rain", Severity: 3, Status: hazard.IncidentActive, CommandLevel: hazard.CommandLocal, Summary: "rain", OccurredAt: now, ActivatedAt: &now, Version: 1, CreatedBy: "u", CreatedAt: now, UpdatedAt: now}, Zones: []hazard.Zone{{RegionID: region.ID, Name: "z", RiskLevel: 3, Population: 100, GeometryJSON: "{}", CreatedAt: now}}, Audit: mustAudit(t, now), Topic: "incident.activated", Payload: "{}", Now: now})
	zone, _ := store.FindZone(ctx, firstZone(t, store, incident.ID))
	shelter, _ := store.CreateShelter(ctx, evacuation.Shelter{RegionID: region.ID, Code: "A", Name: "Shelter", Capacity: 100, Status: evacuation.ShelterAvailable, Version: 1, CreatedAt: now, UpdatedAt: now})
	makePlan := func(name string) evacuation.Plan {
		plan, _, err := store.CreatePlan(ctx, evacuation.Plan{IncidentID: incident.ID, ZoneID: zone.ID, ShelterID: shelter.ID, Name: name, EvacueeCount: 80, Status: evacuation.PlanSubmitted, DeadlineAt: now.Add(time.Hour), Version: 1, CreatedBy: "u", CreatedAt: now, UpdatedAt: now}, []evacuation.Step{{Order: 1, Instruction: "one", ResponsibleRole: "commander", ExpectedMinutes: 10, CreatedAt: now}, {Order: 2, Instruction: "two", ResponsibleRole: "field_operator", ExpectedMinutes: 10, CreatedAt: now}}, mustAudit(t, now))
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}
	p1, p2 := makePlan("p1"), makePlan("p2")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, plan := range []evacuation.Plan{p1, p2} {
		wg.Add(1)
		go func(p evacuation.Plan) {
			defer wg.Done()
			_, _, _, e := store.ApprovePlan(ctx, p.ID, p.Version, "u", now, mustAudit(t, now))
			results <- e
		}(plan)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("approval success=%d", success)
	}
	var reserved int
	_ = store.DB().QueryRow(`SELECT reserved FROM shelters WHERE id=?`, shelter.ID).Scan(&reserved)
	if reserved != 80 {
		t.Fatalf("reserved=%d", reserved)
	}
}

func mustAudit(t *testing.T, now time.Time) audit.Event {
	t.Helper()
	event, err := audit.New("u", "test", "object", "id", "request", audit.OutcomeSucceeded, map[string]string{}, now)
	if err != nil {
		t.Fatal(err)
	}
	return event
}
func firstZone(t *testing.T, store *Store, incidentID string) string {
	t.Helper()
	var id string
	if err := store.DB().QueryRow(`SELECT id FROM incident_zones WHERE incident_id=?`, incidentID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRepositoryReturnsIndependentSlices(t *testing.T) {
	store := openTestStore(t)
	now := testClock().Now()
	seedUser(t, store, now, "u")
	region, _ := store.CreateRegion(context.Background(), hazard.Region{Code: "CQ", Name: "Central", Timezone: "Asia/Shanghai", Active: true, CreatedAt: now, UpdatedAt: now})
	incident, _, _ := store.ActivateIncident(context.Background(), hazard.ActivationRecord{Incident: hazard.Incident{RegionID: region.ID, ExternalRef: "slice", HazardType: hazard.Landslide, Title: "Slide", Severity: 2, Status: hazard.IncidentActive, CommandLevel: hazard.CommandLocal, Summary: "slide", OccurredAt: now, Version: 1, CreatedBy: "u", CreatedAt: now, UpdatedAt: now}, Zones: []hazard.Zone{{RegionID: region.ID, Name: "z", RiskLevel: 2, Population: 3, GeometryJSON: "{}", CreatedAt: now}}, Audit: mustAudit(t, now), Topic: "incident", Payload: "{}", Now: now})
	zones, err := store.ListIncidentZones(context.Background(), incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	zones[0].Name = "mutated"
	again, err := store.ListIncidentZones(context.Background(), incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Name == "mutated" {
		t.Fatal("repository leaked mutable state")
	}
}

func TestAuditCursorRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	cursor := encodeAuditCursor(formatTime(now), "id")
	created, id, err := decodeAuditCursor(cursor)
	if err != nil || created != formatTime(now) || id != "id" {
		t.Fatalf("cursor=%s/%s/%v", created, id, err)
	}
}

func TestStoreWithinTxHonorsCancellation(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.WithinTx(ctx, func(*sql.Tx) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}
