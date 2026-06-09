package main

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestRunPrintsReportAndReturnsZeroWhenNetworkFails(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		}),
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"https://example.com"}, &stdout, &stderr, client)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	output := stdout.String()
	for _, want := range []string{
		`"root_url": "https://example.com"`,
		`"http_status": 0`,
		`"status": "error"`,
		"dial failed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout does not contain %q:\n%s", want, output)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunWithoutURLShowsMessageAndReturnsZero(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{}, &stdout, &stderr, &http.Client{})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "URL is required") {
		t.Fatalf("stderr = %q, want URL required message", stderr.String())
	}
}
