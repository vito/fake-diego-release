package main

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry/yagnats"

	"logger"
	"runtime-schema/bbs"
	"runtime-schema/models"
)

type Handler struct {
	bbs        bbs.StagerBBS
	natsClient yagnats.NATSClient
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	var task *models.Task

	err := json.NewDecoder(request.Body).Decode(&task)
	if err != nil {
		logger.Info("handler.malformed-payload", map[string]interface{}{
			"error": err.Error(),
		})

		writer.WriteHeader(http.StatusBadRequest)

		return
	}

	logger.Info("handler.resolving", map[string]interface{}{
		"task": task.Guid,
	})

	go handler.resolveTask(task)

	writer.WriteHeader(http.StatusOK)
}

func (handler *Handler) resolveTask(task *models.Task) {
	err := handler.bbs.ResolvingTask(task)
	if err != nil {
		logger.Info("handler.resolving-failed", map[string]interface{}{
			"task":  task.Guid,
			"error": err.Error(),
		})

		return
	}

	err = handler.natsClient.Publish(task.ReplyTo, task.ToJSON())
	if err != nil {
		logger.Error("handler.publish-failed", map[string]interface{}{
			"task":  task.Guid,
			"error": err.Error(),
		})

		return
	}

	err = handler.bbs.ResolveTask(task)
	if err != nil {
		logger.Error("handler.resolve-failed", map[string]interface{}{
			"task":  task.Guid,
			"error": err.Error(),
		})

		return
	}

	logger.Info("handler.resolved", map[string]interface{}{
		"task": task.Guid,
	})
}
