package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ResolveSinceSpec parses a since() argument into an absolute cutoff
// time. Entries with creation time on or after the cutoff pass the
// filter; entries before are excluded.
//
// Accepts two forms per d-tac-uww §4:
//
//   - ISO date: YYYY-MM-DD — interpreted as midnight UTC of that day.
//   - Duration: Nd | Nw | Nm | Ny — N is a non-negative integer.
//     `d` and `w` use exact 24h offsets (1d = 24h, 1w = 7×24h).
//     `m` and `y` use calendar arithmetic via time.AddDate so month/
//     year boundaries match human expectations regardless of leap days
//     or month length.
//
// `now` is the reference time the duration form subtracts from. Tests
// supply a fixed clock; the executor passes time.Now().
func ResolveSinceSpec(spec string, now time.Time) (time.Time, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return time.Time{}, fmt.Errorf("since: empty spec (expected ISO date YYYY-MM-DD or duration like 7d, 30d, 2w, 1m, 1y)")
	}

	// ISO date: dashes in positions 4 and 7, exactly 10 characters.
	if len(spec) == 10 && spec[4] == '-' && spec[7] == '-' {
		t, err := time.Parse("2006-01-02", spec)
		if err != nil {
			return time.Time{}, fmt.Errorf("since: invalid ISO date %q: %v", spec, err)
		}
		return t, nil
	}

	// Duration: digits followed by a single unit suffix.
	if n := len(spec); n >= 2 {
		unit := spec[n-1]
		switch unit {
		case 'd', 'w', 'm', 'y':
			amount, err := strconv.Atoi(spec[:n-1])
			if err != nil {
				return time.Time{}, fmt.Errorf("since: invalid duration %q (expected NUMBER%s, got %q before unit)", spec, string(unit), spec[:n-1])
			}
			if amount < 0 {
				return time.Time{}, fmt.Errorf("since: duration must be non-negative, got %d%s", amount, string(unit))
			}
			switch unit {
			case 'd':
				return now.Add(-time.Duration(amount) * 24 * time.Hour), nil
			case 'w':
				return now.Add(-time.Duration(amount) * 7 * 24 * time.Hour), nil
			case 'm':
				return now.AddDate(0, -amount, 0), nil
			case 'y':
				return now.AddDate(-amount, 0, 0), nil
			}
		}
	}

	return time.Time{}, fmt.Errorf("since: unrecognized spec %q (expected YYYY-MM-DD or Nd|Nw|Nm|Ny)", spec)
}
