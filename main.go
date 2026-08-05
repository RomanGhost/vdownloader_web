// Command webui serves a browser UI for the vdownloader worker.
//
// It has no business logic of its own: /api/* and /files/* are reverse-proxied
// straight to the worker's REST API, and the static page builds requests and
// polls job status client-side. See static/app.js.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"webui/internal/config"
)

//go:embed static
var staticFiles embed.FS

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

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/files/", proxy)
	mux.Handle("/", http.FileServerFS(staticRoot))

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("starting webui", "addr", cfg.HTTPAddr, "worker_url", cfg.WorkerURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
		}
	}()

	<-ctx.Done()

	if err := srv.Shutdown(context.Background()); err != nil {
		logger.Error("server shutdown", "err", err)
	}
}
