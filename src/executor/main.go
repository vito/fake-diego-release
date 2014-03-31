package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	"github.com/cloudfoundry/yagnats"
	uuid "github.com/nu7hatch/gouuid"
	"github.com/onsi/ginkgo/cleanup"

	"logger"
	"runtime-schema/bbs"
)

var listenAddr = flag.String("listenAddr", "127.0.0.1:4444", "listening address for api server")

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

var heartbeatInterval = flag.Duration("heartbeatInterval", 60*time.Second, "the interval, in seconds, between heartbeats for maintaining presence")
var convergenceInterval = flag.Duration("convergenceInterval", 30*time.Second, "the interval, in seconds, between convergences")
var timeToClaimTask = flag.Duration("timeToClaimTask", 30*time.Minute, "unclaimed run onces are marked as failed, after this time (in seconds)")
var maxMemory = flag.Int("memoryMB", 1000, "maximum memory capacity")

var stop = make(chan bool)
var tasks = &sync.WaitGroup{}
var once = &sync.Once{}

var executorID string

var MaintainPresenceError = errors.New("failed to maintain presence")

func main() {
	var err error

	runtime.GOMAXPROCS(runtime.NumCPU())

	rand.Seed(time.Now().UnixNano())

	flag.Parse()

	executorUUID, err := uuid.NewV4()
	if err != nil {
		log.Fatalln("could not generate guid:", err)
	}

	executorID = executorUUID.String()

	cleanup.Register(func() {
		once.Do(func() {
			logger.Info("shutting-down", map[string]interface{}{})
			close(stop)
			tasks.Wait()
			logger.Info("shutdown", map[string]interface{}{})
		})
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

	logger.Component = fmt.Sprintf("executor.%s", executorID)

	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)
	err = etcdAdapter.Connect()
	if err != nil {
		logger.Fatal("etcd.connect-failed", map[string]interface{}{
			"error": err.Error(),
		})
	}

	bbs := bbs.New(bbs.NewHurlerKicker(*hurlerAddress), etcdAdapter, timeprovider.NewTimeProvider())

	ready := make(chan bool, 1)

	err = maintainPresence(bbs, ready)
	if err != nil {
		logger.Fatal("initializing-presence", map[string]interface{}{
			"error": err.Error(),
		})
	}

	err = registerHandler(etcdAdapter, *listenAddr, ready)
	if err != nil {
		logger.Fatal("initializing-route", map[string]interface{}{
			"error": err.Error(),
		})
	}

	go handleTasks(bbs, *listenAddr)
	go convergeTasks(bbs)

	<-ready
	<-ready

	logger.Info("up", map[string]interface{}{
		"executor": executorID,
	})

	select {}
}

func maintainPresence(bbs bbs.ExecutorBBS, ready chan<- bool) error {
	p, statusChannel, err := bbs.MaintainExecutorPresence(*heartbeatInterval, executorID)
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
					logger.Fatal("maintain.presence.fatal", map[string]interface{}{})
				}

				if !ok {
					tasks.Done()
					return
				}

			case <-stop:
				p.Remove()

				for _ = range statusChannel {
				}

				tasks.Done()

				return
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
	statusChannel, releaseLock, err := bbs.MaintainConvergeLock(*convergenceInterval, executorID)
	if err != nil {
		logger.Fatal("converge-lock.acquire-failed", map[string]interface{}{
			"error": err.Error(),
		})
	}

	for {
		select {
		case locked, ok := <-statusChannel:
			if !ok {
				return
			}

			if locked {
				t := time.Now()

				logger.Info("converging", map[string]interface{}{})

				bbs.ConvergeTasks(*timeToClaimTask)

				logger.Info("converged", map[string]interface{}{
					"took": time.Since(t),
				})
			} else {
				logger.Info("converge-lock.lost", map[string]interface{}{})
			}
		case <-stop:
			close(releaseLock)

			for _ = range statusChannel {
			}

			return
		}
	}
}

func registerHandler(etcdAdapter *etcdstoreadapter.ETCDStoreAdapter, addr string, ready chan<- bool) error {
	node := storeadapter.StoreNode{
		Key: "/v1/routes/round-robin/executor/" + addr,
		TTL: 60,
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
					logger.Fatal("maintain.route.fatal", map[string]interface{}{})
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
