package crawler

import (
	"context"
	"net/url"
)

type crawlTask struct {
	url   string
	depth int
}

func crawl(ctx context.Context, opts Options, root *url.URL, cache *resourceCache) []Page {
	rootURL := normalizePageURL(root)
	rootHost := root.Host
	rootScheme := root.Scheme

	queue := []crawlTask{{url: rootURL, depth: 0}}
	visited := map[string]struct{}{rootURL: {}}
	var pages []Page

	for len(queue) > 0 {
		if ctx.Err() != nil && len(pages) > 0 {
			break
		}

		task := queue[0]
		queue = queue[1:]

		page, internalLinks := fetchPage(ctx, opts, task.url, task.depth, rootHost, rootScheme, cache)
		pages = append(pages, page)

		nextDepth := task.depth + 1
		if nextDepth >= opts.Depth {
			continue
		}

		for _, link := range internalLinks {
			if _, seen := visited[link]; seen {
				continue
			}
			visited[link] = struct{}{}
			queue = append(queue, crawlTask{url: link, depth: nextDepth})
		}
	}

	return pages
}
