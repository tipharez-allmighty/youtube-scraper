package api

import (
	"context"
	"fmt"
	"log/slog"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/input"
	"tipharez-allmighty/youtube-scraper/internal/storage"
	"tipharez-allmighty/youtube-scraper/internal/youtube"
)

type (
	SearchJobFunc func(ctx context.Context, cfg *config.Config, payload input.InputSchema, job storage.Job)
	ResumeJobFunc func(ctx context.Context, cfg *config.Config, jobID string, payload input.InputSchema, tasks []storage.Task)
)

func YoutubeSearchJob(ctx context.Context, cfg *config.Config, payload input.InputSchema, job storage.Job) {
	store, err := storage.GetStore(cfg, "")
	if err != nil {
		slog.Error("failed to load storage during youtube search job", "error", err)
		return
	}
	defer store.Close()
	defer func() { store.FailInterruptedTasks(ctx, job.ID, err) }()
	client := youtube.New(cfg.YoutubeAPIKey, cfg.YoutubeBaseURL)
	if err = youtube.RunSearch(ctx, cfg, client, store, job, payload); err != nil {
		err = fmt.Errorf("failed to run youtube search: %w", err)
		return
	}
}

func YoutubeResumeJob(ctx context.Context, cfg *config.Config, jobID string, payload input.InputSchema, tasks []storage.Task) {
	store, err := storage.GetStore(cfg, "")
	if err != nil {
		slog.Error("failed to load storage during youtube resume job", "error", err)
		return
	}
	defer store.Close()
	defer func() { store.FailInterruptedTasks(ctx, jobID, err) }()
	client := youtube.New(cfg.YoutubeAPIKey, cfg.YoutubeBaseURL)

	if err = youtube.ResumeSearchTasks(ctx, cfg, client, store, payload, tasks); err != nil {
		err = fmt.Errorf("failed to resume tasks: %w", err)
		return
	}
}
