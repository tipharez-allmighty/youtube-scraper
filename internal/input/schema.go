// Package input.
package input

import "time"

type Order string

const (
	OrderRelevance Order = "relevance"
	OrderDate      Order = "date"
	OrderViewCount Order = "viewCount"
	OrderRating    Order = "rating"
)

type Query struct {
	Text            string    `json:"text"  validate:"required"`
	Order           Order     `json:"order" validate:"omitempty,oneof=relevance date viewCount rating"`
	PublishedAfter  time.Time `json:"published_after"`
	PublishedBefore time.Time `json:"published_before"`
}

type InputSchema struct {
	Queries            []Query `json:"queries"               validate:"required"`
	MaxResultsPerQuery int     `json:"max_results_per_query" validate:"required,min=1,max=50"`
	StateFile          string  `json:"state_file"`
	MaxPages           int     `json:"max_pages" validate:"min=0"`
	MaxThreads         int     `json:"max_threads" validate:"min=0"`
	MaxComments        int     `json:"max_comments" validate:"min=0"`
	Order              Order   `json:"order" validate:"omitempty,oneof=relevance date viewCount rating"`
}
