package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"code/crawler"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:        "hexlet-go-crawler",
		Usage:       "analyze a website structure",
		UsageText:   "hexlet-go-crawler [global options] <url>",
		Description: "A tool to crawl and analyze website structure",
		Version:     "1.0.0",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "depth",
				Usage:   "crawl depth",
				Value:   10,
			},
			&cli.IntFlag{
				Name:    "retries",
				Usage:   "number of retries for failed requests",
				Value:   1,
			},
			&cli.DurationFlag{
				Name:    "delay",
				Usage:   "delay between requests (example: 200ms, 1s)",
				Value:   0,
			},
			&cli.DurationFlag{
				Name:    "timeout",
				Usage:   "per-request timeout",
				Value:   15 * time.Second,
			},
			&cli.StringFlag{
				Name:    "user-agent",
				Usage:   "custom user agent",
				Value:   "",
			},
			&cli.IntFlag{
				Name:    "workers",
				Usage:   "number of concurrent workers",
				Value:   4,
			},
			&cli.BoolFlag{
				Name:    "indent",
				Usage:   "indent JSON output",
				Value:   true,
			},
			&cli.IntFlag{
				Name:    "rps",
				Usage:   "requests per second limit (overrides delay)",
				Value:   0,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			url := c.Args().First()
			if url == "" {
				return cli.Exit("URL is required", 1)
			}

			opts := crawler.Options{
				URL:         url,
				Depth:       c.Int("depth"),
				Retries:     c.Int("retries"),
				Delay:       c.Duration("delay"),
				Timeout:     c.Duration("timeout"),
				UserAgent:   c.String("user-agent"),
				Concurrency: c.Int("workers"),
				IndentJSON:  c.Bool("indent"),
				RPS:         c.Int("rps"),
				HTTPClient:  &http.Client{Timeout: c.Duration("timeout")},
			}

			report, err := crawler.Analyze(ctx, opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}

			fmt.Println(string(report))
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
