package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestAnalyzeUsesInjectedHTTPClientAndReportsRootPage(t *testing.T) {
	var requestedURL string
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedURL = req.URL.String()

			if got := req.Header.Get("User-Agent"); got != "crawler-test" {
				t.Fatalf("User-Agent = %q, want crawler-test", got)
			}

			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	reportBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		Depth:      1,
		UserAgent:  "crawler-test",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if requestedURL != "https://example.com" {
		t.Fatalf("requested URL = %q, want https://example.com", requestedURL)
	}

	var report struct {
		RootURL     string `json:"root_url"`
		Depth       int    `json:"depth"`
		GeneratedAt string `json:"generated_at"`
		Pages       []struct {
			URL        string `json:"url"`
			Depth      int    `json:"depth"`
			HTTPStatus int    `json:"http_status"`
			Status     string `json:"status"`
			Error      string `json:"error"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}

	if report.RootURL != "https://example.com" {
		t.Fatalf("root_url = %q, want https://example.com", report.RootURL)
	}
	if report.Depth != 1 {
		t.Fatalf("depth = %d, want 1", report.Depth)
	}
	if report.GeneratedAt == "" {
		t.Fatal("generated_at is empty")
	}
	if len(report.Pages) != 1 {
		t.Fatalf("pages length = %d, want 1", len(report.Pages))
	}

	page := report.Pages[0]
	if page.URL != "https://example.com" {
		t.Fatalf("page url = %q, want https://example.com", page.URL)
	}
	if page.Depth != 0 {
		t.Fatalf("page depth = %d, want 0", page.Depth)
	}
	if page.HTTPStatus != http.StatusNoContent {
		t.Fatalf("http_status = %d, want %d", page.HTTPStatus, http.StatusNoContent)
	}
	if page.Status != "ok" {
		t.Fatalf("status = %q, want ok", page.Status)
	}
	if page.Error != "" {
		t.Fatalf("error = %q, want empty", page.Error)
	}
}

func TestAnalyzeRecordsNetworkErrorsInReport(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network is unavailable")
		}),
	}

	reportBytes, err := Analyze(context.Background(), Options{
		URL:        "https://example.com",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	var report struct {
		Pages []struct {
			HTTPStatus int    `json:"http_status"`
			Status     string `json:"status"`
			Error      string `json:"error"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if len(report.Pages) != 1 {
		t.Fatalf("pages length = %d, want 1", len(report.Pages))
	}

	page := report.Pages[0]
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
