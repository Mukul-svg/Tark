package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"simplek8/internal/apierror"
	"simplek8/internal/models"
	"simplek8/internal/queue"
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
		slog.Error("failed to parse provision request", "error", err)
		return apierror.Respond(c, apierror.BadRequest(apierror.InvalidRequestBody, "invalid request payload"))
	}
	if req.StackName == "" {
		return apierror.Respond(c, apierror.BadRequest(apierror.InvalidRequestField, "stackName is required"))
	}

	infraDir, sshKeyPath := getInfraDir()

	ctx := c.Request().Context()
	org, err := h.ensureDefaultOrganization(ctx)
	if err != nil {
		slog.Error("failed to ensure default organization", "error", err)
		return apierror.Respond(c, apierror.Internal(apierror.StoreError, "failed to ensure organization", err))
	}

	region := req.Region
	if region == "" {
		region = "southindia"
	}

	clusterID := uuid.New()

	if existingCluster, lookupErr := h.store.GetClusterByName(ctx, req.StackName); lookupErr == nil && existingCluster != nil {
		if existingCluster.Status != "destroyed" && existingCluster.Status != "failed" {
			slog.Warn("cluster already exists and is not in a re-provisionable state", "name", req.StackName, "status", existingCluster.Status)
			return c.JSON(http.StatusConflict, map[string]any{
				"code":      "ALREADY_EXISTS",
				"message":   "cluster with this stack name already exists",
				"clusterId": existingCluster.ID.String(),
				"status":    existingCluster.Status,
			})
		}

		slog.Info("re-provisioning destroyed/failed cluster", "name", req.StackName, "oldStatus", existingCluster.Status)
		clusterID = existingCluster.ID
		if err := h.store.ResetCluster(ctx, clusterID, region, "provisioning"); err != nil {
			slog.Error("failed to reset cluster record", "error", err)
			return apierror.Respond(c, apierror.Internal(apierror.StoreError, "failed to reset cluster record", err))
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
			slog.Error("failed to create cluster record", "error", err)
			return apierror.Respond(c, apierror.Internal(apierror.StoreError, "failed to create cluster record", err))
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

	CreateJobRecord(ctx, h.store, jobID, queue.TaskTypeProvisionCluster, payload, &payload.ClusterID, nil)

	task, err := tasks.NewProvisionClusterTask(payload)
	if err != nil {
		return apierror.Respond(c, apierror.Internal(apierror.QueueError, "failed to create provision task", err))
	}

	taskID := "provision:" + clusterID.String()
	if _, err := h.queueClient.EnqueueProvision(task, taskID); err != nil {
		slog.Error("failed to enqueue provision task", "error", err)
		return apierror.Respond(c, apierror.Internal(apierror.QueueError, "failed to enqueue provision task", err))
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
		return apierror.Respond(c, apierror.BadRequest(apierror.InvalidRequestBody, "invalid request payload"))
	}
	if req.StackName == "" {
		return apierror.Respond(c, apierror.BadRequest(apierror.InvalidRequestField, "stackName is required"))
	}

	infraDir, _ := getInfraDir()

	ctx := c.Request().Context()
	var clusterIDStr string
	if cluster, err := h.store.GetClusterByName(ctx, req.StackName); err == nil {
		switch cluster.Status {
		case "active", "failed":
			clusterIDStr = cluster.ID.String()
		case "destroyed":
			return apierror.Respond(c, apierror.ConflictErr(apierror.Conflict, "cluster is already destroyed"))
		case "provisioning", "installing":
			return apierror.Respond(c, apierror.ConflictErr(apierror.Conflict, "cluster is still being provisioned, cannot destroy yet"))
		default:
			clusterIDStr = cluster.ID.String()
		}
	}

	jobID := uuid.NewString()
	payload := tasks.DestroyClusterPayload{
		JobID:     jobID,
		ClusterID: clusterIDStr,
		StackName: req.StackName,
		InfraDir:  infraDir,
	}

	CreateJobRecord(ctx, h.store, jobID, queue.TaskTypeDestroyCluster, payload, &clusterIDStr, nil)

	task, err := tasks.NewDestroyClusterTask(payload)
	if err != nil {
		return apierror.Respond(c, apierror.Internal(apierror.QueueError, "failed to create destroy task", err))
	}

	taskID := "destroy:" + req.StackName
	if _, err := h.queueClient.EnqueueDestroy(task, taskID); err != nil {
		slog.Error("failed to enqueue destroy task", "error", err)
		return apierror.Respond(c, apierror.Internal(apierror.QueueError, "failed to enqueue destroy task", err))
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"jobId":     jobID,
		"taskId":    taskID,
		"stackName": req.StackName,
		"status":    "queued",
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
		return apierror.Respond(c, apierror.Internal(apierror.StoreError, "failed to ensure organization", err))
	}

	clusters, err := h.store.ListClusters(ctx, org.ID)
	if err != nil {
		return apierror.Respond(c, apierror.Internal(apierror.StoreError, "failed to list clusters", err))
	}

	return c.JSON(http.StatusOK, clusters)
}

func getInfraDir() (infraDir, sshKeyPath string) {
	cwd, _ := os.Getwd()
	base := filepath.Join(cwd, "infra", "azure")
	return base, filepath.Join(base, "azure_rsa")
}
