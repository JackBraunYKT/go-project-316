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
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

type Options struct {
	URL   string
	Depth int
	// Retries sets how many extra attempts are made after a temporary failure.
	Retries int
	// Delay sets the minimum process-wide interval between HTTP requests.
	Delay time.Duration
	// RPS sets the target process-wide requests per second. Positive RPS overrides Delay.
	RPS         float64
	Timeout     time.Duration
	UserAgent   string
	Concurrency int
	IndentJSON  bool
	HTTPClient  *http.Client
}

const retryDelay = 10 * time.Millisecond

type requestLimiter struct {
	delay time.Duration
	mu    sync.Mutex
	last  time.Time
}

type report struct {
	RootURL     string       `json:"root_url"`
	Depth       int          `json:"depth"`
	GeneratedAt string       `json:"generated_at"`
	Pages       []pageReport `json:"pages"`
}

type pageReport struct {
	URL          string             `json:"url"`
	Depth        int                `json:"depth"`
	HTTPStatus   int                `json:"http_status"`
	Status       string             `json:"status"`
	Error        string             `json:"error"`
	SEO          seoReport          `json:"seo"`
	BrokenLinks  []brokenLinkReport `json:"broken_links"`
	Assets       []assetReport      `json:"assets"`
	DiscoveredAt string             `json:"discovered_at"`
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
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
}

type assetReport struct {
	URL        string `json:"url"`
	Type       string `json:"type"`
	StatusCode int    `json:"status_code"`
	SizeBytes  int    `json:"size_bytes"`
	Error      string `json:"error"`
}

type assetRef struct {
	URL  string
	Type string
}

type resourceResult struct {
	StatusCode   int
	Status       string
	SizeBytes    int
	Error        string
	RequestError string
}

type resourceCache struct {
	ctx       context.Context
	client    *http.Client
	userAgent string
	limiter   *requestLimiter
	retries   int
	results   map[string]resourceResult
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
	limiter := newRequestLimiter(requestDelay(opts))
	resources := newResourceCache(requestCtx, client, opts.UserAgent, limiter, opts.Retries)

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

		page, links := crawlPage(requestCtx, client, item.url, item.depth, opts.UserAgent, limiter, opts.Retries, resources)
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

func requestDelay(opts Options) time.Duration {
	if opts.RPS > 0 {
		return time.Duration(float64(time.Second) / opts.RPS)
	}
	if opts.Delay > 0 {
		return opts.Delay
	}
	return 0
}

func newRequestLimiter(delay time.Duration) *requestLimiter {
	if delay <= 0 {
		return nil
	}
	return &requestLimiter{delay: delay}
}

func (limiter *requestLimiter) wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if limiter == nil {
		return ctx.Err()
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	now := time.Now()
	if limiter.last.IsZero() {
		limiter.last = now
		return nil
	}

	waitFor := limiter.last.Add(limiter.delay).Sub(now)
	if waitFor > 0 {
		timer := time.NewTimer(waitFor)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	limiter.last = time.Now()
	return nil
}

func crawlPage(ctx context.Context, client *http.Client, pageURL string, depth int, userAgent string, limiter *requestLimiter, retries int, resources *resourceCache) (pageReport, []string) {
	page := pageReport{
		URL:          pageURL,
		Depth:        depth,
		BrokenLinks:  []brokenLinkReport{},
		Assets:       []assetReport{},
		DiscoveredAt: time.Now().UTC().Format(time.RFC3339),
	}

	resp, err := doRequest(ctx, client, pageURL, userAgent, limiter, retries)
	if err != nil {
		closeResponse(resp)
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
		page.Status = "error"
		page.Error = fmt.Sprintf("unexpected HTTP status: %s", responseStatus(resp))
		return page, nil
	}

	links := extractLinks(pageURL, body)
	page.Status = "ok"
	page.Assets = checkAssets(resources, extractAssets(pageURL, body))
	page.BrokenLinks = findBrokenLinks(resources, links)

	return page, extractPageLinks(pageURL, body)
}

func sameDomain(rootURL *url.URL, link string) bool {
	linkURL, err := url.Parse(link)
	if err != nil {
		return false
	}

	return strings.EqualFold(rootURL.Hostname(), linkURL.Hostname())
}

func findBrokenLinks(resources *resourceCache, links []string) []brokenLinkReport {
	brokenLinks := []brokenLinkReport{}

	for _, link := range links {
		result := resources.get(link)
		if result.RequestError != "" {
			brokenLinks = append(brokenLinks, brokenLinkReport{URL: link, Error: result.RequestError})
			continue
		}
		if result.StatusCode >= http.StatusBadRequest {
			brokenLinks = append(brokenLinks, brokenLinkReport{
				URL:        link,
				StatusCode: result.StatusCode,
				Error:      http.StatusText(result.StatusCode),
			})
		}
	}

	return brokenLinks
}

func newResourceCache(ctx context.Context, client *http.Client, userAgent string, limiter *requestLimiter, retries int) *resourceCache {
	return &resourceCache{
		ctx:       ctx,
		client:    client,
		userAgent: userAgent,
		limiter:   limiter,
		retries:   retries,
		results:   map[string]resourceResult{},
	}
}

func (cache *resourceCache) get(rawURL string) resourceResult {
	if result, exists := cache.results[rawURL]; exists {
		return result
	}

	result := fetchResource(cache.ctx, cache.client, rawURL, cache.userAgent, cache.limiter, cache.retries)
	cache.results[rawURL] = result
	return result
}

func fetchResource(ctx context.Context, client *http.Client, rawURL string, userAgent string, limiter *requestLimiter, retries int) resourceResult {
	result := resourceResult{}

	resp, err := doRequest(ctx, client, rawURL, userAgent, limiter, retries)
	if err != nil {
		if resp != nil {
			result.StatusCode = resp.StatusCode
			result.Status = responseStatus(resp)
			if size, ok, _ := contentLength(resp); ok {
				result.SizeBytes = size
			}
		}
		closeResponse(resp)
		result.Error = err.Error()
		result.RequestError = err.Error()
		return result
	}
	if resp == nil {
		result.Error = "empty response"
		result.RequestError = result.Error
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Status = responseStatus(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = appendError(result.Error, fmt.Sprintf("failed to read response body: %v", err))
		result.RequestError = result.Error
	} else {
		result.SizeBytes = len(body)
	}

	if size, ok, contentLengthError := contentLength(resp); ok {
		result.SizeBytes = size
	} else if contentLengthError != "" {
		result.Error = appendError(result.Error, contentLengthError)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		result.Error = appendError(result.Error, fmt.Sprintf("unexpected HTTP status: %s", result.Status))
	}

	return result
}

func contentLength(resp *http.Response) (int, bool, string) {
	rawSize := strings.TrimSpace(resp.Header.Get("Content-Length"))
	if rawSize == "" {
		return 0, false, ""
	}

	size, err := strconv.Atoi(rawSize)
	if err != nil || size < 0 {
		return 0, false, fmt.Sprintf("invalid Content-Length: %q", rawSize)
	}

	return size, true, ""
}

func responseStatus(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	if resp.Status != "" {
		return resp.Status
	}
	return fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
}

func appendError(current string, next string) string {
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; " + next
}

func checkAssets(resources *resourceCache, assets []assetRef) []assetReport {
	reports := make([]assetReport, 0, len(assets))
	for _, asset := range assets {
		result := resources.get(asset.URL)
		reports = append(reports, assetReport{
			URL:        asset.URL,
			Type:       asset.Type,
			StatusCode: result.StatusCode,
			SizeBytes:  result.SizeBytes,
			Error:      result.Error,
		})
	}
	return reports
}

func doRequest(ctx context.Context, client *http.Client, rawURL string, userAgent string, limiter *requestLimiter, retries int) (*http.Response, error) {
	if retries < 0 {
		retries = 0
	}

	for attempt := 0; ; attempt++ {
		resp, err := doRequestOnce(ctx, client, rawURL, userAgent, limiter)
		if !shouldRetry(ctx, resp, err) || attempt >= retries {
			return resp, err
		}

		closeResponse(resp)
		if err := waitBeforeRetry(ctx); err != nil {
			return nil, err
		}
	}
}

func doRequestOnce(ctx context.Context, client *http.Client, rawURL string, userAgent string, limiter *requestLimiter) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	if err := limiter.wait(ctx); err != nil {
		return nil, err
	}

	return client.Do(req)
}

func shouldRetry(ctx context.Context, resp *http.Response, err error) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}

	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
}

func waitBeforeRetry(ctx context.Context) error {
	timer := time.NewTimer(retryDelay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func closeResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
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
	return extractHTTPLinks(pageURL, body, func(node *html.Node, attr html.Attribute) bool {
		return strings.EqualFold(attr.Key, "href") || strings.EqualFold(attr.Key, "src")
	})
}

func extractPageLinks(pageURL string, body []byte) []string {
	return extractHTTPLinks(pageURL, body, func(node *html.Node, attr html.Attribute) bool {
		return strings.EqualFold(node.Data, "a") && strings.EqualFold(attr.Key, "href")
	})
}

func extractHTTPLinks(pageURL string, body []byte, include func(*html.Node, html.Attribute) bool) []string {
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
				if !include(node, attr) {
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

func extractAssets(pageURL string, body []byte) []assetRef {
	baseURL, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	seen := map[string]struct{}{}
	assets := []assetRef{}
	addAsset := func(rawURL string, assetType string) {
		assetURL, ok := normalizeHTTPLink(baseURL, rawURL)
		if !ok {
			return
		}
		if _, exists := seen[assetURL]; exists {
			return
		}
		seen[assetURL] = struct{}{}
		assets = append(assets, assetRef{URL: assetURL, Type: assetType})
	}

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch {
			case strings.EqualFold(node.Data, "img"):
				addAsset(attrValue(node, "src"), "image")
			case strings.EqualFold(node.Data, "script"):
				addAsset(attrValue(node, "src"), "script")
			case strings.EqualFold(node.Data, "link") && hasRel(node, "stylesheet"):
				addAsset(attrValue(node, "href"), "style")
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	return assets
}

func hasRel(node *html.Node, want string) bool {
	for _, rel := range strings.Fields(attrValue(node, "rel")) {
		if strings.EqualFold(rel, want) {
			return true
		}
	}
	return false
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
