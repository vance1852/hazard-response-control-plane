package hazard

// observationBatchPointer returns a pointer to a copy of observation so that
// every batch result owns a distinct value. Reusing a single package-level
// variable would alias every result to the same observation, causing all
// successful elements to reflect the last ingested command.
func observationBatchPointer(observation Observation) *Observation {
	observationCopy := observation
	return &observationCopy
}
