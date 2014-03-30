package main

import (
	"flag"
	"log"
	"net/http"
	"runtime"
	"strings"

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

var endpoints = flag.String(
	"endpoints",
	"",
	"list of endpoints to which to hurl (dispatch:host:ip:port)",
)

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	table := map[string]Route{}

	for _, endpoint := range strings.Split(*endpoints, ",") {
		segs := strings.SplitN(endpoint, ":", 3)

		dispatch := segs[0]
		host := segs[1]
		addr := segs[2]

		route := table[host]

		if dispatch == "round-robin" {
			route.Dispatch = RoundRobin
		} else if dispatch == "fanout" {
			route.Dispatch = Fanout
		} else {
			log.Fatalln("dispatch must be round-robin or fanout:", dispatch)
		}

		route.Endpoints = append(route.Endpoints, &Endpoint{
			Addr: addr,
		})

		table[host] = route
	}

	handler := Handler{
		table: table,
		pool:  workerpool.NewWorkerPool(1000),
	}

	http.ListenAndServe(":9090", handler)
}
