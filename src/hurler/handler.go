package main

import (
	"io"
	"log"
	"net/http"
	"os"

	"github.com/cloudfoundry/storeadapter/workerpool"
)

type Handler struct {
	table map[string]Route
	pool  *workerpool.WorkerPool
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, ok := h.table[r.Host]
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
