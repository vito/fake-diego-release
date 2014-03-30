package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	"github.com/cloudfoundry/yagnats"
	"github.com/onsi/ginkgo/cleanup"

	"simulator/logger"
)

var etcdNodes int
var nExecutors int
var nTasks int
var executorMemory int
var taskMemory int
var outDir string
var verbose bool
var over time.Duration

func init() {
	flag.IntVar(&nTasks, "run-onces", 1, "# of run onces")
	flag.IntVar(&taskMemory, "run-once-memory", 100, "Task required memory")
	flag.StringVar(&outDir, "output-directory", "", "Output directory")
	flag.BoolVar(&verbose, "v", false, "Verbose mode")
}

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

var etcdAdapter *etcdstoreadapter.ETCDStoreAdapter

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	//make the out dir
	logger.Component = "SIMULATOR"
	if outDir == "" {
		logger.Fatal("out.dir.unspecified")
	}
	err := os.MkdirAll(outDir, 0777)
	if err != nil {
		logger.Fatal("out.dir.creation.failed", err)
	}

	//set up logging
	outputFile, err := os.Create(filepath.Join(outDir, "simulator.log"))
	if err != nil {
		logger.Fatal("failed.to.create.simulator.log", err)
	}

	logger.Writer = io.MultiWriter(os.Stdout, outputFile)

	cleanup.Register(func() {
		outputFile.Sync()
	})

	//write info to the output dir
	writeInfo()

	//start etcd
	natsClient := yagnats.NewClient()

	natsMembers := []yagnats.ConnectionProvider{}

	for _, addr := range strings.Split(*natsAddresses, ",") {
		natsMembers = append(
			natsMembers,
			&yagnats.ConnectionInfo{addr, *natsUsername, *natsPassword},
		)
	}

	natsInfo := &yagnats.ConnectionCluster{Members: natsMembers}

	err = natsClient.Connect(natsInfo)
	if err != nil {
		logger.Fatal("could not connect to nats:", err)
	}

	logger.Component = "simulator"

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

	//monitor etcd
	monitorETCD(etcdAdapter)

	//run the simulator
	runSimulation(natsClient)

	cleanup.Exit(0)
}

func writeInfo() {
	data := fmt.Sprintf(`{ "run_onces": %d, "run_once_memory": %d, "executors": 50 } `, nTasks, taskMemory)

	logger.Info("simulator.running", data)

	ioutil.WriteFile(filepath.Join(outDir, "info.json"), []byte(data), 0644)
}
