package bbs

import (
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/storeadapter"
	"path"
	"runtime-schema/models"
	"time"
)

const ClaimTTL = 10 * time.Second
const ResolvingTTL = 5 * time.Second
const TaskSchemaRoot = SchemaRoot + "run_once"
const ExecutorSchemaRoot = SchemaRoot + "executor"
const LockSchemaRoot = SchemaRoot + "locks"

func taskSchemaPath(task *models.Task) string {
	return path.Join(TaskSchemaRoot, task.Guid)
}

func executorSchemaPath(executorID string) string {
	return path.Join(ExecutorSchemaRoot, executorID)
}

func lockSchemaPath(lockName string) string {
	return path.Join(LockSchemaRoot, lockName)
}

func retryIndefinitelyOnStoreTimeout(callback func() error) error {
	for {
		err := callback()

		if err == storeadapter.ErrorTimeout {
			time.Sleep(time.Second)
			continue
		}

		return err
	}
}

func getAllTasks(store storeadapter.StoreAdapter, state models.TaskState) ([]*models.Task, error) {
	node, err := store.ListRecursively(TaskSchemaRoot)
	if err == storeadapter.ErrorKeyNotFound {
		return []*models.Task{}, nil
	}

	if err != nil {
		return []*models.Task{}, err
	}

	tasks := []*models.Task{}
	for _, node := range node.ChildNodes {
		task, err := models.NewTaskFromJSON(node.Value)
		if err != nil {
			steno.NewLogger("bbs").Errorf("cannot parse task JSON for key %s: %s", node.Key, err.Error())
		} else if task.State == state {
			tasks = append(tasks, &task)
		}
	}

	return tasks, nil
}
