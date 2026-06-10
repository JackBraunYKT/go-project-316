package worker

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"code/internal/checker"
	"code/internal/parser"
	"code/internal/report"
	"code/internal/shared"
)

type Config struct {
	UserAgent   string
	Timeout     time.Duration
	Retries     int
	Concurrency int
	Depth       int
	HTTPClient  *http.Client
}

type Result struct {
	Page        report.PageReport
	Links       []string
	ShouldQueue bool
	NextDepth   int
}

// ProcessURL обрабатывает страницу и возвращает результат для отчета и очереди обхода.
func ProcessURL(
	ctx context.Context,
	pageURL string,
	depth int,
	cfg Config,
	checkerCfg checker.Config,
) Result {

	page := newPageReport(pageURL, depth)

	resp, err := fetchWithRetry(ctx, pageURL, cfg)
	if err != nil {
		return errorResult(page, err)
	}
	if resp == nil {
		return errorResult(page, err)
	}
	defer func() { _ = resp.Body.Close() }()

	page.HTTPStatus = resp.StatusCode
	page.Status = "ok"

	contentType := resp.Header.Get("Content-Type")

	switch {
	case strings.Contains(contentType, "text/html"):
		return handleHTML(ctx, page, resp.Body, pageURL, depth, cfg, checkerCfg)

	case strings.Contains(contentType, "application/xml"),
		strings.Contains(contentType, "text/xml"):
		return handleXML(page, resp.Body)

	default:
		return finalize(page)
	}
}

func fetchWithRetry(ctx context.Context, pageURL string, cfg Config) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}

	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}

	var resp *http.Response
	var lastErr error
	maxAttempts := cfg.Retries + 1

	for attempt := 0; attempt < maxAttempts; attempt++ {

		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err = cfg.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			lastErr = nil
			continue
		}

		return resp, nil
	}

	if resp != nil {
		return resp, lastErr
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, err
}

func handleHTML(
	ctx context.Context,
	page report.PageReport,
	body io.Reader,
	pageURL string,
	depth int,
	cfg Config,
	checkerCfg checker.Config,
) Result {

	raw, err := io.ReadAll(body)
	if err != nil {
		return errorResult(page, err)
	}

	page.SEO = (*report.SEOReport)(parser.ParseSEO(bytes.NewReader(raw)))

	pageURLParsed, _ := url.Parse(pageURL)

	links, _ := parser.ExtractLinks(bytes.NewReader(raw), pageURLParsed)

	brokenLinks := checker.CheckLinks(ctx, pageURL, raw, checkerCfg)
	for _, bl := range brokenLinks {
		page.BrokenLinks = append(page.BrokenLinks, report.BrokenLink(bl))
	}

	assets := checker.CheckAssets(ctx, pageURL, raw, checkerCfg)
	for _, a := range assets {
		page.Assets = append(page.Assets, report.Asset(a))
	}

	page.BrokenLinks = uniqueBrokenLinks(page.BrokenLinks)
	page.Assets = uniqueAssets(page.Assets)

	return Result{
		Page:        page,
		Links:       links,
		ShouldQueue: depth+1 < cfg.Depth,
		NextDepth:   depth + 1,
	}
}

func handleXML(page report.PageReport, body io.Reader) Result {
	raw, err := io.ReadAll(body)
	if err == nil {
		page.SEO = (*report.SEOReport)(parser.ParseSEO(bytes.NewReader(raw)))
	}
	if page.Assets == nil {
		page.Assets = []report.Asset{}
	}
	if page.BrokenLinks == nil {
		page.BrokenLinks = []report.BrokenLink{}
	}
	return Result{Page: page}
}

func newPageReport(pageURL string, depth int) report.PageReport {
	return report.PageReport{
		URL:          shared.NormalizeURLFromItem(pageURL),
		Depth:        depth,
		DiscoveredAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func errorResult(page report.PageReport, err error) Result {
	page.Status = "error"
	if err != nil {
		page.Error = err.Error()
	} else {
		page.Error = "unknown error"
	}
	return finalize(page)
}

func finalize(page report.PageReport) Result {
	if page.Assets == nil {
		page.Assets = []report.Asset{}
	}
	if page.BrokenLinks == nil {
		page.BrokenLinks = []report.BrokenLink{}
	}
	if page.SEO == nil {
		page.SEO = &report.SEOReport{}
	}
	return Result{Page: page}
}

func uniqueBrokenLinks(links []report.BrokenLink) []report.BrokenLink {
	seen := make(map[string]bool)
	result := make([]report.BrokenLink, 0, len(links))
	for _, link := range links {
		if !seen[link.URL] {
			seen[link.URL] = true
			result = append(result, link)
		}
	}
	return result
}

func uniqueAssets(assets []report.Asset) []report.Asset {
	seen := make(map[string]bool)
	result := make([]report.Asset, 0, len(assets))
	for _, asset := range assets {
		if !seen[asset.URL] {
			seen[asset.URL] = true
			result = append(result, asset)
		}
	}
	return result
}
