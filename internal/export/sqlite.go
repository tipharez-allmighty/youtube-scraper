package export

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/storage"
)

func GetStoreForExport(cfg *config.Config, stateFile string) (*storage.Store, error) {
	dbPath := stateFile
	if dbPath == "" {
		dbPath = cfg.StateFile
	}
	db, err := initForExport(dbPath)
	if err != nil {
		return nil, err
	}
	store := storage.NewStore(db)
	return store, nil
}

func ExportSQLite(cfg *config.Config, store *storage.Store, jobID, file, path string) (err error) {
	dbPath := file
	if dbPath == "" {
		dbPath = cfg.StateFile
	}
	dstPath := filepath.Join(path, filepath.Base(dbPath))
	if err = copyDBFile(store, dstPath); err != nil {
		return fmt.Errorf("failed to vacuum sqlite export file: %w", err)
	}
	defer func() {
		if err != nil {
			if err := deleteDBFile(dstPath); err != nil {
				slog.Error("Faield to delete file during failure", "error", err)
			}
		}
	}()
	storeExp, err := GetStoreForExport(cfg, dstPath)
	if err != nil {
		return fmt.Errorf("failed to load export storage: %w", err)
	}
	defer storeExp.Close()
	if err = storeExp.CleanDataByJobID(jobID); err != nil {
		return fmt.Errorf("failed to clean sqlite data: %w", err)
	}
	return nil
}

func initForExport(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func copyDBFile(store *storage.Store, dstPath string) error {
	if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return store.VacuumDB(dstPath)
}

func deleteDBFile(filePath string) error {
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to clean db file %v: %w", filePath, err)
	}
	return nil
}
