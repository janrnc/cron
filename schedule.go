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

func (s *DefaultSchedule) computeMinInterval() time.Duration {
	days := scan(scanStart, 24*time.Hour, calendarCycle, func(t time.Time) bool {
		return s.monthMatch(t) && s.dayMatch(t)
	})
	seconds := scan(scanStart, time.Second, 60, s.secondMatch)
	minutes := scan(scanStart, time.Minute, 60, s.minuteMatch)
	hours := scan(scanStart, time.Hour, 24, s.hourMatch)
	timeOfDay := hours.nest(minutes.nest(seconds))
	if days.first < 0 || timeOfDay.first < 0 {
		return 0
	}
	days.gap = min(days.gap, calendarCycle-days.last+days.first)
	nested := days.nest(timeOfDay)
	return time.Duration(nested.gap) * nested.unit
}

var scanStart = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// span describes the matching indices of a scan, counted in unit: the first
// and last ones (-1 if none) and the smallest distance between two
// consecutive ones (math.MaxInt64 if fewer than two).
type span struct {
	first, last, gap int64
	unit             time.Duration
}

// nest combines s with inner, the span within each unit of s, into a span
// counted in the unit of inner. It assumes inner is the same in every unit of
// s, which holds for independent fields such as hours, minutes and seconds.
// It does not hold for days within months or years, as month lengths, leap
// years and weekdays change the matching days: nesting those needs a
// different strategy.
func (s span) nest(inner span) span {
	if s.first < 0 || inner.first < 0 {
		return span{-1, -1, math.MaxInt64, inner.unit}
	}
	ratio := int64(s.unit / inner.unit)
	nested := span{s.first*ratio + inner.first, s.last*ratio + inner.last, inner.gap, inner.unit}
	if s.gap != math.MaxInt64 {
		nested.gap = min(nested.gap, s.gap*ratio-inner.last+inner.first)
	}
	return nested
}

// scan checks n instants, step apart from start.
func scan(start time.Time, step time.Duration, n int64, match func(time.Time) bool) span {
	s := span{-1, -1, math.MaxInt64, step}
	t := start
	for i := int64(0); i < n; i++ {
		if match(t) {
			if s.first < 0 {
				s.first = i
			} else {
				s.gap = min(s.gap, i-s.last)
			}
			s.last = i
		}
		t = t.Add(step)
	}
	return s
}
