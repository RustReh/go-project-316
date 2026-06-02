package crawler

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRequestLimiter_intervalsWithFakeClock(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	clock := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	advance := func(d time.Duration) {
		mu.Lock()
		clock = clock.Add(d)
		mu.Unlock()
	}

	var waits []time.Duration
	lim := &requestLimiter{
		interval: 100 * time.Millisecond,
		now:      nowFn,
		sleep: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			advance(d)
			return nil
		},
	}

	ctx := context.Background()
	if err := lim.Wait(ctx); err != nil {
		t.Fatalf("first Wait: %v", err)
	}
	if err := lim.Wait(ctx); err != nil {
		t.Fatalf("second Wait: %v", err)
	}
	if err := lim.Wait(ctx); err != nil {
		t.Fatalf("third Wait: %v", err)
	}

	if len(waits) != 2 {
		t.Fatalf("sleeps = %d, want 2 (no wait before first request)", len(waits))
	}
	for i, w := range waits {
		if w < 100*time.Millisecond {
			t.Errorf("sleep[%d] = %v, want >= 100ms", i, w)
		}
	}
}

func TestRequestLimiter_RPSOverridesDelay(t *testing.T) {
	t.Parallel()

	got := requestInterval(Options{
		Delay: 500 * time.Millisecond,
		RPS:   10,
	})
	want := 100 * time.Millisecond
	if got != want {
		t.Errorf("interval = %v, want %v (from RPS)", got, want)
	}
}

func TestRequestLimiter_contextCancelDuringWait(t *testing.T) {
	t.Parallel()

	lim := &requestLimiter{
		interval: time.Second,
		now:      time.Now,
		last:     time.Now(),
		sleep: func(ctx context.Context, d time.Duration) error {
			_ = d
			<-ctx.Done()
			return ctx.Err()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := lim.Wait(ctx); err == nil {
		t.Fatal("Wait() error = nil, want context error")
	}
}
