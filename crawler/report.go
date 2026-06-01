package crawler

import "time"

type report struct {
	RootURL     string      `json:"root_url"`
	Depth       int         `json:"depth"`
	GeneratedAt time.Time   `json:"generated_at"`
	Pages       []pageEntry `json:"pages"`
}

type pageEntry struct {
	URL        string `json:"url"`
	Depth      int    `json:"depth"`
	HTTPStatus int    `json:"http_status"`
	Status     string `json:"status"`
	Error      string `json:"error"`
}
