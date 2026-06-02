package crawler

import (
	"context"
	"io"
	"net/http"
	"time"
)

const minRetryPause = 100 * time.Millisecond

func shouldRetryHTTPStatus(code int) bool {
	if code == http.StatusTooManyRequests {
		return true
	}
	return code >= 500 && code <= 599
}

func retryPauseDuration() time.Duration {
	// Non-zero pause to avoid bursts (requirement).
	// Keep it independent from Delay/RPS (which are global rate limits).
	return minRetryPause
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	return defaultSleeper(ctx, d)
}

// doRequestWithRetry performs the request, retrying only on temporary failures:
// - network errors (no response)
// - HTTP 429 / 5xx
//
// Retries are limited by opts.Retries (total attempts = Retries + 1).
func doRequestWithRetry(ctx context.Context, opts Options, req *http.Request) (*http.Response, error) {
	attemptsLeft := opts.Retries
	pause := retryPauseDuration()
	if pause <= 0 {
		pause = minRetryPause
	}

	for {
		resp, err := doRequest(ctx, opts, req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attemptsLeft <= 0 {
				return nil, err
			}
			attemptsLeft--
			if err := sleepWithContext(ctx, pause); err != nil {
				return nil, err
			}
			continue
		}

		if !shouldRetryHTTPStatus(resp.StatusCode) || attemptsLeft <= 0 {
			return resp, nil
		}

		// Retryable status code: drain and close before retrying.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		attemptsLeft--
		if err := sleepWithContext(ctx, pause); err != nil {
			return nil, err
		}
	}
}
