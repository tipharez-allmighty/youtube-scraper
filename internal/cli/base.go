// Package cli.
package cli

import (
	"sync"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/storage"
)

type CLI struct {
	Run    RunCmd    `cmd:"run" help:"Run a scraping job"`
	Resume ResumeCmd `cmd:"resume" help:"Resume failed tasks"`
	Jobs   JobsCmd   `cmd:"jobs" help:"List jobs"`
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

func closeWhenDone[T any](wg *sync.WaitGroup, ch chan T) {
	wg.Wait()
	close(ch)
}
