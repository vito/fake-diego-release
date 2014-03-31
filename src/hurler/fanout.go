package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

func Fanout(transport *http.Transport, request *http.Request, endpoints []*Endpoint) (*http.Response, error) {
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}

	responses := make(chan *http.Response, len(endpoints))
	errs := make(chan error, len(endpoints))

	for _, e := range endpoints {
		url := *request.URL
		url.Scheme = "http"
		url.Host = e.Addr

		req := *request
		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		req.URL = &url
		req.Close = true

		request := &req

		go func() {
			response, err := transport.RoundTrip(request)

			if err != nil {
				errs <- err
			} else {
				responses <- response
			}
		}()

		defer transport.CancelRequest(request)
	}

	var fanoutErr error
	var response *http.Response

	for i := 0; i < len(endpoints); i++ {
		select {
		case fanoutErr = <-errs:
		case response = <-responses:
			if response.StatusCode < 400 {
				return response, nil
			}
		}
	}

	if response != nil {
		return response, nil
	}

	return nil, fanoutErr
}
