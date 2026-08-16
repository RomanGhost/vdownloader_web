// Command webui serves a browser UI for the vdownloader worker.
//
// GET /api/formats, GET /api/jobs/*, and /files/* are reverse-proxied straight
// to the worker's REST API. POST /api/jobs is handled locally: the browser
// can't speak Kafka directly, so this service publishes the job request to
// the worker's job-requests topic on the browser's behalf and hands back the
// generated file_id for the static page to poll. See static/app.js.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"webui/internal/config"
)

//go:embed static
var staticFiles embed.FS

// Packing static to bin file

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	target, err := url.Parse(cfg.WorkerURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid WORKER_URL: %v\n", err)
		os.Exit(1)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	jobsWriter := &kafka.Writer{
		Addr:                   kafka.TCP(strings.Split(cfg.KafkaBrokers, ",")...),
		Topic:                  cfg.KafkaJobsTopic,
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
	}
	defer jobsWriter.Close()

	mux := http.NewServeMux()
	mux.Handle("/api/jobs", handleCreateJob(jobsWriter, proxy, logger))
	mux.Handle("/api/", proxy)
	mux.Handle("/files/", proxy)
	mux.Handle("/", http.FileServerFS(staticRoot))

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("starting webui", "addr", cfg.HTTPAddr, "worker_url", cfg.WorkerURL, "kafka_jobs_topic", cfg.KafkaJobsTopic)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
		}
	}()

	<-ctx.Done()

	if err := srv.Shutdown(context.Background()); err != nil {
		logger.Error("server shutdown", "err", err)
	}
}

// jobRequest is the JSON body the static page posts to start a download.
// Must stay in sync with the worker's api.jobRequest wire format: Kind is
// "video" (Height + WithAudio apply) or "audio" (AudioFormat applies).
type jobRequest struct {
	FileID   string  `json:"file_id"`
	URL      string  `json:"url"`
	Title    string  `json:"title,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Kind     string  `json:"kind"`

	Height    int  `json:"height,omitempty"`
	WithAudio bool `json:"with_audio,omitempty"`

	AudioFormat string `json:"audio_format,omitempty"`
}

// jobPublisher is the subset of *kafka.Writer that handleCreateJob needs,
// so tests can exercise the handler's validation/response logic with a fake
// in-memory publisher instead of a real Kafka broker.
type jobPublisher interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

// handleCreateJob intercepts POST /api/jobs and publishes the request to
// Kafka instead of proxying it (the worker no longer accepts job submissions
// over HTTP). Any other method on this path (e.g. GET to list jobs) falls
// through to the worker's REST API.
func handleCreateJob(writer jobPublisher, proxy http.Handler, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			proxy.ServeHTTP(w, r)
			return
		}

		var req jobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if req.URL == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}
		if req.Kind != "video" && req.Kind != "audio" {
			http.Error(w, `kind must be "video" or "audio"`, http.StatusBadRequest)
			return
		}
		if req.Kind == "video" && req.Height <= 0 {
			http.Error(w, "height is required for kind=video", http.StatusBadRequest)
			return
		}
		req.FileID = uuid.NewString()

		body, err := json.Marshal(req)
		if err != nil {
			log.Error("marshal job request", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if err := writer.WriteMessages(r.Context(), kafka.Message{
			Key:   []byte(req.FileID),
			Value: body,
		}); err != nil {
			log.Error("publish job request", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		log.Info("job request published", "file_id", req.FileID, "url", req.URL)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"file_id": req.FileID}) //nolint:errcheck
	}
}
