package cron

import (
	"math"
	"sync"
	"time"

	"github.com/gdgvda/cron/internal/matcher"
)

// Schedule describes a job's duty cycle.
type Schedule interface {
	// Next returns the next activation time, later than the given time.
	// Next is invoked initially, and then each time the job is run.
	Next(time.Time) time.Time
}

// Specifies a duty cycle (to the second granularity), based on a
// traditional crontab specification.
type DefaultSchedule struct {
	secondMatch, minuteMatch, hourMatch, dayMatch, monthMatch matcher.Matcher

	// Override location for this schedule.
	location *time.Location

	// Constant delay mode when not zero
	delay time.Duration

	minInterval func() time.Duration
}

func newDefaultSchedule(second, minute, hour, day, month matcher.Matcher, location *time.Location) *DefaultSchedule {
	s := &DefaultSchedule{
		secondMatch: second,
		minuteMatch: minute,
		hourMatch:   hour,
		dayMatch:    day,
		monthMatch:  month,
		location:    location,
	}
	s.minInterval = sync.OnceValue(s.computeMinInterval)
	return s
}

// Next returns the next time this schedule is activated, greater than the given
// time.  If no time can be found to satisfy the schedule, return the zero time.
func (s *DefaultSchedule) Next(t time.Time) time.Time {
	if s.delay != 0 {
		return t.Add(s.delay - time.Duration(t.Nanosecond())*time.Nanosecond)
	}

	// General approach
	//
	// For Month, Day, Hour, Minute, Second:
	// Check if the time value matches.  If yes, continue to the next field.
	// If the field doesn't match the schedule, then increment the field until it matches.
	// While incrementing the field, a wrap-around brings it back to the beginning
	// of the field list (since it is necessary to re-verify previous field
	// values)

	// Convert the given time into the schedule's timezone, if one is specified.
	// Save the original timezone so we can convert back after we find a time.
	// Note that schedules without a time zone specified (time.Local) are treated
	// as local to the time provided.
	origLocation := t.Location()
	loc := s.location
	if loc == time.Local {
		loc = t.Location()
	}
	if s.location != time.Local {
		t = t.In(s.location)
	}

	// Start at the earliest possible time (the upcoming second).
	t = t.Add(1*time.Second - time.Duration(t.Nanosecond())*time.Nanosecond)

	// This flag indicates whether a field has been incremented.
	added := false

	// If no time is found within five years, return zero.
	yearLimit := t.Year() + 5

WRAP:
	if t.Year() > yearLimit {
		return time.Time{}
	}

	// Find the first applicable month.
	// If it's this month, then do nothing.
	for !s.monthMatch(t) {
		// If we have to add a month, reset the other parts to 0.
		if !added {
			added = true
			// Otherwise, set the date at the beginning (since the current time is irrelevant).
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
		}
		t = t.AddDate(0, 1, 0)

		// Wrapped around.
		if t.Month() == time.January {
			goto WRAP
		}
	}

	// Now get a day in that month.
	//
	// NOTE: This causes issues for daylight savings regimes where midnight does
	// not exist.  For example: Sao Paulo has DST that transforms midnight on
	// 11/3 into 1am. Handle that by noticing when the Hour ends up != 0.
	for !s.dayMatch(t) {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		}
		t = t.AddDate(0, 0, 1)
		// Notice if the hour is no longer midnight due to DST.
		// Add an hour if it's 23, subtract an hour if it's 1.
		if t.Hour() != 0 {
			if t.Hour() > 12 {
				t = t.Add(time.Duration(24-t.Hour()) * time.Hour)
			} else {
				t = t.Add(time.Duration(-t.Hour()) * time.Hour)
			}
		}

		if t.Day() == 1 {
			goto WRAP
		}
	}

	for !s.hourMatch(t) {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc)
		}
		t = t.Add(1 * time.Hour)

		if t.Hour() == 0 {
			goto WRAP
		}
	}

	for !s.minuteMatch(t) {
		if !added {
			added = true
			t = t.Truncate(time.Minute)
		}
		t = t.Add(1 * time.Minute)

		if t.Minute() == 0 {
			goto WRAP
		}
	}

	for !s.secondMatch(t) {
		if !added {
			added = true
			t = t.Truncate(time.Second)
		}
		t = t.Add(1 * time.Second)

		if t.Second() == 0 {
			goto WRAP
		}
	}

	return t.In(origLocation)
}

// WithLocation returns a copy of the schedule with the given location.
// If the location is nil, it returns the original schedule.
func (s *DefaultSchedule) WithLocation(l *time.Location) *DefaultSchedule {
	if l == nil {
		return s
	}
	s2 := new(DefaultSchedule)
	*s2 = *s
	s2.location = l
	return s2
}

// MinInterval returns the shortest time between two consecutive activations
// of the schedule, assuming its location keeps a constant UTC offset. The
// result is exact in UTC or any fixed-offset zone. In a location that observes
// daylight saving time, activations on either side of a clock change can be
// closer than this, down to one second apart. It returns 0 if the schedule
// never activates.
func (s *DefaultSchedule) MinInterval() time.Duration {
	if s.delay != 0 {
		return s.delay
	}
	return s.minInterval()
}

// calendarCycle is the number of days after which the Gregorian calendar,
// weekdays included, repeats: 400 years.
const calendarCycle = 146097

const secondsPerDay = 86400

func (s *DefaultSchedule) computeMinInterval() time.Duration {
	base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	firstSec, lastSec, secGap := scan(base, time.Second, secondsPerDay, func(t time.Time) bool {
		return s.hourMatch(t) && s.minuteMatch(t) && s.secondMatch(t)
	})
	firstDay, lastDay, dayGap := scan(base, 24*time.Hour, calendarCycle, func(t time.Time) bool {
		return s.monthMatch(t) && s.dayMatch(t)
	})
	if firstSec < 0 || firstDay < 0 {
		return 0
	}
	dayGap = min(dayGap, calendarCycle-lastDay+firstDay)
	return time.Duration(min(secGap, dayGap*secondsPerDay-lastSec+firstSec)) * time.Second
}

// scan checks n instants, step apart from start, and returns the indices of the
// first and last matching ones (-1 if none) and the smallest distance between
// two consecutive matching ones (math.MaxInt if fewer than two).
func scan(start time.Time, step time.Duration, n int, match func(time.Time) bool) (first, last, gap int) {
	first, last, gap = -1, -1, math.MaxInt
	t := start
	for i := 0; i < n; i++ {
		if match(t) {
			if first < 0 {
				first = i
			} else {
				gap = min(gap, i-last)
			}
			last = i
		}
		t = t.Add(step)
	}
	return first, last, gap
}
