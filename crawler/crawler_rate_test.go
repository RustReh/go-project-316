package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"code/crawler"
)

func TestAnalyze_rateLimit_delayBetweenRequests(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var timestamps []time.Time

	site := newTestSite(t, map[string]string{
		"/":  `<!DOCTYPE html><html><body><a href="/a">a</a><a href="/b">b</a></body></html>`,
		"/a": `<!DOCTYPE html><html><body><p>a</p></body></html>`,
		"/b": `<!DOCTYPE html><html><body><p>b</p></body></html>`,
	})

	client := site.client
	base := client.Transport
	client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		mu.Unlock()
		return base.RoundTrip(req)
	})

	const delay = 80 * time.Millisecond
	start := time.Now()
	rep := analyzeWithDepthAndClient(t, site.rootURL, 10, delay, 0, client)
	elapsed := time.Since(start)

	if len(rep.Pages) != 3 {
		t.Fatalf("pages len = %d, want 3", len(rep.Pages))
	}
	if elapsed < 2*delay {
		t.Errorf("elapsed = %v, want at least %v for 3 spaced requests", elapsed, 2*delay)
	}

	mu.Lock()
	defer mu.Unlock()
	for i := 1; i < len(timestamps); i++ {
		gap := timestamps[i].Sub(timestamps[i-1])
		if gap+5*time.Millisecond < delay {
			t.Errorf("gap[%d] = %v, want >= %v", i, gap, delay)
		}
	}
}

func TestAnalyze_rateLimit_unlimitedIsFast(t *testing.T) {
	t.Parallel()

	site := newTestSite(t, map[string]string{
		"/":  `<!DOCTYPE html><html><body><a href="/a">a</a></body></html>`,
		"/a": `<!DOCTYPE html><html><body><p>a</p></body></html>`,
	})

	start := time.Now()
	rep := analyzeWithDepthAndClient(t, site.rootURL, 10, 0, 0, site.client)
	elapsed := time.Since(start)

	if len(rep.Pages) != 2 {
		t.Fatalf("pages len = %d, want 2", len(rep.Pages))
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("elapsed = %v, expected no artificial delay (< 500ms)", elapsed)
	}
}

func TestAnalyze_rateLimit_contextCancelDuringWait(t *testing.T) {
	t.Parallel()

	requests := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case requests <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><a href="/next">next</a></body></html>`))
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>next</body></html>`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var analyzeErr error
	go func() {
		defer close(done)
		_, analyzeErr = crawler.Analyze(ctx, crawler.Options{
			URL:        srv.URL + "/",
			Depth:      10,
			Delay:      5 * time.Second,
			HTTPClient: srv.Client(),
		})
	}()

	select {
	case <-requests:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first request")
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Analyze did not return promptly after context cancel during rate wait")
	}

	if analyzeErr != nil {
		t.Fatalf("Analyze() error = %v, want nil with partial report", analyzeErr)
	}
}

func analyzeWithDepthAndClient(
	t *testing.T,
	startURL string,
	depth int,
	delay time.Duration,
	rps float64,
	client *http.Client,
) crawlReport {
	t.Helper()

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        startURL,
		Depth:      depth,
		Delay:      delay,
		RPS:        rps,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	return decodeReport(t, data)
}
