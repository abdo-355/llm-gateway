package main

import (
	"context"
	"log"

	"github.com/abdo-355/llm-gateway/internal/app"
	"github.com/abdo-355/llm-gateway/internal/config"
)

func main() {
	if err := config.LoadDotEnv(); err != nil {
		log.Fatal(err)
	}

	app, err := app.New(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	app.Start()
}
