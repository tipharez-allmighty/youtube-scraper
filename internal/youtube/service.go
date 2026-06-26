package youtube

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"tipharez-allmighty/youtube-scraper/internal/storage"

	"github.com/google/uuid"
)

const (
	errCreateTask   = "failed creating task: %w"
	errCompleteTask = "failed to complete task: %w"
)

type PaginationScraperFunc func(pageToken string) (nextPageToken string, err error)

func RunPagination(maxLimit int, fn PaginationScraperFunc) {
	pageToken := ""
	pagesFetched := 0
	for {
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

func GetVideos(c *Client, s *storage.Store, jobID string, q string, maxResults int, pageToken string, videoCh chan<- SearchResponse) (nextPageToken string, err error) {
	params := url.Values{
		"q":          {q},
		"type":       {"video"},
		"part":       {"id,snippet"},
		"maxResults": {fmt.Sprint(maxResults)},
	}
	var pageTokenPtr *string
	if pageToken != "" {
		pageTokenPtr = &pageToken
		params.Set("pageToken", pageToken)
	}

	taskID := getDeterministicID(jobID, q, pageToken)
	var payload []byte
	payload, err = json.Marshal(params)
	if err != nil {
		return "", err
	}
	task := storage.Task{ID: taskID, JobID: jobID, Status: storage.Running, Type: storage.Search, Payload: string(payload), PageToken: pageTokenPtr}
	if err = s.InsertTask(task); err != nil {
		return "", fmt.Errorf(errCreateTask, err)
	}
	defer failTask(s, taskID, &err)

	var searchResponse SearchResponse
	if err = c.get(params, &searchResponse); err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	videos := make([]storage.Video, 0, len(searchResponse.Items))
	for _, item := range searchResponse.Items {
		videos = append(videos, storage.Video{
			ID:          item.ID.VideoID,
			JobID:       jobID,
			QueryText:   q,
			Title:       item.Snippet.Title,
			Description: item.Snippet.Description,
		})
	}
	if err = s.CompleteTask(taskID, func(tx *sql.Tx) error {
		return storage.InsertVideos(tx, videos)
	}); err != nil {
		return "", fmt.Errorf(errCompleteTask, err)
	}
	slog.Info("Videos were found", "items", len(searchResponse.Items))
	videoCh <- searchResponse
	return searchResponse.NextPageToken, nil
}

func GetCommentThreads(c *Client, s *storage.Store, jobID string, videoID string, maxResults int, pageToken string, commentThreadCh chan<- string) (nextPageToken string, err error) {
	params := url.Values{
		"videoId":    {videoID},
		"part":       {"id,snippet,replies"},
		"maxResults": {fmt.Sprint(maxResults)},
	}
	var pageTokenPtr *string
	if pageToken != "" {
		pageTokenPtr = &pageToken
		params.Set("pageToken", pageToken)
	}
	var payload []byte
	payload, err = json.Marshal(params)
	if err != nil {
		return "", err
	}
	taskID := getDeterministicID(jobID, videoID, pageToken)
	task := storage.Task{ID: taskID, JobID: jobID, Status: storage.Running, Type: storage.Thread, Payload: string(payload), PageToken: pageTokenPtr}
	if err = s.InsertTask(task); err != nil {
		return "", fmt.Errorf(errCreateTask, err)
	}
	defer failTask(s, taskID, &err)

	var commentThreadResponse CommentThreadResponse
	if err = c.get(params, &commentThreadResponse); err != nil {
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
				JobID:        jobID,
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
	if err = s.CompleteTask(taskID, func(tx *sql.Tx) error {
		return storage.InsertThreads(tx, threads)
	}); err != nil {
		return "", fmt.Errorf(errCompleteTask, err)
	}
	for _, commentID := range commentIDs {
		commentThreadCh <- commentID
	}
	for _, commentThread := range topLevelReplies {
		if err = processTopLevelReplies(s, jobID, maxResults, commentThread); err != nil {
			return "", err
		}
	}

	return commentThreadResponse.NextPageToken, nil
}

func GetComments(c *Client, s *storage.Store, jobID string, commentID string, pageToken string, maxResults int) (nextPageToken string, err error) {
	params := url.Values{
		"parentId":   {commentID},
		"part":       {"id,snippet"},
		"maxResults": {fmt.Sprint(maxResults)},
	}
	var pageTokenPtr *string
	if pageToken != "" {
		pageTokenPtr = &pageToken
		params.Set("pageToken", pageToken)
	}
	var payload []byte
	payload, err = json.Marshal(params)
	if err != nil {
		return "", err
	}
	taskID := getDeterministicID(jobID, commentID, pageToken)
	task := storage.Task{ID: taskID, JobID: jobID, Status: storage.Running, Type: storage.Reply, Payload: string(payload), PageToken: pageTokenPtr}
	if err = s.InsertTask(task); err != nil {
		return "", fmt.Errorf(errCreateTask, err)
	}
	defer failTask(s, taskID, &err)
	var commentResponse CommentResponse
	if err = c.get(params, &commentResponse); err != nil {
		return "", fmt.Errorf("fetching comments failed: %w", err)
	}
	slog.Info("Comments were found", "items", len(commentResponse.Items))
	comments := make([]storage.Comment, 0, len(commentResponse.Items))
	for _, item := range commentResponse.Items {
		comments = append(comments, storage.Comment{
			CommentBase: storage.CommentBase{
				ID:           item.ID,
				JobID:        jobID,
				Author:       item.Snippet.AuthorDisplayName,
				TextDisplay:  item.Snippet.TextDisplay,
				TextOriginal: item.Snippet.TextOriginal,
				LikeCount:    item.Snippet.LikeCount,
				PublishedAt:  item.Snippet.PublishedAt,
			},
			ThreadID: item.Snippet.ParentID,
		})
	}
	if err = s.CompleteTask(taskID, func(tx *sql.Tx) error {
		return storage.InsertComments(tx, comments)
	}); err != nil {
		return "", fmt.Errorf(errCompleteTask, err)
	}
	return commentResponse.NextPageToken, nil
}

func getDeterministicID(parts ...string) string {
	return uuid.NewMD5(uuid.Nil, []byte(strings.Join(parts, ":"))).String()
}

func failTask(s *storage.Store, taskID string, errPtr *error) {
	if errPtr == nil || *errPtr == nil {
		return
	}
	errMsg := (*errPtr).Error()
	if err := s.UpdateTaskStatus(taskID, storage.Failed, &errMsg); err != nil {
		*errPtr = fmt.Errorf("failed updating task to failed status (%v): %w", err, *errPtr)
	}
}

func processTopLevelReplies(s *storage.Store, jobID string, maxResults int, item CommentThread) error {
	comments := make([]storage.Comment, 0, len(item.Replies.Comments))
	replyParams := url.Values{
		"parentId":   {item.Snippet.TopLevelComment.ID},
		"part":       {"id,snippet"},
		"maxResults": {fmt.Sprint(maxResults)},
	}

	replyPayload, err := json.Marshal(replyParams)
	if err != nil {
		return err
	}
	replyTaskID := getDeterministicID(jobID, "replies", item.ID)
	task := storage.Task{ID: replyTaskID, JobID: jobID, Status: storage.Running, Type: storage.Reply, Payload: string(replyPayload)}
	if err = s.InsertTask(task); err != nil {
		return fmt.Errorf(errCreateTask, err)
	}
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
	if err = s.CompleteTask(replyTaskID, func(tx *sql.Tx) error {
		return storage.InsertComments(tx, comments)
	}); err != nil {
		return fmt.Errorf(errCompleteTask, err)
	}
	return nil
}
