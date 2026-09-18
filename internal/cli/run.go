package cli

import (
	"context"
	"fmt"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/input"
	"tipharez-allmighty/youtube-scraper/internal/storage"
	"tipharez-allmighty/youtube-scraper/internal/youtube"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type RunCmd struct {
	Format string `kong:"help='Input format (json or yaml)',enum='json,yaml',default='json',short='f'"`
}

func (r *RunCmd) Run(ctx context.Context, cfg *config.Config) (err error) {
	var payload input.InputSchema
	if err := input.DecodePayload(r.Format, &payload); err != nil {
		return fmt.Errorf("invalid input format: %w", err)
	}
	inputValidator := validator.New()
	if err := inputValidator.Struct(payload); err != nil {
		return fmt.Errorf("wrong input structure: %w", err)
	}

	store, err := storage.GetStore(cfg, payload.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %v", err)
	}
	defer store.Close()
	jobID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate uuid7 for new job: %w", err)
	}
	job := storage.Job{ID: jobID.String(), Input: payload}
	if err := store.InsertJob(job); err != nil {
		return fmt.Errorf("failed to create a job: %w", err)
	}
	defer func() { store.FailInterruptedTasks(ctx, job.ID, err) }()

	client := youtube.New(cfg.YoutubeAPIKey, cfg.YoutubeBaseURL)
	if err := youtube.RunSearch(ctx, cfg, client, store, job, payload); err != nil {
		return fmt.Errorf("failed to run youtube search: %w", err)
	}
	return nil
}
