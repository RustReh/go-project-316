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
	var submitWG sync.WaitGroup

	queue := make(chan crawlTask, workers*4)
	submit := make(chan crawlTask, workers*16)

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

	go func() {
		for task := range submit {
			queue <- task
		}
		close(queue)
	}()

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

					var childTasks []crawlTask
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

						childTasks = append(childTasks, crawlTask{url: link, depth: nextDepth})
					}

					for range childTasks {
						pending.Add(1)
					}
					submitTasks(ctx, &pending, &submitWG, submit, childTasks, shouldStop)
				}()
			}
		}(queue)
	}

	go func() {
		pending.Wait()
		submitWG.Wait()
		close(submit)
	}()

	wg.Wait()
	return pages
}

// submitTasks enqueues discovered pages without blocking the worker that found them.
// Workers must not block on the main task queue: when it is full, every worker can
// end up waiting to send and nobody reads — deadlock on large sites like go.dev.
func submitTasks(ctx context.Context, pending, submitWG *sync.WaitGroup, submit chan<- crawlTask, tasks []crawlTask, shouldStop func() bool) {
	if len(tasks) == 0 {
		return
	}

	submitWG.Add(1)
	go func() {
		defer submitWG.Done()

		for _, task := range tasks {
			if shouldStop() {
				pending.Done()
				continue
			}

			select {
			case submit <- task:
			case <-ctx.Done():
				pending.Done()
				return
			}
		}
	}()
}
