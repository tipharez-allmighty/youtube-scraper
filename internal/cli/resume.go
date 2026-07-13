package cli

import (
	"encoding/json"
	"fmt"
	"sync"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/input"
	"tipharez-allmighty/youtube-scraper/internal/storage"
	"tipharez-allmighty/youtube-scraper/internal/youtube"
)

type ResumeCmd struct {
	JobID     string `arg:"" help:"Job ID for loading failed tasks"`
	StateFile string `short:"s" optional:"" help:"Path to state file"`
}

type failedTasks struct {
	searchTasks  []storage.Task
	threadTasks  []storage.Task
	commentTasks []storage.Task
}

func (r *ResumeCmd) Run(cfg *config.Config) error {
	store, err := getStore(cfg, r.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %w", err)
	}
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
	client := youtube.New(cfg.YoutubeAPIKey)

	ft := failedTasks{}

	for _, task := range tasks {
		switch task.Type {
		case storage.Search:
			ft.searchTasks = append(ft.searchTasks, task)
		case storage.Thread:
			ft.threadTasks = append(ft.threadTasks, task)
		case storage.Reply:
			ft.commentTasks = append(ft.commentTasks, task)
		}
	}
	switch {
	case len(ft.searchTasks) > 0:
		if err := resumeFromSearch(client, store, ft, jobInput, cfg); err != nil {
			return fmt.Errorf("failed to resume tasks starting from video search: %w", err)
		}
	case len(ft.threadTasks) > 0:
		if err := resumeFromThreads(client, store, ft.threadTasks, ft.commentTasks, jobInput, cfg); err != nil {
			return fmt.Errorf("failed to resume task starting from threads: %w", err)
		}
	case len(ft.commentTasks) > 0:
		if err := resumeFromComments(client, store, ft.commentTasks, jobInput, cfg); err != nil {
			return fmt.Errorf("failed to resume task starting from comments: %w", err)
		}
	}
	return nil
}

func resumeFromSearch(client *youtube.Client, store *storage.Store, ft failedTasks, jobInput *input.InputSchema, cfg *config.Config) error {
	queryCh := make(chan youtube.VideosContext, cfg.BufferSize)
	threadCh := make(chan youtube.ThreadsContext, cfg.BufferSize)
	commentCh := make(chan youtube.CommentsContext, cfg.BufferSize)
	var searchWg sync.WaitGroup
	var threadWg sync.WaitGroup
	var commentWg sync.WaitGroup

	for range cfg.NumWorkers {
		searchWg.Go(func() {
			for queryCtx := range queryCh {
				youtube.RunPagination(jobInput.MaxPages, queryCtx.PageToken, func(pageToken string) (string, error) {
					queryCtx.PageToken = pageToken
					return youtube.GetVideos(
						client,
						store,
						cfg,
						queryCtx,
						threadCh,
					)
				})
			}
		})
	}
	for range cfg.NumWorkers {
		threadWg.Go(func() {
			for threadCtx := range threadCh {
				youtube.RunPagination(jobInput.MaxPages, threadCtx.PageToken, func(pageToken string) (string, error) {
					threadCtx.PageToken = pageToken
					return youtube.GetCommentThreads(
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
				youtube.RunPagination(jobInput.MaxComments, commentCtx.PageToken, func(pageToken string) (string, error) {
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

	for _, task := range ft.searchTasks {
		var queryCtx youtube.VideosContext
		err := json.Unmarshal([]byte(task.Payload), &queryCtx)
		if err != nil {
			return fmt.Errorf("failed to unmarshal video search context payload: %w", err)
		}
		queryCh <- queryCtx
	}
	close(queryCh)
	for _, task := range ft.threadTasks {
		var threadCtx youtube.ThreadsContext
		err := json.Unmarshal([]byte(task.Payload), &threadCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal thread context payload: %w", err)
		}
		threadCh <- threadCtx
	}

	for _, task := range ft.commentTasks {
		var commentCtx youtube.CommentsContext
		err := json.Unmarshal([]byte(task.Payload), &commentCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal comment context payload: %w", err)
		}
		commentCh <- commentCtx
	}
	go closeWhenDone(&searchWg, threadCh)
	go closeWhenDone(&threadWg, commentCh)
	commentWg.Wait()
	return nil
}

func resumeFromThreads(client *youtube.Client, store *storage.Store, threadTasks []storage.Task, commentTasks []storage.Task, jobInput *input.InputSchema, cfg *config.Config) error {
	threadCh := make(chan youtube.ThreadsContext, cfg.BufferSize)
	commentCh := make(chan youtube.CommentsContext, cfg.BufferSize)
	var threadWg sync.WaitGroup
	var commentWg sync.WaitGroup

	for range cfg.NumWorkers {
		threadWg.Go(func() {
			for threadCtx := range threadCh {
				youtube.RunPagination(jobInput.MaxPages, threadCtx.PageToken, func(pageToken string) (string, error) {
					threadCtx.PageToken = pageToken
					return youtube.GetCommentThreads(
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
				youtube.RunPagination(jobInput.MaxComments, commentCtx.PageToken, func(pageToken string) (string, error) {
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
	for _, task := range threadTasks {
		var threadCtx youtube.ThreadsContext
		err := json.Unmarshal([]byte(task.Payload), &threadCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal thread context payload: %w", err)
		}
		threadCh <- threadCtx
	}
	close(threadCh)

	for _, task := range commentTasks {
		var commentCtx youtube.CommentsContext
		err := json.Unmarshal([]byte(task.Payload), &commentCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal comment context payload: %w", err)
		}
		commentCh <- commentCtx
	}
	go closeWhenDone(&threadWg, commentCh)
	commentWg.Wait()
	return nil
}

func resumeFromComments(client *youtube.Client, store *storage.Store, commentTasks []storage.Task, jobInput *input.InputSchema, cfg *config.Config) error {
	commentCh := make(chan youtube.CommentsContext, cfg.BufferSize)
	var commentWg sync.WaitGroup
	for range cfg.NumWorkers {
		commentWg.Go(func() {
			for commentCtx := range commentCh {
				youtube.RunPagination(jobInput.MaxComments, commentCtx.PageToken, func(pageToken string) (string, error) {
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

	for _, task := range commentTasks {
		var commentCtx youtube.CommentsContext
		err := json.Unmarshal([]byte(task.Payload), &commentCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal context payload: %w", err)
		}
		commentCh <- commentCtx
	}
	close(commentCh)
	commentWg.Wait()
	return nil
}
