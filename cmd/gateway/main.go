package main

import (
	"log"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/server"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	env := config.GetEnv()
	logger.Init("llm-gateway", env.Environment)

	srv := server.New()
	srv.Start()
}
