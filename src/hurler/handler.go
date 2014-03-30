package main

import (
	"io"
	"net/http"
	"os"

	"github.com/cloudfoundry/storeadapter/workerpool"
)

type Handler struct {
	table map[string][]*Endpoint
	pool  *workerpool.WorkerPool
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	endpoints, ok := h.table[r.Host]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	res, err := RoundRobin(r, endpoints)
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
