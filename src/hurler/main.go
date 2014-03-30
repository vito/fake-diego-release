package main

import (
	"flag"
	"net/http"
	"runtime"
	"strings"

	"github.com/cloudfoundry/storeadapter/workerpool"
)

type Endpoint struct {
	Addr string
}

var endpoints = flag.String(
	"endpoints",
	"",
	"list of endpoints to fanout to",
)

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	var es []*Endpoint
	for _, addr := range strings.Split(*endpoints, ",") {
		es = append(es, &Endpoint{
			Addr: addr,
		})
	}

	handler := Handler{
		endpoints: es,
		pool:      workerpool.NewWorkerPool(1000),
	}

	http.ListenAndServe(":9090", handler)
}
