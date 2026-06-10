package checker

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"code/internal/parser"
)

type BrokenLink struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
}

type Asset struct {
	URL        string `json:"url"`
	Type       string `json:"type"`
	StatusCode int    `json:"status_code"`
	SizeBytes  int64  `json:"size_bytes"`
	Error      string `json:"error,omitempty"`
}

type Config struct {
	UserAgent   string
	Timeout     time.Duration
	Concurrency int
	HTTPClient  *http.Client
	LinkCache   *LinkCache
}

func (c Config) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: c.Timeout}
}

func doRequest(ctx context.Context, client *http.Client, method, target string, userAgent string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	return client.Do(req)
}

func headWithFallback(ctx context.Context, cfg Config, target string) (*http.Response, error) {
	client := cfg.client()

	resp, err := doRequest(ctx, client, http.MethodHead, target, cfg.UserAgent)
	if err == nil {
		return resp, nil
	}

	return doRequest(ctx, client, http.MethodGet, target, cfg.UserAgent)
}

func runPool[T any, R any](
	ctx context.Context,
	concurrency int,
	input []T,
	worker func(context.Context, T) R,
) []R {

	if concurrency <= 0 {
		concurrency = 5
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	results := make([]R, len(input))

	for i, item := range input {
		select {
		case <-ctx.Done():
			continue
		default:
		}

		sem <- struct{}{}
		wg.Add(1)

		go func(i int, val T) {
			defer wg.Done()
			defer func() { <-sem }()

			results[i] = worker(ctx, val)
		}(i, item)
	}

	wg.Wait()
	return results
}

// CheckLinks проверяет ссылки страницы и возвращает список недоступных ссылок.
func CheckLinks(ctx context.Context, pageURL string, body []byte, cfg Config) []BrokenLink {
	baseURL, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}

	links, _ := parser.ExtractLinks(bytes.NewReader(body), baseURL)
	if len(links) == 0 {
		return []BrokenLink{}
	}

	results := runPool(ctx, cfg.Concurrency, links, func(ctx context.Context, link string) BrokenLink {
		return checkLink(ctx, link, cfg)
	})

	broken := make([]BrokenLink, 0)
	for _, r := range results {
		if r.Error != "" || r.StatusCode >= 400 {
			broken = append(broken, r)
		}
	}
	return broken
}

func checkLink(ctx context.Context, link string, cfg Config) BrokenLink {
	if cfg.LinkCache != nil {
		if result, ok := cfg.LinkCache.Get(link); ok {
			return result
		}
	}

	resp, err := headWithFallback(ctx, cfg, link)
	if err != nil {
		result := BrokenLink{URL: link, Error: err.Error()}
		if cfg.LinkCache != nil {
			cfg.LinkCache.Set(link, result)
		}
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	var result BrokenLink
	if resp.StatusCode >= 400 {
		result = BrokenLink{
			URL:        link,
			StatusCode: resp.StatusCode,
			Error:      http.StatusText(resp.StatusCode),
		}
	} else {
		result = BrokenLink{}
	}

	if cfg.LinkCache != nil {
		cfg.LinkCache.Set(link, result)
	}

	return result
}

// CheckAssets проверяет ресурсы страницы и возвращает сведения о каждом ресурсе.
func CheckAssets(ctx context.Context, pageURL string, body []byte, cfg Config) []Asset {
	baseURL, err := url.Parse(pageURL)
	if err != nil {
		return []Asset{}
	}

	assetInfos, _ := parser.ExtractAssets(bytes.NewReader(body), baseURL)
	if len(assetInfos) == 0 {
		return []Asset{}
	}

	unique := make(map[string]string)
	for _, a := range assetInfos {
		unique[a.URL] = a.Type
	}

	input := make([]Asset, 0, len(unique))
	for u, t := range unique {
		input = append(input, Asset{URL: u, Type: t})
	}

	results := runPool(ctx, cfg.Concurrency, input, func(ctx context.Context, a Asset) Asset {
		return checkAsset(ctx, a, cfg)
	})

	return results
}

func checkAsset(ctx context.Context, a Asset, cfg Config) Asset {
	resp, err := headWithFallback(ctx, cfg, a.URL)
	if err != nil {
		a.Error = err.Error()
		return a
	}
	defer func() { _ = resp.Body.Close() }()

	a.StatusCode = resp.StatusCode

	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if size, err := strconv.ParseInt(cl, 10, 64); err == nil {
			a.SizeBytes = size
		}
	} else {
		body, _ := io.ReadAll(resp.Body)
		a.SizeBytes = int64(len(body))
	}

	if resp.StatusCode >= 400 {
		a.Error = http.StatusText(resp.StatusCode)
	}

	return a
}
