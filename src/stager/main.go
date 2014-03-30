package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	"github.com/cloudfoundry/yagnats"
	"github.com/onsi/ginkgo/cleanup"

	"logger"
	"runtime-schema/bbs"
	"runtime-schema/models"
)

var listenAddr = flag.String("listenAddr", "127.0.0.1:5555", "listening address for api server")

var stagerID = flag.String("stagerID", "stager-id", "the stager's ID")

var etcdCluster = flag.String("etcdCluster", "http://127.0.0.1:4001", "comma-separated list of etcd URIs (http://ip:port)")

var natsAddresses = flag.String(
	"natsAddresses",
	"127.0.0.1:4222",
	"comma-separated list of NATS addresses (ip:port)",
)

var natsUsername = flag.String(
	"natsUsername",
	"nats",
	"Username to connect to nats",
)

var natsPassword = flag.String(
	"natsPassword",
	"nats",
	"Password for nats user",
)

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

	natsClient := yagnats.NewClient()

	natsMembers := []yagnats.ConnectionProvider{}

	for _, addr := range strings.Split(*natsAddresses, ",") {
		natsMembers = append(
			natsMembers,
			&yagnats.ConnectionInfo{addr, *natsUsername, *natsPassword},
		)
	}

	natsInfo := &yagnats.ConnectionCluster{Members: natsMembers}

	err = logger.Connect(natsInfo)
	if err != nil {
		log.Fatalln("could not connect logger:", err)
	}

	err = natsClient.Connect(natsInfo)
	if err != nil {
		log.Fatalln("could not connect to nats:", err)
	}

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

	bbs := bbs.New(bbs.NewHurlerKicker(*hurlerAddress), etcdAdapter, timeprovider.NewTimeProvider())

	ready := make(chan bool, 1)

	err = registerHandler(etcdAdapter, *listenAddr, ready)
	if err != nil {
		logger.Fatal("initializing-route", map[string]interface{}{
			"error": err.Error(),
		})
	}

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

func registerHandler(etcdAdapter *etcdstoreadapter.ETCDStoreAdapter, addr string, ready chan<- bool) error {
	node := storeadapter.StoreNode{
		Key: "/v1/routes/round-robin/stager/" + addr,
		TTL: 10,
	}

	status, clearNode, err := etcdAdapter.MaintainNode(node)
	if err != nil {
		return err
	}

	tasks.Add(1)

	go func() {
		for {
			select {
			case locked, ok := <-status:
				if locked && ready != nil {
					ready <- true
					ready = nil
				}

				if !locked && ok {
					tasks.Done()

					logger.Fatal("maintain.route.fatal", map[string]interface{}{
						"error": err.Error(),
					})
				}

				if !ok {
					tasks.Done()
					return
				}

			case <-stop:
				close(clearNode)

				for _ = range status {
				}

				tasks.Done()

				return
			}
		}
	}()

	return nil
}
