package main

import (
	"io"
	"log"
	"net/http"
	"os"

	"github.com/cloudfoundry/storeadapter/workerpool"
)

type Handler struct {
	endpoints []*Endpoint
	pool      *workerpool.WorkerPool
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := RoundRobin(r, h.endpoints)
	if err != nil {
		log.Println("FANOUT ERR", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	for k, vs := range res.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}

	log.Println("STATUS", res.Status)

	w.WriteHeader(res.StatusCode)

	writer := io.MultiWriter(w, os.Stderr)

	_, err = io.Copy(writer, res.Body)
	if err != nil {
		return
	}

	res.Body.Close()
}
