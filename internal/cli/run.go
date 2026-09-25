// Package cli.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"text/tabwriter"
	"time"

	"tipharez-allmighty/youtube-scraper/internal/channel"
	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/export"
	"tipharez-allmighty/youtube-scraper/internal/input"
	"tipharez-allmighty/youtube-scraper/internal/storage"
	"tipharez-allmighty/youtube-scraper/internal/youtube"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type CLI struct {
	Run     RunCmd    `cmd:"run" help:"Run a scraping job"`
	Resume  ResumeCmd `cmd:"resume" help:"Resume failed tasks"`
	Jobs    JobsCmd   `cmd:"jobs" help:"List jobs"`
	Export  ExportCmd `cmd:"export" help:"Export data in csv format"`
	Verbose int       `help:"Increase verbosity." short:"v" type:"counter"`
}

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

	if err := youtube.ResumeSearchTasks(ctx, cfg, client, store, *jobInput, tasks); err != nil {
		return fmt.Errorf("failed to resume tasks: %w", err)
	}

	return nil
}

type JobsCmd struct {
	List   JobsListCmd   `cmd:"list" help:"List jobs" default:"list"`
	Status JobsStatusCmd `cmd:"status" help:"Show job task counts"`
}

type JobsListCmd struct {
	Limit     int    `short:"l" default:"5" help:"Max jobs to show"`
	StateFile string `short:"s" optional:"" help:"Path to state file"`
}

type JobsStatusCmd struct {
	JobID     string `arg:"" help:"Job ID for checking job status"`
	StateFile string `short:"s" optional:"" help:"Path to state file"`
}

func (j *JobsListCmd) Run(cfg *config.Config) error {
	store, err := storage.GetStore(cfg, j.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %w", err)
	}
	defer store.Close()
	jobs, err := store.SelectJobs(j.Limit)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCREATED\tQUERIES")
	for _, job := range jobs {
		var queries []string
		for _, query := range job.Input.Queries {
			queries = append(queries, query.Text)
		}
		fmt.Fprintf(w, "%v\t%v\t%v\n", job.ID, job.CreatedAt.Format(time.DateTime), queries)
	}
	return w.Flush()
}

func (j *JobsStatusCmd) Run(cfg *config.Config) error {
	store, err := storage.GetStore(cfg, j.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %w", err)
	}
	defer store.Close()
	jobStatus, err := store.SelectJobsStatus(j.JobID)
	if err != nil {
		return err
	}
	var queries []string
	for _, query := range jobStatus.Input.Queries {
		queries = append(queries, query.Text)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tQUERIES\tTOTAL\tRUNNING\tFAILED\n")
	fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n", jobStatus.ID, queries, jobStatus.Total, jobStatus.Running, jobStatus.Failed)
	return w.Flush()
}

type ExportCmd struct {
	JobID     string `arg:"" help:"Job ID for checking job status"`
	Path      string `short:"p" default:"./data/" help:"Path were to save file"`
	Format    string `short:"f" enum:"csv,sqlite" default:"csv" help:"Output format (csv or sqlite)"`
	StateFile string `short:"s" optional:"" help:"Path to state file"`
}

func (e *ExportCmd) Run(cfg *config.Config) (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := storage.GetStore(cfg, e.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %w", err)
	}
	defer store.Close()
	filePath := filepath.Join(e.Path, e.JobID)
	if err := os.MkdirAll(filePath, 0o755); err != nil {
		return fmt.Errorf("failed to create folder for exported data: %w", err)
	}
	if e.Format == "sqlite" {
		if err := export.ExportSQLite(cfg, store, e.JobID, e.StateFile, filePath); err != nil {
			return fmt.Errorf("failed to export sqlite data: %w", err)
		} else {
			return nil
		}
	}
	defer func() {
		if err != nil {
			for _, file := range []string{"videos.csv", "threads.csv", "comments.csv"} {
				if rmErr := os.Remove(filepath.Join(filePath, file)); rmErr != nil && !os.IsNotExist(rmErr) {
					slog.Error("Failed cleaning up after failure", "file", file, "error", rmErr)
				}
			}
		}
	}()
	videosFile, err := os.Create(filepath.Join(filePath, "videos.csv"))
	if err != nil {
		return fmt.Errorf("failed to create videos.csv file: %w", err)
	}
	defer videosFile.Close()

	threadsFile, err := os.Create(filepath.Join(filePath, "threads.csv"))
	if err != nil {
		return fmt.Errorf("failed to create threads.csv file: %w", err)
	}
	defer threadsFile.Close()
	commentsFile, err := os.Create(filepath.Join(filePath, "comments.csv"))
	if err != nil {
		return fmt.Errorf("failed to create comments.csv file: %w", err)
	}
	defer commentsFile.Close()

	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	wg.Go(func() {
		if err := export.WriteCSV(ctx, videosFile, e.JobID, cfg.LimitRows, store.SelectVideos); err != nil {
			cancel()
			errChan <- fmt.Errorf("videos export failed: %w", err)
		}
	})
	wg.Go(func() {
		if err := export.WriteCSV(ctx, threadsFile, e.JobID, cfg.LimitRows, store.SelectThreads); err != nil {
			cancel()
			errChan <- fmt.Errorf("threads export failed: %w", err)
		}
	})
	wg.Go(func() {
		if err := export.WriteCSV(ctx, commentsFile, e.JobID, cfg.LimitRows, store.SelectComments); err != nil {
			cancel()
			errChan <- fmt.Errorf("comments export failed: %w", err)
		}
	})
	channel.CloseWhenDone(&wg, errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
