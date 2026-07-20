// Package cli.
package cli

import (
	"context"
	"log/slog"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/storage"
)

type CLI struct {
	Run     RunCmd    `cmd:"run" help:"Run a scraping job"`
	Resume  ResumeCmd `cmd:"resume" help:"Resume failed tasks"`
	Jobs    JobsCmd   `cmd:"jobs" help:"List jobs"`
	Verbose int       `help:"Increase verbosity." short:"v" type:"counter"`
}

func getStore(cfg *config.Config, stateFile string) (*storage.Store, error) {
	dbPath := stateFile
	if dbPath == "" {
		dbPath = cfg.StateFile
	}
	db, err := storage.Init(dbPath)
	if err != nil {
		return nil, err
	}
	store := storage.NewStore(db)
	return store, nil
}

func failInterruptedTasks(ctx context.Context, s *storage.Store, jobID string) {
	if ctx.Err() != nil {
		errMsg := ctx.Err().Error()
		if err := s.FailRunningTasks(jobID, &errMsg); err != nil {
			slog.Error("failed to mark running tasks as failed", "job_id", jobID, "error", err)
		}
	}
}
