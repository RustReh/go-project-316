package crawler

import (
	"bytes"
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

var checkableSchemes = map[string]struct{}{
	"http":  {},
	"https": {},
}

func extractCheckableLinks(pageURL string, body io.Reader) ([]string, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(body)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var links []string

	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			attrName := linkAttrName(n.Data)
			if attrName != "" {
				for _, a := range n.Attr {
					if a.Key == attrName {
						if abs, ok := resolveCheckableLink(base, a.Val); ok {
							s := normalizePageURL(abs)
							if _, exists := seen[s]; !exists {
								seen[s] = struct{}{}
								links = append(links, s)
							}
						}
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)

	return links, nil
}

func linkAttrName(tag string) string {
	switch tag {
	case "a":
		return "href"
	default:
		return ""
	}
}

func normalizePageURL(u *url.URL) string {
	u = cloneURL(u)
	switch u.Path {
	case "/":
		u.Path = ""
	default:
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return u.String()
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	u2 := *u
	return &u2
}

func sameHost(a, b string) bool {
	return strings.EqualFold(a, b)
}

func extractInternalLinks(pageURL string, body []byte, rootHost string) ([]string, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var links []string

	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key != "href" {
					continue
				}
				abs, ok := resolveCheckableLink(base, a.Val)
				if !ok || !sameHost(abs.Host, rootHost) {
					continue
				}
				s := normalizePageURL(abs)
				if _, exists := seen[s]; exists {
					continue
				}
				seen[s] = struct{}{}
				links = append(links, s)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)

	return links, nil
}

func resolveCheckableLink(base *url.URL, raw string) (*url.URL, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "#" {
		return nil, false
	}

	ref, err := url.Parse(raw)
	if err != nil {
		return nil, false
	}

	abs := base.ResolveReference(ref)
	if _, ok := checkableSchemes[abs.Scheme]; !ok {
		return nil, false
	}
	if abs.Host == "" {
		return nil, false
	}

	abs.RawQuery = ""
	abs.Fragment = ""
	abs.User = nil

	return abs, true
}
