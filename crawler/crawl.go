package crawler

import (
	"context"
	"net/url"
)

type crawlTask struct {
	url   string
	depth int
}

func crawl(ctx context.Context, opts Options, root *url.URL) []pageEntry {
	rootURL := normalizePageURL(root)
	rootHost := root.Host

	queue := []crawlTask{{url: rootURL, depth: 0}}
	visited := map[string]struct{}{rootURL: {}}
	var pages []pageEntry

	for len(queue) > 0 {
		if ctx.Err() != nil && len(pages) > 0 {
			break
		}

		task := queue[0]
		queue = queue[1:]

		page, internalLinks := fetchPage(ctx, opts, task.url, task.depth, rootHost)
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
