package engine

import (
	"fmt"
	"sort"
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
	// fieldGuideFindings holds the last writing-guide findings
	// ([]query.GuideFinding). Written by the guide op's contract
	// (writingGuide, wired by the shell); read here by noGuideFindings.
	fieldGuideFindings = "guideFindings"
	// fieldGuideReviewed marks that the agent has judged the current guide
	// findings (recordGuideReview). The guide runs once per capture: with
	// this set, the guide step passes through to playback instead of
	// re-serving the review; requestGuideRecheck clears it together with the
	// findings for an explicit re-run.
	fieldGuideReviewed = "guideReviewed"
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
	"hasBody":        "body",
	"hasRefs":        "refs",
	"hasTopics":      "topics",
	"hasConfidence":  "confidence",
	"hasKind":        "entryKind",
	"hasLayer":       "layer",
	"hasWidenReport": "widenReport",
	// Identity-kind capture fields (bootstrap's actor/role captures): canonical
	// on an actor, the bound actor canonical on a role. Presence only —
	// resolution is roleActorResolves' job, shape is aliasesWellFormed's.
	"hasCanonical": "canonical",
	"hasRoleActor": "roleActor",
	// A focus's involvement triples: presence only — target resolution is
	// involvementTargetsResolve's job.
	"hasInvolvement":  "involvement",
	"hasAnchor":       "anchor",
	"hasTargets":      "targets",
	"hasGoal":         "goal",
	"hasBrief":        "brief",
	"hasBriefing":     "briefing",
	"hasInspectedIds": "inspectedIds",
	"hasPlan":         "plan",
	"hasContract":     "contract",
	"hasBaseBranch":   "baseBranch",
	"hasWorkBranch":   "workBranch",
	"hasDoneEntry":    "doneEntry",
	"hasCandidates":   "candidates",
	"hasSynthesis":    "synthesis",

	// Bootstrap's brownfield gate: the host agent's repository read must land
	// before the conversation opens on it.
	"hasBrownfieldSynthesis": "brownfieldSynthesis",

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
			Name: "refsInspected",
			Doc: "Every draft ref (refs, supersedes, closes) was served to this session at full depth — " +
				"a show call, an injected chain's primary, or a same-session write. The read set is " +
				"session-level, so evidence inspected by a dispatching parent counts for its seeded children.",
			Reads: []string{"refs", "supersedes", "closes"},
		},
		Fn: func(ctx *Context) (bool, error) {
			return len(uninspectedRefs(ctx)) == 0, nil
		},
		FailMessage: "a draft ref was not inspected at full depth this session",
		FailDetail: func(ctx *Context) string {
			missing := uninspectedRefs(ctx)
			if len(missing) == 0 {
				return ""
			}
			return "refs not inspected at full depth this session: " + strings.Join(missing, ", ") +
				" — read them with show (reads stay free), then report again"
		},
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
			Name: "draftValidates",
			Doc: "The drafted fields satisfy the construction boundary's structural rules for their kind " +
				"(model.EntryConstruction, per the type-system contract) — so a gate never restates a " +
				"per-kind required-field list. Graph-edge resolution stays with refsResolve.",
			Reads: []string{"body", "entryKind", "layer", "refs", "topics", "index", "confidence", "intent", "participants", "supersedes", "closes", "canonical", "aliases", "roleActor", "involvement", "focusActors", "focusWhen"},
		},
		Fn: func(ctx *Context) (bool, error) {
			return len(draftStructuralFindings(ctx)) == 0, nil
		},
		FailMessage: "the draft does not satisfy its kind's structural rules",
		FailDetail: func(ctx *Context) string {
			findings := draftStructuralFindings(ctx)
			if len(findings) == 0 {
				return ""
			}
			messages := make([]string, 0, len(findings))
			for _, f := range findings {
				messages = append(messages, f.Message)
			}
			return "the draft does not satisfy its kind's structural rules: " + strings.Join(messages, "; ")
		},
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
			Name:  "roleActorResolves",
			Doc:   "The roleActor canonical resolves to an actor-identity chain in the graph (the bound actor exists). Absent roleActor passes — presence is hasRoleActor's job.",
			Reads: []string{"roleActor"},
		},
		Fn:          roleActorResolves,
		FailMessage: "roleActor does not name an actor known to the graph — capture the actor first, then bind the role to its canonical",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "aliasesWellFormed",
			Doc:   "Every alias is a non-empty, distinct name, none colliding with the canonical. Absent aliases pass — aliases are optional on an actor.",
			Reads: []string{"aliases", "canonical"},
		},
		Fn:          aliasesWellFormed,
		FailMessage: "an alias is empty, duplicated, or repeats the canonical — list each alternate name once",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "entryKindIsActor",
			Doc:   "The drafted entryKind is actor. Discriminates the kind-conditional assemble gate's identity branch.",
			Reads: []string{"entryKind"},
		},
		Fn:          entryKindIs("actor"),
		FailMessage: "the drafted kind is not actor",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "entryKindIsRole",
			Doc:   "The drafted entryKind is role. Discriminates the kind-conditional assemble gate's role branch.",
			Reads: []string{"entryKind"},
		},
		Fn:          entryKindIs("role"),
		FailMessage: "the drafted kind is not role",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "entryKindIsFocus",
			Doc:   "The drafted entryKind is focus. Discriminates the kind-conditional assemble gate's focus branch.",
			Reads: []string{"entryKind"},
		},
		Fn:          entryKindIs("focus"),
		FailMessage: "the drafted kind is not focus",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "involvementTargetsResolve",
			Doc:   "Every drafted involvement target resolves to an entry in the graph. Absent involvement passes — presence is hasInvolvement's job.",
			Reads: []string{"involvement"},
		},
		Fn:          involvementTargetsResolve,
		FailMessage: "an involvement target does not resolve to a known entry — capture or reference the target first",
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

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "noGuideFindings",
			Doc:   "The writing guide ran for the current draft and returned no findings.",
			Reads: []string{fieldGuideFindings},
		},
		Fn:          noGuideFindings,
		FailMessage: "the writing guide returned findings",
	})

	mustRegisterPredicate(r, Predicate{
		Doc: FuncDoc{
			Name:  "guideReviewed",
			Doc:   "The agent has judged the current guide findings (recorded by recordGuideReview).",
			Reads: []string{fieldGuideReviewed},
		},
		Fn: func(ctx *Context) (bool, error) {
			v, ok := ctx.Store.Get(fieldGuideReviewed)
			if !ok {
				return false, nil
			}
			reviewed, _ := v.(bool)
			return reviewed, nil
		},
		FailMessage: "the guide findings have not been reviewed",
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
			return ctx.Store.WriteEngine(fieldPlaybackConfirmation, playbackConfirmation{
				Snapshot: ctx.Store.StateSnapshot(),
				Step:     ctx.Step,
			})
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
			return ctx.Store.WriteEngine(fieldPreflightOverride, true)
		},
	})

	mustRegisterCommand(r, Command{
		Doc: FuncDoc{
			Name:   "recordGuideReview",
			Doc:    "Records that the agent judged the current guide findings; the guide step then passes through instead of re-serving the review.",
			Reads:  []string{fieldGuideFindings},
			Writes: []string{fieldGuideReviewed},
		},
		Fn: func(ctx *Context) error {
			return ctx.Store.WriteEngine(fieldGuideReviewed, true)
		},
	})

	mustRegisterCommand(r, Command{
		Doc: FuncDoc{
			Name:   "requestGuideRecheck",
			Doc:    "Clears the recorded guide run so the writing guide runs fresh on the next arrival — the explicit path for re-checking a substantially reworked draft.",
			Writes: []string{fieldGuideFindings, fieldGuideReviewed},
		},
		Fn: func(ctx *Context) error {
			if err := ctx.Store.WriteEngine(fieldGuideFindings, nil); err != nil {
				return err
			}
			return ctx.Store.WriteEngine(fieldGuideReviewed, nil)
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
			// A cross-repo ref passes this gate on syntactic validity alone:
			// real resolution runs at capture time against the cached remote
			// graph, never against the local one. Lifecycle fields below get
			// no such carve-out — closes/supersedes never cross the boundary.
			if model.IsCrossRepoID(r.ID) {
				if model.ValidateCrossRepoID(r.ID) != nil {
					return false, nil
				}
				continue
			}
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

// uninspectedRefs returns the draft's ref targets (refs, supersedes, closes)
// not yet served to the session at full depth, sorted. Presence and
// resolution are other predicates' jobs — this one only compares against the
// read set.
func uninspectedRefs(ctx *Context) []string {
	var missing []string
	seen := map[string]bool{}
	check := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		// Cross-repo targets live in another graph and cannot be served from
		// this session's reads; their verification is capture-time resolution
		// against the cached remote graph, not the inspection discipline.
		if model.IsCrossRepoID(id) {
			return
		}
		if ctx.Reads[id] != ReadFull {
			missing = append(missing, id)
		}
	}
	if v, ok := ctx.Store.Get("refs"); ok {
		for _, r := range asRefs(v) {
			check(r.ID)
		}
	}
	for _, field := range []string{"supersedes", "closes"} {
		if v, ok := ctx.Store.Get(field); ok {
			for _, id := range asStrings(v) {
				check(id)
			}
		}
	}
	sort.Strings(missing)
	return missing
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

func roleActorResolves(ctx *Context) (bool, error) {
	if ctx.Graph == nil {
		return false, fmt.Errorf("roleActorResolves needs a graph")
	}
	v, ok := ctx.Store.Get("roleActor")
	if !ok {
		return true, nil
	}
	canonical, ok := v.(string)
	if !ok || canonical == "" {
		return true, nil
	}
	return ctx.Graph.ChainForCanonical(canonical) != nil, nil
}

func involvementTargetsResolve(ctx *Context) (bool, error) {
	if ctx.Graph == nil {
		return false, fmt.Errorf("involvementTargetsResolve needs a graph")
	}
	v, ok := ctx.Store.Get("involvement")
	if !ok {
		return true, nil
	}
	for _, inv := range asInvolvements(v) {
		if _, ok := ctx.Graph.ByID[inv.Target]; !ok {
			return false, nil
		}
	}
	return true, nil
}

// draftStructuralFindings assembles the drafted capture fields as an entry
// and runs the construction boundary's rule set: assembly shape problems,
// stray-field projection findings, and the full per-kind validation including
// capture-only rules. No rule reads ID or Time, so neither is set.
func draftStructuralFindings(ctx *Context) []model.Finding {
	entry, findings := draftEntryFromStore(ctx.Store)
	if len(findings) > 0 {
		return findings
	}
	construction, findings := model.ConstructFromEntry(entry)
	return append(findings, construction.Validate(ctx.Graph)...)
}

// draftEntryFromStore reads the capture state fields into a model.Entry for
// structural validation. It mirrors the write op's draft assembly
// (application.entryFromDraft) — the binding-coverage directive
// (20260812-160448-d-tac-ymk) is the committed answer to holding the two to
// one declaration.
func draftEntryFromStore(store *Store) (*model.Entry, []model.Finding) {
	str := func(field string) string {
		v, _ := store.Get(field)
		s, _ := v.(string)
		return s
	}
	strs := func(field string) []string {
		v, ok := store.Get(field)
		if !ok {
			return nil
		}
		return asStrings(v)
	}

	kind := model.Kind(str("entryKind"))
	var entryType model.EntryType
	switch {
	case kind == "":
		return nil, []model.Finding{{Field: "kind", Message: "entry kind is required"}}
	case model.IsValidKindForType(model.TypeSignal, kind):
		entryType = model.TypeSignal
	case model.IsValidKindForType(model.TypeDecision, kind):
		entryType = model.TypeDecision
	default:
		return nil, []model.Finding{{Field: "kind", Value: string(kind), Message: fmt.Sprintf("unknown entry kind %q", kind)}}
	}

	var findings []model.Finding
	layerValue := str("layer")
	layer := model.Layer(layerValue)
	if expanded, ok := model.LayerFromAbbrev[layerValue]; ok {
		layer = expanded
	}
	entry := &model.Entry{
		Type: entryType, Kind: kind, Layer: layer,
		Content: str("body"), Confidence: str("confidence"),
		Intent:    model.Intent(str("intent")),
		Canonical: str("canonical"), Aliases: strs("aliases"), Actor: str("roleActor"),
		Participants: strs("participants"), Closes: strs("closes"), Supersedes: strs("supersedes"),
		FocusActors: strs("focusActors"),
	}
	if v, ok := store.Get("refs"); ok {
		for _, r := range asRefs(v) {
			entry.Refs = append(entry.Refs, model.Ref{ID: r.ID, Kind: model.RefKind(r.Kind), Desc: r.Desc})
		}
	}
	for _, label := range strs("topics") {
		topic, err := model.ParseTopicPath(label)
		if err != nil {
			findings = append(findings, model.Finding{Field: "topics", Value: label, Message: fmt.Sprintf("topic %q: %v", label, err)})
			continue
		}
		entry.Topics = append(entry.Topics, topic)
	}
	if v, ok := store.Get("index"); ok {
		if doc, ok := v.(map[string]any); ok {
			title, _ := doc["title"].(string)
			topic, _ := doc["topic"].(string)
			index, err := model.NewFactIndex(title, topic)
			if err != nil {
				findings = append(findings, model.Finding{Field: "index", Message: err.Error()})
			} else {
				entry.Index = index
			}
		}
	}
	if v, ok := store.Get("focusWhen"); ok {
		entry.FocusWhen = asFocusWhen(v)
	}
	if v, ok := store.Get("involvement"); ok {
		for _, inv := range asInvolvements(v) {
			entry.Involvement = append(entry.Involvement, model.Involvement{
				Target: inv.Target, Actors: inv.Actors, ActorsSet: inv.ActorsSet,
				When: whenToFocusWhen(inv.When),
			})
		}
	}
	return entry, findings
}

// asFocusWhen normalizes a store when value — typed in-memory or a JSON
// document after replay — to the model shape. A present-but-empty mapping
// yields an empty FocusWhen so the model's at-least-one-bound rule sees it.
func asFocusWhen(v any) *model.FocusWhen {
	switch w := v.(type) {
	case When:
		return &model.FocusWhen{From: w.From, To: w.To}
	case *When:
		return whenToFocusWhen(w)
	case map[string]any:
		from, _ := w["from"].(string)
		to, _ := w["to"].(string)
		return &model.FocusWhen{From: from, To: to}
	default:
		return nil
	}
}

func whenToFocusWhen(w *When) *model.FocusWhen {
	if w == nil {
		return nil
	}
	return &model.FocusWhen{From: w.From, To: w.To}
}

func aliasesWellFormed(ctx *Context) (bool, error) {
	v, ok := ctx.Store.Get("aliases")
	if !ok {
		return true, nil
	}
	aliases := asStrings(v)
	if len(aliases) == 0 {
		return true, nil
	}
	canonical, _ := ctx.Store.Get("canonical")
	canonicalName, _ := canonical.(string)
	seen := make(map[string]bool, len(aliases))
	for _, a := range aliases {
		if strings.TrimSpace(a) == "" {
			return false, nil
		}
		if a == canonicalName {
			return false, nil
		}
		if seen[a] {
			return false, nil
		}
		seen[a] = true
	}
	return true, nil
}

// entryKindIs builds a predicate that holds when the drafted entryKind equals
// kind — the kind discriminator the assemble gate's ordered transitions branch
// on so identity kinds and ordinary kinds carry different requirements.
func entryKindIs(kind string) func(*Context) (bool, error) {
	return func(ctx *Context) (bool, error) {
		v, ok := ctx.Store.Get("entryKind")
		if !ok {
			return false, nil
		}
		s, ok := v.(string)
		return ok && s == kind, nil
	}
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
	// Store values are normalized JSON documents, so findings always come back
	// as the []any/map form; the query type reconstructs the typed findings.
	normalized, err := VarType{Base: TypePreflightFindings}.ValidateValue(v)
	if err != nil {
		return false, fmt.Errorf("findings field has unexpected shape: %w", err)
	}
	findings := normalized.([]query.Finding)
	for _, f := range findings {
		if f.Severity == query.SeverityHigh {
			return false, nil
		}
	}
	return true, nil
}

// noGuideFindings passes only when the writing guide has run and returned an
// empty findings list. Absence fails: the guide op's contract always writes
// guideFindings, so a missing field means the op has not run for this draft.
func noGuideFindings(ctx *Context) (bool, error) {
	v, ok := ctx.Store.Get(fieldGuideFindings)
	if !ok || v == nil {
		// Absent or cleared (requestGuideRecheck) — the guide has not run
		// for the current draft.
		return false, nil
	}
	normalized, err := VarType{Base: TypeGuideFindings}.ValidateValue(v)
	if err != nil {
		return false, fmt.Errorf("guideFindings field has unexpected shape: %w", err)
	}
	return len(normalized.([]query.GuideFinding)) == 0, nil
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

// asInvolvements normalizes a list-of-involvement store value, accepting both
// the validated engine value and the replay/JSON map form.
func asInvolvements(v any) []Involvement {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]Involvement, 0, len(items))
	for _, item := range items {
		switch iv := item.(type) {
		case Involvement:
			out = append(out, iv)
		case map[string]any:
			if inv, err := involvementFromMap(iv); err == nil {
				out = append(out, inv)
			}
		}
	}
	return out
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
