package internal

type YoutubeResponse interface {
	URL() string
}

type SearchID struct {
	Kind    string `json:"kind"`
	VideoID string `json:"videoId"`
}

type SearchSnippet struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type SearchItem struct {
	ID      SearchID      `json:"id"`
	Snippet SearchSnippet `json:"snippet"`
}

type SearchResponse struct {
	Items []SearchItem `json:"items"`
}

func (*SearchResponse) URL() string { return "search" }
