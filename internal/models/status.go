package models

// ClusterStatus represents the lifecycle state of a provisioned cluster.
type ClusterStatus string

const (
	ClusterStatusProvisioning ClusterStatus = "provisioning"
	ClusterStatusInstalling   ClusterStatus = "installing"
	ClusterStatusActive       ClusterStatus = "active"
	ClusterStatusFailed       ClusterStatus = "failed"
	ClusterStatusDestroying   ClusterStatus = "destroying"
	ClusterStatusDestroyed    ClusterStatus = "destroyed"
)

// DeploymentStatus represents the lifecycle state of a model deployment.
type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusBuilding  DeploymentStatus = "building"
	DeploymentStatusDeploying DeploymentStatus = "deploying"
	DeploymentStatusActive    DeploymentStatus = "active"
	DeploymentStatusFailed    DeploymentStatus = "failed"
	DeploymentStatusDeleting  DeploymentStatus = "deleting"
	DeploymentStatusDeleted   DeploymentStatus = "deleted"
	DeploymentStatusOrphaned  DeploymentStatus = "orphaned"
)

// JobStatus represents the state of an async background job.
type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)
