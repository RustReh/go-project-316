package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnalyze_success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	data, err := Analyze(context.Background(), Options{
		URL:        server.URL,
		Depth:      1,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	var rep report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if rep.RootURL != server.URL {
		t.Errorf("root_url = %q, want %q", rep.RootURL, server.URL)
	}
	if len(rep.Pages) != 1 {
		t.Fatalf("pages len = %d, want 1", len(rep.Pages))
	}
	if rep.Pages[0].HTTPStatus != http.StatusOK {
		t.Errorf("http_status = %d, want 200", rep.Pages[0].HTTPStatus)
	}
	if rep.Pages[0].Status != pageStatusOK {
		t.Errorf("status = %q, want ok", rep.Pages[0].Status)
	}
}

func TestAnalyze_networkError(t *testing.T) {
	t.Parallel()

	data, err := Analyze(context.Background(), Options{
		URL:        "https://example.invalid",
		Depth:      1,
		Timeout:    50,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	var rep report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if len(rep.Pages) != 1 {
		t.Fatalf("pages len = %d, want 1", len(rep.Pages))
	}
	if rep.Pages[0].Status != pageStatusError {
		t.Errorf("status = %q, want error", rep.Pages[0].Status)
	}
}

func TestAnalyze_requiresHTTPClient(t *testing.T) {
	t.Parallel()

	_, err := Analyze(context.Background(), Options{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for nil HTTPClient")
	}
}
