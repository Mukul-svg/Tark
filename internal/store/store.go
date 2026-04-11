package store

import (
	"context"
	"simplek8/internal/models"

	"github.com/google/uuid"
)

type Store interface {
	// Organization methods
	CreateOrganization(ctx context.Context, org *models.Organization) error
	GetOrganization(ctx context.Context, id uuid.UUID) (*models.Organization, error)
	GetOrganizationByName(ctx context.Context, name string) (*models.Organization, error)

	// Cluster methods
	CreateCluster(ctx context.Context, cluster *models.Cluster) error
	GetCluster(ctx context.Context, id uuid.UUID) (*models.Cluster, error)
	GetClusterByName(ctx context.Context, name string) (*models.Cluster, error)
	ListClusters(ctx context.Context, orgID uuid.UUID) ([]models.Cluster, error)
	UpdateClusterStatus(ctx context.Context, id uuid.UUID, status models.ClusterStatus) error
	UpdateClusterDetails(ctx context.Context, id uuid.UUID, status models.ClusterStatus, kubeconfig string, publicIP string) error
	ResetCluster(ctx context.Context, id uuid.UUID, region string, status models.ClusterStatus) error

	// Deployment methods
	CreateDeployment(ctx context.Context, deployment *models.Deployment) error
	GetDeployment(ctx context.Context, id uuid.UUID) (*models.Deployment, error)
	ListDeployments(ctx context.Context) ([]models.Deployment, error)
	UpdateDeploymentStatus(ctx context.Context, id uuid.UUID, status models.DeploymentStatus) error
	UpdateDeploymentServiceURL(ctx context.Context, id uuid.UUID, status models.DeploymentStatus, serviceURL string) error
	UpdateDeploymentsStatusByCluster(ctx context.Context, clusterID uuid.UUID, status models.DeploymentStatus) error
	ListActiveDeploymentTargets(ctx context.Context, modelName string) ([]string, error)

	// Job methods
	CreateJob(ctx context.Context, job *models.Job) error
	GetJob(ctx context.Context, jobID string) (*models.Job, error)
	ListJobs(ctx context.Context) ([]models.Job, error)
	UpdateJobStatus(ctx context.Context, jobID string, status models.JobStatus, errorMsg string) error

	// General
	Ping(ctx context.Context) error
	Close()
}
