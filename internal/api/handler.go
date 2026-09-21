// Package api.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"

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
			writeJSONError(w, "Invalid input foramt", http.StatusBadRequest)
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
				slog.Error("String conversion into intreger failed", "value", limitStr, "error", err)
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
			slog.Error("Failed to select jobs", "job_id", jobID, "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJobStatusResponse(w, jobStatus)
	}
}

func ExportSQLite(cfg *config.Config, store *storage.Store) http.HandlerFunc {
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
		if err := export.ExportSQLite(cfg, store, jobStatus.ID, "", tmpDir); err != nil {
			slog.Error("Failed to export sqlite db file", "job_id", jobStatus.ID, "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v.db", jobStatus.ID))
		http.ServeFile(w, r, filepath.Join(tmpDir, filepath.Base(cfg.StateFile)))
	}
}
