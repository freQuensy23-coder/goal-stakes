package domain

import (
	"testing"
	"time"
)

// All period math is UTC-based for v1 (see CurrentPeriod doc). These tests pin
// the stable string keys and the [start,end) bounds that the rest of the
// system relies on (AS4: deadline-based detection for "do" goals).

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

func TestCurrentPeriodDaily(t *testing.T) {
	g := Goal{Cadence: CadenceDaily}

	cases := []struct {
		name string
		now  string
		want Period
	}{
		{"start of day UTC", "2026-05-25T00:00:00Z", "2026-05-25"},
		{"mid day", "2026-05-25T13:45:10Z", "2026-05-25"},
		{"last second of day", "2026-05-25T23:59:59Z", "2026-05-25"},
		{"non-UTC input normalized to UTC", "2026-05-25T23:30:00-02:00", "2026-05-26"},
		{"leap-ish boundary new year", "2025-12-31T23:59:59Z", "2025-12-31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := g.CurrentPeriod(mustParse(t, tc.now))
			if got != tc.want {
				t.Fatalf("CurrentPeriod=%q want %q", got, tc.want)
			}
		})
	}
}

func TestPeriodBoundsDaily(t *testing.T) {
	g := Goal{Cadence: CadenceDaily}
	start, end, err := g.PeriodBounds("2026-05-25")
	if err != nil {
		t.Fatalf("PeriodBounds: %v", err)
	}
	wantStart := mustParse(t, "2026-05-25T00:00:00Z")
	wantEnd := mustParse(t, "2026-05-26T00:00:00Z")
	if !start.Equal(wantStart) {
		t.Fatalf("start=%s want %s", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Fatalf("end=%s want %s", end, wantEnd)
	}
	// end is exclusive: now == end belongs to the NEXT period.
	if got := g.CurrentPeriod(end); got == "2026-05-25" {
		t.Fatalf("end instant must roll to next period, got %q", got)
	}
}

func TestDailyPeriodUsesGoalTimezone(t *testing.T) {
	g := Goal{Cadence: CadenceDaily, Timezone: "America/New_York"}
	if got := g.CurrentPeriod(mustParse(t, "2026-05-25T03:30:00Z")); got != "2026-05-24" {
		t.Fatalf("CurrentPeriod in New York = %q, want local previous day", got)
	}

	start, end, err := g.PeriodBounds("2026-05-25")
	if err != nil {
		t.Fatalf("PeriodBounds: %v", err)
	}
	wantStart := mustParse(t, "2026-05-25T04:00:00Z")
	wantEnd := mustParse(t, "2026-05-26T04:00:00Z")
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("bounds=[%s,%s) want [%s,%s)", start, end, wantStart, wantEnd)
	}
}

func TestCurrentPeriodWeeklyISO(t *testing.T) {
	g := Goal{Cadence: CadenceWeekly}

	cases := []struct {
		name string
		now  string
		want Period
	}{
		// 2026-05-25 is a Monday; ISO week 22 of 2026.
		{"monday start of ISO week", "2026-05-25T00:00:00Z", "2026-W22"},
		{"sunday end of ISO week", "2026-05-31T23:59:59Z", "2026-W22"},
		{"next monday rolls week", "2026-06-01T00:00:00Z", "2026-W23"},
		// ISO week-year edge: 2025-12-29 (Mon) is ISO week 1 of 2026.
		{"iso year differs from calendar year", "2025-12-29T12:00:00Z", "2026-W01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := g.CurrentPeriod(mustParse(t, tc.now))
			if got != tc.want {
				t.Fatalf("CurrentPeriod=%q want %q", got, tc.want)
			}
		})
	}
}

func TestWeeklyPeriodUsesGoalTimezone(t *testing.T) {
	g := Goal{Cadence: CadenceWeekly, Timezone: "America/New_York"}
	if got := g.CurrentPeriod(mustParse(t, "2026-06-01T03:30:00Z")); got != "2026-W22" {
		t.Fatalf("CurrentPeriod in New York = %q, want local previous ISO week", got)
	}

	start, end, err := g.PeriodBounds("2026-W22")
	if err != nil {
		t.Fatalf("PeriodBounds: %v", err)
	}
	wantStart := mustParse(t, "2026-05-25T04:00:00Z")
	wantEnd := mustParse(t, "2026-06-01T04:00:00Z")
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("bounds=[%s,%s) want [%s,%s)", start, end, wantStart, wantEnd)
	}
}

func TestPeriodBoundsWeekly(t *testing.T) {
	g := Goal{Cadence: CadenceWeekly}
	start, end, err := g.PeriodBounds("2026-W22")
	if err != nil {
		t.Fatalf("PeriodBounds: %v", err)
	}
	// ISO week 22 of 2026 starts Monday 2026-05-25 00:00 UTC.
	wantStart := mustParse(t, "2026-05-25T00:00:00Z")
	wantEnd := mustParse(t, "2026-06-01T00:00:00Z")
	if !start.Equal(wantStart) {
		t.Fatalf("start=%s want %s", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Fatalf("end=%s want %s", end, wantEnd)
	}
	if int(end.Sub(start).Hours()) != 7*24 {
		t.Fatalf("weekly span must be 7 days, got %s", end.Sub(start))
	}
}

func TestPeriodBoundsRoundTrip(t *testing.T) {
	// For any instant, the period derived from `now` must contain `now`
	// within its [start,end) bounds. Guards against off-by-one in either fn.
	instants := []string{
		"2026-05-25T00:00:00Z",
		"2026-05-25T23:59:59Z",
		"2025-12-31T23:59:59Z",
		"2026-01-01T00:00:00Z",
	}
	for _, cad := range []Cadence{CadenceDaily, CadenceWeekly} {
		g := Goal{Cadence: cad}
		for _, s := range instants {
			now := mustParse(t, s)
			p := g.CurrentPeriod(now)
			start, end, err := g.PeriodBounds(p)
			if err != nil {
				t.Fatalf("[%s %s] PeriodBounds(%q): %v", cad, s, p, err)
			}
			if now.Before(start) || !now.Before(end) {
				t.Fatalf("[%s] now=%s not in [%s,%s) for period %q", cad, now, start, end, p)
			}
		}
	}
}

func TestPeriodBoundsInvalid(t *testing.T) {
	// GPC6: bad input must error, not silently default.
	cases := []struct {
		cad Cadence
		p   Period
	}{
		{CadenceDaily, "not-a-date"},
		{CadenceDaily, "2026-W21"},    // weekly key for daily cadence
		{CadenceWeekly, "2026-05-25"}, // daily key for weekly cadence
		{CadenceWeekly, "2026-W99"},   // impossible week
		{CadenceCustom, "2026-05-25"}, // custom not supported for v1 bounds
	}
	for _, tc := range cases {
		g := Goal{Cadence: tc.cad}
		if _, _, err := g.PeriodBounds(tc.p); err == nil {
			t.Fatalf("cadence=%s period=%q expected error, got nil", tc.cad, tc.p)
		}
	}
}

func TestCurrentPeriodCustomPanicsOrEmpty(t *testing.T) {
	// Custom cadence has no automatic period in v1; CurrentPeriod returns "".
	g := Goal{Cadence: CadenceCustom}
	if got := g.CurrentPeriod(mustParse(t, "2026-05-25T00:00:00Z")); got != "" {
		t.Fatalf("custom cadence CurrentPeriod=%q want empty", got)
	}
}
