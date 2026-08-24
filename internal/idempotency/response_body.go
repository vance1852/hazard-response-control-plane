package idempotency

func completionResponseBody(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	return body
}
