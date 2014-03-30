package bbs

import (
	"runtime-schema/models"
)

type NopKicker struct{}

func (NopKicker) Desire(*models.Task)   {}
func (NopKicker) Complete(*models.Task) {}
