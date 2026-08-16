package main

import "time"

// Clock provides an interface to resolve the current time.
// This allows us to mock the clock in tests and for time-window switching.
type Clock interface {
	Now() time.Time
}

// RealClock uses the standard system time.
type RealClock struct{}

// Now returns the current system time.
func (RealClock) Now() time.Time {
	return time.Now()
}
