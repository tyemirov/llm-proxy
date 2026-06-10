package proxy

import (
	"io"
	"net/http"
	"sync"

	"github.com/cenkalti/backoff/v4"
)

type limitedHTTPDoer struct {
	next     HTTPDoer
	active   chan struct{}
	admitted chan struct{}
}

type releasingReadCloser struct {
	body        io.ReadCloser
	releaseOnce sync.Once
	release     func()
}

func newLimitedHTTPDoer(next HTTPDoer, workerCount int, queueSize int) HTTPDoer {
	return &limitedHTTPDoer{
		next:     next,
		active:   make(chan struct{}, workerCount),
		admitted: make(chan struct{}, workerCount+queueSize),
	}
}

func (doer *limitedHTTPDoer) Do(httpRequest *http.Request) (*http.Response, error) {
	if admissionError := doer.admit(); admissionError != nil {
		return nil, admissionError
	}
	if acquireError := doer.acquire(httpRequest); acquireError != nil {
		doer.releaseAdmission()
		return nil, acquireError
	}

	httpResponse, requestError := doer.next.Do(httpRequest)
	if requestError != nil {
		doer.releaseActive()
		doer.releaseAdmission()
		return nil, requestError
	}
	httpResponse.Body = &releasingReadCloser{
		body: httpResponse.Body,
		release: func() {
			doer.releaseActive()
			doer.releaseAdmission()
		},
	}
	return httpResponse, nil
}

func (doer *limitedHTTPDoer) admit() error {
	select {
	case doer.admitted <- struct{}{}:
		return nil
	default:
		return backoff.Permanent(errQueueFull)
	}
}

func (doer *limitedHTTPDoer) acquire(httpRequest *http.Request) error {
	select {
	case doer.active <- struct{}{}:
		return nil
	case <-httpRequest.Context().Done():
		return httpRequest.Context().Err()
	}
}

func (doer *limitedHTTPDoer) releaseActive() {
	<-doer.active
}

func (doer *limitedHTTPDoer) releaseAdmission() {
	<-doer.admitted
}

func (body *releasingReadCloser) Read(buffer []byte) (int, error) {
	return body.body.Read(buffer)
}

func (body *releasingReadCloser) Close() error {
	closeError := body.body.Close()
	body.releaseOnce.Do(body.release)
	return closeError
}
