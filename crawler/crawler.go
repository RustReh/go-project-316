package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
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

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	maxDepth := opts.Depth
	if maxDepth <= 0 {
		maxDepth = 1
	}
	opts.Depth = maxDepth
	opts.requestLimiter = newRequestLimiter(opts)

	normalized := normalizePageURL(rootURL)
	cache := newResourceCache()
	pages := crawl(ctx, opts, rootURL, cache)
	sort.Slice(pages, func(i, j int) bool { return pages[i].URL < pages[j].URL })

	rep := Report{
		RootURL:     normalized,
		Depth:       maxDepth,
		GeneratedAt: nowFn().UTC(),
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

func fetchPage(ctx context.Context, opts Options, pageURL string, depth int, rootHost string, rootScheme string, cache *resourceCache) (Page, []string) {
	entry := Page{
		URL:   pageURL,
		Depth: depth,
		SEO:   SEO{},
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
		entry.DiscoveredAt = reportTime(opts).UTC()
		return entry, nil
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := doRequestWithRetry(ctx, opts, req)
	if err != nil {
		entry.Status = pageStatusError
		if ctx.Err() != nil {
			entry.Error = ctx.Err().Error()
		} else {
			entry.Error = err.Error()
		}
		entry.DiscoveredAt = reportTime(opts).UTC()
		return entry, nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		entry.Status = pageStatusError
		entry.Error = err.Error()
		entry.HTTPStatus = resp.StatusCode
		entry.DiscoveredAt = reportTime(opts).UTC()
		return entry, nil
	}

	entry.HTTPStatus = resp.StatusCode
	entry.DiscoveredAt = reportTime(opts).UTC()
	entry.SEO = extractSEO(body)

	var internalLinks []string
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		entry.Status = pageStatusOK
		// For successful page fetch, JSON fixtures expect arrays (even when empty).
		entry.BrokenLinks = []BrokenLink{}
		entry.Assets = []Asset{}
		entry.BrokenLinks = findBrokenLinks(reqCtx, opts, pageURL, body, cache, rootHost, rootScheme)
		entry.Assets = findAssets(reqCtx, opts, pageURL, body, cache)
		internalLinks, _ = extractInternalLinks(pageURL, body, rootHost)
	} else {
		entry.Status = pageStatusError
		entry.Error = resp.Status
	}
	// On error pages (network/HTTP), BrokenLinks/Assets stay nil and marshal as null (fixture behavior).

	return entry, internalLinks
}

func findBrokenLinks(ctx context.Context, opts Options, pageURL string, body []byte, cache *resourceCache, rootHost string, rootScheme string) []BrokenLink {
	linkURLs, err := extractCheckableLinks(pageURL, bytes.NewReader(body))
	if err != nil || len(linkURLs) == 0 {
		return []BrokenLink{}
	}

	var broken []BrokenLink
	for _, linkURL := range linkURLs {
		u, err := url.Parse(linkURL)
		if err != nil || !sameHost(u.Host, rootHost) || !strings.EqualFold(u.Scheme, rootScheme) {
			continue
		}
		if !isLikelyPageLink(u) {
			continue
		}
		if bl := checkLink(ctx, opts, linkURL, cache); bl != nil {
			broken = append(broken, *bl)
		}
	}

	if broken == nil {
		return []BrokenLink{}
	}
	return broken
}

func isLikelyPageLink(u *url.URL) bool {
	// Assets are handled separately in the `assets` section; broken_links focuses on navigational links.
	ext := path.Ext(u.Path)
	if ext == "" || ext == "/" {
		return true
	}
	switch ext {
	case ".html", ".htm", ".xml":
		return true
	default:
		return false
	}
}

func checkLink(ctx context.Context, opts Options, linkURL string, cache *resourceCache) *BrokenLink {
	res := cache.GetOrFetch(ctx, opts, linkURL)

	if res.Error != "" && res.StatusCode == 0 {
		return &BrokenLink{URL: linkURL, StatusCode: 0, Error: res.Error}
	}
	if res.StatusCode >= 400 {
		return &BrokenLink{URL: linkURL, StatusCode: res.StatusCode, Error: res.Error}
	}
	return nil
}

func findAssets(ctx context.Context, opts Options, pageURL string, body []byte, cache *resourceCache) []Asset {
	assets, err := extractAssets(pageURL, body)
	if err != nil || len(assets) == 0 {
		return []Asset{}
	}

	out := make([]Asset, 0, len(assets))
	for _, a := range assets {
		res := cache.GetOrFetch(ctx, opts, a.URL)
		out = append(out, Asset{
			URL:        a.URL,
			Type:       a.Type,
			StatusCode: res.StatusCode,
			SizeBytes:  res.SizeBytes,
			Error:      res.Error,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		pi := assetTypePriority(out[i].Type)
		pj := assetTypePriority(out[j].Type)
		if pi != pj {
			return pi < pj
		}
		return out[i].URL < out[j].URL
	})
	return out
}

func assetTypePriority(t string) int {
	switch t {
	case "image":
		return 1
	case "script":
		return 2
	case "style":
		return 3
	default:
		return 4
	}
}

func reportTime(opts Options) time.Time {
	if opts.Now != nil {
		return opts.Now()
	}
	return time.Now()
}
