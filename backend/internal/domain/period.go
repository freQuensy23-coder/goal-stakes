package domain

import (
	"fmt"
	"strings"
	"time"
)

// Period is a stable, human-readable key identifying one cadence window of a
// goal. It is the unit of uniqueness for check-ins and violations (IV6).
//
// Encoding by cadence:
//   - Daily:  "2006-01-02"  e.g. "2026-05-25"
//   - Weekly: "YYYY-Www"    ISO-8601 week, e.g. "2026-W21"
//   - Custom: not supported in v1 (empty / error)
//
// Period keys are computed in the goal's IANA timezone when set, otherwise UTC.
// Bounds are returned as UTC instants so scheduler and store comparisons stay
// unambiguous.
type Period string

const dailyLayout = "2006-01-02"

// CurrentPeriod returns the Period that the instant `now` falls into for this
// goal's cadence in the goal timezone. For CadenceCustom it returns "" (no
// automatic period in v1).
func (g Goal) CurrentPeriod(now time.Time) Period {
	now = now.In(g.periodLocationOrUTC())
	switch g.Cadence {
	case CadenceDaily:
		return Period(now.Format(dailyLayout))
	case CadenceWeekly:
		year, week := now.ISOWeek()
		return Period(fmt.Sprintf("%04d-W%02d", year, week))
	default:
		return ""
	}
}

// PeriodBounds returns the half-open [start, end) UTC bounds of period `p` for
// this goal's cadence and timezone. `end` is exclusive: an instant equal to
// `end` belongs to the next period. It errors (GPC6) on a malformed key, a key
// that does not match the cadence, an invalid timezone, or CadenceCustom.
func (g Goal) PeriodBounds(p Period) (start, end time.Time, err error) {
	loc, err := g.periodLocation()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	switch g.Cadence {
	case CadenceDaily:
		return dailyBounds(p, loc)
	case CadenceWeekly:
		return weeklyBounds(p, loc)
	case CadenceCustom:
		return time.Time{}, time.Time{}, errInvalidPeriod("custom cadence has no automatic period bounds")
	default:
		return time.Time{}, time.Time{}, errInvalidPeriod("unknown cadence %q", g.Cadence)
	}
}

func (g Goal) periodLocationOrUTC() *time.Location {
	loc, err := g.periodLocation()
	if err != nil {
		return time.UTC
	}
	return loc
}

func (g Goal) periodLocation() (*time.Location, error) {
	tz := strings.TrimSpace(g.Timezone)
	if tz == "" || strings.EqualFold(tz, "UTC") {
		return time.UTC, nil
	}
	if tz == "Local" {
		return nil, errInvalidPeriod("invalid timezone %q", tz)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, errInvalidPeriod("invalid timezone %q: %w", tz, err)
	}
	return loc, nil
}

func dailyBounds(p Period, loc *time.Location) (start, end time.Time, err error) {
	start, perr := time.ParseInLocation(dailyLayout, string(p), loc)
	if perr != nil {
		return time.Time{}, time.Time{}, errInvalidPeriod("invalid daily period %q: %w", p, perr)
	}
	end = start.AddDate(0, 0, 1)
	return start.UTC(), end.UTC(), nil
}

func weeklyBounds(p Period, loc *time.Location) (start, end time.Time, err error) {
	var year, week int
	// %d stops at the first non-digit, so a trailing garbage like "2026-W21x"
	// is rejected by the length check below.
	if n, perr := fmt.Sscanf(string(p), "%04d-W%02d", &year, &week); perr != nil || n != 2 {
		return time.Time{}, time.Time{}, errInvalidPeriod("invalid weekly period %q", p)
	}
	if len(p) != len("2026-W21") {
		return time.Time{}, time.Time{}, errInvalidPeriod("invalid weekly period %q: wrong length", p)
	}
	if week < 1 || week > 53 {
		return time.Time{}, time.Time{}, errInvalidPeriod("invalid weekly period %q: week out of range", p)
	}
	start = isoWeekStart(year, week, loc)
	end = start.AddDate(0, 0, 7)
	// Round-trip guard: re-deriving the ISO week of `start` must yield the same
	// (year, week). This rejects impossible weeks (e.g. W53 of a 52-week year),
	// which time arithmetic would otherwise silently roll forward (GPC6).
	gotYear, gotWeek := start.In(loc).ISOWeek()
	if gotYear != year || gotWeek != week {
		return time.Time{}, time.Time{}, errInvalidPeriod("invalid weekly period %q: no such ISO week", p)
	}
	return start.UTC(), end.UTC(), nil
}

// isoWeekStart returns 00:00:00 local time on the Monday of the given ISO week-year.
// ISO-8601: week 1 is the week containing the year's first Thursday, and weeks
// start on Monday. We anchor on Jan 4 (always in ISO week 1), step back to that
// week's Monday, then add (week-1) weeks.
func isoWeekStart(isoYear, week int, loc *time.Location) time.Time {
	jan4 := time.Date(isoYear, time.January, 4, 0, 0, 0, 0, loc)
	// Go's Weekday: Sunday=0..Saturday=6. Convert so Monday=0..Sunday=6.
	offset := (int(jan4.Weekday()) + 6) % 7
	week1Monday := jan4.AddDate(0, 0, -offset)
	return week1Monday.AddDate(0, 0, (week-1)*7)
}
