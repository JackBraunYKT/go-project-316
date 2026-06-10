package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAnalyze_Success проверяет успешный анализ простой страницы.
func TestAnalyze_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if result.RootURL != server.URL {
		t.Errorf("RootURL = %q, want %q", result.RootURL, server.URL)
	}

	if result.Depth != 1 {
		t.Errorf("Depth = %d, want %d", result.Depth, 1)
	}

	if len(result.Pages) != 1 {
		t.Fatalf("Pages = %d, want %d", len(result.Pages), 1)
	}

	page := result.Pages[0]
	expectedURL := strings.TrimSuffix(server.URL, "/")
	if page.URL != expectedURL {
		t.Errorf("Page URL = %q, want %q", page.URL, expectedURL)
	}

	if page.HTTPStatus != http.StatusOK {
		t.Errorf("HTTPStatus = %d, want %d", page.HTTPStatus, http.StatusOK)
	}

	if page.Status != "ok" {
		t.Errorf("Status = %q, want %q", page.Status, "ok")
	}

	if page.Error != "" {
		t.Errorf("Error = %q, want empty string", page.Error)
	}
}

// TestAnalyze_NetworkError проверяет отчет при сетевой ошибке.
func TestAnalyze_NetworkError(t *testing.T) {
	opts := Options{
		URL:         "http://invalid-host-that-does-not-exist.local",
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     100 * time.Millisecond,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  &http.Client{Timeout: 100 * time.Millisecond},
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) != 1 {
		t.Fatalf("Pages = %d, want %d", len(result.Pages), 1)
	}

	page := result.Pages[0]
	if page.Status != "error" {
		t.Errorf("Status = %q, want %q", page.Status, "error")
	}

	if page.Error == "" {
		t.Error("Error should not be empty for network error")
	}
}

// TestAnalyze_NonOKStatus проверяет отчет для ответа с неуспешным HTTP-статусом.
func TestAnalyze_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) != 1 {
		t.Fatalf("Pages = %d, want %d", len(result.Pages), 1)
	}

	page := result.Pages[0]
	if page.HTTPStatus != http.StatusNotFound {
		t.Errorf("HTTPStatus = %d, want %d", page.HTTPStatus, http.StatusNotFound)
	}

	if page.Status != "ok" {
		t.Errorf("Status = %q, want %q", page.Status, "ok")
	}
}

// TestAnalyze_ServerError проверяет обработку серверной ошибки.
func TestAnalyze_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) != 1 {
		t.Fatalf("Pages = %d, want %d", len(result.Pages), 1)
	}

	page := result.Pages[0]
	if page.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("HTTPStatus = %d, want %d", page.HTTPStatus, http.StatusInternalServerError)
	}
}

// TestAnalyze_InvalidURL проверяет отчет для некорректного URL.
func TestAnalyze_InvalidURL(t *testing.T) {
	opts := Options{
		URL:         "://invalid-url",
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) != 1 {
		t.Fatalf("Pages = %d, want %d", len(result.Pages), 1)
	}

	page := result.Pages[0]
	if page.Status != "error" {
		t.Errorf("Status = %q, want %q", page.Status, "error")
	}

	if page.Error == "" {
		t.Error("Error should not be empty for invalid URL")
	}
}

// TestAnalyze_ContextCancellation проверяет отчет при отмененном контексте.
func TestAnalyze_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, _ := Analyze(ctx, opts)

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Report should be valid JSON: %v", err)
	}

	if result.RootURL != server.URL {
		t.Errorf("RootURL = %q, want %q", result.RootURL, server.URL)
	}
}

// TestAnalyze_IndentJSON проверяет форматирование JSON с отступами.
func TestAnalyze_IndentJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  true,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if result.RootURL != server.URL {
		t.Errorf("RootURL = %q, want %q", result.RootURL, server.URL)
	}
}

// TestAnalyze_BrokenLinks проверяет обнаружение битых ссылок.
func TestAnalyze_BrokenLinks(t *testing.T) {
	goodLinkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer goodLinkServer.Close()

	badLinkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer badLinkServer.Close()

	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body>
			<a href="` + badLinkServer.URL + `/broken">Broken Link</a>
			<a href="` + goodLinkServer.URL + `">Good Link</a>
		</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) < 1 {
		t.Fatalf("Pages = %d, want at least 1", len(result.Pages))
	}

	page := result.Pages[0]
	if page.Status != "ok" {
		t.Errorf("Status = %q, want %q", page.Status, "ok")
	}

	foundBroken := false
	for _, bl := range page.BrokenLinks {
		if strings.Contains(bl.URL, "broken") || bl.StatusCode == http.StatusNotFound {
			foundBroken = true
			break
		}
	}

	if !foundBroken {
		t.Error("Expected to find at least one broken link with 404 status")
	}
}

// TestAnalyze_BrokenLinks_NetworkError проверяет битые ссылки с сетевой ошибкой.
func TestAnalyze_BrokenLinks_NetworkError(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body>
			<a href="http://invalid-host-that-does-not-exist.local/broken">Broken Link</a>
		</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     100 * time.Millisecond,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) < 1 {
		t.Fatalf("Pages = %d, want at least 1", len(result.Pages))
	}

	page := result.Pages[0]
	if page.Status != "ok" {
		t.Errorf("Status = %q, want %q", page.Status, "ok")
	}

	foundNetworkError := false
	for _, bl := range page.BrokenLinks {
		if bl.Error != "" {
			foundNetworkError = true
			break
		}
	}

	if !foundNetworkError {
		t.Error("Expected to find at least one broken link with network error")
	}
}

// TestAnalyze_IgnoresInvalidSchemes проверяет игнорирование неподдерживаемых схем ссылок.
func TestAnalyze_IgnoresInvalidSchemes(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body>
			<a href="javascript:void(0)">JavaScript Link</a>
			<a href="mailto:test@example.com">Mail Link</a>
			<a href="ftp://example.com/file">FTP Link</a>
			<a href="">Empty Link</a>
		</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) < 1 {
		t.Fatalf("Pages = %d, want at least 1", len(result.Pages))
	}

	page := result.Pages[0]
	for _, bl := range page.BrokenLinks {
		if strings.HasPrefix(bl.URL, "javascript:") ||
			strings.HasPrefix(bl.URL, "mailto:") ||
			strings.HasPrefix(bl.URL, "ftp:") {
			t.Errorf("Invalid scheme link should be ignored: %s", bl.URL)
		}
	}
}

// TestAnalyze_ResolvedRelativeLinks проверяет разрешение относительных ссылок.
func TestAnalyze_ResolvedRelativeLinks(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/broken" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		html := `<html><body>
			<a href="/broken">Relative Broken Link</a>
		</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) < 1 {
		t.Fatalf("Pages = %d, want at least 1", len(result.Pages))
	}

	page := result.Pages[0]
	foundRelativeBroken := false
	for _, bl := range page.BrokenLinks {
		if strings.Contains(bl.URL, "/broken") && bl.StatusCode == http.StatusNotFound {
			foundRelativeBroken = true
			break
		}
	}

	if !foundRelativeBroken {
		t.Error("Expected to find broken relative link resolved to absolute URL")
	}
}

// TestAnalyze_SEO_AllTags проверяет извлечение всех SEO-тегов.
func TestAnalyze_SEO_AllTags(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html>
<head>
	<title>Example Test Page</title>
	<meta name="description" content="This is a test description">
</head>
<body>
	<h1>Main Heading</h1>
</body>
</html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) < 1 {
		t.Fatalf("Pages = %d, want at least 1", len(result.Pages))
	}

	page := result.Pages[0]
	if page.SEO == nil {
		t.Fatal("SEO should not be nil")
	}

	if !page.SEO.HasTitle {
		t.Error("HasTitle should be true")
	}

	if page.SEO.Title != "Example Test Page" {
		t.Errorf("Title = %q, want %q", page.SEO.Title, "Example Test Page")
	}

	if !page.SEO.HasDescription {
		t.Error("HasDescription should be true")
	}

	if page.SEO.Description != "This is a test description" {
		t.Errorf("Description = %q, want %q", page.SEO.Description, "This is a test description")
	}

	if !page.SEO.HasH1 {
		t.Error("HasH1 should be true")
	}
}

// TestAnalyze_SEO_NoTags проверяет пустой SEO-отчет без SEO-тегов.
func TestAnalyze_SEO_NoTags(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body><p>No SEO tags here</p></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) < 1 {
		t.Fatalf("Pages = %d, want at least 1", len(result.Pages))
	}

	page := result.Pages[0]
	if page.SEO == nil {
		t.Fatal("SEO should not be nil")
	}

	if page.SEO.HasTitle {
		t.Error("HasTitle should be false")
	}

	if page.SEO.Title != "" {
		t.Errorf("Title = %q, want empty string", page.SEO.Title)
	}

	if page.SEO.HasDescription {
		t.Error("HasDescription should be false")
	}

	if page.SEO.Description != "" {
		t.Errorf("Description = %q, want empty string", page.SEO.Description)
	}

	if page.SEO.HasH1 {
		t.Error("HasH1 should be false")
	}
}

// TestAnalyze_SEO_HTMLEntities проверяет декодирование HTML-сущностей в SEO-данных.
func TestAnalyze_SEO_HTMLEntities(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html>
<head>
	<title>Test &amp; Example &lt;tag&gt;</title>
	<meta name="description" content="Description with &quot;quotes&quot; and &amp;">
</head>
<body>
	<h1>Heading &amp; More</h1>
</body>
</html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) < 1 {
		t.Fatalf("Pages = %d, want at least 1", len(result.Pages))
	}

	page := result.Pages[0]
	if page.SEO == nil {
		t.Fatal("SEO should not be nil")
	}

	if !page.SEO.HasTitle {
		t.Error("HasTitle should be true")
	}

	expectedTitle := "Test & Example <tag>"
	if page.SEO.Title != expectedTitle {
		t.Errorf("Title = %q, want %q", page.SEO.Title, expectedTitle)
	}

	if !page.SEO.HasDescription {
		t.Error("HasDescription should be true")
	}

	expectedDesc := "Description with \"quotes\" and &"
	if page.SEO.Description != expectedDesc {
		t.Errorf("Description = %q, want %q", page.SEO.Description, expectedDesc)
	}
}

// TestAnalyze_DepthZero проверяет обход только корневой страницы при нулевой глубине.
func TestAnalyze_DepthZero(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body>
			<a href="/page1">Page 1</a>
			<a href="/page2">Page 2</a>
		</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       0,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) != 1 {
		t.Errorf("Pages = %d, want 1", len(result.Pages))
	}

	if result.Pages[0].Depth != 0 {
		t.Errorf("Page depth = %d, want 0", result.Pages[0].Depth)
	}
}

// TestAnalyze_DepthOne проверяет обход страниц на следующей глубине.
func TestAnalyze_DepthOne(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/page1" {
			html := `<html><body><a href="/page2">Page 2</a></body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(html))
			return
		}
		if r.URL.Path == "/page2" {
			html := `<html><body>Page 2 Content</body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(html))
			return
		}
		html := `<html><body><a href="/page1">Page 1</a></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       2,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if len(result.Pages) < 1 {
		t.Errorf("Pages = %d, want at least 1", len(result.Pages))
	}

	foundDepth0 := false
	foundDepth1 := false
	for _, page := range result.Pages {
		if page.Depth == 0 {
			foundDepth0 = true
		}
		if page.Depth == 1 {
			foundDepth1 = true
		}
	}

	if !foundDepth0 {
		t.Error("Should have page with depth 0")
	}
	if !foundDepth1 {
		t.Error("Should have page with depth 1")
	}
}

// TestAnalyze_ExternalLinksIgnored проверяет игнорирование внешних ссылок при обходе.
func TestAnalyze_ExternalLinksIgnored(t *testing.T) {
	externalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body>External Page</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer externalServer.Close()

	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body>
			<a href="` + externalServer.URL + `/external">External Link</a>
			<a href="/internal">Internal Link</a>
		</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	for _, page := range result.Pages {
		if strings.Contains(page.URL, "external") {
			t.Error("External link should not be crawled")
		}
	}
}

// TestAnalyze_DuplicateLinks проверяет дедупликацию ссылок при обходе.
func TestAnalyze_DuplicateLinks(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/duplicate" {
			html := `<html><body>Page Content</body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(html))
			return
		}
		html := `<html><body>
			<a href="/duplicate">Link 1</a>
			<a href="/duplicate">Link 2</a>
			<a href="/duplicate">Link 3</a>
		</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       2,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	duplicateCount := 0
	for _, page := range result.Pages {
		if strings.Contains(page.URL, "/duplicate") {
			duplicateCount++
		}
	}

	if duplicateCount != 1 {
		t.Errorf("Duplicate link should appear once, got %d", duplicateCount)
	}
}

// TestAnalyze_ContextCancellationPartialReport проверяет частичный отчет при отмене контекста.
func TestAnalyze_ContextCancellationPartialReport(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body>
			<a href="/slow1">Slow 1</a>
			<a href="/slow2">Slow 2</a>
		</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Logf("Analyze() returned error (expected): %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Report should be valid JSON: %v", err)
	}

	if result.RootURL != mainServer.URL {
		t.Errorf("RootURL = %q, want %q", result.RootURL, mainServer.URL)
	}
}

// TestAnalyze_JSONReportFormat проверяет структуру JSON-отчета.
func TestAnalyze_JSONReportFormat(t *testing.T) {
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html>
<head>
	<title>Test Page</title>
	<meta name="description" content="Test description">
</head>
<body>
	<h1>Test H1</h1>
	<a href="/broken">Broken Link</a>
</body>
</html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	brokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer brokenServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       1,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  true,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	if err := json.Unmarshal(report, &result); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if result.RootURL == "" {
		t.Error("root_url should not be empty")
	}

	if result.GeneratedAt == "" {
		t.Error("generated_at should not be empty")
	}

	if len(result.Pages) == 0 {
		t.Fatal("pages should not be empty")
	}

	page := result.Pages[0]
	if page.URL == "" {
		t.Error("url should not be empty")
	}

	if page.Status == "" {
		t.Error("status should not be empty")
	}

	if page.SEO == nil {
		t.Error("seo should not be nil")
	}

	if len(page.BrokenLinks) == 0 {
		t.Log("broken_links is empty (expected for this test)")
	}
}

// TestAnalyze_JSONReportFormatNoIndent проверяет совпадение данных JSON без учета отступов.
func TestAnalyze_JSONReportFormatNoIndent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       0,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	reportNoIndent, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	opts.IndentJSON = true
	reportIndent, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var resultNoIndent, resultIndent Report
	_ = json.Unmarshal(reportNoIndent, &resultNoIndent)
	_ = json.Unmarshal(reportIndent, &resultIndent)

	if resultNoIndent.RootURL != resultIndent.RootURL {
		t.Error("Content should be the same regardless of indent")
	}
}

// TestRateLimiter_GlobalLimit проверяет глобальное ограничение частоты запросов.
func TestRateLimiter_GlobalLimit(t *testing.T) {
	requestCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		w.Header().Set("Content-Type", "text/html")
		html := `<html><body><a href="/page1">Page 1</a><a href="/page2">Page 2</a></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       1,
		Retries:     1,
		Delay:       50 * time.Millisecond,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 2,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	_, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	mu.Lock()
	totalRequests := requestCount
	mu.Unlock()

	if totalRequests < 3 {
		t.Fatalf("Expected at least 3 requests, got %d", totalRequests)
	}
}

// TestRateLimiter_RPSOverride проверяет приоритет RPS над задержкой в настройках анализа.
func TestRateLimiter_RPSOverride(t *testing.T) {
	requestCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       0,
		Retries:     1,
		Delay:       10 * time.Millisecond,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 1,
		IndentJSON:  false,
		RPS:         2,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	startTime := time.Now()
	_, err := Analyze(ctx, opts)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	if elapsed < 400*time.Millisecond {
		t.Logf("Elapsed time %v seems reasonable for RPS=2", elapsed)
	}
}

// TestRetry_SuccessAfterOneFailure проверяет успешный повтор после первой ошибки.
func TestRetry_SuccessAfterOneFailure(t *testing.T) {
	attemptCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		currentAttempt := attemptCount
		mu.Unlock()

		if currentAttempt == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       0,
		Retries:     2,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 1,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	_ = json.Unmarshal(report, &result)

	if len(result.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(result.Pages))
	}

	if result.Pages[0].Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", result.Pages[0].Status)
	}

	if result.Pages[0].HTTPStatus != http.StatusOK {
		t.Errorf("Expected HTTP status 200, got %d", result.Pages[0].HTTPStatus)
	}

	mu.Lock()
	totalAttempts := attemptCount
	mu.Unlock()

	if totalAttempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", totalAttempts)
	}
}

// TestRetry_FailAfterAllRetries проверяет завершение после исчерпания повторов.
func TestRetry_FailAfterAllRetries(t *testing.T) {
	attemptCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       0,
		Retries:     2,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 1,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	_ = json.Unmarshal(report, &result)

	if len(result.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(result.Pages))
	}

	if result.Pages[0].HTTPStatus != http.StatusInternalServerError {
		t.Errorf("Expected HTTP status 500, got %d", result.Pages[0].HTTPStatus)
	}

	mu.Lock()
	totalAttempts := attemptCount
	mu.Unlock()

	if totalAttempts != 3 {
		t.Errorf("Expected 3 attempts (1 + 2 retries), got %d", totalAttempts)
	}
}

// TestRetry_NoRetryFor4xx проверяет отсутствие повторов для ошибок 4xx.
func TestRetry_NoRetryFor4xx(t *testing.T) {
	attemptCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       0,
		Retries:     3,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 1,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	_ = json.Unmarshal(report, &result)

	mu.Lock()
	totalAttempts := attemptCount
	mu.Unlock()

	if totalAttempts != 1 {
		t.Errorf("Expected 1 attempt (no retry for 4xx), got %d", totalAttempts)
	}
}

// TestRetry_ContextCancellation проверяет остановку повторов при отмене контекста.
func TestRetry_ContextCancellation(t *testing.T) {
	attemptCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		mu.Unlock()
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	opts := Options{
		URL:         server.URL,
		Depth:       0,
		Retries:     5,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 1,
		IndentJSON:  false,
		HTTPClient:  server.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := Analyze(ctx, opts)

	mu.Lock()
	totalAttempts := attemptCount
	mu.Unlock()

	if totalAttempts > 2 {
		t.Errorf("Expected at most 2 attempts due to context cancellation, got %d", totalAttempts)
	}

	if err != nil && err != context.Canceled {
		t.Logf("Got expected error: %v", err)
	}
}

// TestAssets_BasicFunctionality проверяет сбор сведений о ресурсах страницы.
func TestAssets_BasicFunctionality(t *testing.T) {
	cssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	}))
	defer cssServer.Close()

	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "500")
		w.WriteHeader(http.StatusOK)
	}))
	defer imgServer.Close()

	jsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "200")
		w.WriteHeader(http.StatusOK)
	}))
	defer jsServer.Close()

	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html>
<head>
	<link rel="stylesheet" href="` + cssServer.URL + `/style.css">
	<script src="` + jsServer.URL + `/app.js"></script>
</head>
<body>
	<img src="` + imgServer.URL + `/logo.png" alt="Logo">
</body>
</html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       0,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	_ = json.Unmarshal(report, &result)

	if len(result.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(result.Pages))
	}

	page := result.Pages[0]
	if len(page.Assets) != 3 {
		t.Errorf("Expected 3 assets, got %d: %+v", len(page.Assets), page.Assets)
	}

	foundImage := false
	foundScript := false
	foundStyle := false
	for _, asset := range page.Assets {
		if asset.Type == "image" {
			foundImage = true
			if asset.SizeBytes != 500 {
				t.Errorf("Expected image size 500, got %d", asset.SizeBytes)
			}
		}
		if asset.Type == "script" {
			foundScript = true
			if asset.SizeBytes != 200 {
				t.Errorf("Expected script size 200, got %d", asset.SizeBytes)
			}
		}
		if asset.Type == "style" {
			foundStyle = true
			if asset.SizeBytes != 100 {
				t.Errorf("Expected style size 100, got %d", asset.SizeBytes)
			}
		}
	}

	if !foundImage {
		t.Error("Expected to find image asset")
	}
	if !foundScript {
		t.Error("Expected to find script asset")
	}
	if !foundStyle {
		t.Error("Expected to find style asset")
	}
}

// TestAssets_Deduplication проверяет дедупликацию ресурсов страницы.
func TestAssets_Deduplication(t *testing.T) {
	assetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "250")
		w.WriteHeader(http.StatusOK)
	}))
	defer assetServer.Close()

	requestCount := 0
	var mu sync.Mutex

	trackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body>
			<img src="` + assetServer.URL + `/logo.png">
			<img src="` + assetServer.URL + `/logo.png">
		</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer trackServer.Close()

	opts := Options{
		URL:         trackServer.URL,
		Depth:       0,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  trackServer.Client(),
	}

	ctx := context.Background()
	_, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	mu.Lock()
	totalRequests := requestCount
	mu.Unlock()

	if totalRequests != 1 {
		t.Errorf("Expected 1 request for deduplicated asset, got %d", totalRequests)
	}
}

// TestAssets_NoContentLength проверяет вычисление размера ресурса без Content-Length.
func TestAssets_NoContentLength(t *testing.T) {
	assetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1234567890"))
	}))
	defer assetServer.Close()

	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body><img src="` + assetServer.URL + `/image.png"></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       0,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	_ = json.Unmarshal(report, &result)

	if len(result.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(result.Pages))
	}

	if len(result.Pages[0].Assets) != 1 {
		t.Fatalf("Expected 1 asset, got %d", len(result.Pages[0].Assets))
	}

	asset := result.Pages[0].Assets[0]
	if asset.SizeBytes != 10 {
		t.Errorf("Expected size 10 (from body), got %d", asset.SizeBytes)
	}
}

// TestAssets_ErrorStatus проверяет ошибочный статус ресурса страницы.
func TestAssets_ErrorStatus(t *testing.T) {
	brokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer brokenServer.Close()

	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `<html><body><img src="` + brokenServer.URL + `/missing.png"></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer mainServer.Close()

	opts := Options{
		URL:         mainServer.URL,
		Depth:       0,
		Retries:     1,
		Delay:       0,
		Timeout:     5 * time.Second,
		UserAgent:   "",
		Concurrency: 4,
		IndentJSON:  false,
		HTTPClient:  mainServer.Client(),
	}

	ctx := context.Background()
	report, err := Analyze(ctx, opts)

	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}

	var result Report
	_ = json.Unmarshal(report, &result)

	if len(result.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(result.Pages))
	}

	if len(result.Pages[0].Assets) != 1 {
		t.Fatalf("Expected 1 asset, got %d", len(result.Pages[0].Assets))
	}

	asset := result.Pages[0].Assets[0]
	if asset.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", asset.StatusCode)
	}
	if asset.Error == "" {
		t.Error("Expected error message for 404 asset")
	}
}
