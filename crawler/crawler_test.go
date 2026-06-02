package crawler_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code/crawler"
)

// Black-box JSON shapes — only public contract of Analyze(), no internal types.
type crawlReport struct {
	RootURL     string      `json:"root_url"`
	Depth       int         `json:"depth"`
	GeneratedAt string      `json:"generated_at"`
	Pages       []crawlPage `json:"pages"`
}

type crawlPage struct {
	URL          string          `json:"url"`
	Depth        int             `json:"depth"`
	HTTPStatus   int             `json:"http_status"`
	Status       string          `json:"status"`
	Error        string          `json:"error"`
	BrokenLinks  []brokenLinkOut `json:"broken_links"`
	Assets       []assetOut      `json:"assets"`
	DiscoveredAt string          `json:"discovered_at"`
	SEO          seoOut          `json:"seo"`
}

type seoOut struct {
	HasTitle       bool   `json:"has_title"`
	Title          string `json:"title"`
	HasDescription bool   `json:"has_description"`
	Description    string `json:"description"`
	HasH1          bool   `json:"has_h1"`
}

type brokenLinkOut struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
}

type assetOut struct {
	URL        string `json:"url"`
	Type       string `json:"type"`
	StatusCode int    `json:"status_code"`
	SizeBytes  int64  `json:"size_bytes"`
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

	wantRoot := srv.URL + "/"
	if rep.RootURL != wantRoot {
		t.Errorf("root_url = %q, want %q", rep.RootURL, wantRoot)
	}
	if page.URL != wantRoot {
		t.Errorf("page.url = %q, want %q", page.URL, wantRoot)
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
  <a href="/ok.css">ok</a>
  <a href="/missing.css">missing</a>
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
	if bl.Error == "" {
		t.Error("error is empty, want HTTP status text for 404")
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

func TestAnalyze_SEO_allTagsPresent(t *testing.T) {
	t.Parallel()

	const pageHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Example Test</title>
  <meta name="description" content="Short summary">
</head>
<body><h1>Main Heading</h1></body>
</html>`

	page := analyzeHTMLPage(t, pageHTML)

	if !page.SEO.HasTitle || page.SEO.Title != "Example Test" {
		t.Errorf("title: has=%v text=%q, want has=true text=%q", page.SEO.HasTitle, page.SEO.Title, "Example Test")
	}
	if !page.SEO.HasDescription || page.SEO.Description != "Short summary" {
		t.Errorf("description: has=%v text=%q, want has=true text=%q",
			page.SEO.HasDescription, page.SEO.Description, "Short summary")
	}
	if !page.SEO.HasH1 {
		t.Errorf("has_h1 = false, want true")
	}
}

func TestAnalyze_SEO_tagsAbsent(t *testing.T) {
	t.Parallel()

	const pageHTML = `<!DOCTYPE html><html><body><p>no seo tags</p></body></html>`

	page := analyzeHTMLPage(t, pageHTML)

	if page.SEO.HasTitle || page.SEO.Title != "" {
		t.Errorf("title: has=%v text=%q, want has=false text empty", page.SEO.HasTitle, page.SEO.Title)
	}
	if page.SEO.HasDescription || page.SEO.Description != "" {
		t.Errorf("description: has=%v text=%q, want has=false text empty",
			page.SEO.HasDescription, page.SEO.Description)
	}
	if page.SEO.HasH1 {
		t.Errorf("has_h1 = true, want false")
	}
}

func TestAnalyze_SEO_decodesHTMLEntities(t *testing.T) {
	t.Parallel()

	const pageHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Tom &amp; Jerry</title>
  <meta name="description" content="A &amp; B">
</head>
<body><h1>Hello &amp; World</h1></body>
</html>`

	page := analyzeHTMLPage(t, pageHTML)

	if page.SEO.Title != "Tom & Jerry" {
		t.Errorf("title = %q, want %q", page.SEO.Title, "Tom & Jerry")
	}
	if page.SEO.Description != "A & B" {
		t.Errorf("description = %q, want %q", page.SEO.Description, "A & B")
	}
}

func analyzeHTMLPage(t *testing.T, htmlDoc string) crawlPage {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlDoc))
	}))
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	return decodeReport(t, data).Pages[0]
}

func TestAnalyze_depthLimit_onlyRoot(t *testing.T) {
	t.Parallel()

	site := newTestSite(t, map[string]string{
		"/": `<!DOCTYPE html><html><body>
			<a href="/child">child</a>
		</body></html>`,
		"/child": `<!DOCTYPE html><html><body><p>child</p></body></html>`,
	})

	rep := analyzeWithDepth(t, site, site.rootURL, 1)

	if len(rep.Pages) != 1 {
		t.Fatalf("pages len = %d, want 1 (only root at depth limit 1)", len(rep.Pages))
	}
	if rep.Pages[0].Depth != 0 {
		t.Errorf("root depth = %d, want 0", rep.Pages[0].Depth)
	}
}

func TestAnalyze_depthLimit_includesFirstLevel(t *testing.T) {
	t.Parallel()

	site := newTestSite(t, map[string]string{
		"/": `<!DOCTYPE html><html><body>
			<a href="/one">one</a>
			<a href="/two">two</a>
		</body></html>`,
		"/one":  `<!DOCTYPE html><html><body><a href="/deep">deep</a></body></html>`,
		"/two":  `<!DOCTYPE html><html><body><p>two</p></body></html>`,
		"/deep": `<!DOCTYPE html><html><body><p>deep</p></body></html>`,
	})

	rep := analyzeWithDepth(t, site, site.rootURL, 2)

	if len(rep.Pages) != 3 {
		t.Fatalf("pages len = %d, want 3 (root + two children)", len(rep.Pages))
	}

	depths := pageDepths(rep.Pages)
	oneURL := joinURL(site.rootURL, "one")
	twoURL := joinURL(site.rootURL, "two")
	deepURL := joinURL(site.rootURL, "deep")
	if depths[oneURL] != 1 || depths[twoURL] != 1 {
		t.Errorf("child depths = %v, want depth 1 for %q and %q", depths, oneURL, twoURL)
	}
	if _, crawled := depths[deepURL]; crawled {
		t.Errorf("page %q must not be crawled when depth limit is 2", deepURL)
	}
}

func TestAnalyze_externalLinkNotInPages(t *testing.T) {
	t.Parallel()

	external := "https://other.example.com/page"

	site := newTestSite(t, map[string]string{
		"/": `<!DOCTYPE html><html><body>
			<a href="/inside">inside</a>
			<a href="/also">also</a>
			<a href="` + external + `">outside</a>
		</body></html>`,
		"/inside": `<!DOCTYPE html><html><body><p>inside</p></body></html>`,
		"/also":   `<!DOCTYPE html><html><body><p>also</p></body></html>`,
	})

	rep := analyzeWithDepth(t, site, site.rootURL, 10)

	for _, page := range rep.Pages {
		if strings.Contains(page.URL, "other.example.com") {
			t.Errorf("external URL %q must not appear in pages", page.URL)
		}
	}
	if len(rep.Pages) != 3 {
		t.Fatalf("pages len = %d, want 3 internal pages", len(rep.Pages))
	}
}

func TestAnalyze_duplicateInternalLinksCrawledOnce(t *testing.T) {
	t.Parallel()

	site := newTestSite(t, map[string]string{
		"/": `<!DOCTYPE html><html><body>
			<a href="/dup">first</a>
			<a href="/dup">second</a>
		</body></html>`,
		"/dup": `<!DOCTYPE html><html><body><p>dup</p></body></html>`,
	})

	rep := analyzeWithDepth(t, site, site.rootURL, 10)

	dupCount := 0
	for _, page := range rep.Pages {
		if strings.HasSuffix(page.URL, "/dup") {
			dupCount++
		}
	}
	if dupCount != 1 {
		t.Fatalf("/dup appeared %d times in pages, want 1", dupCount)
	}
}

func TestAnalyze_contextCancellation_returnsPartialReport(t *testing.T) {
	t.Parallel()

	secondStarted := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><a href="/second">second</a></body></html>`))
	})
	mux.HandleFunc("/second", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-secondStarted:
		default:
			close(secondStarted)
		}
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>second</body></html>`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	site := &testSite{rootURL: srv.URL + "/", client: srv.Client()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-secondStarted
		cancel()
	}()

	data, err := crawler.Analyze(ctx, crawler.Options{
		URL:        site.rootURL,
		Depth:      10,
		HTTPClient: site.client,
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil with partial report", err)
	}

	var rep crawlReport
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if len(rep.Pages) < 1 {
		t.Fatal("expected at least root page in partial report")
	}
	if len(rep.Pages) > 2 {
		t.Fatalf("pages len = %d, want at most 2 after cancellation", len(rep.Pages))
	}
}

type testSite struct {
	rootURL string
	client  *http.Client
	mux     *http.ServeMux
	server  *httptest.Server
}

func newTestSite(t *testing.T, pages map[string]string) *testSite {
	t.Helper()

	mux := http.NewServeMux()
	for path, htmlDoc := range pages {
		path := path
		htmlDoc := htmlDoc
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(htmlDoc))
		})
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testSite{
		rootURL: srv.URL + "/",
		client:  srv.Client(),
		mux:     mux,
		server:  srv,
	}
}

func analyzeWithDepth(t *testing.T, site *testSite, startURL string, depth int) crawlReport {
	t.Helper()

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        startURL,
		Depth:      depth,
		HTTPClient: site.client,
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	return decodeReport(t, data)
}

func joinURL(base, path string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(path, "/")
}

func pageDepths(pages []crawlPage) map[string]int {
	out := make(map[string]int, len(pages))
	for _, p := range pages {
		out[p.URL] = p.Depth
	}
	return out
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

func TestAnalyze_retries_stopsAfterRetriesPlusOne(t *testing.T) {
	t.Parallel()

	var calls int
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 Internal Server Error",
				Body:       io.NopCloser(strings.NewReader("<html></html>")),
				Request:    req,
			}, nil
		}),
	}

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        "https://example.com",
		Depth:      1,
		Retries:    2,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil (error goes into report)", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (retries + 1)", calls)
	}

	page := decodeReport(t, data).Pages[0]
	if page.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("http_status = %d, want 500", page.HTTPStatus)
	}
	if page.Status != "error" {
		t.Errorf("status = %q, want error", page.Status)
	}
}

func TestAnalyze_retries_succeedsOnSecondAttempt(t *testing.T) {
	t.Parallel()

	var calls int
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			code := http.StatusOK
			status := "200 OK"
			if calls == 1 {
				code = http.StatusInternalServerError
				status = "500 Internal Server Error"
			}
			return &http.Response{
				StatusCode: code,
				Status:     status,
				Body:       io.NopCloser(strings.NewReader("<!DOCTYPE html><html><head><title>x</title></head><body></body></html>")),
				Request:    req,
			}, nil
		}),
	}

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        "https://example.com",
		Depth:      1,
		Retries:    1,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (one retry)", calls)
	}

	page := decodeReport(t, data).Pages[0]
	if page.HTTPStatus != http.StatusOK {
		t.Errorf("http_status = %d, want 200", page.HTTPStatus)
	}
	if page.Status != "ok" {
		t.Errorf("status = %q, want ok", page.Status)
	}
}

func TestAnalyze_assets_cachedAcrossPages_singleRequest(t *testing.T) {
	t.Parallel()

	var assetCalls int

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>
			<a href="/p2">p2</a>
			<script src="/app.js"></script>
		</body></html>`))
	})
	mux.HandleFunc("/p2", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>
			<script src="/app.js"></script>
		</body></html>`))
	})
	mux.HandleFunc("/app.js", func(w http.ResponseWriter, _ *http.Request) {
		assetCalls++
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Content-Length", "4")
		_, _ = w.Write([]byte("abcd"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL + "/",
		Depth:      3,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	rep := decodeReport(t, data)
	if len(rep.Pages) != 2 {
		t.Fatalf("pages len = %d, want 2", len(rep.Pages))
	}
	if assetCalls != 1 {
		t.Fatalf("assetCalls = %d, want 1 (cached by URL)", assetCalls)
	}

	for _, p := range rep.Pages {
		if len(p.Assets) != 1 {
			t.Fatalf("page %q assets len = %d, want 1", p.URL, len(p.Assets))
		}
		a := p.Assets[0]
		if !strings.HasSuffix(a.URL, "/app.js") {
			t.Fatalf("asset url = %q, want /app.js", a.URL)
		}
		if a.Type != "script" {
			t.Errorf("asset type = %q, want script", a.Type)
		}
		if a.StatusCode != http.StatusOK {
			t.Errorf("asset status_code = %d, want 200", a.StatusCode)
		}
		if a.SizeBytes != 4 {
			t.Errorf("asset size_bytes = %d, want 4", a.SizeBytes)
		}
		if a.Error != "" {
			t.Errorf("asset error = %q, want empty", a.Error)
		}
	}
}

func TestAnalyze_assets_sizeFromBodyWhenNoContentLength(t *testing.T) {
	t.Parallel()

	const css = "body{color:red}"

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head>
			<link rel="stylesheet" href="/nolength.css">
		</head><body></body></html>`))
	})
	mux.HandleFunc("/nolength.css", func(w http.ResponseWriter, _ *http.Request) {
		// Do not set Content-Length: should be computed from body.
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte(css))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL + "/",
		Depth:      2,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	page := decodeReport(t, data).Pages[0]
	if len(page.Assets) != 1 {
		t.Fatalf("assets len = %d, want 1", len(page.Assets))
	}
	a := page.Assets[0]
	if a.Type != "style" {
		t.Errorf("type = %q, want style", a.Type)
	}
	if a.StatusCode != http.StatusOK {
		t.Errorf("status_code = %d, want 200", a.StatusCode)
	}
	if a.SizeBytes != int64(len(css)) {
		t.Errorf("size_bytes = %d, want %d", a.SizeBytes, len(css))
	}
	if a.Error != "" {
		t.Errorf("error = %q, want empty", a.Error)
	}
}

func TestAnalyze_assets_errorStatusIncluded(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>
			<img src="/missing.png">
		</body></html>`))
	})
	mux.HandleFunc("/missing.png", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	data, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL + "/",
		Depth:      2,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	page := decodeReport(t, data).Pages[0]
	if len(page.Assets) != 1 {
		t.Fatalf("assets len = %d, want 1", len(page.Assets))
	}
	a := page.Assets[0]
	if a.Type != "image" {
		t.Errorf("type = %q, want image", a.Type)
	}
	if a.StatusCode != http.StatusNotFound {
		t.Errorf("status_code = %d, want 404", a.StatusCode)
	}
	if a.Error == "" {
		t.Error("error is empty, want HTTP status text")
	}
	// size_bytes must be present even on error; value may be 0.
}

func TestAnalyze_JSONMatchesGolden(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2024, 6, 1, 12, 34, 56, 0, time.UTC)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
  <title>Example title</title>
  <meta name="description" content="Example description">
  <link rel="stylesheet" href="/static/app.css">
</head>
<body>
  <h1>Hello</h1>
  <a href="/missing">broken</a>
  <img src="/static/logo.png">
</body>
</html>`))
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	})
	mux.HandleFunc("/static/logo.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "5")
		_, _ = w.Write([]byte("12345"))
	})
	mux.HandleFunc("/static/app.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Content-Length", "2")
		_, _ = w.Write([]byte("/*"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:        srv.URL + "/",
		Depth:      1,
		IndentJSON: true,
		Now:        func() time.Time { return fixed },
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v, want nil", err)
	}

	want := `{
  "root_url": "` + srv.URL + `/",
  "depth": 1,
  "generated_at": "2024-06-01T12:34:56Z",
  "pages": [
    {
      "url": "` + srv.URL + `/",
      "depth": 0,
      "http_status": 200,
      "status": "ok",
      "seo": {
        "has_title": true,
        "title": "Example title",
        "has_description": true,
        "description": "Example description",
        "has_h1": true
      },
      "broken_links": [
        {
          "url": "` + srv.URL + `/missing",
          "status_code": 404,
          "error": "404 Not Found"
        }
      ],
      "assets": [
        {
          "url": "` + srv.URL + `/static/logo.png",
          "type": "image",
          "status_code": 200,
          "size_bytes": 5
        },
        {
          "url": "` + srv.URL + `/static/app.css",
          "type": "style",
          "status_code": 200,
          "size_bytes": 2
        }
      ],
      "discovered_at": "2024-06-01T12:34:56Z"
    }
  ]
}`

	if strings.TrimSpace(string(got)) != strings.TrimSpace(want) {
		t.Fatalf("JSON mismatch\n--- got ---\n%s\n--- want ---\n%s\n", got, want)
	}
}

func TestAnalyze_IndentJSON_changesOnlyFormatting(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2024, 6, 1, 12, 34, 56, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`))
	}))
	t.Cleanup(srv.Close)

	opts := crawler.Options{
		URL:        srv.URL,
		Depth:      1,
		Retries:    0,
		Now:        func() time.Time { return fixed },
		HTTPClient: srv.Client(),
	}

	compact, err := crawler.Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("Analyze(compact) error = %v", err)
	}

	opts.IndentJSON = true
	pretty, err := crawler.Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("Analyze(pretty) error = %v", err)
	}

	var a, b any
	if err := json.Unmarshal(compact, &a); err != nil {
		t.Fatalf("unmarshal compact: %v", err)
	}
	if err := json.Unmarshal(pretty, &b); err != nil {
		t.Fatalf("unmarshal pretty: %v", err)
	}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	if string(aj) != string(bj) {
		t.Fatalf("content differs between compact and pretty JSON")
	}
	if string(compact) == string(pretty) {
		t.Fatalf("expected formatting difference between compact and pretty JSON")
	}
}
