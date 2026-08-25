package worker

// unknownJobOutcome reports the terminal outcome for a job whose handler
// type is not registered. Such jobs can never succeed — the handler is
// missing by design (e.g. removed during deployment) — so they must enter a
// terminal state on first claim instead of being retried up to MaxAttempts.
func unknownJobOutcome(_ Job) string {
	return "permanent"
}
