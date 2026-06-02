package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
)

type resourceResult struct {
	StatusCode int
	SizeBytes  int64
	Error      string
}

type resourceCache struct {
	mu sync.Mutex
	m  map[string]resourceResult
}

func newResourceCache() *resourceCache {
	return &resourceCache{m: make(map[string]resourceResult)}
}

func (c *resourceCache) GetOrFetch(ctx context.Context, opts Options, url string) resourceResult {
	c.mu.Lock()
	res, ok := c.m[url]
	c.mu.Unlock()
	if ok {
		return res
	}

	res = fetchResource(ctx, opts, url)
	c.mu.Lock()
	c.m[url] = res
	c.mu.Unlock()
	return res
}

func fetchResource(ctx context.Context, opts Options, url string) resourceResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return resourceResult{Error: err.Error()}
	}
	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := doRequestWithRetry(ctx, opts, req)
	if err != nil {
		if ctx.Err() != nil {
			return resourceResult{Error: ctx.Err().Error()}
		}
		return resourceResult{Error: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)

	var size int64
	var sizeErr string
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		n, err := strconv.ParseInt(cl, 10, 64)
		if err != nil || n < 0 {
			sizeErr = fmt.Sprintf("invalid Content-Length: %q", cl)
		} else {
			size = n
		}
	} else if readErr == nil {
		size = int64(len(body))
	}

	out := resourceResult{
		StatusCode: resp.StatusCode,
		SizeBytes:  size,
		Error:      "",
	}

	if readErr != nil {
		out.SizeBytes = 0
		out.Error = readErr.Error()
		return out
	}

	if resp.StatusCode >= 400 {
		out.Error = resp.Status
	}
	if out.Error == "" && sizeErr != "" {
		out.SizeBytes = 0
		out.Error = sizeErr
	}

	return out
}
