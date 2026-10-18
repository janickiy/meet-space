package ratelimit

import (
	"time"
)

// Result contains the limiter decision and HTTP budget metadata. Limit is the request budget per
// window; Remaining is its unused portion. RetryAfter bounds the retry wait. A zero ResetAt does
// not produce an X-RateLimit-Reset header.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}
