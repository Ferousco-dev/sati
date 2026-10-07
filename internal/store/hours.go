package store

import (
	"fmt"
	"regexp"
	"time"
)

var weekdayKeys = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

var clockPattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

type DayHours struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

// Hours is the weekly schedule keyed by mon..sun. A missing or null day is closed.
type Hours map[string]*DayHours

func (h Hours) day(t time.Time) *DayHours { return h[weekdayKeys[t.Weekday()]] }

// Normalize returns a copy with all seven days present, null where the store is closed.
func (h Hours) Normalize() Hours {
	out := Hours{}
	for _, k := range weekdayKeys {
		out[k] = h[k]
	}
	return out
}

func (h Hours) Validate() error {
	known := map[string]bool{}
	for _, k := range weekdayKeys {
		known[k] = true
	}
	for k, d := range h {
		if !known[k] {
			return fmt.Errorf("unknown day %q, use mon, tue, wed, thu, fri, sat or sun", k)
		}
		if d == nil {
			continue
		}
		if !clockPattern.MatchString(d.Open) || !clockPattern.MatchString(d.Close) {
			return fmt.Errorf("%s: times must look like 09:00", k)
		}
		if d.Open >= d.Close {
			return fmt.Errorf("%s: closing time must be after opening time", k)
		}
	}
	return nil
}

func at(day time.Time, clock string, loc *time.Location) time.Time {
	var hh, mm int
	fmt.Sscanf(clock, "%d:%d", &hh, &mm)
	return time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, loc)
}

// OpenStatus says whether the store is open at a moment, and when that next changes.
type OpenStatus struct {
	Open       bool       `json:"open_now"`
	NextChange *time.Time `json:"next_change_at"`
	NextKind   string     `json:"next_change"`
}

// Status works out the opening state in the store's timezone. A manual
// "temporarily closed" switch overrides the schedule.
func (h Hours) Status(now time.Time, loc *time.Location, temporarilyClosed bool) OpenStatus {
	local := now.In(loc)

	if !temporarilyClosed {
		if d := h.day(local); d != nil {
			opens, closes := at(local, d.Open, loc), at(local, d.Close, loc)
			if !local.Before(opens) && local.Before(closes) {
				return OpenStatus{Open: true, NextChange: &closes, NextKind: "closes"}
			}
		}
	}

	for i := 0; i <= 7; i++ {
		day := local.AddDate(0, 0, i)
		d := h.day(day)
		if d == nil {
			continue
		}
		if opens := at(day, d.Open, loc); opens.After(local) {
			return OpenStatus{NextChange: &opens, NextKind: "opens"}
		}
	}
	return OpenStatus{}
}
