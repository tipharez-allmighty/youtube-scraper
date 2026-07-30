package youtube

import (
	"context"
	"errors"
	"net/url"
	"testing"
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
	t.Run("Stop on max Retry test", func(t *testing.T) {
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
	t.Run("Stop on non retryable error", func(t *testing.T) {
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
	t.Run("Stop on comments disabled error", func(t *testing.T) {
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
