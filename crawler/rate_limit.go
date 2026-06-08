package crawler

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type sleeper func(context.Context, time.Duration) error

type requestLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
	now      func() time.Time
	sleep    sleeper
}

func defaultSleeper(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func newRequestLimiter(opts Options) *requestLimiter {
	interval := requestInterval(opts)
	if interval <= 0 {
		return nil
	}
	return &requestLimiter{
		interval: interval,
		now:      time.Now,
		sleep:    defaultSleeper,
	}
}

func requestInterval(opts Options) time.Duration {
	if opts.RPS > 0 {
		return time.Duration(float64(time.Second) / opts.RPS)
	}
	if opts.Delay > 0 {
		return opts.Delay
	}
	return 0
}

func (l *requestLimiter) Wait(ctx context.Context) error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	now := l.now()
	var wait time.Duration
	if !l.last.IsZero() {
		waitUntil := l.last.Add(l.interval)
		if now.Before(waitUntil) {
			wait = waitUntil.Sub(now)
			l.last = waitUntil
		} else {
			l.last = now
		}
	} else {
		l.last = now
	}
	l.mu.Unlock()

	if wait > 0 {
		if err := l.sleep(ctx, wait); err != nil {
			return err
		}
	}
	return nil
}

func waitForRequest(ctx context.Context, lim *requestLimiter) error {
	if lim == nil {
		return nil
	}
	return lim.Wait(ctx)
}

func doRequest(ctx context.Context, opts Options, req *http.Request) (*http.Response, error) {
	if err := waitForRequest(ctx, opts.requestLimiter); err != nil {
		return nil, err
	}
	return opts.HTTPClient.Do(req)
}
