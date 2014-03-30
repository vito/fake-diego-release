package main

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"net/http"
)

func RoundRobin(request *http.Request, endpoints []*Endpoint) (*http.Response, error) {
	transport := &http.Transport{}

	startingPoint := rand.Intn(len(endpoints))

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}

	attempts := 0
	i := startingPoint

	for {
		if i == len(endpoints) {
			i = 0
		}

		url := *request.URL
		url.Scheme = "http"
		url.Host = endpoints[i].Addr

		req := *request
		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		req.URL = &url
		req.Close = true

		request := &req

		response, err := transport.RoundTrip(request)

		attempts++
		i++

		if attempts == len(endpoints) {
			return response, err
		}

		if err != nil {
			continue
		}

		if response.StatusCode < 400 {
			return response, nil
		}

		response.Body.Close()
	}

	panic("unreachable")
}
