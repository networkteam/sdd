package engine

import (
	"fmt"
	"strings"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Engine-written store fields owned by the built-in trust machinery. Spec
// load rejects declaring these (they're in the commands' Writes contracts).
const (
	// fieldPlaybackConfirmation holds the confirmation record written by
	// confirmPlayback: the state snapshot it was bound to and the chooser
	// step where it was recorded (the reopen target on staleness).
	fieldPlaybackConfirmation = "playbackConfirmation"
	// fieldPreflightOverride marks the durably-logged, user-only pre-flight
	// skip recorded by recordOverride.
	fieldPreflightOverride = "preflightOverride"
	// fieldFindings holds the last write-gate findings ([]query.Finding).
	// Written by the write command's contract (newEntry, wired by the
	// shell); read here by noHighFindings.
	fieldFindings = "findings"
)

// playbackConfirmation is the value of fieldPlaybackConfirmation.
type playbackConfirmation struct {
	Snapshot string `json:"snapshot"`
	Step     string `json:"step"`
}

// presenceFields maps each presence predicate to the store field it reads.
// One predicate per commonly collected field; the mapping is the documented
// contract (hasKind reads entryKind — the capture spine collects the target
// kind under that name, kind being the param that pre-selects it).
var presenceFields = map[string]string{
	"hasBody":         "body",
	"hasRefs":         "refs",
	"hasTopics":       "topics",
	"hasConfidence":   "confidence",
	"hasKind":         "entryKind",
	"hasLayer":        "layer",
	"hasWidenReport":  "widenReport",
	"hasAnchor":       "anchor",
	"hasTargets":      "targets",
	"hasGoal":         "goal",
	"hasBrief":        "brief",
	"hasBriefing":     "briefing",
	"hasInspectedIds": "inspectedIds",
	"hasPlan":         "plan",
	"hasContract":     "contract",
	"hasDoneEntry":    "doneEntry",
	"hasCandidates":   "candidates",

	// The evaluate lens gate: at least one lens judgment must land before the
	// junction; evidence fields are instructed alongside, never gated.
	"hasInnerEvaluation": "innerEvaluation",
	"hasOuterEvaluation": "outerEvaluation",

	// Engine-written by wipStart (see the shell's Writes contract): presence
	// routes the implementation closeout through wipDone only on tracked runs.
	"hasWipMarker": "wipMarker",
}

func registerBuiltinPredicates(r *Registry) {
	for name, field := range presenceFields {
		mustRegisterPredicate(r, Predicate{
			Doc: FuncDoc{
				Name:  name,
				Doc:   fmt.Sprintf("Field %q is present and non-empty.", field),
				Reads: []string{field},
			},
			Fn: func(ctx *Context) (bool, error) {
				return ctx.Store.Has(field), nil
			},
			FailMessage: fmt.Sprintf("%s is missing or empty", field),
		})
	}

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "anchorsResolve",
			Doc:   "The anchor and every target resolve to existing graph entries.",
			Reads: []string{"anchor", "targets"},
		},
		Fn:          idsResolve("anchor", "targets"),
		FailMessage: "the anchor or a target does not resolve in the graph — engage on entries that exist",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "inspectedIdsResolve",
			Doc:   "Every inspected ID resolves to an existing graph entry.",
			Reads: []string{"inspectedIds"},
		},
		Fn:          idsResolve("inspectedIds"),
		FailMessage: "an inspected ID does not resolve in the graph — the evidence must name entries that exist",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "doneEntryResolves",
			Doc:   "The recorded done signal resolves to an existing graph entry.",
			Reads: []string{"doneEntry"},
		},
		Fn:          idsResolve("doneEntry"),
		FailMessage: "doneEntry does not resolve in the graph — record the closing done signal before closing out",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "refsResolve",
			Doc:   "Every entry referenced by refs, supersedes, and closes exists in the graph (capture-time invariant).",
			Reads: []string{"refs", "supersedes", "closes"},
		},
		Fn:          refsResolve,
		FailMessage: "a referenced entry does not resolve in the graph — every reference must point at an existing entry",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "refKindsValid",
			Doc:   "Every ref kind is from the closed ref-kind set.",
			Reads: []string{"refs"},
		},
		Fn:          refKindsValid,
		FailMessage: "a ref carries a kind outside the closed ref-kind set",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "participantsCanonical",
			Doc:   "All participants resolve to active actor canonicals (grace mode: passes when the graph has no active actors).",
			Reads: []string{"participants"},
		},
		Fn:          participantsCanonical,
		FailMessage: "a participant is not an active actor canonical — resolve aliases to canonicals before capture",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "topicsKnown",
			Doc:   "All topic labels already exist in the graph (guard for flagging new labels in playback).",
			Reads: []string{"topics"},
		},
		Fn:          topicsKnown,
		FailMessage: "a topic label is new to the graph",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "intentPresentIfDirective",
			Doc:   "Intent is supplied whenever the target kind is directive.",
			Reads: []string{"entryKind", "intent"},
		},
		Fn:          intentPresentIfDirective,
		FailMessage: "the target kind is directive but no intent is supplied (pending, guiding, or settled — there is no default)",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "playbackConfirmed",
			Doc:   "A confirmation is recorded and the confirmed state is unchanged since (edits after confirmation reopen playback).",
			Reads: []string{fieldPlaybackConfirmation},
		},
		Fn:          playbackConfirmed,
		FailMessage: "playback is not confirmed for the current state",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "noHighFindings",
			Doc:   "The last gate run produced no high-severity finding.",
			Reads: []string{fieldFindings},
		},
		Fn:          noHighFindings,
		FailMessage: "the last gate run produced high-severity findings",
	})
}

// registerBuiltinCommands registers the dependency-free trust machinery.
// Commands with real side effects (newEntry, replaceSummary, wipStart,
// wipDone) are registered by the shell that owns their dependencies.
func registerBuiltinCommands(r *Registry) {
	mustRegisterCommand(r, Command{
		Doc: FuncDoc{
			Name:   "confirmPlayback",
			Doc:    "Records the user's confirmation bound to the current state snapshot (chooser call, user options only). Any later state edit invalidates it and reopens the recording chooser.",
			Writes: []string{fieldPlaybackConfirmation},
		},
		Fn: func(ctx *Context) error {
			ctx.Store.WriteEngine(fieldPlaybackConfirmation, playbackConfirmation{
				Snapshot: ctx.Store.StateSnapshot(),
				Step:     ctx.Step,
			})
			return nil
		},
	})

	mustRegisterCommand(r, Command{
		Doc: FuncDoc{
			Name:   "recordOverride",
			Doc:    "Records the user-only pre-flight skip, durably logged; the write gate reads it on the re-run.",
			Reads:  []string{fieldFindings},
			Writes: []string{fieldPreflightOverride},
		},
		Fn: func(ctx *Context) error {
			ctx.Store.WriteEngine(fieldPreflightOverride, true)
			return nil
		},
	})
}

// idsResolve builds a predicate checking that every entry ID stored under the
// named fields exists in the graph. Absent fields pass — presence is the
// paired has* predicate's job.
func idsResolve(fields ...string) func(*Context) (bool, error) {
	return func(ctx *Context) (bool, error) {
		if ctx.Graph == nil {
			return false, fmt.Errorf("idsResolve needs a graph")
		}
		for _, field := range fields {
			v, ok := ctx.Store.Get(field)
			if !ok {
				continue
			}
			for _, id := range asStrings(v) {
				if _, ok := ctx.Graph.ByID[id]; !ok {
					return false, nil
				}
			}
		}
		return true, nil
	}
}

func refsResolve(ctx *Context) (bool, error) {
	if ctx.Graph == nil {
		return false, fmt.Errorf("refsResolve needs a graph")
	}
	resolves := func(id string) bool {
		_, ok := ctx.Graph.ByID[id]
		return ok
	}
	if refs, ok := ctx.Store.Get("refs"); ok {
		for _, r := range asRefs(refs) {
			if !resolves(r.ID) {
				return false, nil
			}
		}
	}
	for _, field := range []string{"supersedes", "closes"} {
		v, ok := ctx.Store.Get(field)
		if !ok {
			continue
		}
		for _, id := range asStrings(v) {
			if !resolves(id) {
				return false, nil
			}
		}
	}
	return true, nil
}

func refKindsValid(ctx *Context) (bool, error) {
	v, ok := ctx.Store.Get("refs")
	if !ok {
		return true, nil
	}
	for _, r := range asRefs(v) {
		if !model.IsCapturableRefKind(model.RefKind(r.Kind)) {
			return false, nil
		}
	}
	return true, nil
}

func participantsCanonical(ctx *Context) (bool, error) {
	if ctx.Graph == nil {
		return false, fmt.Errorf("participantsCanonical needs a graph")
	}
	active := ctx.Graph.ActiveActorHeads()
	if len(active) == 0 {
		return true, nil // grace mode: no active actors yet
	}
	canonicals := make(map[string]bool, len(active))
	for _, a := range active {
		canonicals[a.Canonical] = true
	}
	v, ok := ctx.Store.Get("participants")
	if !ok {
		return true, nil
	}
	for _, p := range asStrings(v) {
		if !canonicals[p] {
			return false, nil
		}
	}
	return true, nil
}

func topicsKnown(ctx *Context) (bool, error) {
	if ctx.Graph == nil {
		return false, fmt.Errorf("topicsKnown needs a graph")
	}
	v, ok := ctx.Store.Get("topics")
	if !ok {
		return true, nil
	}
	labels := asStrings(v)
	if len(labels) == 0 {
		return true, nil
	}
	known := make(map[string]bool)
	for _, e := range ctx.Graph.Entries {
		for _, t := range ctx.Graph.EffectiveTopics(e) {
			known[strings.ToLower(t.String())] = true
		}
	}
	for _, l := range labels {
		if !known[strings.ToLower(l)] {
			return false, nil
		}
	}
	return true, nil
}

func intentPresentIfDirective(ctx *Context) (bool, error) {
	kind, _ := ctx.Store.Get("entryKind")
	if kind != "directive" {
		return true, nil
	}
	return ctx.Store.Has("intent"), nil
}

func playbackConfirmed(ctx *Context) (bool, error) {
	v, ok := ctx.Store.Get(fieldPlaybackConfirmation)
	if !ok {
		return false, nil
	}
	conf, ok := asConfirmation(v)
	if !ok {
		return false, nil
	}
	return conf.Snapshot == ctx.Store.StateSnapshot(), nil
}

func noHighFindings(ctx *Context) (bool, error) {
	v, ok := ctx.Store.Get(fieldFindings)
	if !ok {
		// No gate run recorded — nothing to pass. The write op's contract
		// always writes findings, so absence means the gate hasn't run.
		return false, nil
	}
	findings, ok := v.([]query.Finding)
	if !ok {
		// Replay path: findings restored from JSON.
		normalized, err := VarType{Base: TypePreflightFindings}.ValidateValue(v)
		if err != nil {
			return false, fmt.Errorf("findings field has unexpected shape: %w", err)
		}
		findings = normalized.([]query.Finding)
	}
	for _, f := range findings {
		if f.Severity == query.SeverityHigh {
			return false, nil
		}
	}
	return true, nil
}

// asRefs normalizes a refs store value: a validated list holds engine.Ref
// items (or map form on odd paths).
func asRefs(v any) []Ref {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	refs := make([]Ref, 0, len(items))
	for _, item := range items {
		switch r := item.(type) {
		case Ref:
			refs = append(refs, r)
		case map[string]any:
			id, _ := r["id"].(string)
			kind, _ := r["kind"].(string)
			desc, _ := r["desc"].(string)
			refs = append(refs, Ref{ID: id, Kind: kind, Desc: desc})
		}
	}
	return refs
}

// asStrings normalizes a list-of-strings store value.
func asStrings(v any) []string {
	switch vv := v.(type) {
	case []any:
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if vv == "" {
			return nil
		}
		return []string{vv}
	default:
		return nil
	}
}

// asConfirmation normalizes the confirmation record: typed in-memory,
// map-shaped after JSON replay.
func asConfirmation(v any) (playbackConfirmation, bool) {
	switch c := v.(type) {
	case playbackConfirmation:
		return c, true
	case map[string]any:
		snap, _ := c["snapshot"].(string)
		step, _ := c["step"].(string)
		if snap == "" {
			return playbackConfirmation{}, false
		}
		return playbackConfirmation{Snapshot: snap, Step: step}, true
	default:
		return playbackConfirmation{}, false
	}
}
