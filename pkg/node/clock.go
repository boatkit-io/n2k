package node

import "time"

// Ticker is an interface that abstracts *time.Ticker for testing.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// Clock is an interface that abstracts the time package for testing.
type Clock interface {
	NewTicker(d time.Duration) Ticker
}

// realTicker is a wrapper for *time.Ticker that implements the Ticker interface.
type realTicker struct {
	ticker *time.Ticker
}

func (rt *realTicker) C() <-chan time.Time {
	return rt.ticker.C
}

func (rt *realTicker) Stop() {
	rt.ticker.Stop()
}

// realClock is an implementation of the Clock interface that uses the standard time package.
type realClock struct{}

func (rc *realClock) NewTicker(d time.Duration) Ticker {
	return &realTicker{ticker: time.NewTicker(d)}
}

// NewRealClock creates a new clock that uses the standard time package.
func NewRealClock() Clock {
	return &realClock{}
}
