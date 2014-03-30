package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	"github.com/cloudfoundry/yagnats"
	"github.com/onsi/ginkgo/cleanup"

	"logger"
	"runtime-schema/bbs"
)

var listenNetwork = flag.String("listenNetwork", "tcp", "listening network for api server (tcp, unix)")
var listenAddr = flag.String("listenAddr", ":4444", "listening address for api server")

var executorID = flag.String("executorID", "executor-id", "the executor's ID")

var etcdCluster = flag.String("etcdCluster", "http://127.0.0.1:4001", "comma-separated list of etcd URIs (http://ip:port)")

var natsAddress = flag.String("natsAddress", "127.0.0.1:4222", "nats address, used for logging. no, really.")

var heartbeatInterval = flag.Duration("heartbeatInterval", 60*time.Second, "the interval, in seconds, between heartbeats for maintaining presence")
var convergenceInterval = flag.Duration("convergenceInterval", 30*time.Second, "the interval, in seconds, between convergences")
var timeToClaimTask = flag.Duration("timeToClaimTask", 30*time.Minute, "unclaimed run onces are marked as failed, after this time (in seconds)")
var maxMemory = flag.Int("availableMemory", 1000, "amount of available memory")

var stop = make(chan bool)
var tasks = &sync.WaitGroup{}

var MaintainPresenceError = errors.New("failed to maintain presence")

func main() {
	var err error

	runtime.GOMAXPROCS(runtime.NumCPU())

	rand.Seed(time.Now().UnixNano())

	flag.Parse()

	cleanup.Register(func() {
		logger.Info("executor.shutting-down", map[string]interface{}{})
		close(stop)
		tasks.Wait()
		logger.Info("executor.shutdown", map[string]interface{}{})
	})

	logger.Component = fmt.Sprintf("executor.%s", *executorID)

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

	err = logger.Connect(&yagnats.ConnectionInfo{
		Addr: *natsAddress,
	})

	bbs := bbs.New(etcdAdapter, timeprovider.NewTimeProvider())

	ready := make(chan bool, 1)

	err = maintainPresence(bbs, ready)
	if err != nil {
		logger.Fatal("initializing-presence", map[string]interface{}{
			"error": err.Error(),
		})
	}

	go handleTasks(bbs, *listenAddr)
	go convergeTasks(bbs)

	<-ready

	logger.Info("up", map[string]interface{}{
		"executor": *executorID,
	})

	select {}
}

func maintainPresence(bbs bbs.ExecutorBBS, ready chan<- bool) error {
	p, statusChannel, err := bbs.MaintainExecutorPresence(*heartbeatInterval, *executorID)
	if err != nil {
		ready <- false
		return err
	}

	tasks.Add(1)

	go func() {
		for {
			select {
			case locked, ok := <-statusChannel:
				if locked && ready != nil {
					ready <- true
					ready = nil
				}

				if !locked && ok {
					tasks.Done()
					logger.Fatal("maintain.presence.fatal", map[string]interface{}{
						"error": err.Error(),
					})
				}

				if !ok {
					tasks.Done()
					return
				}

			case <-stop:
				p.Remove()
			}
		}
	}()

	return nil
}

func handleTasks(bbs bbs.ExecutorBBS, listenAddr string) {
	err := http.ListenAndServe(listenAddr, &Handler{
		bbs: bbs,

		currentMemory: *maxMemory,
		memoryMutex:   &sync.Mutex{},
	})

	logger.Fatal("handling.failed", map[string]interface{}{
		"error": err.Error(),
	})
}

func convergeTasks(bbs bbs.ExecutorBBS) {
	statusChannel, releaseLock, err := bbs.MaintainConvergeLock(*convergenceInterval, *executorID)
	if err != nil {
		logger.Fatal("converge-lock.acquire-failed", map[string]interface{}{
			"error": err.Error(),
		})
	}

	tasks.Add(1)

	for {
		select {
		case locked, ok := <-statusChannel:
			if !ok {
				tasks.Done()
				return
			}

			if locked {
				t := time.Now()
				logger.Info("converging", map[string]interface{}{})

				bbs.ConvergeTask(*timeToClaimTask)

				logger.Info("converged", map[string]interface{}{
					"took": time.Since(t),
				})
			} else {
				logger.Info("converge-lock.lost", map[string]interface{}{})
			}
		case <-stop:
			releaseLock <- nil
		}
	}
}
