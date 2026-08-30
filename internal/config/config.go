// Package config loads application settings from environment variables.
// Priority (highest → lowest): environment variable > .env file > default.
package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	// WorkerURL is the base URL of the vdownloader worker HTTP server.
	// All /api/* and /files/* requests are reverse-proxied there.
	// Env: WORKER_URL  Default: http://localhost:8080
	WorkerURL string

	// HTTPAddr is the address this service listens on.
	// Env: HTTP_ADDR  Default: :8082
	HTTPAddr string

	// RabbitURL is the RabbitMQ connection URL. POST /api/jobs publishes the
	// job request to the "video.jobs" queue (queue name is a constant in
	// internal/mq).
	// Env: RABBITMQ_URL  Default: amqp://guest:guest@localhost:5672/
	RabbitURL string
}

// Load reads .env (if present) and returns the populated Config.
func Load() Config {
	_ = godotenv.Load()
	return Config{
		WorkerURL: getenv("WORKER_URL", "http://localhost:8080"),
		HTTPAddr:  getenv("HTTP_ADDR", ":8082"),
		RabbitURL: getenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
