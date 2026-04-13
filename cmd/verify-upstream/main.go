package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/verification"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	provider := flag.String("provider", "", "Only test a single provider ID")
	model := flag.String("model", "", "Only test a single model ID")
	timeout := flag.Duration("timeout", 30*time.Second, "Per-request timeout")
	failFast := flag.Bool("fail-fast", false, "Stop on the first failure")
	flag.Parse()

	logger.Init("verify-upstream", getEnvString("ENV", "development"), getEnvString("LOG_LEVEL", "info"))

	report, err := verification.Run(context.Background(), verification.Config{
		Provider: *provider,
		Model:    *model,
		Timeout:  *timeout,
		FailFast: *failFast,
		Progress: os.Stderr,
	})
	if report != nil {
		verification.PrintReport(os.Stdout, report)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getEnvString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
