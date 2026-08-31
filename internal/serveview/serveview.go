// Package serveview is the pure construction layer for what the engine
// serves the agent: it owns the size policy — part kinds, caps, the
// calibrated default budget — and the bounded-part construction the engine
// delegates to, keeping the engine orchestration-only (d-tac-qwc). It is to
// serves what presenters are to finders: rendering policy kept separable
// from the machinery that assembles the data.
package serveview

import "github.com/networkteam/sdd/internal/truncate"

// PartKind classifies a serve part by how it scales and what unit an honest
// cut works in.
type PartKind string

const (
	// PartText is one producer-rendered blob (a view layout, entry chains as
	// rendered text) — cut in bytes at line boundaries.
	PartText PartKind = "text"
	// PartEntryList scales in whole entries — cut only at entry boundaries.
	PartEntryList PartKind = "entry-list"
	// PartLineList is newline-joined single-line rows (topic labels).
	PartLineList PartKind = "line-list"
	// PartItemList is typed rows a template ranges over — cut by count.
	PartItemList PartKind = "item-list"
	// PartStoreValue is an interpolated store value.
	PartStoreValue PartKind = "store-value"
	// PartDraft is the engine-owned draft playback lane.
	PartDraft PartKind = "draft"
	// PartFraming is one shell framing lane.
	PartFraming PartKind = "framing"
	// PartProduced is the terminal serve's engine-written values block.
	PartProduced PartKind = "produced"
)

// Cap bounds one part: MaxBytes for rendered text, MaxItems for typed data.
// The zero value means unbounded.
type Cap struct {
	MaxBytes int
	MaxItems int
}

// Zero reports whether no bound is declared.
func (c Cap) Zero() bool { return c.MaxBytes == 0 && c.MaxItems == 0 }

// Budget is the size policy for one automatic serve.
type Budget struct {
	// Total is the advisory whole-serve target the authoring arithmetic
	// checks against; the engine never enforces it at runtime — everything
	// legitimately cuttable is cut at its own part cap, and a cut
	// instruction is a broken instruction, not a smaller one (d-tac-rzi).
	Total int
	caps  map[PartKind]Cap
}

// Cap returns the budget's default cap for a part kind (zero = unbounded).
func (b Budget) Cap(kind PartKind) Cap { return b.caps[kind] }

// Default is the calibrated default budget. Total is grounded in the
// verified ~10K-token host output floor (20260719-122547-s-tac-40d) at
// roughly 3.5 bytes per token; the per-kind caps come from the 2026-08-30
// calibration of every shipped procedure's serves against the SDD
// repository's own graph (d-tac-rzi slice 1) and join runtime resolution as
// each kind's honest cut mechanism lands.
func Default() Budget {
	return Budget{
		Total: 36000,
		caps: map[PartKind]Cap{
			PartText:       {MaxBytes: 12000},
			PartEntryList:  {MaxBytes: 16000, MaxItems: 24},
			PartLineList:   {MaxBytes: 4000},
			PartItemList:   {MaxItems: 40},
			PartStoreValue: {MaxBytes: 2000},
			PartDraft:      {MaxBytes: 6000},
			PartFraming:    {MaxBytes: 6000},
			PartProduced:   {MaxBytes: 8000},
		},
	}
}

// Effective resolves one part's cap: the spec's declaration wins, else the
// query's registration default. Budget defaults per part kind are wired in
// by the seams as each kind's honest cut lands — a byte default applied to
// an entry list before the pipeline can cut it at entry boundaries would
// reintroduce the mid-entry cut this work removes.
func Effective(declared, registered Cap) Cap {
	if !declared.Zero() {
		return declared
	}
	return registered
}

// BoundValue applies a cap to one part value at a serve seam. A Carrier
// passes its payload through with the producer's cut meta surfaced; a
// string takes the byte cap; any other value passes uncut — its bound lives
// where the value is built, not at the seam. The returned cut is nil when
// nothing was dropped.
func BoundValue(v any, cap Cap) (any, *truncate.Cut) {
	switch t := v.(type) {
	case truncate.Carrier:
		cut := t.CutMeta()
		if cut.Clean() {
			return t.Payload(), nil
		}
		return t.Payload(), &cut
	case string:
		bounded := truncate.Bytes(t, cap.MaxBytes, "")
		if bounded.Cut.Clean() {
			return bounded.Text, nil
		}
		cut := bounded.Cut
		return bounded.Text, &cut
	default:
		return v, nil
	}
}
