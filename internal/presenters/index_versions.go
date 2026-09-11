package presenters

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/charmbracelet/colorprofile"

	"github.com/networkteam/sdd/internal/query"
)

// RenderIndexVersionsTable writes the styled `sdd index gc` report for the
// interactive TTY path, in the shape of `sdd stats`: a title line, then per
// store a heading, a source line, and a header-ruled table of version groups.
// The colorprofile writer strips color for non-terminals and NO_COLOR
// (d-cpt-mvb).
func RenderIndexVersionsTable(dst io.Writer, results []*query.IndexVersionsResult) {
	w := colorprofile.NewWriter(dst, os.Environ())
	fmt.Fprintln(w, " "+clrHeading.Render("sdd index gc")+clrBody.Render(fmt.Sprintf(" — %d store(s)", len(results))))
	for _, r := range results {
		fmt.Fprintln(w)
		fmt.Fprintln(w, " "+clrHeading.Render(r.Label))
		fmt.Fprintln(w, " "+clrBody.Render(fmt.Sprintf("%s · %d entries · %s", HumanBytes(r.Bytes), r.Entries, r.IndexDir)))
		fmt.Fprintln(w)
		if len(r.Groups) == 0 {
			fmt.Fprintln(w, " no stored versions")
			continue
		}
		data := make([][]string, 0, len(r.Groups))
		for _, g := range r.Groups {
			status := "droppable"
			if !g.Droppable {
				status = "kept"
			}
			data = append(data, []string{
				g.Name, strconv.Itoa(g.Versions), strconv.Itoa(g.Entries),
				dayOrDash(g.Oldest), dayOrDash(g.Newest), HumanBytes(g.Bytes), status,
			})
		}
		fmt.Fprintln(w, ruledTable([]string{"GROUP", "VERSIONS", "ENTRIES", "OLDEST", "NEWEST", "SIZE", "STATUS"}, data, 1))
		if r.OrphanFiles > 0 {
			fmt.Fprintln(w, " "+clrWarn.Render(fmt.Sprintf("%d orphan row files (%s) not referenced by the manifest — any --drop run removes them", r.OrphanFiles, HumanBytes(r.OrphanBytes))))
		}
	}
}

func dayOrDash(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02")
}

type indexVersionsJSON struct {
	Store       string             `json:"store"`
	IndexDir    string             `json:"index_dir"`
	Entries     int                `json:"entries"`
	Bytes       int64              `json:"bytes"`
	Groups      []indexVersionJSON `json:"groups"`
	OrphanFiles int                `json:"orphan_files"`
	OrphanBytes int64              `json:"orphan_bytes"`
}

type indexVersionJSON struct {
	Group     string  `json:"group"`
	Versions  int     `json:"versions"`
	Entries   int     `json:"entries"`
	Oldest    *string `json:"oldest"`
	Newest    *string `json:"newest"`
	Bytes     int64   `json:"bytes"`
	Droppable bool    `json:"droppable"`
}

// RenderIndexVersionsJSON writes the same report as structured JSON on the
// agent / non-TTY path — one object per store, groups as an array, no chrome.
func RenderIndexVersionsJSON(w io.Writer, results []*query.IndexVersionsResult) error {
	out := make([]indexVersionsJSON, 0, len(results))
	for _, r := range results {
		store := indexVersionsJSON{Store: r.Label, IndexDir: r.IndexDir, Entries: r.Entries, Bytes: r.Bytes, Groups: []indexVersionJSON{}, OrphanFiles: r.OrphanFiles, OrphanBytes: r.OrphanBytes}
		for _, g := range r.Groups {
			store.Groups = append(store.Groups, indexVersionJSON{
				Group: g.Name, Versions: g.Versions, Entries: g.Entries,
				Oldest: rfc3339OrNull(g.Oldest), Newest: rfc3339OrNull(g.Newest),
				Bytes: g.Bytes, Droppable: g.Droppable,
			})
		}
		out = append(out, store)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func rfc3339OrNull(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// HumanBytes formats a byte count in the unit that keeps it under four digits.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			if value < 10 {
				return fmt.Sprintf("%.1f %s", value, suffix)
			}
			return fmt.Sprintf("%.0f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.0f PB", value/unit)
}
