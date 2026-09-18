package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"tipharez-allmighty/youtube-scraper/internal/api"
	"tipharez-allmighty/youtube-scraper/internal/config"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	mux := http.NewServeMux()

	mux.HandleFunc("POST /youtube/jobs", api.CreateJob(ctx, cfg))
	mux.HandleFunc("GET /jobs/status/{id}", api.GetJobStatus(cfg))
	mux.HandleFunc("GET /jobs", api.GetJobs(cfg))

	serverAddr := ":8080"
	logger.Info("Server is listening.", "port", serverAddr)
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		logger.Error("Server failed to start", "error", err)
	}
}
