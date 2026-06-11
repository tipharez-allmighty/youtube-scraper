// Package internal
package internal

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Client struct {
	apiKey string
}

func New(apiKey string) *Client {
	return &Client{apiKey: apiKey}
}

func (c *Client) Get(params url.Values, out YoutubeResponse) error {
	params.Set("key", c.apiKey)
	reqUrl := BaseUrl + out.Url() + "?" + params.Encode()
	resp, err := http.Get(reqUrl)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
}
