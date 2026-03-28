package handlers

import (
	"context"
	"fmt"
	"net/http"
	"simplek8/internal/models"
	"simplek8/internal/store"
	"simplek8/internal/worker"
	"simplek8/internal/worker/tasks"
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
	}
	if req.ModelURL == "" {
		req.ModelURL = "https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf?download=true"
	}
	if req.NodePort == 0 {
		req.NodePort = 30000
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
		Replicas:  1,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := h.store.CreateDeployment(ctx, deployment); err != nil {
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
		if cluster.Status == "active" || cluster.Status == "installing" {
			return cluster.ID, nil
		}
	}

	return uuid.Nil, fmt.Errorf("no active cluster found; provide clusterId")
}
