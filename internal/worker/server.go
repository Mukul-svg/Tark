package worker

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"simplek8/internal/cache"
	"simplek8/internal/crypto"
	"simplek8/internal/kube"
	"simplek8/internal/models"
	"simplek8/internal/queue"
	"simplek8/internal/store"
	"simplek8/internal/worker/tasks"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	store  store.Store
	rdb    *redis.Client
	cipher *crypto.Cipher // nil when KUBECONFIG_ENCRYPTION_KEY is unset (dev mode)
}

func NewServer(redisAddr, redisPassword string, concurrency int, st store.Store, c *crypto.Cipher) *Server {
	if concurrency <= 0 {
		concurrency = 10
	}

	redisOpt := asynq.RedisClientOpt{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       0,
	}

	asynqServer := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			queue.QueueCritical:       10,
			queue.QueueInfraProvision: 6,
			queue.QueueModelDeploy:    6,
			queue.QueueCleanup:        4,
		},
	})

	workerServer := &Server{
		server: asynqServer,
		mux:    asynq.NewServeMux(),
		store:  st,
		cipher: c,
		rdb: redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       0,
			Protocol: 3,
		}),
	}
	workerServer.registerHandlers()

	return workerServer
}

func (s *Server) Run(ctx context.Context) error {
	defer func() {
		if s.rdb != nil {
			_ = s.rdb.Close()
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.Run(s.mux)
	}()

	select {
	case <-ctx.Done():
		s.server.Shutdown()
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("asynq server failed: %w", err)
		}
		return nil
	}
}

func (s *Server) registerHandlers() {
	s.mux.HandleFunc(queue.TaskTypeProvisionCluster, s.handleProvisionClusterTask)
	s.mux.HandleFunc(queue.TaskTypeDeployModel, s.handleDeployModelTask)
	s.mux.HandleFunc(queue.TaskTypeDeleteModel, s.handleDeleteModelTask)
	s.mux.HandleFunc(queue.TaskTypeDestroyCluster, s.handleDestroyClusterTask)
}

func (s *Server) failJob(ctx context.Context, jobID string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	_ = s.store.UpdateJobStatus(ctx, jobID, models.JobStatusFailed, msg)
}

func (s *Server) completeJob(ctx context.Context, jobID string) {
	_ = s.store.UpdateJobStatus(ctx, jobID, models.JobStatusCompleted, "")
}

func (s *Server) invalidateModelCache(ctx context.Context, model string) {
	if s.rdb == nil || strings.TrimSpace(model) == "" {
		return
	}

	if err := s.rdb.Del(ctx, cache.ModelTargetsKey(model), cache.ModelRoundRobinKey(model)).Err(); err != nil {
		slog.Warn("failed to invalidate model cache from worker", "model", model, "error", err)
	}
}

func (s *Server) handleProvisionClusterTask(ctx context.Context, task *asynq.Task) error {
	payload, err := tasks.ParseProvisionClusterPayload(task)
	if err != nil {
		return fmt.Errorf("parse provision task: %w", err)
	}

	clusterID, err := uuid.Parse(payload.ClusterID)
	if err != nil {
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("invalid cluster id in task payload: %w", err)
	}

	if err := s.store.UpdateClusterStatus(ctx, clusterID, models.ClusterStatusInstalling); err != nil {
		slog.Error("failed to set cluster status to installing", "clusterId", clusterID, "error", err)
	}

	_ = s.store.UpdateJobStatus(ctx, payload.JobID, models.JobStatusRunning, "")

	clusterData, err := ProvisionCluster(ctx, payload.StackName, payload.InfraDir, payload.Config)
	if err != nil {
		_ = s.store.UpdateClusterStatus(ctx, clusterID, models.ClusterStatusFailed)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("provision cluster: %w", err)
	}

	configBytes, err := fetchKubeconfigWithRetry(clusterData.PublicIP, payload.SSHKeyPath)
	if err != nil {
		_ = s.store.UpdateClusterStatus(ctx, clusterID, models.ClusterStatusFailed)
		s.failJob(ctx, payload.JobID, err)
		return err
	}

	re := regexp.MustCompile(`server: https://.*:16443`)
	newServerLine := fmt.Sprintf("server: https://%s:16443", clusterData.PublicIP)
	configBytes = re.ReplaceAll(configBytes, []byte(newServerLine))

	kubeconfigStr := string(configBytes)
	if s.cipher != nil {
		encrypted, encErr := s.cipher.Encrypt(configBytes)
		if encErr != nil {
			_ = s.store.UpdateClusterStatus(ctx, clusterID, models.ClusterStatusFailed)
			s.failJob(ctx, payload.JobID, encErr)
			return fmt.Errorf("encrypt kubeconfig: %w", encErr)
		}
		kubeconfigStr = encrypted
	}

	if err := s.store.UpdateClusterDetails(ctx, clusterID, models.ClusterStatusActive, kubeconfigStr, clusterData.PublicIP); err != nil {
		_ = s.store.UpdateClusterStatus(ctx, clusterID, models.ClusterStatusFailed)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("update cluster details: %w", err)
	}

	s.completeJob(ctx, payload.JobID)

	slog.Info("provision task completed", "jobId", payload.JobID, "clusterId", clusterID, "publicIp", clusterData.PublicIP)
	return nil
}

// decryptKubeconfig returns the raw kubeconfig bytes from whatever is stored in the DB.
// If a cipher is configured it decrypts the stored value; otherwise it returns the raw bytes as-is.
func (s *Server) decryptKubeconfig(stored string) ([]byte, error) {
	if s.cipher != nil {
		return s.cipher.Decrypt(stored)
	}
	return []byte(stored), nil
}

func (s *Server) handleDeployModelTask(ctx context.Context, task *asynq.Task) error {
	payload, err := tasks.ParseDeployModelPayload(task)
	if err != nil {
		return fmt.Errorf("parse deploy task: %w", err)
	}

	s.invalidateModelCache(ctx, payload.Name)

	deploymentID, err := uuid.Parse(payload.DeploymentID)
	if err != nil {
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("invalid deployment id in task payload: %w", err)
	}
	clusterID, err := uuid.Parse(payload.ClusterID)
	if err != nil {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, "failed")
		s.invalidateModelCache(ctx, payload.Name)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("invalid cluster id in task payload: %w", err)
	}

	if err := s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusBuilding); err != nil {
		slog.Error("failed to set deployment status to building", "deploymentId", deploymentID, "error", err)
	}

	_ = s.store.UpdateJobStatus(ctx, payload.JobID, models.JobStatusRunning, "")

	cluster, err := s.store.GetCluster(ctx, clusterID)
	if err != nil {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusFailed)
		s.invalidateModelCache(ctx, payload.Name)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("get cluster: %w", err)
	}

	kubeconfig, err := s.decryptKubeconfig(cluster.Kubeconfig)
	if err != nil {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusFailed)
		s.invalidateModelCache(ctx, payload.Name)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("decrypt kubeconfig: %w", err)
	}

	kubeClient, err := kube.NewFromKubeConfig(kubeconfig)
	if err != nil {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusFailed)
		s.invalidateModelCache(ctx, payload.Name)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("build kube client from cluster kubeconfig: %w", err)
	}

	err = kubeClient.DeployModel(ctx, payload.Namespace, kube.ModelConfig{
		Name:     payload.Name,
		ModelURL: payload.ModelURL,
		NodePort: payload.NodePort,
	})
	if err != nil {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusFailed)
		s.invalidateModelCache(ctx, payload.Name)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("deploy model: %w", err)
	}

	// Manifests applied — now waiting for pods to become ready.
	if err := s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusDeploying); err != nil {
		slog.Error("failed to set deployment status to deploying", "deploymentId", deploymentID, "error", err)
	}

	serviceURL := fmt.Sprintf("http://%s", net.JoinHostPort(cluster.PublicIP, fmt.Sprintf("%d", payload.NodePort)))

	slog.Info("waiting for deployment to become healthy", "deploymentId", deploymentID, "serviceURL", serviceURL)
	targetHealthy := false
	hostPort := net.JoinHostPort(cluster.PublicIP, fmt.Sprintf("%d", payload.NodePort))
	for range 90 { // 90 attempts * 10s = 15 minutes
		conn, dialErr := net.DialTimeout("tcp", hostPort, 2*time.Second)
		if dialErr == nil {
			_ = conn.Close()
			targetHealthy = true
			break
		}
		select {
		case <-ctx.Done():
			_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusFailed)
			s.invalidateModelCache(ctx, payload.Name)
			s.failJob(ctx, payload.JobID, fmt.Errorf("deployment cancelled while waiting for health"))
			return fmt.Errorf("deployment cancelled while waiting for health: %w", ctx.Err())
		case <-time.After(10 * time.Second):
		}
	}

	if !targetHealthy {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusFailed)
		s.invalidateModelCache(ctx, payload.Name)
		s.failJob(ctx, payload.JobID, fmt.Errorf("deployment timed out waiting to become healthy"))
		return fmt.Errorf("deployment timed out waiting to become healthy: %s", serviceURL)
	}

	if err := s.store.UpdateDeploymentServiceURL(ctx, deploymentID, models.DeploymentStatusActive, serviceURL); err != nil {
		s.invalidateModelCache(ctx, payload.Name)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("mark deployment active: %w", err)
	}

	s.completeJob(ctx, payload.JobID)
	s.invalidateModelCache(ctx, payload.Name)

	slog.Info("deploy task completed", "jobId", payload.JobID, "deploymentId", deploymentID, "serviceURL", serviceURL)
	return nil
}

func (s *Server) handleDestroyClusterTask(ctx context.Context, task *asynq.Task) error {
	payload, err := tasks.ParseDestroyClusterPayload(task)
	if err != nil {
		return fmt.Errorf("parse destroy task: %w", err)
	}

	_ = s.store.UpdateJobStatus(ctx, payload.JobID, models.JobStatusRunning, "")

	if payload.ClusterID != "" {
		if clusterID, parseErr := uuid.Parse(payload.ClusterID); parseErr == nil {
			_ = s.store.UpdateClusterStatus(ctx, clusterID, models.ClusterStatusDestroying)
		}
	}

	if err := DestroyCluster(ctx, payload.StackName, payload.InfraDir); err != nil {
		if payload.ClusterID != "" {
			if clusterID, parseErr := uuid.Parse(payload.ClusterID); parseErr == nil {
				_ = s.store.UpdateClusterStatus(ctx, clusterID, models.ClusterStatusFailed)
			}
		}
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("destroy cluster: %w", err)
	}

	if payload.ClusterID != "" {
		if clusterID, parseErr := uuid.Parse(payload.ClusterID); parseErr == nil {
			if err := s.store.UpdateClusterDetails(ctx, clusterID, models.ClusterStatusDestroyed, "", ""); err != nil {
				s.failJob(ctx, payload.JobID, err)
				return fmt.Errorf("update destroyed status: %w", err)
			}

			// Cascade the destruction event to any running model deployments on this cluster.
			if err := s.store.UpdateDeploymentsStatusByCluster(ctx, clusterID, models.DeploymentStatusOrphaned); err != nil {
				slog.Error("failed to mark deployments as orphaned during cluster destroy", "clusterId", clusterID, "error", err)
			}
		}
	}

	s.completeJob(ctx, payload.JobID)

	slog.Info("destroy task completed", "jobId", payload.JobID, "stackName", payload.StackName)
	return nil
}

func fetchKubeconfigWithRetry(publicIP string, sshKeyPath string) ([]byte, error) {
	var configBytes []byte
	var err error
	maxRetries := 30
	for i := range maxRetries {
		slog.Info("worker fetching kubeconfig", "attempt", i+1, "publicIp", publicIP)
		configBytes, err = FetchRemoteKubeConfig(publicIP, sshKeyPath)
		if err == nil {
			return configBytes, nil
		}
		if i > 25 {
			slog.Error("worker failed to fetch kubeconfig", "error", err)
		}
		time.Sleep(10 * time.Second)
	}
	return nil, fmt.Errorf("failed to fetch kubeconfig after retries: %w", err)
}

func (s *Server) handleDeleteModelTask(ctx context.Context, task *asynq.Task) error {
	payload, err := tasks.ParseDeleteModelPayload(task)
	if err != nil {
		return fmt.Errorf("parse delete task: %w", err)
	}

	deploymentID, err := uuid.Parse(payload.DeploymentID)
	if err != nil {
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("invalid deployment id: %w", err)
	}
	clusterID, err := uuid.Parse(payload.ClusterID)
	if err != nil {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, "failed")
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("invalid cluster id: %w", err)
	}

	if err := s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusDeleting); err != nil {
		slog.Error("failed to update state", "error", err)
	}
	s.invalidateModelCache(ctx, payload.Name)

	_ = s.store.UpdateJobStatus(ctx, payload.JobID, models.JobStatusRunning, "")

	cluster, err := s.store.GetCluster(ctx, clusterID)
	if err != nil {
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("get cluster: %w", err)
	}

	kubeconfig, err := s.decryptKubeconfig(cluster.Kubeconfig)
	if err != nil {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusFailed)
		s.invalidateModelCache(ctx, payload.Name)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("decrypt kubeconfig: %w", err)
	}

	kubeClient, err := kube.NewFromKubeConfig(kubeconfig)
	if err != nil {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusFailed)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("kube client error: %w", err)
	}

	if err := kubeClient.DeleteModel(ctx, payload.Namespace, payload.Name); err != nil {
		_ = s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusFailed)
		s.failJob(ctx, payload.JobID, err)
		return fmt.Errorf("delete model: %w", err)
	}

	if err := s.store.UpdateDeploymentStatus(ctx, deploymentID, models.DeploymentStatusDeleted); err != nil {
		return fmt.Errorf("mark deployment deleted: %w", err)
	}

	s.completeJob(ctx, payload.JobID)

	slog.Info("delete task completed", "deploymentId", deploymentID)
	return nil
}
