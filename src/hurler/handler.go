package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
)

type Handler struct {
	table map[string]Route
	pool  *workerpool.WorkerPool

	sync.RWMutex
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.RLock()
	route, ok := h.table[r.Host]
	h.RUnlock()

	if !ok {
		log.Println("unknown host:", r.Host)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	res, err := route.Dispatch(h.pool, r, route.Endpoints)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	for k, vs := range res.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(res.StatusCode)

	writer := io.MultiWriter(w, os.Stderr)

	_, err = io.Copy(writer, res.Body)
	if err != nil {
		return
	}

	res.Body.Close()
}

func (h *Handler) syncTable(etcd *etcdstoreadapter.ETCDStoreAdapter, syncInterval time.Duration) {
	for {
		allNodes, _ := etcd.ListRecursively("/v1/routes")

		newTable := map[string]Route{}

		fanouts, _ := allNodes.Lookup("fanout")
		roundRobins, _ := allNodes.Lookup("round-robin")

		for _, host := range fanouts.ChildNodes {
			hostname := path.Base(host.Key)

			route := newTable[hostname]
			route.Dispatch = Fanout

			for _, endpoint := range host.ChildNodes {
				route.Endpoints = append(
					route.Endpoints,
					&Endpoint{Addr: path.Base(endpoint.Key)},
				)

				log.Println("registering", hostname, endpoint.Key)
			}

			newTable[hostname] = route
		}

		for _, host := range roundRobins.ChildNodes {
			hostname := path.Base(host.Key)

			route := newTable[hostname]
			route.Dispatch = RoundRobin

			for _, endpoint := range host.ChildNodes {
				route.Endpoints = append(
					route.Endpoints,
					&Endpoint{Addr: path.Base(endpoint.Key)},
				)

				log.Println("registering", hostname, endpoint.Key)
			}

			newTable[hostname] = route
		}

		h.Lock()
		h.table = newTable
		h.Unlock()

		time.Sleep(syncInterval)
	}
}
