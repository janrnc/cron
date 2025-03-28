package parser

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdgvda/cron/internal/matcher"
)

var dowToInt = map[string]uint{
	"sun": 0,
	"mon": 1,
	"tue": 2,
	"wed": 3,
	"thu": 4,
	"fri": 5,
	"sat": 6,
}

func ParseDow(expression string) (matcher.Matcher, error) {
	options, err := splitOptions(expression)
	if err != nil {
		return nil, err
	}
	matches := []matcher.Matcher{}
	for _, option := range options {
		match, err := parseDow(option)
		if err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	return matcher.Or(matches...), nil
}

func parseDow(expression string) (matcher.Matcher, error) {
	rangeAndStep := strings.Split(expression, "/")
	lowAndHigh := strings.Split(rangeAndStep[0], "-")

	if len(lowAndHigh) > 2 || len(rangeAndStep) > 2 {
		return nil, fmt.Errorf("%s: invalid expression", expression)
	}

	if lowAndHigh[0] == "*" || lowAndHigh[0] == "?" {
		if len(lowAndHigh) > 1 {
			return nil, fmt.Errorf("%s: invalid expression", expression)
		}
		lowAndHigh[0] = "0-6"
	} else {
		if !strings.HasSuffix(lowAndHigh[0], "L") && len(lowAndHigh) == 1 && len(rangeAndStep) == 2 {
			lowAndHigh[0] += "-6"
		}
	}

	if lowAndHigh[0] == "L" {
		if len(lowAndHigh) > 1 {
			return nil, fmt.Errorf("%s: invalid expression", expression)
		}
		lowAndHigh[0] = "6"
	}
	if strings.HasSuffix(lowAndHigh[0], "L") {
		if len(lowAndHigh) > 1 || len(rangeAndStep) > 1 {
			return nil, fmt.Errorf("%s: invalid expression", expression)
		}
		dow, err := parseIntOrName(strings.TrimSuffix(lowAndHigh[0], "L"), dowToInt)
		if err != nil {
			return nil, err
		}
		if dow > 6 {
			return nil, fmt.Errorf("%s: value %d out of valid range [0, 6]", expression, dow)
		}
		return func(t time.Time) bool {
			next := t.AddDate(0, 0, 7)
			return next.Month() != t.Month() && t.Weekday() == time.Weekday(dow)
		}, nil
	}

	if strings.Contains(lowAndHigh[0], "#") {
		if len(lowAndHigh) > 1 || len(rangeAndStep) > 1 {
			return nil, fmt.Errorf("%s: invalid expression", expression)
		}
		dowAndOccurrence := strings.Split(lowAndHigh[0], "#")
		if len(dowAndOccurrence) != 2 {
			return nil, fmt.Errorf("%s: invalid expression", expression)
		}
		dow, err := parseIntOrName(dowAndOccurrence[0], dowToInt)
		if err != nil {
			return nil, err
		}
		if dow > 6 {
			return nil, fmt.Errorf("%s: value %d out of valid range [0, 6]", expression, dow)
		}
		occurrence, err := mustParseInt(dowAndOccurrence[1])
		if err != nil {
			return nil, err
		}
		if occurrence < 1 || occurrence > 5 {
			return nil, fmt.Errorf("%s: value %d out of valid range [1, 5]", expression, occurrence)
		}
		return func(t time.Time) bool {
			start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
			end := start.AddDate(0, 1, -1)
			for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
				if d.Weekday() == time.Weekday(dow) {
					return t.Day() == d.AddDate(0, 0, 7*(int(occurrence)-1)).Day()
				}
			}
			return false
		}, nil
	}

	expression = strings.Join(lowAndHigh, "-")
	if len(rangeAndStep) > 1 {
		expression += "/" + rangeAndStep[1]
	}

	activations, err := span(expression, 0, 6, dowToInt)
	if err != nil {
		return nil, err
	}

	return func(t time.Time) bool {
		for _, dow := range activations {
			if t.Weekday() == time.Weekday(dow) {
				return true
			}
		}
		return false
	}, nil
}
