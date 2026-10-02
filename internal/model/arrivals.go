package model

import (
	"fmt"
	"strings"
	"time"
)

// ParseFileArrivals parses `git log --first-parent --diff-filter=A
// --name-only --format=%x00%cI` output into each path's arrival time: the
// committer time of the first-parent commit that added it. Log order is
// newest first, so a path added more than once keeps its latest addition —
// the one that brought the current file.
func ParseFileArrivals(gitLogOutput string) (map[string]time.Time, error) {
	arrivals := make(map[string]time.Time)
	var current time.Time
	for line := range strings.SplitSeq(gitLogOutput, "\n") {
		if stamp, ok := strings.CutPrefix(line, "\x00"); ok {
			t, err := time.Parse(time.RFC3339, stamp)
			if err != nil {
				return nil, fmt.Errorf("parsing commit time %q: %w", stamp, err)
			}
			current = t
			continue
		}
		if line == "" || current.IsZero() {
			continue
		}
		if _, seen := arrivals[line]; !seen {
			arrivals[line] = current
		}
	}
	return arrivals, nil
}
