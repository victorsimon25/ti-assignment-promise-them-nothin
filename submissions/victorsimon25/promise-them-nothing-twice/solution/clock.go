package main

import (
	"net/http"
	"os"
	"time"
)

// Clock provides an interface to resolve the current time.
// It accepts an optional HTTP request to extract mock times for testing.
type Clock interface {
	Now(r *http.Request) time.Time
}

// RealClock resolves either the system time or the request-scoped X-Fake-Time
// header if ALLOW_MOCK_TIME=true is configured in the environment.
type RealClock struct{}

// Now returns the resolved time.
func (RealClock) Now(r *http.Request) time.Time {
	if os.Getenv("ALLOW_MOCK_TIME") == "true" && r != nil {
		fakeTimeStr := r.Header.Get("X-Fake-Time")
		if fakeTimeStr != "" {
			if t, err := time.Parse(time.RFC3339, fakeTimeStr); err == nil {
				return t.UTC()
			}
		}
	}
	return time.Now().UTC()
}
