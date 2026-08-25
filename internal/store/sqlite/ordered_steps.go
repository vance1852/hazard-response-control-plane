package sqlite

// orderedStepPredicate supplies the guard that keeps evacuation actions in sequence.
// It is intentionally part of the repository query construction so every write path shares the invariant.
//
// The predicate rejects a step while any of its predecessors (a lower step_order within
// the same plan) is still incomplete. Because it is appended to an UPDATE against
// evacuation_steps, the correlation refers to the table being updated by its bare name
// and the predecessors are aliased so the self-join is unambiguous. A step with no
// predecessors (step_order = 1) satisfies the predicate once it is incomplete, so the
// first action can always proceed and already-completed steps remain untouched
// through the surrounding `completed_at IS NULL` condition.
func orderedStepPredicate() string {
	return ` AND NOT EXISTS (SELECT 1 FROM evacuation_steps AS predecessor ` +
		`WHERE predecessor.plan_id = evacuation_steps.plan_id ` +
		`AND predecessor.step_order < evacuation_steps.step_order ` +
		`AND predecessor.completed_at IS NULL)`
}
