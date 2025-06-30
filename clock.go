package cron

import "time"

type Clock interface {
	Now() time.Time
	Timer(time.Time) (timer <-chan struct{}, stop func())
}

func immediateTimer() (<-chan struct{}, func()) {
	out := make(chan struct{}, 1)
	out <- struct{}{}
	return out, func() {}
}

func neverTimer() (<-chan struct{}, func()) {
	out := make(chan struct{})
	return out, func() {}
}
