package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"simplek8/internal/apierror"
	"simplek8/internal/models"
	"simplek8/internal/store"

	"github.com/google/uuid"
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

// statusResource is the linked entity (cluster or deployment) associated with a job.
type statusResource struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Status string `json:"status"`
}

// statusResponse is the enriched response for GET /api/status/:jobId.
type statusResponse struct {
	JobID       string          `json:"jobId"`
	TaskType    string          `json:"taskType"`
	Status      string          `json:"status"`
	Error       *string         `json:"error,omitempty"`
	StartedAt   interface{}     `json:"startedAt,omitempty"`
	CompletedAt interface{}     `json:"completedAt,omitempty"`
	Resource    *statusResource `json:"resource,omitempty"`
}

// GetStatusByJobID returns a unified view of a job and its linked resource.
// It resolves the cluster or deployment status alongside the job state,
// giving callers a single endpoint to poll the full lifecycle.
func (h *JobsHandler) GetStatusByJobID(c echo.Context) error {
	jobID := c.Param("jobId")
	if jobID == "" {
		return apierror.Respond(c, apierror.BadRequest(apierror.InvalidRequestField, "jobId is required"))
	}

	ctx := c.Request().Context()
	job, err := h.store.GetJob(ctx, jobID)
	if err != nil {
		return apierror.Respond(c, apierror.NotFoundCode(apierror.NotFound, "job not found"))
	}

	resp := &statusResponse{
		JobID:       job.JobID,
		TaskType:    job.TaskType,
		Status:      job.Status,
		Error:       job.Error,
		StartedAt:   job.StartedAt,
		CompletedAt: job.CompletedAt,
	}

	// Resolve the linked resource status for richer polling responses.
	if job.DeploymentID != nil {
		if depID, parseErr := uuid.Parse(*job.DeploymentID); parseErr == nil {
			if dep, depErr := h.store.GetDeployment(ctx, depID); depErr == nil {
				resp.Resource = &statusResource{
					Type:   "deployment",
					ID:     dep.ID.String(),
					Status: dep.Status,
				}
			}
		}
	} else if job.ClusterID != nil {
		if clsID, parseErr := uuid.Parse(*job.ClusterID); parseErr == nil {
			if cls, clsErr := h.store.GetCluster(ctx, clsID); clsErr == nil {
				resp.Resource = &statusResource{
					Type:   "cluster",
					ID:     cls.ID.String(),
					Status: cls.Status,
				}
			}
		}
	}

	return c.JSON(http.StatusOK, resp)
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
		Status:   string(models.JobStatusQueued),
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
