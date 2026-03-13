package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"siptunnel/internal/smoketest"
)

func main() {
	var (
		baseURL    = flag.String("base-url", "http://127.0.0.1:18080", "gateway base url")
		configPath = flag.String("config", "./configs/config.yaml", "gateway config path")
		timeout    = flag.Duration("timeout", 3*time.Second, "single check timeout")
	)
	flag.Parse()

	ctx := context.Background()
	result := smoketest.Run(ctx, smoketest.Options{
		BaseURL:    *baseURL,
		ConfigPath: *configPath,
		Timeout:    *timeout,
	})
	fmt.Print(smoketest.FormatSummary(result))
	if !result.Passed() {
		os.Exit(1)
	}
}
