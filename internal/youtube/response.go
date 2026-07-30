package youtube

import "time"

type YoutubeResponse interface {
	URL() string
}

type SearchID struct {
	Kind    string `json:"kind"`
	VideoID string `json:"videoId"`
}

type SearchSnippet struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PublishedAt time.Time `json:"publishedAt"`
}

type SearchItem struct {
	ID      SearchID      `json:"id"`
	Snippet SearchSnippet `json:"snippet"`
}

type SearchResponse struct {
	NextPageToken string       `json:"nextPageToken"`
	Items         []SearchItem `json:"items"`
}

func (*SearchResponse) URL() string { return "search" }

type CommentSnippet struct {
	ParentID          string    `json:"parentId,omitempty"`
	TextDisplay       string    `json:"textDisplay"`
	TextOriginal      string    `json:"textOriginal"`
	AuthorDisplayName string    `json:"authorDisplayName"`
	LikeCount         int       `json:"likeCount"`
	PublishedAt       time.Time `json:"publishedAt"`
}

type Comment struct {
	ID      string         `json:"id"`
	Snippet CommentSnippet `json:"snippet"`
}

type CommentThreadSnippet struct {
	VideoID         string  `json:"videoId"`
	TopLevelComment Comment `json:"topLevelComment"`
	TotalReplyCount int     `json:"totalReplyCount"`
}

type CommentReplies struct {
	Comments []Comment `json:"comments"`
}

type CommentThread struct {
	ID      string               `json:"id"`
	Snippet CommentThreadSnippet `json:"snippet"`
	Replies *CommentReplies      `json:"replies"`
}

type CommentThreadResponse struct {
	NextPageToken string          `json:"nextPageToken"`
	Items         []CommentThread `json:"items"`
}

func (*CommentThreadResponse) URL() string { return "commentThreads" }

type CommentResponse struct {
	NextPageToken string    `json:"nextPageToken"`
	Items         []Comment `json:"items"`
}

func (*CommentResponse) URL() string { return "comments" }
