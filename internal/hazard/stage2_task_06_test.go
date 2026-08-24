package hazard

import (
 "context"
 "testing"
 "time"
 "github.com/vance1852/hazard-response-control-plane/internal/clock"
 "github.com/vance1852/hazard-response-control-plane/internal/identity"
)
func TestStage2CancelledBatchDoesNotProcessTail(t *testing.T){
 svc,err:=NewService(validationRepo{},clock.NewManual(time.Now().UTC()));if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 commands:=[]ObservationCommand{{SensorID:"s1"},{SensorID:"s2"}}
 results:=svc.IngestObservationBatch(ctx,identity.Actor{UserID:"field-6",Role:identity.RoleFieldOperator},commands)
 if len(results)!=2{t.Fatalf("result count = %d",len(results))}
 for i,result:=range results{if result.ErrorCode!="request_cancelled"{t.Fatalf("result %d error = %q, want request_cancelled",i,result.ErrorCode)}}
}
