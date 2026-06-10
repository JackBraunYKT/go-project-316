package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type Options struct {
	URL         string
	Depth       int
	Retries     int
	Delay       time.Duration
	Timeout     time.Duration
	UserAgent   string
	Concurrency int
	IndentJSON  bool
	HTTPClient  *http.Client
}

type report struct {
	RootURL     string       `json:"root_url"`
	Depth       int          `json:"depth"`
	GeneratedAt string       `json:"generated_at"`
	Pages       []pageReport `json:"pages"`
}

type pageReport struct {
	URL         string             `json:"url"`
	Depth       int                `json:"depth"`
	HTTPStatus  int                `json:"http_status"`
	Status      string             `json:"status"`
	Error       string             `json:"error"`
	SEO         seoReport          `json:"seo"`
	BrokenLinks []brokenLinkReport `json:"broken_links"`
}

type seoReport struct {
	HasTitle       bool   `json:"has_title"`
	Title          string `json:"title"`
	HasDescription bool   `json:"has_description"`
	Description    string `json:"description"`
	HasH1          bool   `json:"has_h1"`
}

type brokenLinkReport struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	if opts.URL == "" {
		return nil, errors.New("url is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	rootURL, err := url.Parse(opts.URL)
	if err != nil {
		return nil, err
	}
	rootURL.Fragment = ""
	startURL := rootURL.String()

	type crawlItem struct {
		url   string
		depth int
	}

	maxPageDepth := opts.Depth - 1
	if maxPageDepth < 0 {
		maxPageDepth = 0
	}

	pages := []pageReport{}
	queue := []crawlItem{{url: startURL, depth: 0}}
	seen := map[string]struct{}{startURL: {}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		page, links := crawlPage(requestCtx, client, item.url, item.depth, opts.UserAgent)
		pages = append(pages, page)

		if requestCtx.Err() != nil {
			break
		}
		if page.Status != "ok" || item.depth >= maxPageDepth {
			continue
		}

		for _, link := range links {
			if !sameDomain(rootURL, link) {
				continue
			}
			if _, exists := seen[link]; exists {
				continue
			}
			seen[link] = struct{}{}
			queue = append(queue, crawlItem{url: link, depth: item.depth + 1})
		}
	}

	output := report{
		RootURL:     opts.URL,
		Depth:       opts.Depth,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Pages:       pages,
	}

	if opts.IndentJSON {
		return json.MarshalIndent(output, "", "  ")
	}

	return json.Marshal(output)
}

func crawlPage(ctx context.Context, client *http.Client, pageURL string, depth int, userAgent string) (pageReport, []string) {
	page := pageReport{
		URL:         pageURL,
		Depth:       depth,
		BrokenLinks: []brokenLinkReport{},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		page.Status = "error"
		page.Error = err.Error()
		return page, nil
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		page.Status = "error"
		page.Error = err.Error()
		return page, nil
	}
	if resp == nil {
		page.Status = "error"
		page.Error = "empty response"
		return page, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	page.HTTPStatus = resp.StatusCode
	if err != nil {
		page.Status = "error"
		page.Error = err.Error()
		return page, nil
	}

	page.SEO = extractSEO(body)
	if resp.StatusCode != http.StatusOK {
		status := resp.Status
		if status == "" {
			status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		}
		page.Status = "error"
		page.Error = fmt.Sprintf("unexpected HTTP status: %s", status)
		return page, nil
	}

	links := extractLinks(pageURL, body)
	page.Status = "ok"
	page.BrokenLinks = findBrokenLinks(ctx, client, links, userAgent)

	return page, links
}

func sameDomain(rootURL *url.URL, link string) bool {
	linkURL, err := url.Parse(link)
	if err != nil {
		return false
	}

	return strings.EqualFold(rootURL.Hostname(), linkURL.Hostname())
}

func findBrokenLinks(ctx context.Context, client *http.Client, links []string, userAgent string) []brokenLinkReport {
	brokenLinks := []brokenLinkReport{}

	for _, link := range links {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
		if err != nil {
			brokenLinks = append(brokenLinks, brokenLinkReport{URL: link, Error: err.Error()})
			continue
		}
		if userAgent != "" {
			req.Header.Set("User-Agent", userAgent)
		}

		resp, err := client.Do(req)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			brokenLinks = append(brokenLinks, brokenLinkReport{URL: link, Error: err.Error()})
			continue
		}
		if resp == nil {
			brokenLinks = append(brokenLinks, brokenLinkReport{URL: link, Error: "empty response"})
			continue
		}
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		if resp.StatusCode >= http.StatusBadRequest {
			brokenLinks = append(brokenLinks, brokenLinkReport{URL: link, StatusCode: resp.StatusCode})
		}
	}

	return brokenLinks
}

func extractSEO(body []byte) seoReport {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return seoReport{}
	}

	seo := seoReport{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "title":
				if !seo.HasTitle {
					seo.HasTitle = true
					seo.Title = cleanText(nodeText(node))
				}
			case "meta":
				if !seo.HasDescription && isDescriptionMeta(node) {
					seo.HasDescription = true
					seo.Description = cleanText(attrValue(node, "content"))
				}
			case "h1":
				seo.HasH1 = true
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	return seo
}

func isDescriptionMeta(node *html.Node) bool {
	return strings.EqualFold(attrValue(node, "name"), "description")
}

func attrValue(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)

	return builder.String()
}

func cleanText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func extractLinks(pageURL string, body []byte) []string {
	baseURL, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	seen := map[string]struct{}{}
	links := []string{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			for _, attr := range node.Attr {
				if attr.Key != "href" && attr.Key != "src" {
					continue
				}
				link, ok := normalizeHTTPLink(baseURL, attr.Val)
				if !ok {
					continue
				}
				if _, exists := seen[link]; exists {
					continue
				}
				seen[link] = struct{}{}
				links = append(links, link)
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	return links
}

func normalizeHTTPLink(baseURL *url.URL, rawLink string) (string, bool) {
	rawLink = strings.TrimSpace(rawLink)
	if rawLink == "" || strings.HasPrefix(rawLink, "#") {
		return "", false
	}

	linkURL, err := url.Parse(rawLink)
	if err != nil {
		return "", false
	}

	absoluteURL := baseURL.ResolveReference(linkURL)
	if absoluteURL.Scheme != "http" && absoluteURL.Scheme != "https" {
		return "", false
	}
	if absoluteURL.Host == "" {
		return "", false
	}

	absoluteURL.Fragment = ""
	return absoluteURL.String(), true
}
