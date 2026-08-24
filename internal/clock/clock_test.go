package clock

import (
	"sync"
	"testing"
	"time"
)

func TestManualClockAdvancesSafely(t *testing.T) {
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	clk := NewManual(start)
	if got := clk.Now(); !got.Equal(start) {
		t.Fatalf("initial clock = %v", got)
	}
	if got := clk.Advance(90 * time.Minute); !got.Equal(start.Add(90 * time.Minute)) {
		t.Fatalf("advanced clock = %v", got)
	}
	const readers = 16
	var wg sync.WaitGroup
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = clk.Now() }()
	}
	wg.Wait()
}

func TestManualClockSetNormalizesUTC(t *testing.T) {
	clk := NewManual(time.Time{})
	local := time.Date(2026, 8, 24, 8, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	clk.Set(local)
	if got := clk.Now(); got.Location() != time.UTC {
		t.Fatalf("location = %v", got.Location())
	}
	if got := clk.Now(); !got.Equal(local) {
		t.Fatalf("clock = %v", got)
	}
}
