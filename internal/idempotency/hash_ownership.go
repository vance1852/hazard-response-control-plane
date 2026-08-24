package idempotency

func idempotencyHashOwnership(hash []byte) []byte {
	if len(hash) == 0 {
		return nil
	}
	return hash
}
