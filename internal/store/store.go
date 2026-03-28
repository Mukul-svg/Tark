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
	UpdateClusterStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdateClusterDetails(ctx context.Context, id uuid.UUID, status string, kubeconfig string, publicIP string) error

	// Deployment methods
	CreateDeployment(ctx context.Context, deployment *models.Deployment) error
	GetDeployment(ctx context.Context, id uuid.UUID) (*models.Deployment, error)
	UpdateDeploymentStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdateDeploymentServiceURL(ctx context.Context, id uuid.UUID, status string, serviceURL string) error
	ListActiveDeploymentTargets(ctx context.Context, modelName string) ([]string, error)

	// General
	Close()
}
