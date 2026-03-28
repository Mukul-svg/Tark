package queue

const (
	QueueCritical       = "critical"
	QueueInfraProvision = "infra-provision"
	QueueModelDeploy    = "model-deploy"
	QueueCleanup        = "cleanup"
)

const (
	TaskTypeProvisionCluster = "cluster:provision"
	TaskTypeDeployModel      = "model:deploy"
	TaskTypeDestroyCluster   = "cluster:destroy"
)
