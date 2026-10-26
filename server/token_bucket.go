package main

const (
	capacity    int64 = 5000
	tokenCost   int64 = 1000
	refillPerMs int64 = 1
)

// calculateTokens computes refilled tokens and makes
// the allow/deny decision without changing storage.
func calculateTokens(
	tokens int64,
	lastRefillMs int64,
	nowMs int64,
) (int64, bool, int64) {

	elapsed := nowMs - lastRefillMs
	if elapsed < 0 {
		elapsed = 0
	}

	// Add one complete token for every 1000 ms.
	// This calculation also supports partial tokens.
	refilled := tokens + elapsed*refillPerMs
	if refilled > capacity {
		refilled = capacity
	}

	if refilled >= tokenCost {
		return refilled - tokenCost, true, 0
	}

	missing := tokenCost - refilled
	retryAfterMs := (missing + refillPerMs - 1) / refillPerMs

	return refilled, false, retryAfterMs
}
