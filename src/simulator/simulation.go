package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"github.com/cloudfoundry/yagnats"
	"github.com/onsi/ginkgo/cleanup"

	"runtime-schema/models"
	"simulator/logger"
)

type taskData struct {
	Guid           string  `json:"guid"`
	DesiredTime    float64 `json:"desired_time"`
	CompletionTime float64 `json:"completed_time"`
	ExecutorIndex  int     `json:"executor"`
	NumCompletions int     `json:"num_completions"`
}

type errorData struct {
	Time float64 `json:"time"`
	Line string  `json:"line"`
}

var simulationLock *sync.Mutex
var simulationWait *sync.WaitGroup
var taskTracker map[string]*taskData
var errorLines []*errorData

type stagingMessage struct {
	Count    int `json:"count"`
	MemoryMB int `json:"memory"`
}

func runSimulation(natsClient yagnats.NATSClient) {
	simulationLock = &sync.Mutex{}
	simulationWait = &sync.WaitGroup{}
	taskTracker = map[string]*taskData{}

	msg := stagingMessage{
		Count:    nTasks,
		MemoryMB: taskMemory,
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}

	t := time.Now()

	logger.Info("simulation.start", nTasks)

	simulationWait.Add(nTasks)

	_, err = natsClient.Subscribe("info.stager.*.staging-request.desire", func(msg *yagnats.Message) {
		var desiredLog struct {
			Timestamp time.Time   `json:"_timestamp"`
			Task      models.Task `json:"task"`
		}

		err := json.Unmarshal(msg.Payload, &desiredLog)
		if err != nil {
			panic(err)
		}

		registerDesired(desiredLog.Task.Guid, desiredLog.Timestamp)
	})

	_, err = natsClient.Subscribe("error.>", func(msg *yagnats.Message) {
		var errorLog struct {
			Timestamp time.Time `json:"_timestamp"`
			Error     string    `json:"error"`
		}

		err := json.Unmarshal(msg.Payload, &errorLog)
		if err != nil {
			panic(err)
		}

		registerError(msg.Subject+": "+errorLog.Error, errorLog.Timestamp)
	})

	_, err = natsClient.Subscribe("fatal.>", func(msg *yagnats.Message) {
		var errorLog struct {
			Timestamp time.Time `json:"_timestamp"`
			Error     string    `json:"error"`
		}

		err := json.Unmarshal(msg.Payload, &errorLog)
		if err != nil {
			panic(err)
		}

		registerError(msg.Subject+": "+errorLog.Error, errorLog.Timestamp)
	})

	executorIndexes := map[string]int{}

	_, err = natsClient.Subscribe("completed-task", func(msg *yagnats.Message) {
		var task *models.Task

		err := json.Unmarshal(msg.Payload, &task)
		if err != nil {
			panic(err)
		}

		simulationLock.Lock()

		index, ok := executorIndexes[task.ExecutorID]
		if !ok {
			index = len(executorIndexes) + 1
			executorIndexes[task.ExecutorID] = index
		}

		data, ok := taskTracker[task.Guid]
		if !ok {
			logger.Error("uknown.runonce.completed", task.Guid, "executor", task.ExecutorID)
			simulationLock.Unlock()
			return
		}

		data.CompletionTime = float64(time.Now().UnixNano()) / 1e9
		logger.Info("runonce.completed", task.Guid, "executor", task.ExecutorID, "duration", data.CompletionTime-data.DesiredTime)
		data.ExecutorIndex = index
		data.NumCompletions++

		simulationLock.Unlock()

		simulationWait.Done()
	})
	if err != nil {
		panic(err)
	}

	err = natsClient.PublishWithReplyTo("stage", "completed-task", payload)
	if err != nil {
		panic(err)
	}

	cleanup.Register(func() {
		dt := time.Since(t)
		logger.Info("simulation.end", nTasks, "runtime", dt)

		simulationResult(dt)
		simulationErrors()
	})

	simulationWait.Wait()
}

func registerDesired(guid string, timestamp time.Time) {
	simulationLock.Lock()

	taskTracker[guid] = &taskData{
		Guid:        guid,
		DesiredTime: float64(timestamp.UnixNano()) / 1e9,
	}

	simulationLock.Unlock()
}

func registerError(line string, timestamp time.Time) {
	simulationLock.Lock()

	errorLines = append(errorLines, &errorData{
		Time: float64(timestamp.UnixNano()) / 1e9,
		Line: line,
	})

	simulationLock.Unlock()
}

func simulationResult(dt time.Duration) {
	simulationLock.Lock()
	defer simulationLock.Unlock()

	executorDistribution := map[string]int{}
	for _, runData := range taskTracker {
		executorDistribution[fmt.Sprintf("%d", runData.ExecutorIndex)]++
	}
	executorDistributionData, _ := json.Marshal(executorDistribution)

	encodedTaskData, _ := json.Marshal(taskTracker)

	data := fmt.Sprintf(`{
	"elapsed_time": %.4f,
	"executor_distribution": %s,
	"run_once_data": %s
}
`, float64(dt)/1e9, string(executorDistributionData), string(encodedTaskData))

	ioutil.WriteFile(filepath.Join(outDir, "result.json"), []byte(data), 0777)
}

func simulationErrors() {
	data, err := json.Marshal(errorLines)
	if err != nil {
		panic(err)
	}

	ioutil.WriteFile(filepath.Join(outDir, "errors.json"), []byte(data), 0644)
}
