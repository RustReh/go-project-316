package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCrawl_manyLinksDoesNotDeadlockWithFewWorkers(t *testing.T) {
	t.Parallel()

	var links strings.Builder
	links.WriteString(`<!DOCTYPE html><html><body>`)
	for i := 0; i < 64; i++ {
		_, _ = fmt.Fprintf(&links, `<a href="/p%d">p%d</a>`, i, i)
	}
	links.WriteString(`</body></html>`)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(links.String()))
	})
	for i := 0; i < 64; i++ {
		path := fmt.Sprintf("/p%d", i)
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>ok</body></html>`))
		})
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	root, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatalf("parse root: %v", err)
	}

	pages := crawl(context.Background(), Options{
		Concurrency: 2,
		Depth:       2,
		HTTPClient:  srv.Client(),
	}, root, newResourceCache())
	if len(pages) != 65 {
		t.Fatalf("pages len = %d, want 65", len(pages))
	}
}
