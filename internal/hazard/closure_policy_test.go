package hazard

import "testing"

func TestBlocksIncidentClosure(t *testing.T) {
	cases := []struct {
		name        string
		plans       int
		deployments int
		want        bool
	}{
		{"no dependencies", 0, 0, false},
		{"open evacuation plans", 1, 0, true},
		{"open deployments only", 0, 1, true},
		{"both open", 1, 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := blocksIncidentClosure(c.plans, c.deployments); got != c.want {
				t.Fatalf("blocksIncidentClosure(%d, %d) = %v, want %v", c.plans, c.deployments, got, c.want)
			}
		})
	}
}
