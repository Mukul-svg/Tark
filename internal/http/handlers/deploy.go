package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"simplek8/internal/apierror"
	"simplek8/internal/models"
	"simplek8/internal/queue"
	"simplek8/internal/store"
	"simplek8/internal/worker"
	"simplek8/internal/worker/tasks"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type DeployHandler struct {
	store       store.Store
	queueClient *worker.Client
}

func NewDeployHandler(st store.Store, queueClient *worker.Client) *DeployHandler {
	return &DeployHandler{
		store:       st,
		queueClient: queueClient,
	}
}

type deployRequest struct {
	Namespace string `json:"namespace" validate:"omitempty,min=1,max=63"`
	Name      string `json:"name" validate:"omitempty,min=1,max=63"`
	ModelURL  string `json:"modelUrl" validate:"omitempty,http_url"`
	NodePort  int32  `json:"nodePort" validate:"omitempty,min=30000,max=32767"`
	ClusterID string `json:"clusterId" validate:"omitempty,uuid4"`
}

type deployResponse struct {
	JobID        string `json:"jobId"`
	TaskID       string `json:"taskId"`
	DeploymentID string `json:"deploymentId"`
	Status       string `json:"status"`
}

func (h *DeployHandler) PostDeploy(c echo.Context) error {
	req := new(deployRequest)
	if err := c.Bind(req); err != nil {
		return apierror.Respond(c, apierror.BadRequest(apierror.InvalidRequestBody, "invalid request payload"))
	}
	if err := c.Validate(req); err != nil {
		return apierror.Respond(c, validationError(err))
	}

	// Apply defaults for omitted optional fields.
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Name == "" {
		req.Name = "vllm"
	} else {
		req.Name = strings.ReplaceAll(req.Name, "_", "-")
		req.Name = strings.ToLower(req.Name)
	}
	if req.ModelURL == "" {
		req.ModelURL = "https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf?download=true"
	}
	if req.NodePort == 0 {
		req.NodePort = 30000
	}

	ctx := c.Request().Context()
	clusterID, apiErr := h.resolveClusterID(ctx, req.ClusterID)
	if apiErr != nil {
		return apierror.Respond(c, apiErr)
	}

	deploymentID := uuid.New()
	deployment := &models.Deployment{
		ID:        deploymentID,
		ClusterID: clusterID,
		ModelName: req.Name,
		Namespace: req.Namespace,
		NodePort:  req.NodePort,
		ModelURL:  req.ModelURL,
		Replicas:  1,
		Status:    string(models.DeploymentStatusPending),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := h.store.CreateDeployment(ctx, deployment); err != nil {
		slog.Error("failed to create deployment record", "error", err)
		return apierror.Respond(c, apierror.Internal(apierror.StoreError, "failed to create deployment record", err))
	}

	jobID := uuid.NewString()
	payload := tasks.DeployModelPayload{
		JobID:        jobID,
		DeploymentID: deploymentID.String(),
		ClusterID:    clusterID.String(),
		Namespace:    req.Namespace,
		Name:         req.Name,
		ModelURL:     req.ModelURL,
		NodePort:     req.NodePort,
	}

	clusterStr := clusterID.String()
	deploymentStr := deploymentID.String()
	CreateJobRecord(ctx, h.store, jobID, queue.TaskTypeDeployModel, payload, &clusterStr, &deploymentStr)

	task, err := tasks.NewDeployModelTask(payload)
	if err != nil {
		return apierror.Respond(c, apierror.Internal(apierror.QueueError, "failed to create deploy task", err))
	}

	taskID := "deploy:" + deploymentID.String()
	if _, err := h.queueClient.EnqueueDeploy(task, taskID); err != nil {
		return apierror.Respond(c, apierror.Internal(apierror.QueueError, "failed to enqueue deploy task", err))
	}

	return c.JSON(http.StatusAccepted, &deployResponse{
		JobID:        jobID,
		TaskID:       taskID,
		DeploymentID: deploymentID.String(),
		Status:       "queued",
	})
}

// resolveClusterID validates and returns the cluster UUID, or an apierror.
func (h *DeployHandler) resolveClusterID(ctx context.Context, clusterIDRaw string) (uuid.UUID, *apierror.Error) {
	if clusterIDRaw != "" {
		clusterID, err := uuid.Parse(clusterIDRaw)
		if err != nil {
			return uuid.Nil, apierror.BadRequest(apierror.InvalidRequestField, "invalid clusterId")
		}
		cluster, err := h.store.GetCluster(ctx, clusterID)
		if err != nil {
			return uuid.Nil, apierror.BadRequest(apierror.NotFound, "cluster not found")
		}
		if cluster.Status != "active" {
			return uuid.Nil, apierror.BadRequest(apierror.Conflict, fmt.Sprintf("cluster is not active (status: %s)", cluster.Status))
		}
		return clusterID, nil
	}

	org, err := h.store.GetOrganizationByName(ctx, "default")
	if err != nil {
		return uuid.Nil, apierror.Internal(apierror.StoreError, "default organization not found", err)
	}
	clusters, err := h.store.ListClusters(ctx, org.ID)
	if err != nil {
		return uuid.Nil, apierror.Internal(apierror.StoreError, "failed to list clusters", err)
	}
	for _, cluster := range clusters {
		if cluster.Status == "active" {
			return cluster.ID, nil
		}
	}

	return uuid.Nil, apierror.BadRequest(apierror.InvalidRequestField, "no active cluster found; provide clusterId")
}

func (h *DeployHandler) HandleListDeployments(c echo.Context) error {
	ctx := c.Request().Context()
	deployments, err := h.store.ListDeployments(ctx)
	if err != nil {
		return apierror.Respond(c, apierror.Internal(apierror.StoreError, "failed to list deployments", err))
	}
	return c.JSON(http.StatusOK, deployments)
}

func (h *DeployHandler) HandleDeleteDeployment(c echo.Context) error {
	idParam := c.Param("id")
	deploymentID, err := uuid.Parse(idParam)
	if err != nil {
		return apierror.Respond(c, apierror.BadRequest(apierror.NotFound, "invalid deployment id"))
	}

	ctx := c.Request().Context()
	deployment, err := h.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return apierror.Respond(c, apierror.NotFoundCode(apierror.NotFound, "deployment not found"))
	}

	switch deployment.Status {
	case "deleted", "deleting", "orphaned":
		return apierror.Respond(c, apierror.ConflictErr(apierror.Conflict, fmt.Sprintf("deployment is already in terminal state (%s)", deployment.Status)))
	}

	ns := deployment.Namespace
	if ns == "" {
		ns = "default"
	}

	jobID := uuid.NewString()
	payload := tasks.DeleteModelPayload{
		JobID:        jobID,
		DeploymentID: deployment.ID.String(),
		ClusterID:    deployment.ClusterID.String(),
		Namespace:    ns,
		Name:         deployment.ModelName,
	}

	task, err := tasks.NewDeleteModelTask(payload)
	if err != nil {
		return apierror.Respond(c, apierror.Internal(apierror.QueueError, "failed to create delete task", err))
	}

	taskID := "delete:" + deploymentID.String()
	if _, err := h.queueClient.EnqueueDeleteModel(task, taskID); err != nil {
		return apierror.Respond(c, apierror.Internal(apierror.QueueError, "failed to enqueue delete task", err))
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"jobId":        jobID,
		"taskId":       taskID,
		"deploymentId": deploymentID.String(),
		"status":       "queued",
	})
}
