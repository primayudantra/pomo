package main

import "time"

// Clock abstracts the current time so services can be tested with a fake.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
