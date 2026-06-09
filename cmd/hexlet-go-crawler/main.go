package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"code/crawler"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, &http.Client{}))
}

func run(args []string, stdout, stderr io.Writer, client *http.Client) int {
	fs := flag.NewFlagSet("hexlet-go-crawler", flag.ContinueOnError)
	fs.SetOutput(stderr)

	depth := fs.Int("depth", 10, "crawl depth")
	retries := fs.Int("retries", 1, "number of retries for failed requests")
	delay := fs.Duration("delay", 0*time.Second, "delay between requests (example: 200ms, 1s)")
	timeout := fs.Duration("timeout", 15*time.Second, "per-request timeout")
	rps := fs.Float64("rps", 0, "limit requests per second (overrides delay)")
	userAgent := fs.String("user-agent", "", "custom user agent")
	workers := fs.Int("workers", 4, "number of concurrent workers")

	fs.Usage = func() {
		fmt.Fprintln(stderr, "NAME:")
		fmt.Fprintln(stderr, "   hexlet-go-crawler - analyze a website structure")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "USAGE:")
		fmt.Fprintln(stderr, "   hexlet-go-crawler [global options] <url>")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "GLOBAL OPTIONS:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 0
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "URL is required")
		fs.Usage()
		return 0
	}

	effectiveDelay := *delay
	if *rps > 0 {
		effectiveDelay = time.Duration(float64(time.Second) / *rps)
	}

	report, err := crawler.Analyze(context.Background(), crawler.Options{
		URL:         fs.Arg(0),
		Depth:       *depth,
		Retries:     *retries,
		Delay:       effectiveDelay,
		Timeout:     *timeout,
		UserAgent:   *userAgent,
		Concurrency: *workers,
		IndentJSON:  true,
		HTTPClient:  client,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 0
	}

	_, _ = stdout.Write(report)
	fmt.Fprintln(stdout)
	return 0
}
