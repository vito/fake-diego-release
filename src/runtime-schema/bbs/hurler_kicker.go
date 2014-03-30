package bbs

import (
	"bytes"
	"io/ioutil"
	"log"
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
	log.Println("kicking desire")

	json := task.ToJSON()

	res, err := http.DefaultClient.Do(&http.Request{
		Method: "POST",

		URL: &url.URL{
			Scheme: "http",
			Host:   kicker.hurlerAddress,
			Path:   "/tasks",
		},

		Host: "executor",

		Body:          ioutil.NopCloser(bytes.NewBuffer(json)),
		ContentLength: int64(len(json)),

		Header: map[string][]string{
			"Content-Type": []string{"application/json"},
		},
	})

	if err != nil {
		log.Println("kick desire error", err)
		return
	}

	log.Println("kick desire", res.Status)
}

func (kicker *HurlerKicker) Complete(task *models.Task) {
	log.Println("kicking complete")

	json := task.ToJSON()

	res, err := http.DefaultClient.Do(&http.Request{
		Method: "POST",

		URL: &url.URL{
			Scheme: "http",
			Host:   kicker.hurlerAddress,
			Path:   "/tasks",
		},

		Host: "stager",

		Body:          ioutil.NopCloser(bytes.NewBuffer(json)),
		ContentLength: int64(len(json)),

		Header: map[string][]string{
			"Content-Type": []string{"application/json"},
		},
	})

	if err != nil {
		log.Println("kick complete error", err)
		return
	}

	log.Println("kick complete", res.Status)
}
