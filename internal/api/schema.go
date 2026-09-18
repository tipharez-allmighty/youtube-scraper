package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"tipharez-allmighty/youtube-scraper/internal/storage"
)

type JobSubmitResponse struct {
	ID string `json:"job_id"`
}

func writeJobAcceptedResponse(w http.ResponseWriter, jobID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(JobSubmitResponse{ID: jobID}); err != nil {
		slog.Error("failed to encode JSON error response", "error", err)
	}
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: message}); err != nil {
		slog.Error("failed to encode JSON error response", "error", err)
	}
}

func writeJobStatusResponse(w http.ResponseWriter, jobStatus *storage.JobStatus) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(jobStatus); err != nil {
		slog.Error("failed to encode JSON job status response", "error", err)
	}
}

func writeJobsResponse(w http.ResponseWriter, jobs []storage.Job) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(jobs); err != nil {
		slog.Error("failed to encode JSON jobs response", "error", err)
	}
}
