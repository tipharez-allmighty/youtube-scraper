package export

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/storage"
)

func getStoreForExport(cfg *config.Config, stateFile string) (*storage.Store, error) {
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

func ExportSQLite(cfg *config.Config, jobID, file, path string) (err error) {
	dbPath := file
	if dbPath == "" {
		dbPath = cfg.StateFile
	}
	dstPath := filepath.Join(path, dbPath)
	if err = copyDBFile(dbPath, dstPath); err != nil {
		return fmt.Errorf("failed to create sqlite export file: %w", err)
	}
	defer func() {
		if err != nil {
			deleteDBFile(dstPath)
		}
	}()
	store, err := getStoreForExport(cfg, dstPath)
	if err != nil {
		return fmt.Errorf("failed to load export storage: %w", err)
	}
	defer store.Close()
	if err = store.CleanDataByJobID(jobID); err != nil {
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

func copyDBFile(dbPath, dstPath string) error {
	srcFile, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	defer dstFile.Sync()
	return nil
}

func deleteDBFile(filePath string) error {
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to clean db file %v: %w", filePath, err)
	}
	return nil
}
