package storage

import (
	"time"

	"tipharez-allmighty/youtube-scraper/internal/input"
)

type Status string

const (
	Running Status = "running"
	Failed  Status = "failed"
	Done    Status = "done"
)

type Type string

const (
	Search Type = "search"
	Thread Type = "thread"
	Reply  Type = "reply"
)

type Job struct {
	ID        string
	Input     input.InputSchema
	CreatedAt time.Time
	UpdatedAt time.Time
}

type JobStatus struct {
	ID      string
	Input   input.InputSchema
	Total   int
	Running int
	Failed  int
}

type Task struct {
	ID        string
	JobID     string
	Type      Type
	Status    Status
	Payload   string
	PageToken *string
	Error     *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Video struct {
	ID          string
	JobID       string
	QueryText   string
	Title       string
	Description string
	CreatedAt   time.Time
}

type CommentBase struct {
	ID           string
	JobID        string
	Author       string
	TextDisplay  string
	TextOriginal string
	LikeCount    int
	PublishedAt  time.Time
	CreatedAt    time.Time
}

type CommentThread struct {
	CommentBase
	VideoID         string
	TotalReplyCount int
}

type Comment struct {
	CommentBase
	ThreadID string
}
