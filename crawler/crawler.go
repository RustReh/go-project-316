package crawler

import (
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

// Analyze fetches the root URL and returns a JSON crawl report.
func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	if opts.HTTPClient == nil {
		return nil, fmt.Errorf("HTTPClient is required")
	}

	rootURL, err := url.Parse(opts.URL)
	if err != nil || rootURL.Scheme == "" || rootURL.Host == "" {
		return nil, fmt.Errorf("invalid URL: %q", opts.URL)
	}

	normalized := rootURL.String()
	page := fetchPage(ctx, opts, normalized, 0)

	rep := report{
		RootURL:     normalized,
		Depth:       opts.Depth,
		GeneratedAt: time.Now().UTC(),
		Pages:       []pageEntry{page},
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

func fetchPage(ctx context.Context, opts Options, pageURL string, depth int) pageEntry {
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
		return entry
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	var resp *http.Response
	for attempt := 0; attempt <= opts.Retries; attempt++ {
		if attempt > 0 && opts.Delay > 0 {
			select {
			case <-ctx.Done():
				entry.Status = pageStatusError
				entry.Error = ctx.Err().Error()
				return entry
			case <-time.After(opts.Delay):
			}
		}

		resp, err = opts.HTTPClient.Do(req)
		if err == nil {
			break
		}
	}
	if err != nil {
		entry.Status = pageStatusError
		entry.Error = err.Error()
		return entry
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	entry.HTTPStatus = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		entry.Status = pageStatusOK
	} else {
		entry.Status = pageStatusError
		entry.Error = resp.Status
	}

	return entry
}
