package types

import (
	"fmt"
	"time"
)

// Warning represents a validation issue found on a graph entry.
type Warning struct {
	Field   string // "refs", "closes", "supersedes"
	Value   string // the offending ID or value
	Message string // human-readable description
}

// Involvement is one entry in a kind: focus decision's involvement: list.
// Required: Target (entry ID this involvement is about). Optional: Actors
// (canonical-only; per-involvement override of focus-level default — explicit
// empty list means "deliberately unattributed / pull-available", distinct
// from the unset case which inherits the focus-level default). Optional:
// When (per-involvement temporal scope override).
type Involvement struct {
	Target string
	// Actors carries canonical-only names. Distinguishing "unset" (inherit
	// focus-level default) from "explicit empty" (deliberately
	// pull-available) requires the ActorsSet field; YAML's natural decoding
	// merges both into a nil slice, so we capture the distinction at
	// frontmatter parse time.
	Actors    []string
	ActorsSet bool
	When      *FocusWhen
}

// FocusWhen is the temporal scope for a focus or one of its involvement
// triples. At least one of From or To must be set when the field is present;
// the absent end means "open-ended in that direction." Dates are ISO
// YYYY-MM-DD format on disk and parsed into time.Time at validation time.
type FocusWhen struct {
	From string `yaml:"from,omitempty"`
	To   string `yaml:"to,omitempty"`
}

// IsZero reports whether neither end is set. A FocusWhen with IsZero() == true
// is invalid in frontmatter (the field should have been omitted entirely);
// validators surface this as a shape error.
func (w *FocusWhen) IsZero() bool {
	return w == nil || (w.From == "" && w.To == "")
}

// Validate checks ISO date shape on each end and that at least one end is set.
// Returns a descriptive error or nil. Pure — no I/O, safe to call from
// validators and finders.
func (w *FocusWhen) Validate() error {
	if w == nil {
		return nil
	}
	if w.From == "" && w.To == "" {
		return fmt.Errorf("when: at least one of `from` or `to` is required")
	}
	if w.From != "" {
		if _, err := time.Parse("2006-01-02", w.From); err != nil {
			return fmt.Errorf("when.from: %w", err)
		}
	}
	if w.To != "" {
		if _, err := time.Parse("2006-01-02", w.To); err != nil {
			return fmt.Errorf("when.to: %w", err)
		}
	}
	return nil
}
