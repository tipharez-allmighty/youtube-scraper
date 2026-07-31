// Package youtube.
package youtube

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Client struct {
	apiKey  string
	baseURL string
}

func New(apiKey, baseURL string) *Client {
	return &Client{apiKey: apiKey, baseURL: baseURL}
}

func (c *Client) get(params url.Values, out YoutubeResponse) error {
	params.Set("key", c.apiKey)
	reqURL := c.baseURL + out.URL() + "?" + params.Encode()
	resp, err := http.Get(reqURL)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr APIError
		if err := json.Unmarshal(body, &apiErr); err != nil {
			return fmt.Errorf("failed to unmarshal error response: %w, error status code: %v", err, resp.StatusCode)
		}
		if apiErr.ErrorData != nil {
			return apiErr
		}
	}
	return json.Unmarshal(body, out)
}
