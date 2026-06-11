package internal

type YoutubeResponse interface {
	URL() string
}

type SearchResponse struct {
	Items []struct {
		ID struct {
			Kind    string `json:"kind"`
			VideoID string `json:"videoId"`
		} `json:"id"`
		Snippet struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"snippet"`
	} `json:"items"`
}

func (*SearchResponse) URL() string { return "search" }
