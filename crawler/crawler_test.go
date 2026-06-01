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
	URL        string `json:"url"`
	Depth      int    `json:"depth"`
	HTTPStatus int    `json:"http_status"`
	Status     string `json:"status"`
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
