package tasks

import (
	"encoding/json"
	"fmt"
	"simplek8/internal/queue"

	"github.com/hibiken/asynq"
)

type ProvisionClusterPayload struct {
	JobID          string            `json:"jobId"`
	ClusterID      string            `json:"clusterId"`
	OrganizationID string            `json:"organizationId"`
	StackName      string            `json:"stackName"`
	InfraDir       string            `json:"infraDir"`
	SSHKeyPath     string            `json:"sshKeyPath"`
	Config         map[string]string `json:"config"`
}

type DeployModelPayload struct {
	JobID        string `json:"jobId"`
	DeploymentID string `json:"deploymentId"`
	ClusterID    string `json:"clusterId"`
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	ModelURL     string `json:"modelUrl"`
	NodePort     int32  `json:"nodePort"`
}

type DeleteModelPayload struct {
	JobID        string `json:"jobId"`
	DeploymentID string `json:"deploymentId"`
	ClusterID    string `json:"clusterId"`
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
}

type DestroyClusterPayload struct {
	JobID     string `json:"jobId"`
	ClusterID string `json:"clusterId"`
	StackName string `json:"stackName"`
	InfraDir  string `json:"infraDir"`
}

func NewProvisionClusterTask(payload ProvisionClusterPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal provision payload: %w", err)
	}
	return asynq.NewTask(queue.TaskTypeProvisionCluster, data), nil
}

func NewDeployModelTask(payload DeployModelPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal deploy payload: %w", err)
	}
	return asynq.NewTask(queue.TaskTypeDeployModel, data), nil
}

func NewDeleteModelTask(payload DeleteModelPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal delete model payload: %w", err)
	}
	return asynq.NewTask(queue.TaskTypeDeleteModel, data), nil
}

func NewDestroyClusterTask(payload DestroyClusterPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal destroy payload: %w", err)
	}
	return asynq.NewTask(queue.TaskTypeDestroyCluster, data), nil
}

func ParseProvisionClusterPayload(task *asynq.Task) (*ProvisionClusterPayload, error) {
	var payload ProvisionClusterPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal provision payload: %w", err)
	}
	return &payload, nil
}

func ParseDeployModelPayload(task *asynq.Task) (*DeployModelPayload, error) {
	var payload DeployModelPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal deploy payload: %w", err)
	}
	return &payload, nil
}

func ParseDestroyClusterPayload(task *asynq.Task) (*DestroyClusterPayload, error) {
	var payload DestroyClusterPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal destroy payload: %w", err)
	}
	return &payload, nil
}

func ParseDeleteModelPayload(task *asynq.Task) (*DeleteModelPayload, error) {
	var payload DeleteModelPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal delete payload: %w", err)
	}
	return &payload, nil
}
