package handlers

import (
	"net/http"
	"simplek8/internal/worker"

	"github.com/labstack/echo/v4"
)

type JobsHandler struct {
	queueClient *worker.Client
}

func NewJobsHandler(queueClient *worker.Client) *JobsHandler {
	return &JobsHandler{queueClient: queueClient}
}

func (h *JobsHandler) GetJobStatus(c echo.Context) error {
	taskID := c.Param("id")
	if taskID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "job id is required"})
	}

	taskInfo, err := h.queueClient.GetTaskInfo(taskID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, taskInfo)
}
