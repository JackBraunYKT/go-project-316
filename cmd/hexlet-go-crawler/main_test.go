package main

import (
	"bytes"
	"encoding/json"
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

func TestRunPrintsOnlyJSONWithTrailingNewline(t *testing.T) {
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
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	output := stdout.String()
	if !strings.HasSuffix(output, "\n") {
		t.Fatalf("stdout must end with one newline: %q", output)
	}

	jsonPayload := strings.TrimSuffix(output, "\n")
	if strings.HasSuffix(jsonPayload, "\n") {
		t.Fatalf("stdout has more than one trailing newline: %q", output)
	}
	if jsonPayload == "" || jsonPayload[0] != '{' {
		t.Fatalf("stdout must start with JSON object: %q", output)
	}
	if jsonPayload[len(jsonPayload)-1] != '}' {
		t.Fatalf("stdout must contain no extra data before final newline: %q", output)
	}
	if !json.Valid([]byte(jsonPayload)) {
		t.Fatalf("stdout before final newline must be valid JSON only: %q", output)
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
