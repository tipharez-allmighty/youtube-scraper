package cli

import (
	"context"
	"fmt"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/storage"
	"tipharez-allmighty/youtube-scraper/internal/youtube"
)

type ResumeCmd struct {
	JobID     string `arg:"" help:"Job ID for loading failed tasks"`
	StateFile string `short:"s" optional:"" help:"Path to state file"`
}

func (r *ResumeCmd) Run(ctx context.Context, cfg *config.Config) (err error) {
	store, err := storage.GetStore(cfg, r.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %w", err)
	}
	defer store.Close()
	defer func() { store.FailInterruptedTasks(ctx, r.JobID, err) }()
	jobInput, err := store.SelectJobInput(r.JobID)
	if err != nil {
		return fmt.Errorf("failed to load job's input: %w", err)
	}
	tasks, err := store.SelectFailedTasks(r.JobID)
	if err != nil {
		return fmt.Errorf("failed to load failed tasks: %w", err)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("no failed tasks found for this job")
	}
	client := youtube.New(cfg.YoutubeAPIKey, cfg.YoutubeBaseURL)

	if err := youtube.ResumeSearchTasks(ctx, cfg, client, store, jobInput, tasks); err != nil {
		return fmt.Errorf("failed to resume tasks: %w", err)
	}

	return nil
}
