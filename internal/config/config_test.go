package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"WORKER_URL", "HTTP_ADDR", "KAFKA_BROKERS", "KAFKA_JOBS_TOPIC"} {
		t.Setenv(key, "")
	}

	got := Load()
	want := Config{
		WorkerURL:      "http://localhost:8080",
		HTTPAddr:       ":8082",
		KafkaBrokers:   "localhost:9092",
		KafkaJobsTopic: "video.jobs",
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadEnvOverridesDefaults(t *testing.T) {
	t.Setenv("WORKER_URL", "http://worker:8080")
	t.Setenv("HTTP_ADDR", ":9999")

	got := Load()
	if got.WorkerURL != "http://worker:8080" {
		t.Errorf("WorkerURL = %q, want %q", got.WorkerURL, "http://worker:8080")
	}
	if got.HTTPAddr != ":9999" {
		t.Errorf("HTTPAddr = %q, want %q", got.HTTPAddr, ":9999")
	}
}
