package crawler

import (
	"net/http"
	"time"
)

// Options configures a crawl run.
type Options struct {
	URL         string
	Depth       int
	Retries     int
	Delay       time.Duration
	Timeout     time.Duration
	RPS         float64
	UserAgent   string
	Concurrency int
	IndentJSON  bool
	HTTPClient  *http.Client
}
