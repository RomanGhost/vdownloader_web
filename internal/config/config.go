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

	// KafkaBrokers is a comma-separated list of Kafka broker addresses.
	// Env: KAFKA_BROKERS  Default: localhost:9092
	KafkaBrokers string

	// KafkaJobsTopic is the topic download job requests are published to.
	// Env: KAFKA_JOBS_TOPIC  Default: video.jobs
	KafkaJobsTopic string
}

// Load reads .env (if present) and returns the populated Config.
func Load() Config {
	_ = godotenv.Load()
	return Config{
		WorkerURL:      getenv("WORKER_URL", "http://localhost:8080"),
		HTTPAddr:       getenv("HTTP_ADDR", ":8082"),
		KafkaBrokers:   getenv("KAFKA_BROKERS", "localhost:9092"),
		KafkaJobsTopic: getenv("KAFKA_JOBS_TOPIC", "video.jobs"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
