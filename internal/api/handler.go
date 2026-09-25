// Package api.
package api

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/google/uuid"

	"tipharez-allmighty/youtube-scraper/internal/channel"
	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/export"
	"tipharez-allmighty/youtube-scraper/internal/input"
	"tipharez-allmighty/youtube-scraper/internal/storage"

	"github.com/go-playground/validator/v10"
)

const defaultJobsLimit = 5

func CreateJob(ctx context.Context, cfg *config.Config, store *storage.Store, ytSearchJob SearchJobFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload input.InputSchema
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, "Invalid input format", http.StatusBadRequest)
			return
		}
		inputValidator := validator.New()
		if err := inputValidator.Struct(payload); err != nil {
			writeJSONError(w, "Wrong input structure", http.StatusBadRequest)
			return
		}
		jobID, err := uuid.NewV7()
		if err != nil {
			slog.Error("Failed to generate uuid7 for new job", "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		job := storage.Job{ID: jobID.String(), Input: payload}
		if err := store.InsertJob(job); err != nil {
			slog.Error("Failed to create a job", "job_id", jobID.String(), "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		go ytSearchJob(ctx, cfg, payload, job)

		writeJobAcceptedResponse(w, job.ID)
	}
}

func ResumeTasks(ctx context.Context, cfg *config.Config, store *storage.Store, ytResumeJob ResumeJobFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("id")
		if jobID == "" {
			writeJSONError(w, "Job ID is required", http.StatusBadRequest)
			return
		}
		jobInput, err := store.SelectJobInput(jobID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, fmt.Sprintf("There is no job with id: %v", jobID), http.StatusNotFound)
				return
			}
			slog.Error("Failed to select job input data", "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		tasks, err := store.SelectFailedTasks(jobID)
		if err != nil {
			slog.Error("Failed to load failed tasks", "job_id", jobID, "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if len(tasks) == 0 {
			writeJSONError(w, "No failed tasks to resume", http.StatusConflict)
			return
		}
		go ytResumeJob(ctx, cfg, jobID, *jobInput, tasks)
		writeJobAcceptedResponse(w, jobID)
	}
}

func GetJobs(cfg *config.Config, store *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := defaultJobsLimit
		limitStr := r.URL.Query().Get("limit")
		if limitStr != "" {
			limitParsed, err := strconv.Atoi(limitStr)
			if err != nil {
				slog.Error("String conversion into integer failed", "value", limitStr, "error", err)
				writeJSONError(w, "limit should be numeric", http.StatusBadRequest)
				return
			}
			if limitParsed <= 0 {
				slog.Error("Limit should be a positive number", "value", limitStr)
				writeJSONError(w, "limit should be a positive number", http.StatusBadRequest)
				return
			}
			limit = limitParsed
		}
		jobs, err := store.SelectJobs(limit)
		if err != nil {
			slog.Error("Failed to select jobs", "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJobsResponse(w, jobs)
	}
}

func GetJobStatus(cfg *config.Config, store *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("id")
		if jobID == "" {
			writeJSONError(w, "Job ID is required", http.StatusBadRequest)
			return
		}
		jobStatus, err := store.SelectJobsStatus(jobID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, fmt.Sprintf("There is no job with id: %v", jobID), http.StatusNotFound)
				return
			}
			slog.Error("Failed to select jobs", "job_id", jobID, "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJobStatusResponse(w, jobStatus)
	}
}

type ExportSQLFunc func(cfg *config.Config, store *storage.Store, jobID, file, path string) error

func ExportSQLite(cfg *config.Config, store *storage.Store, exportSQL ExportSQLFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("id")
		if jobID == "" {
			writeJSONError(w, "Job ID is required", http.StatusBadRequest)
			return
		}
		jobStatus, err := store.SelectJobsStatus(jobID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, fmt.Sprintf("There is no job with id: %v", jobID), http.StatusNotFound)
				return
			}
			slog.Error("Failed to select jobs", "job_id", jobID, "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if jobStatus.Failed > 0 || jobStatus.Running > 0 || jobStatus.Total == 0 {
			writeJSONError(w, fmt.Sprintf("Job cannot be processed in its current state (Total: %d, Running: %d, Failed: %d)",
				jobStatus.Total, jobStatus.Running, jobStatus.Failed), http.StatusConflict)
			return
		}
		tmpDir, err := os.MkdirTemp("", "export-*")
		if err != nil {
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				slog.Error("Failed to remove temporary directory", "error", err)
			}
		}()
		if err := exportSQL(cfg, store, jobStatus.ID, "", tmpDir); err != nil {
			slog.Error("Failed to export sqlite db file", "job_id", jobStatus.ID, "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v.db", jobStatus.ID))
		http.ServeFile(w, r, filepath.Join(tmpDir, filepath.Base(cfg.StateFile)))
	}
}

func ExportCSV(cfg *config.Config, store *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		jobID := r.PathValue("id")
		if jobID == "" {
			writeJSONError(w, "Job ID is required", http.StatusBadRequest)
			return
		}
		jobStatus, err := store.SelectJobsStatus(jobID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, fmt.Sprintf("There is no job with id: %v", jobID), http.StatusNotFound)
				return
			}
			slog.Error("Failed to select jobs", "job_id", jobID, "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if jobStatus.Failed > 0 || jobStatus.Running > 0 || jobStatus.Total == 0 {
			writeJSONError(w, fmt.Sprintf("Job cannot be processed in its current state (Total: %d, Running: %d, Failed: %d)",
				jobStatus.Total, jobStatus.Running, jobStatus.Failed), http.StatusConflict)
			return
		}
		tmpDir, err := os.MkdirTemp("", "export-*")
		if err != nil {
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				slog.Error("Failed to remove temporary directory", "error", err)
			}
		}()
		var createErrs []error

		videosFile, err := os.Create(filepath.Join(tmpDir, "videos.csv"))
		if err != nil {
			createErrs = append(createErrs, fmt.Errorf("failed to create videos.csv: %w", err))
		}
		defer videosFile.Close()

		threadsFile, err := os.Create(filepath.Join(tmpDir, "threads.csv"))
		if err != nil {
			createErrs = append(createErrs, fmt.Errorf("failed to create threads.csv: %w", err))
		}
		defer threadsFile.Close()
		commentsFile, err := os.Create(filepath.Join(tmpDir, "comments.csv"))
		if err != nil {
			createErrs = append(createErrs, fmt.Errorf("failed to create comments.csv: %w", err))
		}
		defer commentsFile.Close()

		if len(createErrs) > 0 {
			slog.Error(
				"Failure during CSV file creation", "job_id", jobID, "error", errors.Join(createErrs...),
			)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		var wg sync.WaitGroup
		errChan := make(chan error, 3)

		wg.Go(func() {
			if err := export.WriteCSV(ctx, videosFile, jobID, cfg.LimitRows, store.SelectVideos); err != nil {
				cancel()
				errChan <- fmt.Errorf("videos export failed: %w", err)
			}
		})
		wg.Go(func() {
			if err := export.WriteCSV(ctx, threadsFile, jobID, cfg.LimitRows, store.SelectThreads); err != nil {
				cancel()
				errChan <- fmt.Errorf("threads export failed: %w", err)
			}
		})
		wg.Go(func() {
			if err := export.WriteCSV(ctx, commentsFile, jobID, cfg.LimitRows, store.SelectComments); err != nil {
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
			slog.Error("Failure during csv export", "job_id", jobID, "error", errors.Join(errs...))
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v.zip", jobID))
		zw := zip.NewWriter(w)
		defer zw.Close()
		for _, name := range []string{"videos.csv", "threads.csv", "comments.csv"} {
			file, err := os.Open(filepath.Join(tmpDir, name))
			if err != nil {
				slog.Error("Failed to open temp CSV for zipping", "file", name, "error", err)
				return
			}
			entry, err := zw.Create(name)
			if err != nil {
				file.Close()
				slog.Error("Failed to create zip entry", "file", name, "error", err)
				return
			}
			if _, err := io.Copy(entry, file); err != nil {
				file.Close()
				slog.Error("Failed to copy CSV into zip", "file", name, "error", err)
				return
			}
			file.Close()
		}
	}
}
