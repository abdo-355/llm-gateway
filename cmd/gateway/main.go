package main

import (
	"context"
	"log"

	"github.com/abdo-355/llm-gateway/internal/app"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	app, err := app.New(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	app.Start()
}
