package logger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudfoundry/yagnats"
	"github.com/onsi/ginkgo/cleanup"
)

var Component string

var natsClient = yagnats.NewClient()

func Connect(provider yagnats.ConnectionProvider) error {
	return natsClient.Connect(provider)
}

func Info(subject string, data map[string]interface{}) {
	log("info", subject, data)
}

func Error(subject string, data map[string]interface{}) {
	log("error", subject, data)
}

func Fatal(subject string, data map[string]interface{}) {
	log("fatal", subject, data)
	cleanup.Exit(1)
}

func log(level, subject string, data map[string]interface{}) {
	data["_timestamp"] = time.Now()

	payload, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	natsClient.Publish(
		fmt.Sprintf("%s.%s.%s", level, Component, subject),
		payload,
	)
}
