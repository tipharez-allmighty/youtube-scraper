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
	ID        string            `json:"id" validate:"required"`
	Input     input.InputSchema `json:"input" validate:"required"`
	CreatedAt time.Time         `json:"created_at" validate:"required"`
	UpdatedAt time.Time         `json:"updated_at" validate:"required"`
}

type JobStatus struct {
	ID      string            `json:"id" validate:"required"`
	Input   input.InputSchema `json:"input" validate:"required"`
	Total   int               `json:"total" validate:"required,gte=0"`
	Running int               `json:"running" validate:"required,gte=0"`
	Failed  int               `json:"failed" validate:"required,gte=0"`
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
	PublishedAt time.Time
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

