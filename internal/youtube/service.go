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
	"time"

	"tipharez-allmighty/youtube-scraper/internal/channel"
	"tipharez-allmighty/youtube-scraper/internal/config"
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
	PageToken string
	Query     string
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
