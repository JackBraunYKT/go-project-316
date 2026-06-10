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
	BrokenLinks []brokenLinkReport `json:"broken_links"`
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

	page := pageReport{
		URL:         opts.URL,
		Depth:       0,
		BrokenLinks: []brokenLinkReport{},
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return nil, err
	}
	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		page.Status = "error"
		page.Error = err.Error()
	} else {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(resp.Body)
		page.HTTPStatus = resp.StatusCode
		if readErr != nil {
			page.Status = "error"
			page.Error = readErr.Error()
		} else if resp.StatusCode == http.StatusOK {
			page.Status = "ok"
			page.BrokenLinks = findBrokenLinks(requestCtx, client, opts.URL, body, opts.UserAgent)
		} else {
			status := resp.Status
			if status == "" {
				status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
			}
			page.Status = "error"
			page.Error = fmt.Sprintf("unexpected HTTP status: %s", status)
		}
	}

	output := report{
		RootURL:     opts.URL,
		Depth:       opts.Depth,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Pages:       []pageReport{page},
	}

	if opts.IndentJSON {
		return json.MarshalIndent(output, "", "  ")
	}

	return json.Marshal(output)
}

func findBrokenLinks(ctx context.Context, client *http.Client, pageURL string, body []byte, userAgent string) []brokenLinkReport {
	brokenLinks := []brokenLinkReport{}

	for _, link := range extractLinks(pageURL, body) {
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
