package integrations

import (
	"net/http"
	"time"
)

func Retry(
	attempts int,
	sleep time.Duration,
	fn func(req *http.Request) (*http.Response, error),
	req *http.Request,
) (*http.Response, error) {
	response, err := fn(req)
	if err != nil {
		if attempts--; attempts > 0 {
			time.Sleep(sleep)
			sleep *= 2
			return Retry(attempts, sleep, fn, req)
		}
		return nil, err
	}
	return response, nil
}
