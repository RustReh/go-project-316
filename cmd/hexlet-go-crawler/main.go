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
		Name:      "hexlet-go-crawler",
		Usage:     "analyze a website structure",
		ArgsUsage: "<url>",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "depth",
				Usage: "crawl depth",
				Value: 10,
			},
			&cli.IntFlag{
				Name:  "retries",
				Usage: "number of retries for failed requests",
				Value: 1,
			},
			&cli.DurationFlag{
				Name:  "delay",
				Usage: "delay between requests (example: 200ms, 1s)",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "per-request timeout",
				Value: 15 * time.Second,
			},
			&cli.Float64Flag{
				Name:  "rps",
				Usage: "limit requests per second (overrides delay)",
			},
			&cli.StringFlag{
				Name:  "user-agent",
				Usage: "custom user agent",
			},
			&cli.IntFlag{
				Name:  "workers",
				Usage: "number of concurrent workers",
				Value: 4,
			},
		},
		Action: run,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() == 0 {
		return cli.ShowAppHelp(cmd)
	}

	timeout := cmd.Duration("timeout")
	client := &http.Client{Timeout: timeout}

	opts := crawler.Options{
		URL:         cmd.Args().First(),
		Depth:       cmd.Int("depth"),
		Retries:     cmd.Int("retries"),
		Delay:       cmd.Duration("delay"),
		Timeout:     timeout,
		RPS:         cmd.Float64("rps"),
		UserAgent:   cmd.String("user-agent"),
		Concurrency: cmd.Int("workers"),
		HTTPClient:  client,
	}

	data, err := crawler.Analyze(ctx, opts)
	if err != nil {
		return err
	}

	_, err = fmt.Println(string(data))
	return err
}
