package crawler

import "time"

type report struct {
	RootURL     string      `json:"root_url"`
	Depth       int         `json:"depth"`
	GeneratedAt time.Time   `json:"generated_at"`
	Pages       []pageEntry `json:"pages"`
}

type pageEntry struct {
	URL          string       `json:"url"`
	Depth        int          `json:"depth"`
	HTTPStatus   int          `json:"http_status"`
	Status       string       `json:"status"`
	Error        string       `json:"error,omitempty"`
	BrokenLinks  []brokenLink `json:"broken_links,omitempty"`
	DiscoveredAt time.Time    `json:"discovered_at"`
	SEO          seoInfo      `json:"seo"`
}

type seoInfo struct {
	HasTitle       bool   `json:"has_title"`
	Title          string `json:"title"`
	HasDescription bool   `json:"has_description"`
	Description    string `json:"description"`
	HasH1          bool   `json:"has_h1"`
}

type brokenLink struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}
