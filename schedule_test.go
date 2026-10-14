package cron

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestActivation(t *testing.T) {
	tests := []struct {
		time, spec string
		expected   bool
	}{
		// Every fifteen minutes.
		{"Mon Jul 9 15:00 2012", "0/15 * * * *", true},
		{"Mon Jul 9 15:45 2012", "0/15 * * * *", true},
		{"Mon Jul 9 15:40 2012", "0/15 * * * *", false},

		// Every fifteen minutes, starting at 5 minutes.
		{"Mon Jul 9 15:05 2012", "5/15 * * * *", true},
		{"Mon Jul 9 15:20 2012", "5/15 * * * *", true},
		{"Mon Jul 9 15:50 2012", "5/15 * * * *", true},

		// Named months
		{"Sun Jul 15 15:00 2012", "0/15 * * Jul *", true},
		{"Sun Jul 15 15:00 2012", "0/15 * * Jun *", false},

		// Everything set.
		{"Sun Jul 15 08:30 2012", "30 08 ? Jul Sun", true},
		{"Sun Jul 15 08:30 2012", "30 08 15 Jul ?", true},
		{"Mon Jul 16 08:30 2012", "30 08 ? Jul Sun", false},
		{"Mon Jul 16 08:30 2012", "30 08 15 Jul ?", false},

		// Predefined schedules
		{"Mon Jul 9 15:00 2012", "@hourly", true},
		{"Mon Jul 9 15:04 2012", "@hourly", false},
		{"Mon Jul 9 15:00 2012", "@daily", false},
		{"Mon Jul 9 00:00 2012", "@daily", true},
		{"Mon Jul 9 00:00 2012", "@weekly", false},
		{"Sun Jul 8 00:00 2012", "@weekly", true},
		{"Sun Jul 8 01:00 2012", "@weekly", false},
		{"Sun Jul 8 00:00 2012", "@monthly", false},
		{"Sun Jul 1 00:00 2012", "@monthly", true},

		// Test interaction of DOW and DOM.
		// If both are restricted, then only one needs to match.
		{"Sun Jul 15 00:00 2012", "* * 1,15 * Sun", true},
		{"Fri Jun 15 00:00 2012", "* * 1,15 * Sun", true},
		{"Wed Aug 1 00:00 2012", "* * 1,15 * Sun", true},
		{"Sun Jul 15 00:00 2012", "* * */10 * Sun", true}, // verifies #70

		// However, if one has a star, then both need to match.
		{"Sun Jul 15 00:00 2012", "* * * * Mon", false},
		{"Mon Jul 9 00:00 2012", "* * 1,15 * *", false},
		{"Sun Jul 15 00:00 2012", "* * 1,15 * *", true},
		{"Sun Jul 15 00:00 2012", "* * */2 * Sun", true},

		// Last day of month
		{"Sun Jul 15 00:00 2012", "* * L * *", false},
		{"Tue Jul 31 00:00 2012", "* * L * *", true},
		{"Tue Jul 31 00:00 2012", "* * L * Mon", true},
		{"Tue Feb 1 00:00 2012", "* * L-28 2 *", true},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("now=%s,spec=%s", test.time, strings.Replace(test.spec, "/", "|", -1)), func(t *testing.T) {
			sched, err := standardParser.Parse(test.spec)
			if err != nil {
				t.Fatal(err)
			}
			actual := sched.Next(getTime(test.time).Add(-1 * time.Second))
			expected := getTime(test.time)
			if test.expected && expected != actual || !test.expected && expected == actual {
				t.Fatalf("Fail evaluating %s on %s: (expected) %s != %s (actual)",
					test.spec, test.time, expected, actual)
			}
		})
	}
}

func TestNext(t *testing.T) {
	runs := []struct {
		time, spec string
		expected   string
	}{
		// Simple cases
		{"Mon Jul 9 14:45 2012", "0 0/15 * * * *", "Mon Jul 9 15:00 2012"},
		{"Mon Jul 9 14:59 2012", "0 0/15 * * * *", "Mon Jul 9 15:00 2012"},
		{"Mon Jul 9 14:59:59 2012", "0 0/15 * * * *", "Mon Jul 9 15:00 2012"},

		// Wrap around hours
		{"Mon Jul 9 15:45 2012", "0 20-35/15 * * * *", "Mon Jul 9 16:20 2012"},

		// Wrap around days
		{"Mon Jul 9 23:46 2012", "0 */15 * * * *", "Tue Jul 10 00:00 2012"},
		{"Mon Jul 9 23:45 2012", "0 20-35/15 * * * *", "Tue Jul 10 00:20 2012"},
		{"Mon Jul 9 23:35:51 2012", "15/35 20-35/15 * * * *", "Tue Jul 10 00:20:15 2012"},
		{"Mon Jul 9 23:35:51 2012", "15/35 20-35/15 1/2 * * *", "Tue Jul 10 01:20:15 2012"},
		{"Mon Jul 9 23:35:51 2012", "15/35 20-35/15 10-12 * * *", "Tue Jul 10 10:20:15 2012"},

		{"Mon Jul 9 23:35:51 2012", "15/35 20-35/15 1/2 */2 * *", "Thu Jul 11 01:20:15 2012"},
		{"Mon Jul 9 23:35:51 2012", "15/35 20-35/15 * 9-20 * *", "Wed Jul 10 00:20:15 2012"},
		{"Mon Jul 9 23:35:51 2012", "15/35 20-35/15 * 9-20 Jul *", "Wed Jul 10 00:20:15 2012"},

		// Wrap around months
		{"Mon Jul 9 23:35 2012", "0 0 0 9 Apr-Oct ?", "Thu Aug 9 00:00 2012"},
		{"Mon Jul 9 23:35 2012", "0 0 0 */5 Apr,Aug,Oct Mon", "Tue Aug 1 00:00 2012"},
		{"Mon Jul 9 23:35 2012", "0 0 0 */5 Oct Mon", "Mon Oct 1 00:00 2012"},

		// Wrap around years
		{"Mon Jul 9 23:35 2012", "0 0 0 * Feb Mon", "Mon Feb 4 00:00 2013"},
		{"Mon Jul 9 23:35 2012", "0 0 0 * Feb Mon/2", "Fri Feb 1 00:00 2013"},

		// Wrap around minute, hour, day, month, and year
		{"Mon Dec 31 23:59:45 2012", "0 * * * * *", "Tue Jan 1 00:00:00 2013"},

		// Leap year
		{"Mon Jul 9 23:35 2012", "0 0 0 29 Feb ?", "Mon Feb 29 00:00 2016"},

		// Daylight savings time 2am EST (-5) -> 3am EDT (-4)
		{"2012-03-11T00:00:00-0500", "TZ=America/New_York 0 30 2 11 Mar ?", "2013-03-11T02:30:00-0400"},

		// hourly job
		{"2012-03-11T00:00:00-0500", "TZ=America/New_York 0 0 * * * ?", "2012-03-11T01:00:00-0500"},
		{"2012-03-11T01:00:00-0500", "TZ=America/New_York 0 0 * * * ?", "2012-03-11T03:00:00-0400"},
		{"2012-03-11T03:00:00-0400", "TZ=America/New_York 0 0 * * * ?", "2012-03-11T04:00:00-0400"},
		{"2012-03-11T04:00:00-0400", "TZ=America/New_York 0 0 * * * ?", "2012-03-11T05:00:00-0400"},

		// hourly job using CRON_TZ
		{"2012-03-11T00:00:00-0500", "CRON_TZ=America/New_York 0 0 * * * ?", "2012-03-11T01:00:00-0500"},
		{"2012-03-11T01:00:00-0500", "CRON_TZ=America/New_York 0 0 * * * ?", "2012-03-11T03:00:00-0400"},
		{"2012-03-11T03:00:00-0400", "CRON_TZ=America/New_York 0 0 * * * ?", "2012-03-11T04:00:00-0400"},
		{"2012-03-11T04:00:00-0400", "CRON_TZ=America/New_York 0 0 * * * ?", "2012-03-11T05:00:00-0400"},

		// 1am nightly job
		{"2012-03-11T00:00:00-0500", "TZ=America/New_York 0 0 1 * * ?", "2012-03-11T01:00:00-0500"},
		{"2012-03-11T01:00:00-0500", "TZ=America/New_York 0 0 1 * * ?", "2012-03-12T01:00:00-0400"},

		// 2am nightly job (skipped)
		{"2012-03-11T00:00:00-0500", "TZ=America/New_York 0 0 2 * * ?", "2012-03-12T02:00:00-0400"},

		// Daylight savings time 2am EDT (-4) => 1am EST (-5)
		{"2012-11-04T00:00:00-0400", "TZ=America/New_York 0 30 2 04 Nov ?", "2012-11-04T02:30:00-0500"},
		{"2012-11-04T01:45:00-0400", "TZ=America/New_York 0 30 1 04 Nov ?", "2012-11-04T01:30:00-0500"},

		// hourly job
		{"2012-11-04T00:00:00-0400", "TZ=America/New_York 0 0 * * * ?", "2012-11-04T01:00:00-0400"},
		{"2012-11-04T01:00:00-0400", "TZ=America/New_York 0 0 * * * ?", "2012-11-04T01:00:00-0500"},
		{"2012-11-04T01:00:00-0500", "TZ=America/New_York 0 0 * * * ?", "2012-11-04T02:00:00-0500"},

		// 1am nightly job (runs twice)
		{"2012-11-04T00:00:00-0400", "TZ=America/New_York 0 0 1 * * ?", "2012-11-04T01:00:00-0400"},
		{"2012-11-04T01:00:00-0400", "TZ=America/New_York 0 0 1 * * ?", "2012-11-04T01:00:00-0500"},
		{"2012-11-04T01:00:00-0500", "TZ=America/New_York 0 0 1 * * ?", "2012-11-05T01:00:00-0500"},

		// 2am nightly job
		{"2012-11-04T00:00:00-0400", "TZ=America/New_York 0 0 2 * * ?", "2012-11-04T02:00:00-0500"},
		{"2012-11-04T02:00:00-0500", "TZ=America/New_York 0 0 2 * * ?", "2012-11-05T02:00:00-0500"},

		// 3am nightly job
		{"2012-11-04T00:00:00-0400", "TZ=America/New_York 0 0 3 * * ?", "2012-11-04T03:00:00-0500"},
		{"2012-11-04T03:00:00-0500", "TZ=America/New_York 0 0 3 * * ?", "2012-11-05T03:00:00-0500"},

		// hourly job
		{"TZ=America/New_York 2012-11-04T00:00:00-0400", "0 0 * * * ?", "2012-11-04T01:00:00-0400"},
		{"TZ=America/New_York 2012-11-04T01:00:00-0400", "0 0 * * * ?", "2012-11-04T01:00:00-0500"},
		{"TZ=America/New_York 2012-11-04T01:00:00-0500", "0 0 * * * ?", "2012-11-04T02:00:00-0500"},

		// 1am nightly job (runs twice)
		{"TZ=America/New_York 2012-11-04T00:00:00-0400", "0 0 1 * * ?", "2012-11-04T01:00:00-0400"},
		{"TZ=America/New_York 2012-11-04T01:00:00-0400", "0 0 1 * * ?", "2012-11-04T01:00:00-0500"},
		{"TZ=America/New_York 2012-11-04T01:00:00-0500", "0 0 1 * * ?", "2012-11-05T01:00:00-0500"},

		// 2am nightly job
		{"TZ=America/New_York 2012-11-04T00:00:00-0400", "0 0 2 * * ?", "2012-11-04T02:00:00-0500"},
		{"TZ=America/New_York 2012-11-04T02:00:00-0500", "0 0 2 * * ?", "2012-11-05T02:00:00-0500"},

		// 3am nightly job
		{"TZ=America/New_York 2012-11-04T00:00:00-0400", "0 0 3 * * ?", "2012-11-04T03:00:00-0500"},
		{"TZ=America/New_York 2012-11-04T03:00:00-0500", "0 0 3 * * ?", "2012-11-05T03:00:00-0500"},

		// Unsatisfiable
		{"Mon Jul 9 23:35 2012", "0 0 0 30 Feb ?", ""},
		{"Mon Jul 9 23:35 2012", "0 0 0 31 Apr ?", ""},

		// Monthly job
		{"TZ=America/New_York 2012-11-04T00:00:00-0400", "0 0 3 3 * ?", "2012-12-03T03:00:00-0500"},

		// Test the scenario of DST resulting in midnight not being a valid time.
		// https://github.com/robfig/cron/issues/157
		{"2018-10-17T05:00:00-0400", "TZ=America/Sao_Paulo 0 0 9 10 * ?", "2018-11-10T06:00:00-0500"},
		{"2018-02-14T05:00:00-0500", "TZ=America/Sao_Paulo 0 0 9 22 * ?", "2018-02-22T07:00:00-0500"},

		// Last day of month
		{"Tue Jan 2 23:35 2024", "0 0 0 L * *", "Wed Jan 31 00:00 2024"},
		{"Fri Feb 2 23:35 2024", "0 0 0 L * *", "Thu Feb 29 00:00 2024"},
		{"Thu Feb 2 23:35 2023", "0 0 0 L * *", "Tue Feb 28 00:00 2023"},
		{"Sat Nov 2 23:35 2024", "0 0 0 L * *", "Sat Nov 30 00:00 2024"},
		{"Sat Nov 30 18:25 2024", "0 20 18 L * *", "Tue Dec 31 18:20 2024"},
		{"Sat Nov 30 18:25 2024", "0 20 18 L-1 * *", "Tue Dec 30 18:20 2024"},
		{"Sat Nov 30 18:25 2024", "0 20 18 L-2 * *", "Tue Dec 29 18:20 2024"},
		{"Tue Jan 2 23:35 2024", "0 0 0 L,10 * *", "Wed Jan 10 00:00 2024"},
		{"Tue Jan 11 23:35 2024", "0 0 0 L,10 * *", "Wed Jan 31 00:00 2024"},
	}

	for _, c := range runs {
		t.Run(fmt.Sprintf("now=%s,spec=%s", c.time, strings.Replace(c.spec, "/", "|", -1)), func(t *testing.T) {
			sched, err := secondParser.Parse(c.spec)
			if err != nil {
				t.Fatal(err)
			}
			actual := sched.Next(getTime(c.time))
			expected := getTime(c.expected)
			if !actual.Equal(expected) {
				t.Fatalf("%s, \"%s\": (expected) %v != %v (actual)", c.time, c.spec, expected, actual)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	invalidSpecs := []string{
		"xyz",
		"60 0 * * *",
		"0 60 * * *",
		"0 0 * * XYZ",
	}
	for _, spec := range invalidSpecs {
		t.Run(strings.Replace(spec, "/", "|", -1), func(t *testing.T) {
			_, err := standardParser.Parse(spec)
			if err == nil {
				t.Fatal("expected an error parsing: ", spec)
			}
		})
	}
}

func getTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}

	var location = time.Local
	if strings.HasPrefix(value, "TZ=") {
		parts := strings.Fields(value)
		loc, err := time.LoadLocation(parts[0][len("TZ="):])
		if err != nil {
			panic("could not parse location:" + err.Error())
		}
		location = loc
		value = parts[1]
	}

	var layouts = []string{
		"Mon Jan 2 15:04 2006",
		"Mon Jan 2 15:04:05 2006",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, location); err == nil {
			return t
		}
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05-0700", value, location); err == nil {
		return t
	}
	panic("could not parse time value " + value)
}

func TestNextWithTz(t *testing.T) {
	runs := []struct {
		time, spec string
		expected   string
	}{
		// Failing tests
		{"2016-01-03T13:09:03+0530", "14 14 * * *", "2016-01-03T14:14:00+0530"},
		{"2016-01-03T04:09:03+0530", "14 14 * * ?", "2016-01-03T14:14:00+0530"},

		// Passing tests
		{"2016-01-03T14:09:03+0530", "14 14 * * *", "2016-01-03T14:14:00+0530"},
		{"2016-01-03T14:00:00+0530", "14 14 * * ?", "2016-01-03T14:14:00+0530"},
	}
	for _, c := range runs {
		t.Run(fmt.Sprintf("now=%s,spec=%s", c.time, strings.Replace(c.spec, "/", "|", -1)), func(t *testing.T) {
			sched, err := standardParser.Parse(c.spec)
			if err != nil {
				t.Fatal(err)
			}
			actual := sched.Next(getTimeTZ(c.time))
			expected := getTimeTZ(c.expected)
			if !actual.Equal(expected) {
				t.Fatalf("%s, \"%s\": (expected) %v != %v (actual)", c.time, c.spec, expected, actual)
			}
		})
	}
}

func getTimeTZ(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse("Mon Jan 2 15:04 2006", value)
	if err != nil {
		t, err = time.Parse("Mon Jan 2 15:04:05 2006", value)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05-0700", value)
			if err != nil {
				panic(err)
			}
		}
	}

	return t
}

// https://github.com/robfig/cron/issues/144
func TestSlash0NoHang(t *testing.T) {
	schedule := "TZ=America/New_York 15/0 * * * *"
	_, err := standardParser.Parse(schedule)
	if err == nil {
		t.Fatal("expected an error on 0 increment")
	}
}

func TestWithLocation(t *testing.T) {
	sched, err := secondParser.Parse("CRON_TZ=UTC 0 0 12 * * ?")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2012, 7, 9, 0, 0, 0, 0, time.UTC)

	next1 := sched.Next(now)
	if expected := time.Date(2012, 7, 9, 12, 0, 0, 0, time.UTC); !next1.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next1)
	}

	sched2 := sched.WithLocation(time.FixedZone("UTC+2", 2*60*60))
	next2 := sched2.Next(now)
	if expected := time.Date(2012, 7, 9, 10, 0, 0, 0, time.UTC); !next2.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next2)
	}
}

func TestWithLocationNil(t *testing.T) {
	sched, err := secondParser.Parse("0 0 12 * * ?")
	if err != nil {
		t.Fatal(err)
	}

	sched2 := sched.WithLocation(nil)
	if sched2 != sched {
		t.Fatalf("expected nil location to return same schedule")
	}
}

func TestMinInterval(t *testing.T) {
	tests := []struct {
		spec     string
		expected time.Duration
	}{
		// Constant delay
		{"@every 4s", 4 * time.Second},
		{"@every 1h30m", 90 * time.Minute},

		// Within a minute
		{"* * * * * *", time.Second},
		{"*/5 * * * * *", 5 * time.Second},
		{"0,3 0 0 29 2 *", 3 * time.Second},
		{"10,20-22/2,40 * * * * *", 2 * time.Second},

		// Across minutes
		{"0,57 * * * * *", 3 * time.Second},
		{"0,57 10,11 * * * *", 3 * time.Second},
		{"0,57 */2 * * * *", 57 * time.Second},
		{"0,30 */2 * * * *", 30 * time.Second},
		{"0 * * * * *", time.Minute},
		{"0 */15 * * * *", 15 * time.Minute},
		{"0 10,50 * * * *", 20 * time.Minute},

		// Across hours
		{"0,57 59,0 * * * *", 3 * time.Second},
		{"0,57 59,0 */2 * * *", 57 * time.Second},
		{"57 59,0 */2 * * *", 59 * time.Minute},
		{"@hourly", time.Hour},
		{"0 30 8,20 * * *", 12 * time.Hour},

		// Across days
		{"0,57 59,0 23,0 * * *", 3 * time.Second},
		{"0 0 22,2 * * *", 4 * time.Hour},
		{"@daily", 24 * time.Hour},
		{"0 0 9 * * 1,4", 3 * 24 * time.Hour},
		{"@weekly", 7 * 24 * time.Hour},
		{"0 0 0 L * 5", 24 * time.Hour},
		{"0 0 0 1,L * *", 24 * time.Hour},

		// Across months and years
		{"@monthly", 28 * 24 * time.Hour},
		{"0 0 0 L * *", 28 * 24 * time.Hour},
		{"0 0 12 */7 * *", 24 * time.Hour},
		{"0 0 0 31 1,3 *", 59 * 24 * time.Hour},
		{"@yearly", 365 * 24 * time.Hour},
		{"0 0 0 29 2 *", 1461 * 24 * time.Hour},

		// Relative to the end of the month, nearest weekday, nth weekday
		{"0 0 0 L-4 * *", 28 * 24 * time.Hour},
		{"0 0 0 L-28 * *", 29 * 24 * time.Hour},
		{"0 0 0 * * 5L", 28 * 24 * time.Hour},
		{"0 0 0 * * 5#1", 28 * 24 * time.Hour},
		{"0 0 0 * * 5#4,5L", 7 * 24 * time.Hour},
		{"0 0 0 1W * *", 28 * 24 * time.Hour},
		{"0 0 0 31W * *", 29 * 24 * time.Hour},
		{"0 0 0 LW * *", 28 * 24 * time.Hour},
		{"0 0 0 1W,LW * *", 24 * time.Hour},

		// Never activates
		{"0 0 0 30 2 *", 0},
		{"0 0 0 30W 2 *", 0},

		// Location offset is assumed constant
		{"TZ=Asia/Tokyo 0 30 04 * * *", 24 * time.Hour},
		{"TZ=America/New_York 0 30 1 * * *", 24 * time.Hour},
	}

	for _, test := range tests {
		sched, err := secondParser.Parse(test.spec)
		if err != nil {
			t.Error(err)
			continue
		}
		actual := sched.MinInterval()
		if actual != test.expected {
			t.Errorf("%s: (expected) %v != %v (actual)", test.spec, test.expected, actual)
		}
	}
}

func TestMinIntervalTimeOfDay(t *testing.T) {
	fields := []string{"0", "0,59", "*/7", "5-9,30", "1,2,58"}
	hours := []string{"0", "0,23", "*/5", "3,4,22"}
	for _, second := range fields {
		for _, minute := range fields {
			for _, hour := range hours {
				spec := fmt.Sprintf("%s %s %s * * *", second, minute, hour)
				sched, err := secondParser.Parse(spec)
				if err != nil {
					t.Fatal(err)
				}
				expected := scan(scanStart, time.Second, int64(48*time.Hour/time.Second), func(t time.Time) bool {
					return sched.hourMatch(t) && sched.minuteMatch(t) && sched.secondMatch(t)
				}).gap
				actual := sched.MinInterval()
				if actual != time.Duration(expected)*time.Second {
					t.Errorf("%s: (expected) %vs != %v (actual)", spec, expected, actual)
				}
			}
		}
	}
}

func TestMinIntervalConcurrent(t *testing.T) {
	sched, err := secondParser.Parse("0 0 0 L * *")
	if err != nil {
		t.Fatal(err)
	}
	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	copied := sched.WithLocation(nyc)

	var wg sync.WaitGroup
	results := make([]time.Duration, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				results[i] = sched.MinInterval()
			} else {
				results[i] = copied.MinInterval()
			}
		}(i)
	}
	wg.Wait()

	for i, actual := range results {
		if actual != 28*24*time.Hour {
			t.Errorf("call %d: (expected) %v != %v (actual)", i, 28*24*time.Hour, actual)
		}
	}
}

func BenchmarkMinInterval(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sched, err := secondParser.Parse("0 0 12 */7 * *")
		if err != nil {
			b.Fatal(err)
		}
		sched.MinInterval()
	}
}
