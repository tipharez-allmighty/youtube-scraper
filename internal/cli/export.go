package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"tipharez-allmighty/youtube-scraper/internal/channel"
	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/export"
)

type ExportCmd struct {
	JobID     string `arg:"" help:"Job ID for checking job status"`
	Path      string `short:"p" default:"./data/" help:"Path were to save file"`
	Format    string `short:"f" enum:"csv,sqlite" default:"csv" help:"Output format (csv or sqlite)"`
	StateFile string `short:"s" optional:"" help:"Path to state file"`
}

func (e *ExportCmd) Run(cfg *config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filePath := filepath.Join(e.Path, e.JobID)
	if err := os.MkdirAll(filePath, 0o755); err != nil {
		return fmt.Errorf("failed to create folder for exported data: %w", err)
	}
	if e.Format == "sqlite" {
		if err := export.ExportSQLite(cfg, e.JobID, e.StateFile, filePath); err != nil {
			return fmt.Errorf("failed to export sqlite data: %w", err)
		} else {
			return nil
		}
	}
	store, err := getStore(cfg, e.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %w", err)
	}
	defer store.Close()
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	wg.Go(func() {
		if err := export.WriteCSV(ctx, filepath.Join(filePath, "videos.csv"), e.JobID, cfg.LimitRows, store.SelectVideos); err != nil {
			cancel()
			errChan <- fmt.Errorf("videos export failed: %w", err)
		}
	})
	wg.Go(func() {
		if err := export.WriteCSV(ctx, filepath.Join(filePath, "threads.csv"), e.JobID, cfg.LimitRows, store.SelectThreads); err != nil {
			cancel()
			errChan <- fmt.Errorf("threads export failed: %w", err)
		}
	})
	wg.Go(func() {
		if err := export.WriteCSV(ctx, filepath.Join(filePath, "comments.csv"), e.JobID, cfg.LimitRows, store.SelectComments); err != nil {
			cancel()
			errChan <- fmt.Errorf("comments export failed: %w", err)
		}
	})
	channel.CloseWhenDone(&wg, errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
