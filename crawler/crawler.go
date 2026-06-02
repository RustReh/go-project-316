package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	pageStatusOK    = "ok"
	pageStatusError = "error"
)

// Analyze fetches pages up to the configured depth and returns a JSON crawl report.
func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	if opts.HTTPClient == nil {
		return nil, fmt.Errorf("HTTPClient is required")
	}

	rootURL, err := url.Parse(opts.URL)
	if err != nil || rootURL.Scheme == "" || rootURL.Host == "" {
		return nil, fmt.Errorf("invalid URL: %q", opts.URL)
	}

	maxDepth := opts.Depth
	if maxDepth <= 0 {
		maxDepth = 1
	}
	opts.Depth = maxDepth
	opts.requestLimiter = newRequestLimiter(opts)

	normalized := normalizePageURL(rootURL)
	pages := crawl(ctx, opts, rootURL)

	rep := report{
		RootURL:     normalized,
		Depth:       maxDepth,
		GeneratedAt: time.Now().UTC(),
		Pages:       pages,
	}

	var data []byte
	if opts.IndentJSON {
		data, err = json.MarshalIndent(rep, "", "  ")
	} else {
		data, err = json.Marshal(rep)
	}
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}

	return data, nil
}

func fetchPage(ctx context.Context, opts Options, pageURL string, depth int, rootHost string) (pageEntry, []string) {
	entry := pageEntry{
		URL:   pageURL,
		Depth: depth,
	}

	reqCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, pageURL, nil)
	if err != nil {
		entry.Status = pageStatusError
		entry.Error = err.Error()
		entry.DiscoveredAt = time.Now().UTC()
		return entry, nil
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	var resp *http.Response
	for attempt := 0; attempt <= opts.Retries; attempt++ {
		resp, err = doRequest(ctx, opts, req)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			entry.Status = pageStatusError
			entry.Error = ctx.Err().Error()
			entry.DiscoveredAt = time.Now().UTC()
			return entry, nil
		}
	}
	if err != nil {
		entry.Status = pageStatusError
		entry.Error = err.Error()
		entry.DiscoveredAt = time.Now().UTC()
		return entry, nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		entry.Status = pageStatusError
		entry.Error = err.Error()
		entry.HTTPStatus = resp.StatusCode
		entry.DiscoveredAt = time.Now().UTC()
		return entry, nil
	}

	entry.HTTPStatus = resp.StatusCode
	entry.DiscoveredAt = time.Now().UTC()
	entry.SEO = extractSEO(body)

	var internalLinks []string
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		entry.Status = pageStatusOK
		entry.BrokenLinks = findBrokenLinks(reqCtx, opts, pageURL, body)
		internalLinks, _ = extractInternalLinks(pageURL, body, rootHost)
	} else {
		entry.Status = pageStatusError
		entry.Error = resp.Status
	}

	return entry, internalLinks
}

func findBrokenLinks(ctx context.Context, opts Options, pageURL string, body []byte) []brokenLink {
	linkURLs, err := extractCheckableLinks(pageURL, bytes.NewReader(body))
	if err != nil || len(linkURLs) == 0 {
		return nil
	}

	var broken []brokenLink
	for _, linkURL := range linkURLs {
		if bl := checkLink(ctx, opts, linkURL); bl != nil {
			broken = append(broken, *bl)
		}
	}

	return broken
}

func checkLink(ctx context.Context, opts Options, linkURL string) *brokenLink {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, linkURL, nil)
	if err != nil {
		return &brokenLink{URL: linkURL, Error: err.Error()}
	}
	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := doRequest(ctx, opts, req)
	if err != nil {
		if ctx.Err() != nil {
			return &brokenLink{URL: linkURL, Error: ctx.Err().Error()}
		}
		return &brokenLink{URL: linkURL, Error: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return &brokenLink{URL: linkURL, StatusCode: resp.StatusCode}
	}

	return nil
}
