package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"logger"
	"runtime-schema/bbs"
	"runtime-schema/models"
)

type Handler struct {
	bbs bbs.ExecutorBBS

	currentMemory int
	memoryMutex   *sync.Mutex
}

var ErrAlreadyClaimed = errors.New("already claimed")
var ErrNoCapacity = errors.New("no capacity")

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	var task *models.Task

	err := json.NewDecoder(request.Body).Decode(&task)
	if err != nil {
		logger.Error("handler.malformed-payload", map[string]interface{}{
			"error": err.Error(),
		})

		writer.WriteHeader(http.StatusBadRequest)

		return
	}

	sleepForARandomInterval("handler.hesitate", 0, 100, map[string]interface{}{
		"task": task.Guid,
	})

	ok := handler.reserveMemory(task.MemoryMB)
	if !ok {
		logger.Info("handler.full", map[string]interface{}{
			"task": task.Guid,
		})

		writer.WriteHeader(http.StatusServiceUnavailable)

		return
	}

	logger.Info("claiming.runonce", map[string]interface{}{
		"task": task.Guid,
	})

	err = handler.bbs.ClaimTask(task, executorID)
	if err != nil {
		handler.releaseMemory(task.MemoryMB)

		logger.Info("handler.claim-failed", map[string]interface{}{
			"task":  task.Guid,
			"error": err.Error(),
		})

		writer.WriteHeader(http.StatusConflict)

		return
	}

	go handler.runTask(task)

	writer.WriteHeader(http.StatusCreated)
}

func (handler *Handler) runTask(task *models.Task) {
	defer handler.releaseMemory(task.MemoryMB)

	logger.Info("task.claimed", map[string]interface{}{
		"task": task.Guid,
	})

	sleepForARandomInterval("task.create-container", 500, 1000, map[string]interface{}{
		"task": task.Guid,
	})

	logger.Info("task.start", map[string]interface{}{
		"task": task.Guid,
	})

	err := handler.bbs.StartTask(task, "container")
	if err != nil {
		logger.Error("task.start-failed", map[string]interface{}{
			"task":  task.Guid,
			"error": err.Error(),
		})

		return
	}

	sleepForARandomInterval("task.run", 5000, 5001, map[string]interface{}{
		"task": task.Guid,
	})

	logger.Info("task.completing", map[string]interface{}{
		"task": task.Guid,
	})

	err = handler.bbs.CompleteTask(task, false, "", "")
	if err != nil {
		logger.Error("task.complete-failed", map[string]interface{}{
			"task":  task.Guid,
			"error": err.Error(),
		})

		return
	}

	logger.Info("task.completed", map[string]interface{}{
		"task": task.Guid,
	})
}

func (handler *Handler) reserveMemory(memory int) bool {
	handler.memoryMutex.Lock()
	defer handler.memoryMutex.Unlock()

	if handler.currentMemory >= memory {
		handler.currentMemory = handler.currentMemory - memory
		return true
	}

	return false
}

func (handler *Handler) releaseMemory(memory int) {
	handler.memoryMutex.Lock()
	defer handler.memoryMutex.Unlock()

	handler.currentMemory = handler.currentMemory + memory
}

func sleepForARandomInterval(reason string, minSleepTime, maxSleepTime int, data map[string]interface{}) {
	interval := rand.Intn(maxSleepTime-minSleepTime) + minSleepTime
	duration := time.Duration(interval) * time.Millisecond

	data["duration"] = fmt.Sprintf("%s", duration)

	logger.Info(reason, data)

	time.Sleep(duration)
}
