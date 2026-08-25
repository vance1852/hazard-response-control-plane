package hazard

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
)

// batchTrackingRepo counts write-chain entries (FindSensor + InsertObservation)
// so a regression that lets cancelled tail elements reach the repository is
// detectable.
type batchTrackingRepo struct {
	validationRepo
	inserts     atomic.Int32
	findSensors atomic.Int32
	cancelAfter atomic.Bool
	cancel      context.CancelFunc
}

func (r *batchTrackingRepo) FindSensor(context.Context, string) (Sensor, error) {
	r.findSensors.Add(1)
	return Sensor{ID: "s", RegionID: "r", Active: true, Kind: SensorRainfall}, nil
}

func (r *batchTrackingRepo) InsertObservation(ctx context.Context, o Observation) (Observation, bool, error) {
	r.inserts.Add(1)
	// Cancel the batch context after the first successful write so the
	// remaining tail elements must short-circuit on the next boundary check.
	if r.cancel != nil && r.inserts.Load() == 1 {
		r.cancel()
	}
	return Observation{SensorID: o.SensorID, SourceSequence: o.SourceSequence}, false, nil
}

func validBatchCommands(n int) []ObservationCommand {
	out := make([]ObservationCommand, n)
	now := time.Now()
	for i := range out {
		out[i] = ObservationCommand{
			SensorID:       "s",
			SourceSequence: "seq",
			ObservedAt:     now,
			Metric:         "rainfall",
			Unit:           "mm",
			Quality:        QualityProvisional,
			Payload:        map[string]any{},
		}
	}
	return out
}

// TestIngestObservationBatchCancelsAllTailElements verifies the fix for the
// reported bug: when the request context is already cancelled, every element
// must return a cancellation result and none may enter the write chain. Before
// the fix only index 0 short-circuited and the tail kept writing.
func TestIngestObservationBatchCancelsAllTailElements(t *testing.T) {
	repo := &batchTrackingRepo{}
	svc, _ := NewService(repo, clock.NewManual(time.Now()))
	actor := identity.Actor{UserID: "u", Role: identity.RoleFieldOperator}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // client cancelled before the batch starts

	results := svc.IngestObservationBatch(ctx, actor, validBatchCommands(3))

	if got := repo.findSensors.Load(); got != 0 {
		t.Fatalf("expected zero sensor lookups after cancellation, got %d", got)
	}
	if got := repo.inserts.Load(); got != 0 {
		t.Fatalf("expected zero writes after cancellation, got %d", got)
	}
	for i, r := range results {
		if r.ErrorCode != "request_cancelled" {
			t.Fatalf("element %d: want error_code request_cancelled, got %q", i, r.ErrorCode)
		}
		if r.Observation != nil {
			t.Fatalf("element %d: expected no observation, got %+v", i, r.Observation)
		}
	}
}

// TestIngestObservationBatchStopsTailAfterMidBatchCancel verifies that once the
// context is cancelled mid-batch (after the first element writes), every
// not-yet-started tail element short-circuits instead of continuing to access
// the sensor and database.
func TestIngestObservationBatchStopsTailAfterMidBatchCancel(t *testing.T) {
	repo := &batchTrackingRepo{}
	svc, _ := NewService(repo, clock.NewManual(time.Now()))
	actor := identity.Actor{UserID: "u", Role: identity.RoleFieldOperator}

	ctx, cancel := context.WithCancel(context.Background())
	repo.cancel = cancel

	results := svc.IngestObservationBatch(ctx, actor, validBatchCommands(3))

	// Only the first element should have entered the write chain.
	if got := repo.inserts.Load(); got != 1 {
		t.Fatalf("expected exactly one write (first element), got %d", got)
	}
	if results[0].ErrorCode != "" || results[0].Observation == nil {
		t.Fatalf("first element should succeed, got %+v", results[0])
	}
	for i := 1; i < len(results); i++ {
		if results[i].ErrorCode != "request_cancelled" {
			t.Fatalf("tail element %d: want request_cancelled, got %q", i, results[i].ErrorCode)
		}
		if results[i].Observation != nil {
			t.Fatalf("tail element %d: expected no observation, got %+v", i, results[i].Observation)
		}
	}
}

var _ = apperr.Classify
