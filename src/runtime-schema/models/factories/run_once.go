package factories

import (
	"github.com/nu7hatch/gouuid"
	"runtime-schema/models"
)

func GenerateGuid() string {
	guid, err := uuid.NewV4()
	if err != nil {
		panic("Failed to generate a GUID.  Craziness.")
	}

	return guid.String()
}

func BuildTaskWithRunAction(stack string, memoryMB int, diskMB int, script string) *models.Task {
	return &models.Task{
		Guid:     GenerateGuid(),
		MemoryMB: memoryMB,
		DiskMB:   diskMB,
		Actions: []models.ExecutorAction{
			{Action: models.RunAction{
				Script: script,
			}},
		},
		Stack: stack,
	}
}
