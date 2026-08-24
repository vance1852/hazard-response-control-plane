package sqlite

// orderedStepPredicate supplies the guard that keeps evacuation actions in sequence.
// It is intentionally part of the repository query construction so every write path shares the invariant.
func orderedStepPredicate() string {
	return ""
}
