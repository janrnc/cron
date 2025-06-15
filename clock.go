package cron

import "time"

type Clock interface {
	Register(*Cron) []Option
	Now() time.Time
	Timer(time.Time) (timer <-chan struct{}, stop func())
	NopTimer() (timer <-chan struct{}, stop func())
}
