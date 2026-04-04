package handlers

import (
	"net/http"
	"simplek8/internal/apierror"
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
		return apierror.Respond(c, apierror.BadRequest(apierror.InvalidRequestField, "job id is required"))
	}

	taskInfo, err := h.queueClient.GetTaskInfo(taskID)
	if err != nil {
		return apierror.Respond(c, apierror.NotFoundCode(apierror.NotFound, "job not found"))
	}

	return c.JSON(http.StatusOK, taskInfo)
}
