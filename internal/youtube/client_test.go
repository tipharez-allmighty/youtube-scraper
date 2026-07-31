package youtube

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClientGetSuccess(t *testing.T) {
	var path string
	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		query = r.URL.Query()
		fmt.Fprint(w, `{"nextPageToken":"tok123","items":[{"id":{"videoId":"v1"}}]}`)
	}))
	defer server.Close()

	c := New("secret-key", server.URL+"/")
	params := url.Values{"q": []string{"golang"}}
	var out SearchResponse
	if err := c.get(params, &out); err != nil {
		t.Fatalf("get() returned error: %v", err)
	}

	if path != "/search" {
		t.Errorf("path is wrong %q != %q", path, out.URL())
	}
	if got := query.Get("key"); got != "secret-key" {
		t.Errorf("key is wrong %q != %q", got, "secret-key")
	}
	if got := query.Get("q"); got != "golang" {
		t.Errorf("query is wrong %q != %q", got, "golang")
	}
	if out.NextPageToken != "tok123" {
		t.Errorf("token is wrong %q != %q", out.NextPageToken, "tok123")
	}

	if len(out.Items) != 1 || out.Items[0].ID.VideoID != "v1" {
		t.Errorf("output wrong items: %v", out.Items)
	}
}

func TestClientGetAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"quota exceeded","errors":[{"reason":"quotaExceeded"}]}}`)
	}))
	defer server.Close()

	c := New("key", server.URL+"/")
	var out SearchResponse
	err := c.get(url.Values{}, &out)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	apiErr, ok := err.(APIError)
	if !ok {
		t.Fatalf("wrong error type %T != APIError", err)
	}
	if apiErr.ErrorData.Code != 403 {
		t.Errorf("wrong code %d != 403", apiErr.ErrorData.Code)
	}
	if apiErr.Reason() != QuotaExceeded {
		t.Errorf("wrong reason %q != %q", apiErr.Reason(), QuotaExceeded)
	}
}
