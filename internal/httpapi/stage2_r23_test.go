package httpapi
import("context";"errors";"net/http/httptest";"testing")
type cancelledHealthProbe struct{seen error}
func(h *cancelledHealthProbe)Ping(ctx context.Context)error{h.seen=ctx.Err();return ctx.Err()}
func TestStage2ReadinessPreservesProbeCancellation(t *testing.T){health:=&cancelledHealthProbe{};server:=&Server{deps:Deps{Health:health}};ctx,cancel:=context.WithCancel(context.Background());cancel();request:=httptest.NewRequest("GET","http://service/ready",nil).WithContext(ctx);response:=httptest.NewRecorder();server.ready(response,request);if !errors.Is(health.seen,context.Canceled){t.Fatalf("database probe context error = %v",health.seen)};if response.Code==200{t.Fatalf("cancelled readiness returned status %d",response.Code)}}
