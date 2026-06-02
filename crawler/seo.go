package crawler

import (
	"bytes"
	stdhtml "html"
	"strings"

	"golang.org/x/net/html"
)

func extractSEO(body []byte) seoInfo {
	info := seoInfo{}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return info
	}

	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if !info.HasTitle {
					info.HasTitle = true
					info.Title = cleanText(textContent(n))
				}
			case "meta":
				if !info.HasDescription && isDescriptionMeta(n) {
					info.HasDescription = true
					info.Description = cleanText(metaContent(n))
				}
			case "h1":
				info.HasH1 = true
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)

	return info
}

func isDescriptionMeta(n *html.Node) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "name") && strings.EqualFold(strings.TrimSpace(a.Val), "description") {
			return true
		}
	}
	return false
}

func metaContent(n *html.Node) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "content") {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func cleanText(s string) string {
	s = stdhtml.UnescapeString(s)
	s = strings.TrimSpace(s)
	return strings.Join(strings.Fields(s), " ")
}
