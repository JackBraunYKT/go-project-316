package parser

import (
	"io"
	"net/url"
	"strings"

	"code/internal/shared"

	"golang.org/x/net/html"
)

type SEOReport struct {
	HasTitle       bool   `json:"has_title"`
	Title          string `json:"title"`
	HasDescription bool   `json:"has_description"`
	Description    string `json:"description"`
	HasH1          bool   `json:"has_h1"`
}

type AssetInfo struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

func walk(n *html.Node, fn func(*html.Node)) {
	for ; n != nil; n = n.NextSibling {
		fn(n)
		if n.FirstChild != nil {
			walk(n.FirstChild, fn)
		}
	}
}

// ParseSEO извлекает SEO-данные из HTML-документа.
func ParseSEO(r io.Reader) *SEOReport {
	doc, err := html.Parse(r)
	if err != nil {
		return &SEOReport{}
	}

	report := &SEOReport{}

	walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}

		switch n.Data {
		case "title":
			if !report.HasTitle && n.FirstChild != nil {
				title := strings.TrimSpace(n.FirstChild.Data)
				if title != "" {
					report.Title = title
					report.HasTitle = true
				}
			}

		case "meta":
			if report.HasDescription {
				return
			}
			if strings.ToLower(getAttr(n, "name")) == "description" {
				content := strings.TrimSpace(getAttr(n, "content"))
				if content != "" {
					report.Description = content
					report.HasDescription = true
				}
			}

		case "h1":
			report.HasH1 = true
		}
	})

	return report
}

// ExtractLinks извлекает ссылки из HTML-документа и приводит их к абсолютному виду.
func ExtractLinks(r io.Reader, baseURL *url.URL) ([]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	var links []string

	walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "a" {
			return
		}

		href := getAttr(n, "href")
		if href == "" {
			return
		}

		if strings.HasPrefix(href, "javascript:") ||
			strings.HasPrefix(href, "mailto:") ||
			strings.HasPrefix(href, "tel:") ||
			strings.HasPrefix(href, "ftp:") {
			return
		}

		if baseURL != nil {
			href = shared.ResolveURL(baseURL, href)
		}

		if href != "" {
			links = append(links, href)
		}
	})

	return links, nil
}

// ExtractAssets извлекает ссылки на ресурсы страницы из HTML-документа.
func ExtractAssets(r io.Reader, baseURL *url.URL) ([]AssetInfo, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	var assets []AssetInfo

	walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}

		switch n.Data {

		case "img":
			addAsset(&assets, baseURL, getAttr(n, "src"), "image")

		case "script":
			addAsset(&assets, baseURL, getAttr(n, "src"), "script")

		case "link":
			if strings.ToLower(getAttr(n, "rel")) == "stylesheet" {
				addAsset(&assets, baseURL, getAttr(n, "href"), "style")
			}
		}
	})

	return assets, nil
}

func addAsset(assets *[]AssetInfo, base *url.URL, rawURL, assetType string) {
	if rawURL == "" {
		return
	}

	if base != nil {
		rawURL = shared.ResolveURL(base, rawURL)
	}

	if rawURL == "" {
		return
	}

	*assets = append(*assets, AssetInfo{
		URL:  rawURL,
		Type: assetType,
	})
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}
