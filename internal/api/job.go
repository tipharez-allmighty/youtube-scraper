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

type JobFunc func(ctx context.Context, cfg *config.Config, payload input.InputSchema, job storage.Job)

func YoutubeSearchJob(ctx context.Context, cfg *config.Config, payload input.InputSchema, job storage.Job) {
	store, err := storage.GetStore(cfg, payload.StateFile)
	if err != nil {
		slog.Error("failed to load storage during youtube search", "error", err)
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

