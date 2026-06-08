package crawler

import (
	"context"
	"net/url"
	"sync"
)

type crawlTask struct {
	url   string
	depth int
}

func crawl(ctx context.Context, opts Options, root *url.URL, cache *resourceCache) []Page {
	rootURL := normalizePageURL(root)
	rootHost := root.Host
	rootScheme := root.Scheme

	workers := opts.Concurrency
	if workers == 0 {
		workers = 4
	}

	var pending sync.WaitGroup
	queue := make(chan crawlTask, workers*4)
	pending.Add(1)
	queue <- crawlTask{url: rootURL, depth: 0}

	var wg sync.WaitGroup
	wg.Add(workers)

	var visitedMU sync.Mutex
	visited := map[string]struct{}{rootURL: {}}

	var pagesMU sync.Mutex
	pages := make([]Page, 0)

	shouldStop := func() bool {
		if ctx.Err() == nil {
			return false
		}
		pagesMU.Lock()
		defer pagesMU.Unlock()
		return len(pages) > 0
	}

	for range workers {
		go func(tasks <-chan crawlTask) {
			defer wg.Done()
			for task := range tasks {
				func() {
					defer pending.Done()

					page, internalLinks := fetchPage(ctx, opts, task.url, task.depth, rootHost, rootScheme, cache)
					pagesMU.Lock()
					pages = append(pages, page)
					pagesMU.Unlock()

					nextDepth := task.depth + 1
					if nextDepth >= opts.Depth || shouldStop() {
						return
					}

					for _, link := range internalLinks {
						if shouldStop() {
							break
						}

						visitedMU.Lock()
						if _, seen := visited[link]; seen {
							visitedMU.Unlock()
							continue
						}
						visited[link] = struct{}{}
						visitedMU.Unlock()

						pending.Add(1)
						queue <- crawlTask{url: link, depth: nextDepth}
					}
				}()
			}
		}(queue)
	}

	go func() {
		pending.Wait()
		close(queue)
	}()

	wg.Wait()
	return pages
}
