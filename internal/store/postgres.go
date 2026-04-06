package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"simplek8/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

//go:embed migrations/*.sql
var embedMigrations embed.FS

func NewPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	slog.Info("running migrations")
	if err := RunEmbeddedMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

// --- Organization Methods ---

func (s *PostgresStore) CreateOrganization(ctx context.Context, org *models.Organization) error {
	query := `
		INSERT INTO organizations (id, name, created_at)
		VALUES ($1, $2, $3)
	`
	_, err := s.pool.Exec(ctx, query, org.ID, org.Name, org.CreatedAt)
	return err
}

func (s *PostgresStore) GetOrganization(ctx context.Context, id uuid.UUID) (*models.Organization, error) {
	query := `SELECT id, name, created_at FROM organizations WHERE id = $1`
	var org models.Organization
	err := s.pool.QueryRow(ctx, query, id).Scan(&org.ID, &org.Name, &org.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, err
	}
	return &org, nil
}

func (s *PostgresStore) GetOrganizationByName(ctx context.Context, name string) (*models.Organization, error) {
	query := `SELECT id, name, created_at FROM organizations WHERE name = $1`
	var org models.Organization
	err := s.pool.QueryRow(ctx, query, name).Scan(&org.ID, &org.Name, &org.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, err
	}
	return &org, nil
}

// --- Cluster Methods ---

func (s *PostgresStore) CreateCluster(ctx context.Context, cluster *models.Cluster) error {
	query := `
		INSERT INTO clusters (id, org_id, name, region, status, kubeconfig, public_ip, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := s.pool.Exec(ctx, query,
		cluster.ID, cluster.OrgID, cluster.Name, cluster.Region, cluster.Status,
		cluster.Kubeconfig, cluster.PublicIP, cluster.CreatedAt, cluster.UpdatedAt,
	)
	return err
}

func (s *PostgresStore) GetCluster(ctx context.Context, id uuid.UUID) (*models.Cluster, error) {
	query := `
		SELECT id, org_id, name, region, status, kubeconfig, public_ip, created_at, updated_at
		FROM clusters WHERE id = $1
	`
	var c models.Cluster
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.OrgID, &c.Name, &c.Region, &c.Status,
		&c.Kubeconfig, &c.PublicIP, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("cluster not found")
		}
		return nil, err
	}
	return &c, nil
}

func (s *PostgresStore) GetClusterByName(ctx context.Context, name string) (*models.Cluster, error) {
	query := `
		SELECT id, org_id, name, region, status, kubeconfig, public_ip, created_at, updated_at
		FROM clusters WHERE name = $1
	`
	var c models.Cluster
	err := s.pool.QueryRow(ctx, query, name).Scan(
		&c.ID, &c.OrgID, &c.Name, &c.Region, &c.Status,
		&c.Kubeconfig, &c.PublicIP, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("cluster not found")
		}
		return nil, err
	}
	return &c, nil
}

func (s *PostgresStore) ListClusters(ctx context.Context, orgID uuid.UUID) ([]models.Cluster, error) {
	query := `
		SELECT id, org_id, name, region, status, public_ip, created_at, updated_at
		FROM clusters WHERE org_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clusters []models.Cluster
	for rows.Next() {
		var c models.Cluster
		err := rows.Scan(
			&c.ID, &c.OrgID, &c.Name, &c.Region, &c.Status,
			&c.PublicIP, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		clusters = append(clusters, c)
	}
	return clusters, nil
}

func (s *PostgresStore) UpdateClusterStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE clusters SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := s.pool.Exec(ctx, query, status, id)
	return err
}

func (s *PostgresStore) UpdateClusterDetails(ctx context.Context, id uuid.UUID, status string, kubeconfig string, publicIP string) error {
	query := `
		UPDATE clusters 
		SET status = $1, kubeconfig = $2, public_ip = $3, updated_at = NOW() 
		WHERE id = $4
	`
	_, err := s.pool.Exec(ctx, query, status, kubeconfig, publicIP, id)
	return err
}

func (s *PostgresStore) ResetCluster(ctx context.Context, id uuid.UUID, region string, status string) error {
	query := `
		UPDATE clusters 
		SET status = $1, region = $2, kubeconfig = '', public_ip = '', updated_at = NOW() 
		WHERE id = $3
	`
	_, err := s.pool.Exec(ctx, query, status, region, id)
	return err
}

// --- Deployment Methods ---

func (s *PostgresStore) CreateDeployment(ctx context.Context, deployment *models.Deployment) error {
	query := `
		INSERT INTO deployments (id, cluster_id, model_name, namespace, node_port, model_url, replicas, status, service_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := s.pool.Exec(ctx, query,
		deployment.ID, deployment.ClusterID, deployment.ModelName, deployment.Namespace,
		deployment.NodePort, deployment.ModelURL, deployment.Replicas,
		deployment.Status, deployment.ServiceURL, deployment.CreatedAt, deployment.UpdatedAt,
	)
	return err
}

func (s *PostgresStore) GetDeployment(ctx context.Context, id uuid.UUID) (*models.Deployment, error) {
	query := `
		SELECT id, cluster_id, model_name, namespace, node_port, model_url, replicas, status, service_url, created_at, updated_at
		FROM deployments WHERE id = $1
	`
	var d models.Deployment
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&d.ID, &d.ClusterID, &d.ModelName, &d.Namespace, &d.NodePort, &d.ModelURL,
		&d.Replicas, &d.Status, &d.ServiceURL, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("deployment not found")
		}
		return nil, err
	}
	return &d, nil
}

func (s *PostgresStore) ListDeployments(ctx context.Context) ([]models.Deployment, error) {
	query := `
		SELECT id, cluster_id, model_name, namespace, node_port, model_url, replicas, status, service_url, created_at, updated_at
		FROM deployments
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []models.Deployment
	for rows.Next() {
		var d models.Deployment
		err := rows.Scan(
			&d.ID, &d.ClusterID, &d.ModelName, &d.Namespace, &d.NodePort, &d.ModelURL,
			&d.Replicas, &d.Status, &d.ServiceURL, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, nil
}

func (s *PostgresStore) UpdateDeploymentStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE deployments SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := s.pool.Exec(ctx, query, status, id)
	return err
}

func (s *PostgresStore) UpdateDeploymentServiceURL(ctx context.Context, id uuid.UUID, status string, serviceURL string) error {
	query := `
		UPDATE deployments
		SET status = $1, service_url = $2, updated_at = NOW()
		WHERE id = $3
	`
	_, err := s.pool.Exec(ctx, query, status, serviceURL, id)
	return err
}

func (s *PostgresStore) UpdateDeploymentsStatusByCluster(ctx context.Context, clusterID uuid.UUID, status string) error {
	query := `
		UPDATE deployments
		SET status = $1, updated_at = NOW()
		WHERE cluster_id = $2 AND status != 'deleted' AND status != 'failed'
	`
	_, err := s.pool.Exec(ctx, query, status, clusterID)
	return err
}

func (s *PostgresStore) ListActiveDeploymentTargets(ctx context.Context, modelName string) ([]string, error) {
	query := `
		SELECT d.service_url
		FROM deployments d
		JOIN clusters c ON d.cluster_id = c.id
		WHERE d.model_name = $1
		  AND d.status = 'active'
		  AND c.status = 'active'
		  AND d.service_url IS NOT NULL
		  AND d.service_url <> ''
		ORDER BY d.updated_at DESC
	`

	rows, err := s.pool.Query(ctx, query, modelName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]string, 0)
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return targets, nil
}

// --- Job Methods ---

func (s *PostgresStore) CreateJob(ctx context.Context, job *models.Job) error {
	query := `
		INSERT INTO jobs (id, job_id, task_type, status, payload, cluster_id, deployment_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, query,
		job.ID, job.JobID, job.TaskType, job.Status, job.Payload,
		job.ClusterID, job.DeploymentID, job.CreatedAt, job.UpdatedAt,
	)
	return err
}

func (s *PostgresStore) GetJob(ctx context.Context, jobID string) (*models.Job, error) {
	query := `
		SELECT id, job_id, task_type, status, payload,
		       cluster_id, deployment_id, error,
		       created_at, updated_at, started_at, completed_at
		FROM jobs WHERE job_id = $1
	`
	var j models.Job
	var clusterID, deploymentID, errMsg sql.NullString
	var startedAt, completedAt sql.NullTime

	err := s.pool.QueryRow(ctx, query, jobID).Scan(
		&j.ID, &j.JobID, &j.TaskType, &j.Status, &j.Payload,
		&clusterID, &deploymentID, &errMsg,
		&j.CreatedAt, &j.UpdatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("job not found")
		}
		return nil, err
	}

	mapNullableFields(&j, clusterID, deploymentID, errMsg, startedAt, completedAt)
	return &j, nil
}

func (s *PostgresStore) ListJobs(ctx context.Context) ([]models.Job, error) {
	query := `
		SELECT id, job_id, task_type, status, payload,
		       cluster_id, deployment_id, error,
		       created_at, updated_at, started_at, completed_at
		FROM jobs
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var j models.Job
		var clusterID, deploymentID, errMsg sql.NullString
		var startedAt, completedAt sql.NullTime

		if err := rows.Scan(
			&j.ID, &j.JobID, &j.TaskType, &j.Status, &j.Payload,
			&clusterID, &deploymentID, &errMsg,
			&j.CreatedAt, &j.UpdatedAt, &startedAt, &completedAt,
		); err != nil {
			return nil, err
		}

		mapNullableFields(&j, clusterID, deploymentID, errMsg, startedAt, completedAt)
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func mapNullableFields(
	j *models.Job,
	clusterID, deploymentID, errMsg sql.NullString,
	startedAt, completedAt sql.NullTime,
) {
	if clusterID.Valid {
		j.ClusterID = &clusterID.String
	}
	if deploymentID.Valid {
		j.DeploymentID = &deploymentID.String
	}
	if errMsg.Valid {
		j.Error = &errMsg.String
	}
	if startedAt.Valid {
		j.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
}

func (s *PostgresStore) UpdateJobStatus(ctx context.Context, jobID string, status string, errMsg string) error {
	query := `
		UPDATE jobs
		SET status = $1, error = NULLIF($2, ''), updated_at = NOW(),
		    started_at = CASE WHEN $1 = 'running' AND started_at IS NULL THEN NOW() ELSE started_at END,
		    completed_at = CASE WHEN $1 IN ('completed', 'failed') THEN NOW() ELSE completed_at END
		WHERE job_id = $3
	`
	_, err := s.pool.Exec(ctx, query, status, errMsg, jobID)
	return err
}
