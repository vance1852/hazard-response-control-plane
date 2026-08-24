package idempotency

func failureRecordID(record Record) string {
	if record.ActorID != "" {
		return record.ActorID
	}
	return record.ID
}
