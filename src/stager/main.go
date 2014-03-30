package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	"github.com/cloudfoundry/yagnats"
	"github.com/onsi/ginkgo/cleanup"

	"logger"
	"runtime-schema/bbs"
	"runtime-schema/models"
)

var listenNetwork = flag.String("listenNetwork", "tcp", "listening network for api server (tcp, unix)")
var listenAddr = flag.String("listenAddr", ":5555", "listening address for api server")

var stagerID = flag.String("stagerID", "stager-id", "the stager's ID")

var etcdCluster = flag.String("etcdCluster", "http://127.0.0.1:4001", "comma-separated list of etcd URIs (http://ip:port)")
var natsAddress = flag.String("natsAddress", "127.0.0.1:4222", "nats address, used for logging. no, really.")
var hurlerAddress = flag.String("hurlerAddress", "127.0.0.1:9090", "hurler address")

var stop = make(chan bool)
var tasks = &sync.WaitGroup{}

type stagingMessage struct {
	Count    int `json:"count"`
	MemoryMB int `json:"memory"`
}

func main() {
	var err error

	runtime.GOMAXPROCS(runtime.NumCPU())

	rand.Seed(time.Now().UnixNano())

	flag.Parse()

	cleanup.Register(func() {
		logger.Info("shutting-down", map[string]interface{}{})
		close(stop)
		tasks.Wait()
		logger.Info("shutdown", map[string]interface{}{})
	})

	logger.Component = fmt.Sprintf("stager.%s", *stagerID)

	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)
	err = etcdAdapter.Connect()
	if err != nil {
		logger.Fatal("etcd-connect", map[string]interface{}{
			"error": err.Error(),
		})
	}

	natsInfo := &yagnats.ConnectionInfo{
		Addr: *natsAddress,
	}

	err = logger.Connect(natsInfo)
	if err != nil {
		logger.Fatal("logger-connect", map[string]interface{}{
			"error": err.Error(),
		})
	}

	bbs := bbs.New(bbs.NewHurlerKicker(*hurlerAddress), etcdAdapter, timeprovider.NewTimeProvider())

	natsClient := yagnats.NewClient()

	err = natsClient.Connect(natsInfo)
	if err != nil {
		logger.Fatal("nats-connect", map[string]interface{}{
			"error": err.Error(),
		})
	}

	ready := make(chan bool, 1)

	go handleTasks(bbs, natsClient, *listenAddr)

	handleStaging(bbs, natsClient)

	<-ready

	logger.Info("up", map[string]interface{}{
		"stager": *stagerID,
	})

	select {}
}

func handleTasks(bbs bbs.StagerBBS, natsClient yagnats.NATSClient, listenAddr string) {
	err := http.ListenAndServe(listenAddr, &Handler{
		bbs:        bbs,
		natsClient: natsClient,
	})

	logger.Fatal("handling.failed", map[string]interface{}{
		"error": err.Error(),
	})
}

func handleStaging(bbs bbs.StagerBBS, natsClient yagnats.NATSClient) {
	var task uint64

	natsClient.Subscribe("stage", func(msg *yagnats.Message) {
		var message stagingMessage

		err := json.Unmarshal(msg.Payload, &message)
		if err != nil {
			logger.Fatal("staging-request.invalid", map[string]interface{}{
				"error":   err.Error(),
				"payload": string(msg.Payload),
			})
			return
		}

		for i := 0; i < message.Count; i++ {
			guid := atomic.AddUint64(&task, 1)

			task := &models.Task{
				Guid:     fmt.Sprintf("task-%d", guid),
				MemoryMB: message.MemoryMB,

				ReplyTo: msg.ReplyTo,
			}

			logger.Info("staging-request.desire", map[string]interface{}{
				"task": task,
			})

			go bbs.DesireTask(task)
		}
	})
}
