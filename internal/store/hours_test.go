package store

import (
	"testing"
	"time"
)

func lagos(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func schedule() Hours {
	return Hours{
		"mon": {Open: "09:00", Close: "21:00"},
		"tue": {Open: "09:00", Close: "21:00"},
		"sat": {Open: "10:00", Close: "18:00"},
		"sun": nil,
	}
}

func TestStatus(t *testing.T) {
	loc := lagos(t)
	// 2026-10-05 is a Monday.
	at := func(day, clock string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", day+" "+clock, loc)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	tests := []struct {
		name     string
		now      time.Time
		closed   bool
		wantOpen bool
		kind     string
		change   time.Time
	}{
		{"open mid-day", at("2026-10-05", "12:00"), false, true, "closes", at("2026-10-05", "21:00")},
		{"exactly at opening", at("2026-10-05", "09:00"), false, true, "closes", at("2026-10-05", "21:00")},
		{"exactly at closing", at("2026-10-05", "21:00"), false, false, "opens", at("2026-10-06", "09:00")},
		{"before opening", at("2026-10-05", "07:30"), false, false, "opens", at("2026-10-05", "09:00")},
		{"after closing opens next day", at("2026-10-05", "22:00"), false, false, "opens", at("2026-10-06", "09:00")},
		{"wednesday has no hours, next is saturday", at("2026-10-07", "12:00"), false, false, "opens", at("2026-10-10", "10:00")},
		{"sunday closed, next is monday", at("2026-10-11", "12:00"), false, false, "opens", at("2026-10-12", "09:00")},
		{"temporary closure overrides the schedule", at("2026-10-05", "12:00"), true, false, "opens", at("2026-10-06", "09:00")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schedule().Status(tt.now, loc, tt.closed)
			if got.Open != tt.wantOpen || got.NextKind != tt.kind || got.NextChange == nil || !got.NextChange.Equal(tt.change) {
				t.Fatalf("got %+v, want open=%v %s at %v", got, tt.wantOpen, tt.kind, tt.change)
			}
		})
	}
}

func TestStatusUsesTheStoreTimezoneNotUTC(t *testing.T) {
	loc := lagos(t)
	// 20:30 UTC is 21:30 in Lagos, after closing, even though the UTC clock says 20:30.
	now := time.Date(2026, 10, 5, 20, 30, 0, 0, time.UTC)
	if schedule().Status(now, loc, false).Open {
		t.Fatal("the store closed at 21:00 Lagos time")
	}
	// 08:30 UTC is 09:30 in Lagos, so the store is open although the UTC clock is before 09:00.
	now = time.Date(2026, 10, 5, 8, 30, 0, 0, time.UTC)
	if !schedule().Status(now, loc, false).Open {
		t.Fatal("the store opened at 09:00 Lagos time")
	}
}

func TestNoHoursMeansNeverOpen(t *testing.T) {
	got := Hours{}.Status(time.Now(), lagos(t), false)
	if got.Open || got.NextChange != nil {
		t.Fatalf("%+v", got)
	}
}

func TestValidate(t *testing.T) {
	if err := schedule().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]Hours{
		"unknown day":       {"monday": {Open: "09:00", Close: "10:00"}},
		"bad time":          {"mon": {Open: "9am", Close: "10:00"}},
		"hour out of range": {"mon": {Open: "09:00", Close: "25:00"}},
		"closes before":     {"mon": {Open: "10:00", Close: "09:00"}},
		"same time":         {"mon": {Open: "10:00", Close: "10:00"}},
	}
	for name, h := range bad {
		if err := h.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestNormalizeFillsAllDays(t *testing.T) {
	n := Hours{"mon": {Open: "09:00", Close: "10:00"}}.Normalize()
	if len(n) != 7 || n["tue"] != nil || n["mon"] == nil {
		t.Fatalf("%+v", n)
	}
}
