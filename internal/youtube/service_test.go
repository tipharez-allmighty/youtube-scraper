package youtube

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"reflect"
	"testing"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/storage"
)

func TestRunPagination(t *testing.T) {
	tests := []struct {
		name     string
		maxLimit int
		testFunc func(calls *int, cancel context.CancelFunc) PaginationScraperFunc
	}{
		{
			name:     "stops on empty token",
			maxLimit: 0,
			testFunc: func(calls *int, cancel context.CancelFunc) PaginationScraperFunc {
				return func(pageToken string) (string, error) {
					*calls++
					if pageToken == "" {
						return "tok1", nil
					}
					return "", nil
				}
			},
		},
		{
			name:     "stops on max limit",
			maxLimit: 3,
			testFunc: func(calls *int, cancel context.CancelFunc) PaginationScraperFunc {
				return func(pageToken string) (string, error) {
					*calls++
					return "tok1", nil
				}
			},
		},
		{
			name:     "stops on cancel",
			maxLimit: 0,
			testFunc: func(calls *int, cancel context.CancelFunc) PaginationScraperFunc {
				return func(pageToken string) (string, error) {
					*calls++
					if *calls == 2 {
						cancel()
					}
					return "tok1", nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			calls := 0
			RunPagination(ctx, tt.maxLimit, "", tt.testFunc(&calls, cancel))
		})
	}
}

func TestExpBackoff(t *testing.T) {
	t.Run("max retries exhausted", func(t *testing.T) {
		calls := 0
		maxRetries := 3
		// always retryable (429), so it should exhaust every retry before returning
		mockFn := func(params url.Values, out YoutubeResponse) error {
			calls++
			return APIError{ErrorData: &ErrorPayload{Code: 429}}
		}
		if err := WithExpBackoff(mockFn, url.Values{}, &SearchResponse{}, maxRetries); err != nil {
			if calls != maxRetries {
				t.Fatalf("expected func to be called %d times, but was called %d", maxRetries, calls)
			}
			if _, ok := errors.AsType[APIError](err); !ok {
				t.Fatalf("backoff max test failed by returning wrong error %q != ApiError", err.Error())
			}
		} else {
			t.Fatalf("backoff didnt return Error")
		}
	})
	t.Run("non-retryable error", func(t *testing.T) {
		t.Parallel()
		calls := 0
		maxRetries := 3
		// not an APIError, so WithExpBackoff must bail after the first call, not retry
		mockFn := func(params url.Values, out YoutubeResponse) error {
			calls++
			return errors.New("some error")
		}
		if err := WithExpBackoff(mockFn, url.Values{}, &SearchResponse{}, maxRetries); err != nil {
			if calls > 1 {
				t.Fatalf("expected func to be called %d times, but was called %d", 1, calls)
			}
		} else {
			t.Fatalf("backoff didnt return Error")
		}
	})
	t.Run("comments disabled", func(t *testing.T) {
		t.Parallel()
		calls := 0
		maxRetries := 3
		// CommentsDisabled short-circuits retries even on the first attempt, unlike a plain 429/403
		mockFn := func(params url.Values, out YoutubeResponse) (err error) {
			calls++
			return APIError{&ErrorPayload{Code: 403, Errors: []ErrorDetail{{CommentsDisabled}}}}
		}
		if err := WithExpBackoff(mockFn, url.Values{}, &SearchResponse{}, maxRetries); err != nil {
			if calls != 1 {
				t.Fatalf("expected func to be called %d times, but was called %d", 1, calls)
			}
			apiError, ok := errors.AsType[APIError](err)
			if !ok {
				t.Fatalf("backoff max test failed by returning wrong error %q != ApiError", err.Error())
			} else {
				if apiError.ErrorData.Code != 403 || apiError.Reason() != CommentsDisabled {
					t.Fatalf("wrong error code %d != 403 and reason %v", apiError.ErrorData.Code, apiError.Reason())
				}
			}
		} else {
			t.Fatalf("backoff didnt return Error")
		}
	})
}

type MockStore struct{}

type MockYoutubeClient struct {
	resp YoutubeResponse
	err  error
}

func (m *MockYoutubeClient) get(params url.Values, out YoutubeResponse) error {
	if m.err != nil {
		return m.err
	}
	reflect.ValueOf(out).Elem().Set(reflect.ValueOf(m.resp).Elem())
	return nil
}

type mockDataStore struct {
	insertTaskErr   error
	completeTaskErr error
	updateStatusErr error
	tasks           []storage.Task
}

func (m *mockDataStore) InsertTask(t storage.Task) error {
	m.tasks = append(m.tasks, t)
	return m.insertTaskErr
}

func (m *mockDataStore) UpdateTaskStatus(id string, status storage.Status, errMsg *string) error {
	return m.updateStatusErr
}

func (m *mockDataStore) CompleteTask(taskID string, insertFn func(storage.TxExecutable) error) error {
	if m.completeTaskErr != nil {
		return m.completeTaskErr
	}
	return insertFn(&mockTx{store: m})
}

type mockTx struct{ store *mockDataStore }

func (m *mockTx) Exec(query string, args ...any) (sql.Result, error) {
	return nil, nil
}

func TestGetVideos(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := MockYoutubeClient{
			resp: &SearchResponse{
				NextPageToken: "vid1",
				Items: []SearchItem{
					{
						ID: SearchID{VideoID: "video_id1"},
					},
				},
			},
		}
		dataStore := mockDataStore{}
		threadCh := make(chan ThreadsContext, 10)
		nextToken, err := GetVideos(
			context.Background(), &client, &dataStore, &config.Config{MaxRetries: 3},
			VideosContext{Query: "cats", Context: Context{JobID: "job1", MaxResults: 5}}, threadCh)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		close(threadCh)
		var threadsContext []ThreadsContext
		for tc := range threadCh {
			threadsContext = append(threadsContext, tc)
		}
		if len(threadsContext) != 1 {
			t.Fatalf("unexpected threads response: %v", threadsContext)
		} else if threadsContext[0].VideoID != "video_id1" {
			t.Fatalf("unexpected threards response id %q != 'video_id1'", threadsContext[0].VideoID)
		}
		if nextToken != "vid1" {
			t.Fatalf("wrong token %q != 'vid1'", nextToken)
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		client := MockYoutubeClient{
			resp: &SearchResponse{
				NextPageToken: "vid1",
				Items: []SearchItem{
					{ID: SearchID{VideoID: "video_id1"}},
				},
			},
		}
		dataStore := mockDataStore{}
		threadCh := make(chan ThreadsContext)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := GetVideos(
			ctx, &client, &dataStore, &config.Config{MaxRetries: 3},
			VideosContext{Query: "cats", Context: Context{JobID: "job1", MaxResults: 5}}, threadCh)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}

func TestGetCommentThreads(t *testing.T) {
	// TotalReplyCount > len(Replies.Comments) so the reply must be fetched separately,
	// which routes the item through commentIDs -> channel.TryChannel(commentCh).
	newResp := func() *CommentThreadResponse {
		return &CommentThreadResponse{
			NextPageToken: "thread1",
			Items: []CommentThread{
				{
					ID: "ct1",
					Snippet: CommentThreadSnippet{
						VideoID:         "vid1",
						TopLevelComment: Comment{ID: "c1"},
						TotalReplyCount: 5,
					},
					Replies: &CommentReplies{Comments: []Comment{{ID: "r1"}}},
				},
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		client := MockYoutubeClient{resp: newResp()}
		dataStore := mockDataStore{}
		commentCh := make(chan CommentsContext, 10)

		nextToken, err := GetCommentThreads(
			context.Background(), &client, &dataStore, &config.Config{MaxRetries: 3},
			ThreadsContext{VideoID: "vid1", Context: Context{JobID: "job1", MaxResults: 5}}, commentCh)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		close(commentCh)
		var commentsContext []CommentsContext
		for cc := range commentCh {
			commentsContext = append(commentsContext, cc)
		}
		if len(commentsContext) != 1 {
			t.Fatalf("unexpected comments response: %v", commentsContext)
		} else if commentsContext[0].CommentID != "c1" {
			t.Fatalf("unexpected comment id %q != 'c1'", commentsContext[0].CommentID)
		}
		if nextToken != "thread1" {
			t.Fatalf("wrong token %q != 'thread1'", nextToken)
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		client := MockYoutubeClient{resp: newResp()}
		dataStore := mockDataStore{}
		// Unbuffered with no reader: TryChannel can never send, so it must return via ctx.Done().
		commentCh := make(chan CommentsContext)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := GetCommentThreads(
			ctx, &client, &dataStore, &config.Config{MaxRetries: 3},
			ThreadsContext{VideoID: "vid1", Context: Context{JobID: "job1", MaxResults: 5}}, commentCh)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}

func TestGetComments(t *testing.T) {
	// GetComments takes no context.Context and never touches a channel, so there is no
	// cancellation path to exercise here (unlike GetVideos/GetCommentThreads).
	t.Run("success", func(t *testing.T) {
		client := MockYoutubeClient{
			resp: &CommentResponse{
				NextPageToken: "comment1",
				Items: []Comment{
					{ID: "reply1"},
				},
			},
		}
		dataStore := mockDataStore{}

		nextToken, err := GetComments(
			&client, &dataStore, &config.Config{MaxRetries: 3},
			CommentsContext{CommentID: "c1", Context: Context{JobID: "job1", MaxResults: 5}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nextToken != "comment1" {
			t.Fatalf("wrong token %q != 'comment1'", nextToken)
		}
	})
}
