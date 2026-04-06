package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"simplek8/internal/apierror"
	"simplek8/internal/models"
	"simplek8/internal/store"

	"github.com/labstack/echo/v4"
)

type JobsHandler struct {
	store store.Store
}

func NewJobsHandler(st store.Store) *JobsHandler {
	return &JobsHandler{store: st}
}

func (h *JobsHandler) GetJobStatus(c echo.Context) error {
	jobID := c.Param("id")
	if jobID == "" {
		return apierror.Respond(c, apierror.BadRequest(apierror.InvalidRequestField, "job id is required"))
	}

	job, err := h.store.GetJob(c.Request().Context(), jobID)
	if err != nil {
		return apierror.Respond(c, apierror.NotFoundCode(apierror.NotFound, "job not found"))
	}

	return c.JSON(http.StatusOK, job)
}

func (h *JobsHandler) ListJobs(c echo.Context) error {
	jobs, err := h.store.ListJobs(c.Request().Context())
	if err != nil {
		return apierror.Respond(c, apierror.Internal(apierror.StoreError, "failed to list jobs", err))
	}

	return c.JSON(http.StatusOK, jobs)
}

func CreateJobRecord(ctx context.Context, st store.Store, jobID, taskType string, payload any, clusterID, deploymentID *string) *models.Job {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("failed to marshal job payload", "jobId", jobID, "error", err)
		return nil
	}
	job := &models.Job{
		JobID:    jobID,
		TaskType: taskType,
		Status:   "queued",
		Payload:  string(payloadJSON),
	}
	if clusterID != nil {
		job.ClusterID = clusterID
	}
	if deploymentID != nil {
		job.DeploymentID = deploymentID
	}
	if err := st.CreateJob(ctx, job); err != nil {
		slog.Warn("failed to create job record", "jobId", jobID, "error", err)
	}
	return job
}
