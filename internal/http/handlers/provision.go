package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"simplek8/internal/models"
	"simplek8/internal/store"
	"simplek8/internal/worker"
	"simplek8/internal/worker/tasks"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type ProvisionRequest struct {
	StackName      string `json:"stackName"`
	Region         string `json:"region"`
	VMSize         string `json:"vmSize"`
	SubscriptionID string `json:"subscriptionId"`
}

type ProvisionResponse struct {
	JobID     string `json:"jobId"`
	TaskID    string `json:"taskId"`
	ClusterID string `json:"clusterId"`
	Status    string `json:"status"`
}

type ProvisionHandler struct {
	store       store.Store
	queueClient *worker.Client
}

func NewProvisionHandler(st store.Store, queueClient *worker.Client) *ProvisionHandler {
	return &ProvisionHandler{
		store:       st,
		queueClient: queueClient,
	}
}

func (h *ProvisionHandler) HandleProvision(c echo.Context) error {
	var req ProvisionRequest
	if err := c.Bind(&req); err != nil {
		slog.Error("Failed to parse provision request", "error", err)
		return c.String(http.StatusBadRequest, "Invalid request payload")
	}
	if req.StackName == "" {
		return c.String(http.StatusBadRequest, "stackName is required")
	}

	cwd, _ := os.Getwd()
	infraDir := filepath.Join(cwd, "infra", "azure")
	sshKeyPath := filepath.Join(cwd, "infra", "azure", "azure_rsa")

	ctx := c.Request().Context()
	org, err := h.ensureDefaultOrganization(ctx)
	if err != nil {
		slog.Error("Failed to ensure default organization", "error", err)
		return c.String(http.StatusInternalServerError, "Database error: failed to ensure organization")
	}

	region := req.Region
	if region == "" {
		region = "southindia"
	}

	clusterID := uuid.New()

	if existingCluster, lookupErr := h.store.GetClusterByName(ctx, req.StackName); lookupErr == nil && existingCluster != nil {
		if existingCluster.Status != "destroyed" && existingCluster.Status != "failed" {
			slog.Warn("Cluster already exists and is not in a re-provisionable state", "name", req.StackName, "status", existingCluster.Status)
			return c.JSON(http.StatusConflict, map[string]string{
				"error":     "Cluster with this stack name already exists",
				"clusterId": existingCluster.ID.String(),
				"status":    existingCluster.Status,
			})
		}

		// Reuse the existing record for re-provisioning
		slog.Info("Re-provisioning destroyed/failed cluster", "name", req.StackName, "oldStatus", existingCluster.Status)
		clusterID = existingCluster.ID
		if err := h.store.ResetCluster(ctx, clusterID, region, "provisioning"); err != nil {
			slog.Error("Failed to reset cluster record", "error", err)
			return c.String(http.StatusInternalServerError, "Database error: failed to reset cluster record")
		}
	} else {
		clusterRecord := &models.Cluster{
			ID:        clusterID,
			OrgID:     org.ID,
			Name:      req.StackName,
			Region:    region,
			Status:    "provisioning",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := h.store.CreateCluster(ctx, clusterRecord); err != nil {
			slog.Error("Failed to create cluster record", "error", err)
			return c.String(http.StatusInternalServerError, "Database error: failed to create cluster record")
		}
	}

	configMap := map[string]string{
		"location":       region,
		"azure:location": region,
	}
	if req.VMSize != "" {
		configMap["vmSize"] = req.VMSize
	}
	if req.SubscriptionID != "" {
		configMap["azure:subscriptionId"] = req.SubscriptionID
	}

	jobID := uuid.NewString()
	payload := tasks.ProvisionClusterPayload{
		JobID:          jobID,
		ClusterID:      clusterID.String(),
		OrganizationID: org.ID.String(),
		StackName:      req.StackName,
		InfraDir:       infraDir,
		SSHKeyPath:     sshKeyPath,
		Config:         configMap,
	}

	task, err := tasks.NewProvisionClusterTask(payload)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to create provision task")
	}

	taskID := "provision:" + clusterID.String()
	if _, err := h.queueClient.EnqueueProvision(task, taskID); err != nil {
		slog.Error("Failed to enqueue provision task", "error", err)
		return c.String(http.StatusInternalServerError, "Failed to enqueue provision task")
	}

	return c.JSON(http.StatusAccepted, ProvisionResponse{
		JobID:     jobID,
		TaskID:    taskID,
		ClusterID: clusterID.String(),
		Status:    "queued",
	})
}

type DestroyRequest struct {
	StackName string `json:"stackName"`
}

func (h *ProvisionHandler) HandleDestroy(c echo.Context) error {
	var req DestroyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}
	if req.StackName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Stack name is required"})
	}

	cwd, _ := os.Getwd()
	infraDir := filepath.Join(cwd, "infra", "azure")

	ctx := c.Request().Context()
	clusterID := ""
	if cluster, err := h.store.GetClusterByName(ctx, req.StackName); err == nil {
		// Only allow destroying clusters that are active or failed
		switch cluster.Status {
		case "active", "failed":
			clusterID = cluster.ID.String()
		case "destroyed":
			return c.JSON(http.StatusConflict, map[string]string{"error": "cluster is already destroyed"})
		case "provisioning", "installing":
			return c.JSON(http.StatusConflict, map[string]string{"error": "cluster is still being provisioned, cannot destroy yet"})
		default:
			clusterID = cluster.ID.String()
		}
	}

	jobID := uuid.NewString()
	payload := tasks.DestroyClusterPayload{
		JobID:     jobID,
		ClusterID: clusterID,
		StackName: req.StackName,
		InfraDir:  infraDir,
	}
	task, err := tasks.NewDestroyClusterTask(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create destroy task"})
	}

	taskID := "destroy:" + req.StackName
	if _, err := h.queueClient.EnqueueDestroy(task, taskID); err != nil {
		slog.Error("Failed to enqueue destroy task", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to enqueue destroy task"})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"status": "queued",
		"stack":  req.StackName,
		"jobId":  jobID,
		"taskId": taskID,
	})
}

func (h *ProvisionHandler) ensureDefaultOrganization(ctx context.Context) (*models.Organization, error) {
	orgName := "default"
	org, err := h.store.GetOrganizationByName(ctx, orgName)
	if err == nil {
		return org, nil
	}

	newOrg := &models.Organization{
		ID:        uuid.New(),
		Name:      orgName,
		CreatedAt: time.Now(),
	}
	if err := h.store.CreateOrganization(ctx, newOrg); err != nil {
		if existing, lookupErr := h.store.GetOrganizationByName(ctx, orgName); lookupErr == nil {
			return existing, nil
		}
		return nil, err
	}

	return newOrg, nil
}

func (h *ProvisionHandler) HandleListClusters(c echo.Context) error {
	ctx := c.Request().Context()
	org, err := h.ensureDefaultOrganization(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to ensure default organization"})
	}

	clusters, err := h.store.ListClusters(ctx, org.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list clusters"})
	}

	return c.JSON(http.StatusOK, clusters)
}
