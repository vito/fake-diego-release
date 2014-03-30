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
	"list of endpoints to which to hurl (host:ip:port)",
)

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	table := map[string][]*Endpoint{}
	for _, endpoint := range strings.Split(*endpoints, ",") {
		segs := strings.SplitN(endpoint, ":", 2)

		table[segs[0]] = append(table[segs[0]], &Endpoint{
			Addr: segs[1],
		})
	}

	handler := Handler{
		table: table,
		pool:  workerpool.NewWorkerPool(1000),
	}

	http.ListenAndServe(":9090", handler)
}
