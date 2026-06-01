package crawler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"code/crawler"
)

// Black-box JSON shapes — only public contract of Analyze(), no internal types.
type crawlReport struct {
	RootURL string      `json:"root_url"`
	Depth   int         `json:"depth"`
	Pages   []crawlPage `json:"pages"`
}

type crawlPage struct {
	URL          string          `json:"url"`
	Depth        int             `json:"depth"`
	HTTPStatus   int             `json:"http_status"`
	Status       string          `json:"status"`
	Error        string          `json:"error"`
	BrokenLinks  []brokenLinkOut `json:"broken_links"`
	DiscoveredAt string          `json:"discovered_at"`
}

type brokenLinkOut struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
}

func decodeReport(t *testing.T, data []byte) crawlReport {
	t.Helper()

	var rep crawlReport
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("invalid JSON report: %v\nraw: %s", err, data)
	}
	if len(rep.Pages) == 0 {
		t.Fatal("report has no pages")
	}
	return rep
}

func TestAnalyze_HTTP_200OK(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL,
		Depth:      1,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	rep := decodeReport(t, data)
	page := rep.Pages[0]

	if rep.RootURL != srv.URL {
		t.Errorf("root_url = %q, want %q", rep.RootURL, srv.URL)
	}
	if page.URL != srv.URL {
		t.Errorf("page.url = %q, want %q", page.URL, srv.URL)
	}
	if page.HTTPStatus != http.StatusOK {
		t.Errorf("http_status = %d, want 200", page.HTTPStatus)
	}
	if page.Status != "ok" {
		t.Errorf("status = %q, want ok", page.Status)
	}
	if page.Error != "" {
		t.Errorf("error = %q, want empty", page.Error)
	}
}

func TestAnalyze_HTTP_404(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	page := decodeReport(t, data).Pages[0]

	if page.HTTPStatus != http.StatusNotFound {
		t.Errorf("http_status = %d, want 404", page.HTTPStatus)
	}
	if page.Status != "error" {
		t.Errorf("status = %q, want error", page.Status)
	}
	if page.Error == "" {
		t.Error("error field is empty, want HTTP status text")
	}
}

func TestAnalyze_HTTP_500(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	page := decodeReport(t, data).Pages[0]

	if page.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("http_status = %d, want 500", page.HTTPStatus)
	}
	if page.Status != "error" {
		t.Errorf("status = %q, want error", page.Status)
	}
}

func TestAnalyze_HTTP_timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL,
		Timeout:    20 * time.Millisecond,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	page := decodeReport(t, data).Pages[0]

	if page.Status != "error" {
		t.Errorf("status = %q, want error on timeout", page.Status)
	}
	if page.Error == "" {
		t.Error("error field is empty, want timeout message")
	}
}

func TestAnalyze_HTTP_networkFailure(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}),
	}

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        "https://example.com",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	page := decodeReport(t, data).Pages[0]

	if page.HTTPStatus != 0 {
		t.Errorf("http_status = %d, want 0 when request failed", page.HTTPStatus)
	}
	if page.Status != "error" {
		t.Errorf("status = %q, want error", page.Status)
	}
	if page.Error == "" {
		t.Error("error field is empty, want network error message")
	}
}

func TestAnalyze_missingHTTPClient(t *testing.T) {
	t.Parallel()

	_, err := crawler.Analyze(context.Background(), crawler.Options{
		URL: "https://example.com",
	})
	if err == nil {
		t.Fatal("Analyze() error = nil, want error when HTTPClient is missing")
	}
}

func TestAnalyze_brokenLinks_onlyBrokenReported(t *testing.T) {
	t.Parallel()

	const pageHTML = `<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="/ok.css">
  <link rel="stylesheet" href="/missing.css">
</head>
<body>
  <a href="mailto:noreply@example.com">mail</a>
  <a href="javascript:void(0)">js</a>
  <a href="#">fragment</a>
  <a href="">empty</a>
</body>
</html>`

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(pageHTML))
	})
	mux.HandleFunc("/ok.css", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/missing.css", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL + "/",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	page := decodeReport(t, data).Pages[0]

	if page.Status != "ok" {
		t.Errorf("page status = %q, want ok", page.Status)
	}
	if page.DiscoveredAt == "" {
		t.Error("discovered_at is empty")
	}
	if len(page.BrokenLinks) != 1 {
		t.Fatalf("broken_links len = %d, want 1", len(page.BrokenLinks))
	}

	bl := page.BrokenLinks[0]
	wantURL := srv.URL + "/missing.css"
	if bl.URL != wantURL {
		t.Errorf("broken url = %q, want %q", bl.URL, wantURL)
	}
	if bl.StatusCode != http.StatusNotFound {
		t.Errorf("status_code = %d, want 404", bl.StatusCode)
	}
	if bl.Error != "" {
		t.Errorf("error = %q, want empty for HTTP 404", bl.Error)
	}

	for _, link := range page.BrokenLinks {
		if link.URL == srv.URL+"/ok.css" {
			t.Errorf("working link %q must not be in broken_links", link.URL)
		}
	}
}

func TestAnalyze_brokenLinks_omittedWhenAllOK(t *testing.T) {
	t.Parallel()

	const pageHTML = `<!DOCTYPE html><html><body><a href="/ok.css">ok</a></body></html>`

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(pageHTML))
	})
	mux.HandleFunc("/ok.css", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	page := decodeReport(t, data).Pages[0]
	if len(page.BrokenLinks) != 0 {
		t.Errorf("broken_links = %+v, want empty or omitted", page.BrokenLinks)
	}
}

func TestAnalyze_invalidURL(t *testing.T) {
	t.Parallel()

	_, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        "not-a-valid-url",
		HTTPClient: &http.Client{},
	})
	if err == nil {
		t.Fatal("Analyze() error = nil, want error for invalid URL")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
