package worker

func unknownJobOutcome(job Job) string {
	if job.Attempts < job.MaxAttempts {
		return "retryable"
	}
	return "permanent"
}
