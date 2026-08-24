package sqlite

import("context";"testing";"time")
func TestStage2CancelReleasesExactReservation(t *testing.T){
 store:=openTestStore(t);ctx:=context.Background();now:=testClock().Now();seedUser(t,store,now,"commander-8")
 statements:=[]string{"INSERT INTO regions(id,code,name,timezone,active,created_at,updated_at) VALUES('r8','R8','Region','UTC',1,'2026-08-24T08:00:00Z','2026-08-24T08:00:00Z')","INSERT INTO shelters(id,region_id,code,name,capacity,reserved,status,version,created_at,updated_at) VALUES('s8','r8','S8','Shelter',50,10,'available',1,'2026-08-24T08:00:00Z','2026-08-24T08:00:00Z')","INSERT INTO incidents(id,region_id,external_ref,hazard_type,title,severity,status,command_level,summary,occurred_at,version,created_by,created_at,updated_at) VALUES('i8','r8','ext8','landslide','Slope',3,'active','local','risk','2026-08-24T08:00:00Z',1,'commander-8','2026-08-24T08:00:00Z','2026-08-24T08:00:00Z')","INSERT INTO incident_zones(id,incident_id,region_id,name,risk_level,population,geometry_json,created_at) VALUES('z8','i8','r8','Hill',3,10,'{}','2026-08-24T08:00:00Z')","INSERT INTO evacuation_plans(id,incident_id,zone_id,shelter_id,name,evacuee_count,status,deadline_at,version,created_by,created_at,updated_at) VALUES('p8','i8','z8','s8','Cancel',10,'approved','2026-08-24T10:00:00Z',1,'commander-8','2026-08-24T08:00:00Z','2026-08-24T08:00:00Z')","INSERT INTO shelter_reservations(id,shelter_id,plan_id,people,status,created_at) VALUES('rv8','s8','p8',10,'active','2026-08-24T08:00:00Z')"}
 for _,statement:=range statements{if _,err:=store.DB().ExecContext(ctx,statement);err!=nil{t.Fatal(err)}}
 plan,released,err:=store.CancelPlan(ctx,"p8",1,"road closed",now,mustAudit(t,now));if err!=nil{t.Fatal(err)}
 if plan.Status!="cancelled"||released==nil||released.People!=10{t.Fatalf("cancel result = %#v %#v",plan,released)}
 var reserved int;if err:=store.DB().QueryRowContext(ctx,"SELECT reserved FROM shelters WHERE id='s8'").Scan(&reserved);err!=nil{t.Fatal(err)}
 if reserved!=0{t.Fatalf("reserved capacity after cancellation = %d",reserved)}
 _=time.Second
}
