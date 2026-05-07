package finders

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// rankSpec is the resolved intent of a rank(<algo>) call: algorithm
// name plus decay name. by(date) is the structural special case — it
// sorts entries by creation time without computing meaningful scores,
// signalled by Algorithm == "by" + ByField == "date".
type rankSpec struct {
	Algorithm string // "heat", "in-degree", "mult", "add", "log", "by"
	Decay     string // "" when not applicable (in-degree, by)
	ByField   string // "date" for by(date); "" otherwise
}

// knownAlgorithms is the user-facing list shown in unknown-algorithm
// errors so users see what's available without consulting docs.
var knownAlgorithms = []string{"heat", "in-degree", "mult", "add", "log", "by(date)"}

// parseRankArg validates rank()'s single argument and resolves it to a
// rankSpec. Accepts both bare identifier form (rank(heat) — shorthand
// for rank(heat()) using the algorithm's default decay) and function-
// call form (rank(heat(exp-14d))). The bare form keeps simple cases
// terse; the call form lets the user pin a decay.
func parseRankArg(args []model.FunctionArg) (*rankSpec, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("rank requires exactly one argument (the algorithm), got %d", len(args))
	}
	a := args[0]
	var fn model.Function
	switch a.Kind {
	case model.ArgKindFunc:
		if a.Func == nil {
			return nil, fmt.Errorf("rank: empty algorithm function")
		}
		fn = *a.Func
	case model.ArgKindIdent, model.ArgKindString:
		// Bare identifier shorthand: rank(heat) == rank(heat()) — the
		// algorithm function with no decay arg, which then picks up
		// DefaultDecayName in parseAlgorithm.
		fn = model.Function{Name: a.String}
	default:
		return nil, fmt.Errorf("rank argument must be an algorithm name or call (e.g. rank(heat) or rank(heat(exp-14d))), got %s", a.Kind)
	}
	return parseAlgorithm(fn)
}

// parseAlgorithm resolves the algorithm function (with its decay arg,
// if any) to a rankSpec, applying default decay where the algorithm
// supports it. Decay names are validated up front so an unknown name
// fails at parse time rather than during scoring.
func parseAlgorithm(fn model.Function) (*rankSpec, error) {
	spec := &rankSpec{Algorithm: fn.Name}

	switch fn.Name {
	case "heat", "mult", "add", "log":
		switch len(fn.Args) {
		case 0:
			spec.Decay = model.DefaultDecayName
		case 1:
			name, err := argString(fn.Args[0])
			if err != nil {
				return nil, fmt.Errorf("rank %s: %w", fn.Name, err)
			}
			if _, err := model.DecayByName(name); err != nil {
				return nil, fmt.Errorf("rank %s: %w", fn.Name, err)
			}
			spec.Decay = name
		default:
			return nil, fmt.Errorf("rank %s: takes at most one decay argument, got %d", fn.Name, len(fn.Args))
		}

	case "in-degree":
		// in-degree ignores any decay arg silently per the design — the
		// algorithm has no recency component to weight, so the grammar
		// accepts but the executor discards it.
		spec.Decay = ""

	case "by":
		if len(fn.Args) != 1 {
			return nil, fmt.Errorf("rank by: requires exactly one field argument (e.g. by(date))")
		}
		field, err := argString(fn.Args[0])
		if err != nil {
			return nil, fmt.Errorf("rank by: %w", err)
		}
		if field != "date" {
			return nil, fmt.Errorf("rank by: only by(date) is supported (got by(%s))", field)
		}
		spec.ByField = field

	default:
		return nil, fmt.Errorf("unknown rank algorithm %q (known: %s)", fn.Name, strings.Join(knownAlgorithms, ", "))
	}

	return spec, nil
}

// suffix returns the rank-context fragment that follows a header
// prefix — always shaped as "by <thing>" so concatenation with any
// prefix produces a readable label:
//
//	"Top"                    + "by heat (exp-14d)"  → "Top by heat (exp-14d)"
//	"Topic: infrastructure/cli" + "by in-degree"    → "Topic: infrastructure/cli by in-degree"
//	"Done"                   + "by date"            → "Done by date"
//	"Insights"               + "by date"            → "Insights by date"
//
// Returning the suffix without the implicit "Top " prefix lets the
// resolver in section_spec.go compose flexibly: a baked prefix gets
// the suffix appended; bare rank() (no prefix) gets "Top " prepended
// at resolution time. Centralizing the suffix shape here keeps the
// rank-vs-display contract narrow (the suffix is purely "what the
// algorithm sorts by"; the prefix is "what the section is about").
func (s *rankSpec) suffix() string {
	if s == nil {
		return ""
	}
	switch s.Algorithm {
	case "by":
		// by(date) is the only ByField shape today; future ByFields
		// fall through to the same "by <field>" form.
		return "by " + s.ByField
	case "in-degree":
		return "by in-degree"
	case "heat", "mult", "add", "log":
		if s.Decay != "" {
			return fmt.Sprintf("by %s (%s)", s.Algorithm, s.Decay)
		}
		return "by " + s.Algorithm
	default:
		return "by " + s.Algorithm
	}
}

// argString returns the string content of an argument that should be
// either a bare identifier or a quoted string. Used by primitives whose
// args accept both forms (kind, layer, topic, rank's algorithm and
// decay names). Callers wrap the returned error with their primitive's
// name for clear diagnostics.
func argString(a model.FunctionArg) (string, error) {
	if a.Kind != model.ArgKindIdent && a.Kind != model.ArgKindString {
		return "", fmt.Errorf("argument must be an identifier or string, got %s", a.Kind)
	}
	return a.String, nil
}

// applyRanking scores and sorts entries per the rankSpec, returning the
// sorted entries and parallel scores. by(date) returns nil scores —
// it's a sort, not a ranking, and the renderer omits the score segment
// when scores are nil. All other algorithms return scores aligned with
// the sorted entries.
func applyRanking(g *model.Graph, entries []*model.Entry, spec *rankSpec, now time.Time) ([]*model.Entry, []float64) {
	if spec.Algorithm == "by" {
		sorted := make([]*model.Entry, len(entries))
		copy(sorted, entries)
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].Time.After(sorted[j].Time)
		})
		return sorted, nil
	}

	var decay model.DecayFunc
	if spec.Decay != "" {
		decay, _ = model.DecayByName(spec.Decay) // pre-validated in parseAlgorithm
	}

	scores := make([]float64, len(entries))
	for i, e := range entries {
		switch spec.Algorithm {
		case "heat":
			scores[i] = model.HeatScore(g, e, decay, now)
		case "in-degree":
			scores[i] = model.InDegreeScore(g, e)
		case "mult":
			scores[i] = model.MultScore(g, e, decay, now)
		case "add":
			scores[i] = model.AddScore(g, e, decay, now)
		case "log":
			scores[i] = model.LogScore(g, e, decay, now)
		}
	}

	// Sort entries and scores together, descending by score. Indirect
	// sort via indices keeps the two slices aligned without copying
	// per-step.
	indices := make([]int, len(entries))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		return scores[indices[a]] > scores[indices[b]]
	})
	sortedEntries := make([]*model.Entry, len(entries))
	sortedScores := make([]float64, len(entries))
	for i, idx := range indices {
		sortedEntries[i] = entries[idx]
		sortedScores[i] = scores[idx]
	}
	return sortedEntries, sortedScores
}
