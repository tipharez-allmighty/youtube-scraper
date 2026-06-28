package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"text/tabwriter"
	"time"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/input"
	"tipharez-allmighty/youtube-scraper/internal/storage"
	"tipharez-allmighty/youtube-scraper/internal/youtube"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type RunCmd struct{}

func (r *RunCmd) Run(cfg *config.Config) error {
	var payload input.InputSchema
	if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	inputValidator := validator.New()
	if err := inputValidator.Struct(payload); err != nil {
		return fmt.Errorf("wrong input format: %w", err)
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

	client := youtube.New(cfg.YoutubeAPIKey)

	queryCh := make(chan input.Query, cfg.BufferSize)
	videoCh := make(chan youtube.SearchResponse, cfg.BufferSize)
	commentThreadCh := make(chan string, cfg.BufferSize)
	var searchWg sync.WaitGroup
	var commentThreadWg sync.WaitGroup
	var commentWg sync.WaitGroup

	for range cfg.NumWorkers {
		searchWg.Go(func() {
			for query := range queryCh {
				youtube.RunPagination(payload.MaxPages, func(pageToken string) (string, error) {
					return youtube.GetVideos(
						client,
						store,
						job.ID,
						query.Text,
						payload.MaxResultsPerQuery,
						pageToken,
						videoCh,
					)
				})
			}
		})
	}
	for range cfg.NumWorkers {
		commentThreadWg.Go(func() {
			for video := range videoCh {
				for _, item := range video.Items {
					youtube.RunPagination(payload.MaxThreads, func(pageToken string) (string, error) {
						return youtube.GetCommentThreads(
							client,
							store,
							job.ID,
							item.ID.VideoID,
							payload.MaxResultsPerQuery,
							pageToken,
							commentThreadCh,
						)
					})
				}
			}
		})
	}
	for range cfg.NumWorkers {
		commentWg.Go(func() {
			for commentID := range commentThreadCh {
				youtube.RunPagination(payload.MaxComments, func(pageToken string) (string, error) {
					return youtube.GetComments(
						client,
						store,
						job.ID,
						commentID,
						pageToken,
						payload.MaxResultsPerQuery,
					)
				})
			}
		})
	}
	go closeWhenDone(&searchWg, videoCh)
	go closeWhenDone(&commentThreadWg, commentThreadCh)
	for _, query := range payload.Queries {
		queryCh <- query
	}
	close(queryCh)
	commentWg.Wait()
	return nil
}

type JobsCmd struct {
	List   JobsListCmd   `cmd:"list" help:"List jobs" default:"list"`
	Status JobsStatusCmd `cmd:"status" help:"Show job task counts"`
}

type JobsListCmd struct {
	Limit     int    `short:"l" default:"5" help:"Max jobs to show"`
	StateFile string `short:"s" otional:"" help:"Path to state file"`
}

type JobsStatusCmd struct {
	JobID     string `arg:"" help:"Job ID for checking job status"`
	StateFile string `short:"s" otional:"" help:"Path to state file"`
}

func (j *JobsListCmd) Run(cfg *config.Config) error {
	store, err := getStore(cfg, j.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %v", err)
	}
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
	store, err := getStore(cfg, j.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %v", err)
	}
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

type ResumeCmd struct{}

func getStore(cfg *config.Config, stateFile string) (*storage.Store, error) {
	dbPath := stateFile
	if dbPath == "" {
		dbPath = cfg.StateFile
	}
	db, err := storage.Init(dbPath)
	if err != nil {
		return nil, err
	}
	store := storage.NewStore(db)
	return store, nil
}

func closeWhenDone[T any](wg *sync.WaitGroup, ch chan T) {
	wg.Wait()
	close(ch)
}
