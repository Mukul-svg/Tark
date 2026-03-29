package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"simplek8/internal/models"
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
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	ModelURL  string `json:"modelUrl"`
	NodePort  int32  `json:"nodePort"`
	ClusterID string `json:"clusterId"`
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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Name == "" {
		req.Name = "vllm"
	} else {
		// Kubernetes requires valid DNS-1123 subdomains for names.
		req.Name = strings.ReplaceAll(req.Name, "_", "-")
		req.Name = strings.ToLower(req.Name)
	}
	if req.ModelURL == "" {
		req.ModelURL = "https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf?download=true"
	}
	if req.NodePort == 0 {
		req.NodePort = 30000
	}

	if req.NodePort < 30000 || req.NodePort > 32767 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "nodePort must be between 30000 and 32767"})
	}

	ctx := c.Request().Context()
	clusterID, err := h.resolveClusterID(ctx, req.ClusterID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := h.store.CreateDeployment(ctx, deployment); err != nil {
		slog.Error("Failed to create deployment record", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create deployment record"})
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
	task, err := tasks.NewDeployModelTask(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create deploy task"})
	}

	taskID := "deploy:" + deploymentID.String()
	if _, err := h.queueClient.EnqueueDeploy(task, taskID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enqueue deploy task"})
	}

	return c.JSON(http.StatusAccepted, &deployResponse{
		JobID:        jobID,
		TaskID:       taskID,
		DeploymentID: deploymentID.String(),
		Status:       "queued",
	})
}

func (h *DeployHandler) resolveClusterID(ctx context.Context, clusterIDRaw string) (uuid.UUID, error) {
	if clusterIDRaw != "" {
		clusterID, err := uuid.Parse(clusterIDRaw)
		if err != nil {
			return uuid.Nil, fmt.Errorf("invalid clusterId: %w", err)
		}
		// Verify the cluster exists and is usable
		cluster, err := h.store.GetCluster(ctx, clusterID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("cluster not found: %s", clusterIDRaw)
		}
		if cluster.Status != "active" {
			return uuid.Nil, fmt.Errorf("cluster %s is not active (status: %s)", clusterIDRaw, cluster.Status)
		}
		return clusterID, nil
	}

	org, err := h.store.GetOrganizationByName(ctx, "default")
	if err != nil {
		return uuid.Nil, fmt.Errorf("default organization not found")
	}
	clusters, err := h.store.ListClusters(ctx, org.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to list clusters")
	}
	for _, cluster := range clusters {
		if cluster.Status == "active" {
			return cluster.ID, nil
		}
	}

	return uuid.Nil, fmt.Errorf("no active cluster found; provide clusterId")
}

func (h *DeployHandler) HandleListDeployments(c echo.Context) error {
	ctx := c.Request().Context()
	deployments, err := h.store.ListDeployments(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list deployments"})
	}
	return c.JSON(http.StatusOK, deployments)
}

func (h *DeployHandler) HandleDeleteDeployment(c echo.Context) error {
	idParam := c.Param("id")
	deploymentID, err := uuid.Parse(idParam)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid deployment id"})
	}

	ctx := c.Request().Context()
	deployment, err := h.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "deployment not found"})
	}

	// Guard: reject delete on already-terminal deployments
	switch deployment.Status {
	case "deleted", "deleting", "orphaned":
		return c.JSON(http.StatusConflict, map[string]string{
			"error":  "deployment is already in terminal state",
			"status": deployment.Status,
		})
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create delete task"})
	}

	taskID := "delete:" + deploymentID.String()
	if _, err := h.queueClient.EnqueueDeleteModel(task, taskID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enqueue delete task"})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"jobId":        jobID,
		"taskId":       taskID,
		"deploymentId": deploymentID.String(),
		"status":       "queued",
	})
}
