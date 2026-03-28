package worker

import (
	"fmt"
	"simplek8/internal/queue"

	"github.com/hibiken/asynq"
)

type Client struct {
	client    *asynq.Client
	inspector *asynq.Inspector
}

func NewClient(redisAddr, redisPassword string) *Client {
	redisOpt := asynq.RedisClientOpt{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       0,
	}
	return &Client{
		client:    asynq.NewClient(redisOpt),
		inspector: asynq.NewInspector(redisOpt),
	}
}

func (c *Client) Close() error {
	if err := c.client.Close(); err != nil {
		return err
	}
	return c.inspector.Close()
}

func (c *Client) EnqueueProvision(task *asynq.Task, taskID string) (*asynq.TaskInfo, error) {
	info, err := c.client.Enqueue(task,
		asynq.Queue(queue.QueueInfraProvision),
		asynq.TaskID(taskID),
		asynq.MaxRetry(10),
	)
	if err != nil {
		return nil, fmt.Errorf("enqueue provision task: %w", err)
	}
	return info, nil
}

func (c *Client) EnqueueDeploy(task *asynq.Task, taskID string) (*asynq.TaskInfo, error) {
	info, err := c.client.Enqueue(task,
		asynq.Queue(queue.QueueModelDeploy),
		asynq.TaskID(taskID),
		asynq.MaxRetry(10),
	)
	if err != nil {
		return nil, fmt.Errorf("enqueue deploy task: %w", err)
	}
	return info, nil
}

func (c *Client) EnqueueDestroy(task *asynq.Task, taskID string) (*asynq.TaskInfo, error) {
	info, err := c.client.Enqueue(task,
		asynq.Queue(queue.QueueCleanup),
		asynq.TaskID(taskID),
		asynq.MaxRetry(10),
	)
	if err != nil {
		return nil, fmt.Errorf("enqueue destroy task: %w", err)
	}
	return info, nil
}

func (c *Client) GetTaskInfo(taskID string) (*asynq.TaskInfo, error) {
	queues := []string{
		queue.QueueInfraProvision,
		queue.QueueModelDeploy,
		queue.QueueCleanup,
		queue.QueueCritical,
	}

	for _, queueName := range queues {
		info, err := c.inspector.GetTaskInfo(queueName, taskID)
		if err == nil {
			return info, nil
		}
	}

	return nil, fmt.Errorf("task not found: %s", taskID)
}
