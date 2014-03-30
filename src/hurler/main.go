package main

import (
	"flag"
	"log"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
)

type Route struct {
	Dispatch  Dispatch
	Endpoints []*Endpoint
}

type Endpoint struct {
	Addr string
}

type Dispatch func(*workerpool.WorkerPool, *http.Request, []*Endpoint) (*http.Response, error)

var listenAddr = flag.String(
	"listenAddr",
	":9090",
	"listening address",
)

var etcdCluster = flag.String(
	"etcdCluster",
	"http://127.0.0.1:4001",
	"comma-separated list of etcd URIs (http://ip:port)",
)

var syncInterval = flag.Duration(
	"syncInterval",
	10*time.Second,
	"how often to re-sync with etcd",
)

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	table := map[string]Route{}

	handler := &Handler{
		table: table,
		pool:  workerpool.NewWorkerPool(1000),
	}

	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)
	err := etcdAdapter.Connect()
	if err != nil {
		log.Fatalln("can't connect to etcd:", err)
	}

	go handler.syncTable(etcdAdapter, *syncInterval)

	http.ListenAndServe(*listenAddr, handler)
}
