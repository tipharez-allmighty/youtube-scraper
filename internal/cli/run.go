package cli

import (
	"context"
	"fmt"
	"sync"

	"tipharez-allmighty/youtube-scraper/internal/channel"
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

func (r *RunCmd) Run(ctx context.Context, cfg *config.Config) error {
	var payload input.InputSchema
	if err := input.DecodePayload(r.Format, &payload); err != nil {
		return fmt.Errorf("invalid input format: %w", err)
	}
	inputValidator := validator.New()
	if err := inputValidator.Struct(payload); err != nil {
		return fmt.Errorf("wrong input structure: %w", err)
	}

	store, err := getStore(cfg, payload.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %v", err)
	}
	jobID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate uuid7 for new job: %w", err)
	}
	job := storage.Job{ID: jobID.String(), Input: payload}
	if err := store.InsertJob(job); err != nil {
		return fmt.Errorf("failed to create a job: %w", err)
	}
	defer failInterruptedTasks(ctx, store, job.ID)

	client := youtube.New(cfg.YoutubeAPIKey)

	queryCh := make(chan input.Query, cfg.BufferSize)
	threadCh := make(chan youtube.ThreadsContext, cfg.BufferSize)
	commentCh := make(chan youtube.CommentsContext, cfg.BufferSize)
	var searchWg sync.WaitGroup
	var commentThreadWg sync.WaitGroup
	var commentWg sync.WaitGroup

	queryContext := youtube.Context{
		JobID:      job.ID,
		MaxResults: payload.MaxResultsPerQuery,
	}
	for range cfg.NumWorkers {
		searchWg.Go(func() {
			for query := range queryCh {
				youtube.RunPagination(ctx, payload.MaxPages, "", func(pageToken string) (string, error) {
					return youtube.GetVideos(
						ctx,
						client,
						store,
						cfg,
						youtube.VideosContext{
							Context:   queryContext,
							PageToken: pageToken,
							Query:     query.Text,
						},
						threadCh,
					)
				})
			}
		})
	}
	for range cfg.NumWorkers {
		commentThreadWg.Go(func() {
			for threadCtx := range threadCh {
				youtube.RunPagination(ctx, payload.MaxThreads, "", func(pageToken string) (string, error) {
					threadCtx.PageToken = pageToken
					return youtube.GetCommentThreads(
						ctx,
						client,
						store,
						cfg,
						threadCtx,
						commentCh,
					)
				})
			}
		})
	}
	for range cfg.NumWorkers {
		commentWg.Go(func() {
			for commentCtx := range commentCh {
				youtube.RunPagination(ctx, payload.MaxComments, "", func(pageToken string) (string, error) {
					commentCtx.PageToken = pageToken
					return youtube.GetComments(
						client,
						store,
						cfg,
						commentCtx,
					)
				})
			}
		})
	}
	go channel.CloseWhenDone(&searchWg, threadCh)
	go channel.CloseWhenDone(&commentThreadWg, commentCh)
	for _, query := range payload.Queries {
		if err := channel.TryChannel(ctx, queryCh, query); err != nil {
			close(queryCh)
			return err
		}
	}
	close(queryCh)
	commentWg.Wait()
	return nil
}
