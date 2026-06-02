package crawler

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

type assetCandidate struct {
	URL  string
	Type string
}

const (
	assetTypeImage  = "image"
	assetTypeScript = "script"
	assetTypeStyle  = "style"
)

func extractAssets(pageURL string, body []byte) ([]assetCandidate, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var out []assetCandidate

	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "img":
				_ = addAsset(base, n, "src", assetTypeImage, seen, &out)
			case "script":
				_ = addAsset(base, n, "src", assetTypeScript, seen, &out)
			case "link":
				if isStylesheetLink(n) {
					_ = addAsset(base, n, "href", assetTypeStyle, seen, &out)
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)

	return out, nil
}

func isStylesheetLink(n *html.Node) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "rel") && strings.EqualFold(strings.TrimSpace(a.Val), "stylesheet") {
			return true
		}
	}
	return false
}

func addAsset(base *url.URL, n *html.Node, attr, typ string, seen map[string]struct{}, out *[]assetCandidate) bool {
	for _, a := range n.Attr {
		if a.Key != attr {
			continue
		}
		abs, ok := resolveCheckableLink(base, a.Val)
		if !ok {
			return false
		}
		u := abs.String()
		if _, exists := seen[u]; exists {
			return false
		}
		seen[u] = struct{}{}
		*out = append(*out, assetCandidate{URL: u, Type: typ})
		return true
	}
	return false
}
