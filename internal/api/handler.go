// Package api.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"tipharez-allmighty/youtube-scraper/internal/config"
	"tipharez-allmighty/youtube-scraper/internal/input"
	"tipharez-allmighty/youtube-scraper/internal/storage"
	"tipharez-allmighty/youtube-scraper/internal/youtube"

	"github.com/go-playground/validator/v10"
)

const defaultJobsLimit = 5

func CreateJob(ctx context.Context, cfg *config.Config) http.HandlerFunc {
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
		store, err := storage.GetStore(cfg, "")
		if err != nil {
			slog.Error("failed to load storage", "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer store.Close()
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
		go func() {
			store, err := storage.GetStore(cfg, payload.StateFile)
			if err != nil {
				slog.Error("failed to load storage during youtube search", "error", err)
				return
			}
			defer store.Close()
			defer func() { store.FailInterruptedTasks(ctx, job.ID, err) }()
			client := youtube.New(cfg.YoutubeAPIKey, cfg.YoutubeBaseURL)
			if err = youtube.RunSearch(ctx, cfg, client, store, job, payload); err != nil {
				err = fmt.Errorf("failed to run youtube search: %w", err)
				return
			}
		}()
		writeJobAcceptedResponse(w, job.ID)
	}
}

func GetJobs(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := defaultJobsLimit
		limitStr := r.URL.Query().Get("limit")
		if limitStr != "" {
			limitParsed, err := strconv.Atoi(limitStr)
			if err != nil {
				slog.Error("string conversion into intreger failed", "value", limitStr, "error", err)
				writeJSONError(w, "limit should be numeric", http.StatusBadRequest)
				return
			}
			if limitParsed <= 0 {
				slog.Error("limit should be a positive number", "value", limitStr)
				writeJSONError(w, "limit should be a positive number", http.StatusBadRequest)
			}
			limit = limitParsed
		}
		store, err := storage.GetStore(cfg, "")
		if err != nil {
			slog.Error("failed to load storage", "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer store.Close()
		jobs, err := store.SelectJobs(limit)
		if err != nil {
			slog.Error("failed to select jobs", "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJobsResponse(w, jobs)
	}
}

func GetJobStatus(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("id")
		if jobID == "" {
			writeJSONError(w, "Job ID is required", http.StatusBadRequest)
			return
		}
		store, err := storage.GetStore(cfg, "")
		if err != nil {
			slog.Error("failed to load storage", "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer store.Close()
		jobStatus, err := store.SelectJobsStatus(jobID)
		if err != nil {
			slog.Error("failed to select jobs", "job_id", jobID, "error", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJobStatusResponse(w, jobStatus)
	}
}
