package sqlite

import ("context"; "testing"; "time")

func TestStage2ShelterCompletionReleasesCapacity(t *testing.T) {
 store := openTestStore(t); now := testClock().Now(); ctx := context.Background(); seedUser(t, store, now, "commander")
 if _, err := store.DB().ExecContext(ctx, "INSERT INTO regions(id,code,name,timezone,active,created_at,updated_at) VALUES('r1','CQ','Central','UTC',1,?,?)", formatTime(now), formatTime(now)); err != nil { t.Fatal(err) }
 if _, err := store.DB().ExecContext(ctx, "INSERT INTO shelters(id,region_id,code,name,capacity,reserved,status,version,created_at,updated_at) VALUES('s1','r1','S1','Safe',100,10,'available',1,?,?)", formatTime(now), formatTime(now)); err != nil { t.Fatal(err) }
 if _, err := store.DB().ExecContext(ctx, "INSERT INTO incidents(id,region_id,external_ref,hazard_type,title,severity,status,command_level,summary,occurred_at,activated_at,version,created_by,created_at,updated_at) VALUES('i1','r1','ext','rainstorm','Rain',3,'active','local','flood',?,?,1,'commander',?,?)", formatTime(now), formatTime(now), formatTime(now), formatTime(now)); err != nil { t.Fatal(err) }
 if _, err := store.DB().ExecContext(ctx, "INSERT INTO incident_zones(id,incident_id,region_id,name,risk_level,population,geometry_json,created_at) VALUES('z1','i1','r1','Zone',2,20,'{}',?)", formatTime(now)); err != nil { t.Fatal(err) }
 if _, err := store.DB().ExecContext(ctx, "INSERT INTO evacuation_plans(id,incident_id,zone_id,shelter_id,name,evacuee_count,status,deadline_at,version,created_by,created_at,updated_at) VALUES('p1','i1','z1','s1','Plan',10,'executing',?,1,'commander',?,?)", formatTime(now.Add(time.Hour)), formatTime(now), formatTime(now)); err != nil { t.Fatal(err) }
 if _, err := store.DB().ExecContext(ctx, "INSERT INTO evacuation_steps(id,plan_id,step_order,instruction,responsible_role,expected_minutes,completed_at,completed_by,created_at) VALUES('st1','p1',1,'Evacuate','commander',10,?,?,?)", formatTime(now), "commander", formatTime(now)); err != nil { t.Fatal(err) }
 if _, err := store.DB().ExecContext(ctx, "INSERT INTO shelter_reservations(id,shelter_id,plan_id,people,status,created_at) VALUES('rv1','s1','p1',10,'active',?)", formatTime(now)); err != nil { t.Fatal(err) }
 _, reservation, err := store.CompletePlan(ctx, "p1", 1, now, mustAudit(t, now)); if err != nil { t.Fatal(err) }
 if reservation.Status != "consumed" { t.Fatalf("reservation status = %s", reservation.Status) }
 var reserved int; if err := store.DB().QueryRowContext(ctx, "SELECT reserved FROM shelters WHERE id='s1'").Scan(&reserved); err != nil { t.Fatal(err) }
 if reserved != 0 { t.Fatalf("consumed evacuees left reserved capacity at %d", reserved) }
}
