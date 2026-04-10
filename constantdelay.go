package cron

import (
	"fmt"
	"time"
)

// every returns a crontab Schedule that activates once every duration.
// Delays that are less than on second or not a multiple of a second will return an error.
func every(duration time.Duration) (*DefaultSchedule, error) {
	if duration < time.Second {
		return nil, fmt.Errorf("delay must be at least one second but was %s", duration.String())
	} else if duration%time.Second != 0 {
		return nil, fmt.Errorf("delay must be a multiple of one second but was %s", duration.String())
	}
	return &DefaultSchedule{delay: duration}, nil
}
