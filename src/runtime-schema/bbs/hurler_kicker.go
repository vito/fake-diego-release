package bbs

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"

	"runtime-schema/models"
)

type HurlerKicker struct {
	hurlerAddress string
}

func NewHurlerKicker(address string) *HurlerKicker {
	return &HurlerKicker{
		hurlerAddress: address,
	}
}

func (kicker *HurlerKicker) Desire(task *models.Task) {
	request := &http.Request{
		Method: "POST",

		URL: &url.URL{
			Scheme: "http",
			Host:   kicker.hurlerAddress,
			Path:   "/tasks",
		},

		Host: "executor",

		Body: ioutil.NopCloser(bytes.NewBuffer(task.ToJSON())),
	}

	request.Header.Set("Content-Type", "application/json")

	http.DefaultClient.Do(request)
}

func (kicker *HurlerKicker) Complete(task *models.Task) {
	request := &http.Request{
		Method: "POST",

		URL: &url.URL{
			Scheme: "http",
			Host:   kicker.hurlerAddress,
			Path:   "/tasks",
		},

		Host: "stager",

		Body: ioutil.NopCloser(bytes.NewBuffer(task.ToJSON())),
	}

	request.Header.Set("Content-Type", "application/json")

	http.DefaultClient.Do(request)
}
