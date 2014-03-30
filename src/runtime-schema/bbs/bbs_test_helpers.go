package bbs

import (
	"fmt"
	"github.com/cloudfoundry/storeadapter"
	"runtime-schema/models"
)

func (self *BBS) GetAllPendingTasks() ([]*models.Task, error) {
	return getAllTasks(self.store, models.TaskStatePending)
}

func (self *BBS) GetAllClaimedTasks() ([]*models.Task, error) {
	return getAllTasks(self.store, models.TaskStateClaimed)
}

func (self *BBS) GetAllStartingTasks() ([]*models.Task, error) {
	return getAllTasks(self.store, models.TaskStateRunning)
}

func (self *BBS) GetAllCompletedTasks() ([]*models.Task, error) {
	return getAllTasks(self.store, models.TaskStateCompleted)
}

func (self *BBS) GetAllExecutors() ([]string, error) {
	nodes, err := self.store.ListRecursively(ExecutorSchemaRoot)
	if err == storeadapter.ErrorKeyNotFound {
		return []string{}, nil
	} else if err != nil {
		return nil, err
	}

	executors := []string{}

	for _, node := range nodes.ChildNodes {
		executors = append(executors, node.KeyComponents()[2])
	}

	return executors, nil
}

func (self *BBS) printNodes(message string, nodes []storeadapter.StoreNode) {
	fmt.Println(message)
	for _, node := range nodes {
		fmt.Printf("    %s [%d]: %s\n", node.Key, node.TTL, string(node.Value))
	}
}
