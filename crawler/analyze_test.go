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

func decodeReport(t *testing.T, reportBytes []byte) analyzeReport {
	t.Helper()

	var report analyzeReport
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if len(report.Pages) != 1 {
		t.Fatalf("pages length = %d, want 1", len(report.Pages))
	}

	return report
}

func newClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
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
