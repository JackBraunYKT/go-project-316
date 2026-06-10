package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type analyzeReport struct {
	RootURL     string        `json:"root_url"`
	Depth       int           `json:"depth"`
	GeneratedAt string        `json:"generated_at"`
	Pages       []analyzePage `json:"pages"`
}

type analyzePage struct {
	URL        string `json:"url"`
	Depth      int    `json:"depth"`
	HTTPStatus int    `json:"http_status"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	SEO        struct {
		HasTitle       bool   `json:"has_title"`
		Title          string `json:"title"`
		HasDescription bool   `json:"has_description"`
		Description    string `json:"description"`
		HasH1          bool   `json:"has_h1"`
	} `json:"seo"`
	BrokenLinks []struct {
		URL        string `json:"url"`
		StatusCode int    `json:"status_code"`
		Error      string `json:"error"`
	} `json:"broken_links"`
}

func decodeReport(t *testing.T, reportBytes []byte) analyzeReport {
	t.Helper()

	var report analyzeReport
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}

	return report
}

func newClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func requirePageCount(t *testing.T, report analyzeReport, want int) {
	t.Helper()

	if len(report.Pages) != want {
		t.Fatalf("pages length = %d, want %d: %#v", len(report.Pages), want, report.Pages)
	}
}

func countPagesByURL(pages []analyzePage, url string) int {
	count := 0
	for _, page := range pages {
		if page.URL == url {
			count++
		}
	}
	return count
}

func TestAnalyzeReportsSuccessfulHTTPResponse(t *testing.T) {
	client := newClient(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.String(); got != "https://example.com" {
			t.Fatalf("requested URL = %q, want https://example.com", got)
		}
		if got := req.Header.Get("User-Agent"); got != "crawler-test" {
			t.Fatalf("User-Agent = %q, want crawler-test", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("<html></html>")),
		}, nil
	})

	reportBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		Depth:      1,
		UserAgent:  "crawler-test",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	report := decodeReport(t, reportBytes)
	if report.RootURL != "https://example.com" {
		t.Fatalf("root_url = %q, want https://example.com", report.RootURL)
	}
	if report.Depth != 1 {
		t.Fatalf("depth = %d, want 1", report.Depth)
	}
	if report.GeneratedAt == "" {
		t.Fatal("generated_at is empty")
	}

	page := report.Pages[0]
	if page.URL != "https://example.com" {
		t.Fatalf("page url = %q, want https://example.com", page.URL)
	}
	if page.Depth != 0 {
		t.Fatalf("page depth = %d, want 0", page.Depth)
	}
	if page.HTTPStatus != http.StatusOK {
		t.Fatalf("http_status = %d, want %d", page.HTTPStatus, http.StatusOK)
	}
	if page.Status != "ok" {
		t.Fatalf("status = %q, want ok", page.Status)
	}
	if page.Error != "" {
		t.Fatalf("error = %q, want empty", page.Error)
	}
}

func TestAnalyzeCrawlsInternalPagesWithinDepth(t *testing.T) {
	rootHTML := `
		<html>
			<body>
				<a href="/about">About</a>
				<a href="/contacts">Contacts</a>
				<a href="https://external.test/pricing">External</a>
			</body>
		</html>`

	client := newClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://example.com":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(rootHTML)),
			}, nil
		case "https://example.com/about":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("<html><title>About</title></html>")),
			}, nil
		case "https://example.com/contacts":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("<html><title>Contacts</title></html>")),
			}, nil
		case "https://external.test/pricing":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("<html></html>")),
			}, nil
		default:
			t.Fatalf("unexpected request URL: %s", req.URL.String())
		}
		return nil, nil
	})

	shallowBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		Depth:      1,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	shallowReport := decodeReport(t, shallowBytes)
	requirePageCount(t, shallowReport, 1)
	if shallowReport.Pages[0].URL != "https://example.com" {
		t.Fatalf("page url = %q, want https://example.com", shallowReport.Pages[0].URL)
	}
	if shallowReport.Pages[0].Depth != 0 {
		t.Fatalf("page depth = %d, want 0", shallowReport.Pages[0].Depth)
	}

	deeperBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		Depth:      2,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	deeperReport := decodeReport(t, deeperBytes)
	requirePageCount(t, deeperReport, 3)

	wantPages := map[string]int{
		"https://example.com":          0,
		"https://example.com/about":    1,
		"https://example.com/contacts": 1,
	}
	for _, page := range deeperReport.Pages {
		wantDepth, exists := wantPages[page.URL]
		if !exists {
			t.Fatalf("unexpected page in report: %s", page.URL)
		}
		if page.Depth != wantDepth {
			t.Fatalf("page %s depth = %d, want %d", page.URL, page.Depth, wantDepth)
		}
	}
	if countPagesByURL(deeperReport.Pages, "https://external.test/pricing") != 0 {
		t.Fatal("external URL appeared in pages")
	}
}

func TestAnalyzeReportsDuplicateInternalLinksOnce(t *testing.T) {
	rootHTML := `
		<html>
			<body>
				<a href="/guide">Guide</a>
				<a href="https://example.com/guide">Guide duplicate</a>
				<a href="/guide#install">Guide duplicate with fragment</a>
			</body>
		</html>`

	client := newClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://example.com":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(rootHTML)),
			}, nil
		case "https://example.com/guide":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("<html><title>Guide</title></html>")),
			}, nil
		default:
			t.Fatalf("unexpected request URL: %s", req.URL.String())
		}
		return nil, nil
	})

	reportBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		Depth:      2,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	report := decodeReport(t, reportBytes)
	requirePageCount(t, report, 2)
	if got := countPagesByURL(report.Pages, "https://example.com/guide"); got != 1 {
		t.Fatalf("guide page count = %d, want 1", got)
	}
}

func TestAnalyzeReturnsPartialJSONWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstRequests := 0
	rootHTML := `
		<html>
			<body>
				<a href="/first">First</a>
				<a href="/second">Second</a>
			</body>
		</html>`

	client := newClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://example.com":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(rootHTML)),
			}, nil
		case "https://example.com/first":
			firstRequests++
			if firstRequests == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader("<html></html>")),
				}, nil
			}
			cancel()
			return nil, context.Canceled
		case "https://example.com/second":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("<html></html>")),
			}, nil
		default:
			t.Fatalf("unexpected request URL: %s", req.URL.String())
		}
		return nil, nil
	})

	reportBytes, err := Analyze(ctx, Options{
		URL:        "https://example.com",
		Depth:      2,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	report := decodeReport(t, reportBytes)
	requirePageCount(t, report, 2)
	if report.Pages[0].URL != "https://example.com" {
		t.Fatalf("first page url = %q, want https://example.com", report.Pages[0].URL)
	}
	if report.Pages[1].URL != "https://example.com/first" {
		t.Fatalf("second page url = %q, want https://example.com/first", report.Pages[1].URL)
	}
	if report.Pages[1].Status != "error" {
		t.Fatalf("canceled page status = %q, want error", report.Pages[1].Status)
	}
	if !strings.Contains(report.Pages[1].Error, context.Canceled.Error()) {
		t.Fatalf("canceled page error = %q, want context canceled", report.Pages[1].Error)
	}
}

func TestAnalyzeReportsOnlyBrokenLinks(t *testing.T) {
	html := `
		<html>
			<head>
				<link rel="stylesheet" href="/assets/ok.css">
				<script src="/assets/missing.js"></script>
			</head>
			<body>
				<a href="mailto:support@example.com">mail</a>
				<a href="javascript:void(0)">noop</a>
				<a href="">empty</a>
			</body>
		</html>`

	client := newClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://example.com/blog/index.html":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(html)),
			}, nil
		case "https://example.com/assets/ok.css":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("body{}")),
			}, nil
		case "https://example.com/assets/missing.js":
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Body:       io.NopCloser(strings.NewReader("not found")),
			}, nil
		default:
			t.Fatalf("unexpected request URL: %s", req.URL.String())
		}
		return nil, nil
	})

	reportBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com/blog/index.html",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	page := decodeReport(t, reportBytes).Pages[0]
	if len(page.BrokenLinks) != 1 {
		t.Fatalf("broken_links length = %d, want 1: %#v", len(page.BrokenLinks), page.BrokenLinks)
	}

	brokenLink := page.BrokenLinks[0]
	if brokenLink.URL != "https://example.com/assets/missing.js" {
		t.Fatalf("broken link URL = %q, want https://example.com/assets/missing.js", brokenLink.URL)
	}
	if brokenLink.StatusCode != http.StatusNotFound {
		t.Fatalf("broken link status_code = %d, want %d", brokenLink.StatusCode, http.StatusNotFound)
	}
	if brokenLink.Error != "" {
		t.Fatalf("broken link error = %q, want empty", brokenLink.Error)
	}
}

func TestAnalyzeReportsSEOWhenTagsExist(t *testing.T) {
	html := `
		<html>
			<head>
				<title>Example &amp; Test</title>
				<meta name="description" content="Readable &amp; useful page">
			</head>
			<body>
				<h1>Welcome</h1>
			</body>
		</html>`
	client := newClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(html)),
		}, nil
	})

	reportBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	seo := decodeReport(t, reportBytes).Pages[0].SEO
	if !seo.HasTitle {
		t.Fatal("has_title = false, want true")
	}
	if seo.Title != "Example & Test" {
		t.Fatalf("title = %q, want Example & Test", seo.Title)
	}
	if !seo.HasDescription {
		t.Fatal("has_description = false, want true")
	}
	if seo.Description != "Readable & useful page" {
		t.Fatalf("description = %q, want Readable & useful page", seo.Description)
	}
	if !seo.HasH1 {
		t.Fatal("has_h1 = false, want true")
	}
}

func TestAnalyzeReportsEmptySEOWhenTagsAreMissing(t *testing.T) {
	client := newClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("<html><head></head><body><p>No SEO tags</p></body></html>")),
		}, nil
	})

	reportBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	seo := decodeReport(t, reportBytes).Pages[0].SEO
	if seo.HasTitle {
		t.Fatal("has_title = true, want false")
	}
	if seo.Title != "" {
		t.Fatalf("title = %q, want empty", seo.Title)
	}
	if seo.HasDescription {
		t.Fatal("has_description = true, want false")
	}
	if seo.Description != "" {
		t.Fatalf("description = %q, want empty", seo.Description)
	}
	if seo.HasH1 {
		t.Fatal("has_h1 = true, want false")
	}
}

func TestAnalyzeReportsInvalidHTTPStatusAsError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		statusText string
	}{
		{name: "not found", statusCode: http.StatusNotFound, statusText: "404 Not Found"},
		{name: "server error", statusCode: http.StatusInternalServerError, statusText: "500 Internal Server Error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newClient(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.statusCode,
					Status:     tt.statusText,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			})

			reportBytes, err := Analyze(context.Background(), Options{
				URL:        "https://example.com",
				HTTPClient: client,
			})
			if err != nil {
				t.Fatalf("Analyze returned error: %v", err)
			}

			page := decodeReport(t, reportBytes).Pages[0]
			if page.HTTPStatus != tt.statusCode {
				t.Fatalf("http_status = %d, want %d", page.HTTPStatus, tt.statusCode)
			}
			if page.Status != "error" {
				t.Fatalf("status = %q, want error", page.Status)
			}
			if !strings.Contains(page.Error, tt.statusText) {
				t.Fatalf("error = %q, want it to mention %q", page.Error, tt.statusText)
			}
		})
	}
}

func TestAnalyzeReportsNetworkErrors(t *testing.T) {
	client := newClient(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network is unavailable")
	})

	reportBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	page := decodeReport(t, reportBytes).Pages[0]
	if page.HTTPStatus != 0 {
		t.Fatalf("http_status = %d, want 0", page.HTTPStatus)
	}
	if page.Status != "error" {
		t.Fatalf("status = %q, want error", page.Status)
	}
	if !strings.Contains(page.Error, "network is unavailable") {
		t.Fatalf("error = %q, want network error text", page.Error)
	}
}

func TestAnalyzeReportsTimeouts(t *testing.T) {
	client := newClient(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})

	reportBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		Timeout:    time.Nanosecond,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	page := decodeReport(t, reportBytes).Pages[0]
	if page.HTTPStatus != 0 {
		t.Fatalf("http_status = %d, want 0", page.HTTPStatus)
	}
	if page.Status != "error" {
		t.Fatalf("status = %q, want error", page.Status)
	}
	if !strings.Contains(page.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("error = %q, want deadline exceeded", page.Error)
	}
}
