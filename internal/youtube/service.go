package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"strings"
	"sync"
	"time"

	"tipharez-allmighty/youtube-scraper/internal/channel"
	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/input"
	"tipharez-allmighty/youtube-scraper/internal/storage"

	"github.com/google/uuid"
)

const (
	errCreateTask   = "failed creating task: %w"
	errCompleteTask = "failed to complete task: %w"
)

type (
	PaginationScraperFunc func(pageToken string) (nextPageToken string, err error)
	NetworkRequestFunc    func(parms url.Values, out YoutubeResponse) (err error)
)

type Context struct {
	JobID      string
	MaxResults int
}
type VideosContext struct {
	Context
	PageToken       string
	Query           string
	Order           input.Order
	PublishedBefore time.Time
	PublishedAfter  time.Time
}

type ThreadsContext struct {
	Context
	PageToken string
	VideoID   string
}

type CommentsContext struct {
	Context
	PageToken string
	CommentID string
}

func RunPagination(ctx context.Context, maxLimit int, startToken string, fn PaginationScraperFunc) {
	pageToken := startToken
	pagesFetched := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		nextPageToken, err := fn(pageToken)
		if err != nil {
			slog.Error("failed to get data", "error", err)
			break
		}
		pagesFetched++
		if nextPageToken == "" {
			break
		}
		pageToken = nextPageToken
		if maxLimit != 0 && pagesFetched >= maxLimit {
			break
		}
	}
}

func WithExpBackoff(fn NetworkRequestFunc, params url.Values, out YoutubeResponse, maxRetries int) error {
	var err error
	for retry := range maxRetries {
		err = fn(params, out)
		if err != nil {
			if apiErr, ok := errors.AsType[APIError](err); ok {
				errCode := apiErr.ErrorData.Code
				if errCode == 429 || errCode == 403 {
					if retry == maxRetries-1 || apiErr.Reason() == CommentsDisabled {
						return err
					}
					retryDelay := 1 << retry
					time.Sleep(time.Duration(float64(retryDelay)+rand.Float64()) * time.Second)
					continue
				}
				return err
			}
		}
		return err
	}
	return err
}

func RunSearch(ctx context.Context, cfg *config.Config, client YoutubeClient, store *storage.Store, job storage.Job, payload input.InputSchema) error {
	queryCh := make(chan input.Query, cfg.BufferSize)
	threadCh := make(chan ThreadsContext, cfg.BufferSize)
	commentCh := make(chan CommentsContext, cfg.BufferSize)
	var searchWg sync.WaitGroup
	var commentThreadWg sync.WaitGroup
	var commentWg sync.WaitGroup

	queryContext := Context{
		JobID:      job.ID,
		MaxResults: payload.MaxResultsPerQuery,
	}
	for range cfg.NumWorkers {
		searchWg.Go(func() {
			for query := range queryCh {
				RunPagination(ctx, payload.MaxPages, "", func(pageToken string) (string, error) {
					return GetVideos(
						ctx,
						client,
						store,
						cfg,
						VideosContext{
							Context:         queryContext,
							PageToken:       pageToken,
							Query:           query.Text,
							Order:           query.Order,
							PublishedBefore: query.PublishedBefore,
							PublishedAfter:  query.PublishedAfter,
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
				RunPagination(ctx, payload.MaxThreads, "", func(pageToken string) (string, error) {
					threadCtx.PageToken = pageToken
					return GetCommentThreads(
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
				RunPagination(ctx, payload.MaxComments, "", func(pageToken string) (string, error) {
					commentCtx.PageToken = pageToken
					return GetComments(
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

type failedTasks struct {
	searchTasks  []storage.Task
	threadTasks  []storage.Task
	commentTasks []storage.Task
}

func ResumeSearchTasks(ctx context.Context, cfg *config.Config, client YoutubeClient, store *storage.Store, jobInput *input.InputSchema, tasks []storage.Task) error {
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
		if err := resumeFromSearch(ctx, client, store, ft, jobInput, cfg); err != nil {
			return fmt.Errorf("failed to resume tasks starting from video search: %w", err)
		}
	case len(ft.threadTasks) > 0:
		if err := resumeFromThreads(ctx, client, store, ft.threadTasks, ft.commentTasks, jobInput, cfg); err != nil {
			return fmt.Errorf("failed to resume task starting from threads: %w", err)
		}
	case len(ft.commentTasks) > 0:
		if err := resumeFromComments(ctx, client, store, ft.commentTasks, jobInput, cfg); err != nil {
			return fmt.Errorf("failed to resume task starting from comments: %w", err)
		}
	}
	return nil
}

func resumeFromSearch(ctx context.Context, client YoutubeClient, store *storage.Store, ft failedTasks, jobInput *input.InputSchema, cfg *config.Config) error {
	queryCh := make(chan VideosContext, cfg.BufferSize)
	threadCh := make(chan ThreadsContext, cfg.BufferSize)
	commentCh := make(chan CommentsContext, cfg.BufferSize)
	var searchWg sync.WaitGroup
	var threadWg sync.WaitGroup
	var commentWg sync.WaitGroup

	for range cfg.NumWorkers {
		searchWg.Go(func() {
			for queryCtx := range queryCh {
				RunPagination(ctx, jobInput.MaxPages, queryCtx.PageToken, func(pageToken string) (string, error) {
					queryCtx.PageToken = pageToken
					return GetVideos(
						ctx,
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
				RunPagination(ctx, jobInput.MaxPages, threadCtx.PageToken, func(pageToken string) (string, error) {
					threadCtx.PageToken = pageToken
					return GetCommentThreads(
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
				RunPagination(ctx, jobInput.MaxComments, commentCtx.PageToken, func(pageToken string) (string, error) {
					commentCtx.PageToken = pageToken
					return GetComments(
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
		var queryCtx VideosContext
		err := json.Unmarshal([]byte(task.Payload), &queryCtx)
		if err != nil {
			return fmt.Errorf("failed to unmarshal video search context payload: %w", err)
		}
		if err := channel.TryChannel(ctx, queryCh, queryCtx); err != nil {
			close(queryCh)
			return ctx.Err()
		}
	}
	close(queryCh)
	for _, task := range ft.threadTasks {
		var threadCtx ThreadsContext
		err := json.Unmarshal([]byte(task.Payload), &threadCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal thread context payload: %w", err)
		}
		if err := channel.TryChannel(ctx, threadCh, threadCtx); err != nil {
			return err
		}
	}

	for _, task := range ft.commentTasks {
		var commentCtx CommentsContext
		err := json.Unmarshal([]byte(task.Payload), &commentCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal comment context payload: %w", err)
		}
		if err := channel.TryChannel(ctx, commentCh, commentCtx); err != nil {
			return err
		}
	}
	go channel.CloseWhenDone(&searchWg, threadCh)
	go channel.CloseWhenDone(&threadWg, commentCh)
	commentWg.Wait()
	return nil
}

func resumeFromThreads(ctx context.Context, client YoutubeClient, store *storage.Store, threadTasks []storage.Task, commentTasks []storage.Task, jobInput *input.InputSchema, cfg *config.Config) error {
	threadCh := make(chan ThreadsContext, cfg.BufferSize)
	commentCh := make(chan CommentsContext, cfg.BufferSize)
	var threadWg sync.WaitGroup
	var commentWg sync.WaitGroup

	for range cfg.NumWorkers {
		threadWg.Go(func() {
			for threadCtx := range threadCh {
				RunPagination(ctx, jobInput.MaxPages, threadCtx.PageToken, func(pageToken string) (string, error) {
					threadCtx.PageToken = pageToken
					return GetCommentThreads(
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
				RunPagination(ctx, jobInput.MaxComments, commentCtx.PageToken, func(pageToken string) (string, error) {
					commentCtx.PageToken = pageToken
					return GetComments(
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
		var threadCtx ThreadsContext
		err := json.Unmarshal([]byte(task.Payload), &threadCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal thread context payload: %w", err)
		}
		if err := channel.TryChannel(ctx, threadCh, threadCtx); err != nil {
			close(threadCh)
			return err
		}
	}
	close(threadCh)

	for _, task := range commentTasks {
		var commentCtx CommentsContext
		err := json.Unmarshal([]byte(task.Payload), &commentCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal comment context payload: %w", err)
		}
		if err := channel.TryChannel(ctx, commentCh, commentCtx); err != nil {
			return err
		}
	}
	go channel.CloseWhenDone(&threadWg, commentCh)
	commentWg.Wait()
	return nil
}

func resumeFromComments(ctx context.Context, client YoutubeClient, store *storage.Store, commentTasks []storage.Task, jobInput *input.InputSchema, cfg *config.Config) error {
	commentCh := make(chan CommentsContext, cfg.BufferSize)
	var commentWg sync.WaitGroup
	for range cfg.NumWorkers {
		commentWg.Go(func() {
			for commentCtx := range commentCh {
				RunPagination(ctx, jobInput.MaxComments, commentCtx.PageToken, func(pageToken string) (string, error) {
					commentCtx.PageToken = pageToken
					return GetComments(
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
		var commentCtx CommentsContext
		err := json.Unmarshal([]byte(task.Payload), &commentCtx)
		if err != nil {
			return fmt.Errorf("failed to umarshal context payload: %w", err)
		}
		if err := channel.TryChannel(ctx, commentCh, commentCtx); err != nil {
			close(commentCh)
			return err
		}
	}
	close(commentCh)
	commentWg.Wait()
	return nil
}

type DataStore interface {
	InsertTask(t storage.Task) error
	UpdateTaskStatus(id string, status storage.Status, error *string) error
	CompleteTask(taskID string, insertFn func(storage.TxExecutable) error) error
}

type YoutubeClient interface {
	get(params url.Values, out YoutubeResponse) error
}

func GetVideos(ctx context.Context, c YoutubeClient, s DataStore, cfg *config.Config, vctx VideosContext, threadCh chan<- ThreadsContext) (nextPageToken string, err error) {
	params := url.Values{
		"q":          {vctx.Query},
		"type":       {"video"},
		"part":       {"id,snippet"},
		"maxResults": {fmt.Sprint(vctx.MaxResults)},
	}
	if vctx.Order != "" {
		params.Set("order", string(vctx.Order))
	}
	if !vctx.PublishedBefore.IsZero() {
		params.Set("publishedBefore", vctx.PublishedBefore.Format(time.RFC3339))
	}

	if !vctx.PublishedAfter.IsZero() {
		params.Set("publishedAfter", vctx.PublishedAfter.Format(time.RFC3339))
	}

	var pageTokenPtr *string
	if vctx.PageToken != "" {
		pageTokenPtr = &vctx.PageToken
		params.Set("pageToken", vctx.PageToken)
	}

	taskID := getDeterministicID(vctx.JobID, vctx.Query, vctx.PageToken)
	var payload []byte
	payload, err = json.Marshal(vctx)
	if err != nil {
		return "", err
	}
	task := storage.Task{ID: taskID, JobID: vctx.JobID, Status: storage.Running, Type: storage.Search, Payload: string(payload), PageToken: pageTokenPtr}
	if err = s.InsertTask(task); err != nil {
		return "", fmt.Errorf(errCreateTask, err)
	}
	defer failTask(s, taskID, &err)

	var searchResponse SearchResponse
	if err = WithExpBackoff(c.get, params, &searchResponse, cfg.MaxRetries); err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	videos := make([]storage.Video, 0, len(searchResponse.Items))
	for _, item := range searchResponse.Items {
		videos = append(videos, storage.Video{
			ID:          item.ID.VideoID,
			JobID:       vctx.JobID,
			QueryText:   vctx.Query,
			Title:       item.Snippet.Title,
			Description: item.Snippet.Description,
			PublishedAt: item.Snippet.PublishedAt,
		})
	}
	if err = s.CompleteTask(taskID, func(tx storage.TxExecutable) error {
		return storage.InsertVideos(tx, videos)
	}); err != nil {
		return "", fmt.Errorf(errCompleteTask, err)
	}
	slog.Info("Videos were found", "items", len(searchResponse.Items))
	for _, item := range searchResponse.Items {
		threadContext := ThreadsContext{
			Context: Context{
				JobID:      vctx.JobID,
				MaxResults: vctx.MaxResults,
			},
			VideoID:   item.ID.VideoID,
			PageToken: "",
		}
		if err := channel.TryChannel(ctx, threadCh, threadContext); err != nil {
			return "", err
		}
	}
	return searchResponse.NextPageToken, nil
}

func GetCommentThreads(ctx context.Context, c YoutubeClient, s DataStore, cfg *config.Config, tctx ThreadsContext, commentCh chan<- CommentsContext) (nextPageToken string, err error) {
	params := url.Values{
		"videoId":    {tctx.VideoID},
		"part":       {"id,snippet,replies"},
		"maxResults": {fmt.Sprint(tctx.MaxResults)},
	}
	var pageTokenPtr *string
	if tctx.PageToken != "" {
		pageTokenPtr = &tctx.PageToken
		params.Set("pageToken", tctx.PageToken)
	}
	var payload []byte
	payload, err = json.Marshal(tctx)
	if err != nil {
		return "", err
	}
	taskID := getDeterministicID(tctx.JobID, tctx.VideoID, tctx.PageToken)
	task := storage.Task{ID: taskID, JobID: tctx.JobID, Status: storage.Running, Type: storage.Thread, Payload: string(payload), PageToken: pageTokenPtr}
	if err = s.InsertTask(task); err != nil {
		return "", fmt.Errorf(errCreateTask, err)
	}
	defer failTask(s, taskID, &err)

	var commentThreadResponse CommentThreadResponse
	err = WithExpBackoff(c.get, params, &commentThreadResponse, cfg.MaxRetries)
	if err != nil {
		if apiErr, ok := errors.AsType[APIError](err); ok {
			if apiErr.Reason() == CommentsDisabled {
				slog.Info("Comments for video are disabled", "video_id", tctx.VideoID, "error", err)
				if err = s.CompleteTask(taskID, func(tx storage.TxExecutable) error {
					return nil
				}); err != nil {
					return "", fmt.Errorf(errCompleteTask, err)
				}
				return "", nil
			}
		}
		return "", fmt.Errorf("fetching comment threads failed: %w", err)
	}
	slog.Info("Comment threads were found", "items", len(commentThreadResponse.Items))
	threads := make([]storage.CommentThread, 0, len(commentThreadResponse.Items))
	var topLevelReplies []CommentThread
	var commentIDs []string
	for _, item := range commentThreadResponse.Items {
		thread := storage.CommentThread{
			CommentBase: storage.CommentBase{
				ID:           item.ID,
				JobID:        tctx.JobID,
				Author:       item.Snippet.TopLevelComment.Snippet.AuthorDisplayName,
				TextDisplay:  item.Snippet.TopLevelComment.Snippet.TextDisplay,
				TextOriginal: item.Snippet.TopLevelComment.Snippet.TextOriginal,
				LikeCount:    item.Snippet.TopLevelComment.Snippet.LikeCount,
				PublishedAt:  item.Snippet.TopLevelComment.Snippet.PublishedAt,
			},
			VideoID:         item.Snippet.VideoID,
			TotalReplyCount: item.Snippet.TotalReplyCount,
		}
		threads = append(threads, thread)
		if item.Replies == nil {
			slog.Info("No replies for a given comment", "item", item)
			continue
		}
		if item.Snippet.TotalReplyCount > len(item.Replies.Comments) {
			commentIDs = append(commentIDs, item.Snippet.TopLevelComment.ID)
			slog.Info("Reply endpoint should be called", "item", item)
		} else {
			slog.Info("Fetch all comments in thread, no need for reply call", "item", item)
			topLevelReplies = append(topLevelReplies, item)
		}
	}
	if err = s.CompleteTask(taskID, func(tx storage.TxExecutable) error {
		return storage.InsertThreads(tx, threads)
	}); err != nil {
		return "", fmt.Errorf(errCompleteTask, err)
	}
	for _, commentID := range commentIDs {
		commentCtx := CommentsContext{
			Context:   tctx.Context,
			CommentID: commentID,
			PageToken: "",
		}
		if err := channel.TryChannel(ctx, commentCh, commentCtx); err != nil {
			return "", err
		}
	}
	for _, commentThread := range topLevelReplies {
		if err = processTopLevelReplies(s, tctx.JobID, tctx.MaxResults, commentThread); err != nil {
			return "", err
		}
	}

	return commentThreadResponse.NextPageToken, nil
}

func GetComments(c YoutubeClient, s DataStore, cfg *config.Config, cctx CommentsContext) (nextPageToken string, err error) {
	params := url.Values{
		"parentId":   {cctx.CommentID},
		"part":       {"id,snippet"},
		"maxResults": {fmt.Sprint(cctx.MaxResults)},
	}
	var pageTokenPtr *string
	if cctx.PageToken != "" {
		pageTokenPtr = &cctx.PageToken
		params.Set("pageToken", cctx.PageToken)
	}
	var payload []byte
	payload, err = json.Marshal(cctx)
	if err != nil {
		return "", err
	}
	taskID := getDeterministicID(cctx.JobID, cctx.CommentID, cctx.PageToken)
	task := storage.Task{ID: taskID, JobID: cctx.JobID, Status: storage.Running, Type: storage.Reply, Payload: string(payload), PageToken: pageTokenPtr}
	if err = s.InsertTask(task); err != nil {
		return "", fmt.Errorf(errCreateTask, err)
	}
	defer failTask(s, taskID, &err)
	var commentResponse CommentResponse
	if err = WithExpBackoff(c.get, params, &commentResponse, cfg.MaxRetries); err != nil {
		return "", fmt.Errorf("fetching comments failed: %w", err)
	}
	slog.Info("Comments were found", "items", len(commentResponse.Items))
	comments := make([]storage.Comment, 0, len(commentResponse.Items))
	for _, item := range commentResponse.Items {
		comments = append(comments, storage.Comment{
			CommentBase: storage.CommentBase{
				ID:           item.ID,
				JobID:        cctx.JobID,
				Author:       item.Snippet.AuthorDisplayName,
				TextDisplay:  item.Snippet.TextDisplay,
				TextOriginal: item.Snippet.TextOriginal,
				LikeCount:    item.Snippet.LikeCount,
				PublishedAt:  item.Snippet.PublishedAt,
			},
			ThreadID: item.Snippet.ParentID,
		})
	}
	if err = s.CompleteTask(taskID, func(tx storage.TxExecutable) error {
		return storage.InsertComments(tx, comments)
	}); err != nil {
		return "", fmt.Errorf(errCompleteTask, err)
	}
	return commentResponse.NextPageToken, nil
}

func processTopLevelReplies(s DataStore, jobID string, maxResults int, item CommentThread) error {
	cctx := CommentsContext{
		Context: Context{
			JobID:      jobID,
			MaxResults: maxResults,
		},
		CommentID: item.Snippet.TopLevelComment.ID,
		PageToken: "",
	}
	replyPayload, err := json.Marshal(cctx)
	if err != nil {
		return err
	}
	replyTaskID := getDeterministicID(jobID, "replies", item.ID)
	task := storage.Task{ID: replyTaskID, JobID: jobID, Status: storage.Running, Type: storage.Reply, Payload: string(replyPayload)}
	if err = s.InsertTask(task); err != nil {
		return fmt.Errorf(errCreateTask, err)
	}
	comments := make([]storage.Comment, 0, len(item.Replies.Comments))
	for _, comment := range item.Replies.Comments {
		comments = append(comments, storage.Comment{
			CommentBase: storage.CommentBase{
				ID:           comment.ID,
				JobID:        jobID,
				Author:       comment.Snippet.AuthorDisplayName,
				TextDisplay:  comment.Snippet.TextDisplay,
				TextOriginal: comment.Snippet.TextOriginal,
				LikeCount:    comment.Snippet.LikeCount,
				PublishedAt:  comment.Snippet.PublishedAt,
			},
			ThreadID: item.ID,
		},
		)
		slog.Info("Fetching comment", "comment", comment)
	}
	if err = s.CompleteTask(replyTaskID, func(tx storage.TxExecutable) error {
		return storage.InsertComments(tx, comments)
	}); err != nil {
		return fmt.Errorf(errCompleteTask, err)
	}
	return nil
}

func getDeterministicID(parts ...string) string {
	return uuid.NewMD5(uuid.Nil, []byte(strings.Join(parts, ":"))).String()
}

func failTask(s DataStore, taskID string, errPtr *error) {
	if errPtr == nil || *errPtr == nil {
		return
	}
	errMsg := (*errPtr).Error()
	if err := s.UpdateTaskStatus(taskID, storage.Failed, &errMsg); err != nil {
		*errPtr = fmt.Errorf("failed updating task to failed status (%v): %w", err, *errPtr)
	}
}
