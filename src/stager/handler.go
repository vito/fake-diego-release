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

	err = handler.bbs.ResolvingTask(task)
	if err != nil {
		logger.Error("handler.resolving-failed", map[string]interface{}{
			"task":  task.Guid,
			"error": err.Error(),
		})

		writer.WriteHeader(http.StatusInternalServerError)

		return
	}

	err = handler.natsClient.Publish(task.ReplyTo, task.ToJSON())
	if err != nil {
		logger.Error("handler.publish-failed", map[string]interface{}{
			"task":  task.Guid,
			"error": err.Error(),
		})

		writer.WriteHeader(http.StatusInternalServerError)

		return
	}

	err = handler.bbs.ResolveTask(task)
	if err != nil {
		logger.Error("handler.resolve-failed", map[string]interface{}{
			"task":  task.Guid,
			"error": err.Error(),
		})

		writer.WriteHeader(http.StatusInternalServerError)

		return
	}

	logger.Error("handler.resolved", map[string]interface{}{
		"task": task.Guid,
	})

	writer.WriteHeader(http.StatusOK)
}
