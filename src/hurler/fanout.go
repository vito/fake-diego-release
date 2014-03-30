package main

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/cloudfoundry/storeadapter/workerpool"
)

func Fanout(pool *workerpool.WorkerPool, request *http.Request, endpoints []*Endpoint) (*http.Response, error) {
	transport := &http.Transport{}

	requests := []*http.Request{}
	writeClosers := []io.WriteCloser{}

	responses := make(chan *http.Response, len(endpoints))
	errs := make(chan error, len(endpoints))

	for _, e := range endpoints {
		r, w := io.Pipe()

		url := *request.URL
		url.Scheme = "http"
		url.Host = e.Addr

		req := *request
		req.Body = r
		req.URL = &url
		req.Close = true

		request := &req

		requests = append(requests, request)
		writeClosers = append(writeClosers, w)

		pool.ScheduleWork(func() {
			defer r.Close()

			response, err := transport.RoundTrip(request)

			if err != nil {
				errs <- err
			} else {
				responses <- response
			}
		})
	}

	defer func() {
		for _, req := range requests {
			transport.CancelRequest(req)
		}
	}()

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}

	for _, wc := range writeClosers {
		go func(wc io.WriteCloser) {
			wc.Write(body)
			wc.Close()
		}(wc)
	}

	var fanoutErr error
	var response *http.Response

	for i := 0; i < len(endpoints); i++ {
		select {
		case fanoutErr = <-errs:
			log.Println("FANOUT ERR", fanoutErr)
		case response = <-responses:
			log.Println("RESPONSE", response.StatusCode)

			if response.StatusCode < 400 {
				return response, nil
			}
		}
	}

	log.Println("NONE OK")

	if response != nil {
		return response, nil
	}

	return nil, fanoutErr
}
