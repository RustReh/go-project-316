package crawler

import (
	"net/http"
	"time"
)

// Options configures a crawl run.
type Options struct {
	URL     string
	Depth   int
	Retries int
	// Delay is the minimum interval between any HTTP requests (pages and link checks).
	// Ignored when RPS is greater than zero.
	Delay   time.Duration
	Timeout time.Duration
	// RPS limits how many HTTP requests per second the crawler may perform globally.
	// When both Delay and RPS are set, RPS takes priority.
	RPS         float64
	UserAgent   string
	Concurrency int
	IndentJSON  bool
	HTTPClient  *http.Client
	// Now allows tests to control timestamps in the JSON report.
	// If nil, time.Now is used.
	Now func() time.Time

	requestLimiter *requestLimiter
}
