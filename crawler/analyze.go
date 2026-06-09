package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
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
	URL        string `json:"url"`
	Depth      int    `json:"depth"`
	HTTPStatus int    `json:"http_status"`
	Status     string `json:"status"`
	Error      string `json:"error"`
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
		URL:   opts.URL,
		Depth: 0,
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
		_, _ = io.Copy(io.Discard, resp.Body)
		page.HTTPStatus = resp.StatusCode
		page.Status = "ok"
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
