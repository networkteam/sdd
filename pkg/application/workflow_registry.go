package application

import (
	"fmt"
	"strings"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/serveview"
	"github.com/networkteam/sdd/internal/truncate"
	"github.com/networkteam/sdd/pkg/application/types"
)

// serveChainBudget bounds each direction of a served entry chain: the
// entry-list default from the serve budget caps nodes, and the fan-out cap
// keeps one hub entry from spending the whole node budget on its own edges.
// Explicit pulls (sdd show, the MCP show tool) never pass a budget.
var serveChainBudget = types.ShowTreeBudget{
	MaxNodes:    serveview.Default().Cap(serveview.PartEntryList).MaxItems,
	MaxChildren: 8,
}

// servedViewBudget bounds a pushed view's scaling shapes at whole units:
// focus and participant groups, WIP markers, whole bodies, per-entry ref
// sub-lines. Explicit pulls (the view tool, the CLI) pass no budget.
var servedViewBudget = types.ViewBudget{
	GroupItems:   12,
	RefsPerEntry: 6,
	BodyBytes:    serveview.Default().Cap(serveview.PartText).MaxBytes,
}

func (w *WorkflowSession) buildRegistry() (*engine.Registry, error) {
	registry := engine.NewRegistry()
	for _, register := range []func(*engine.Registry) error{
		w.registerWorkflowQueries,
		w.registerWorkflowPredicates,
		w.registerWorkflowWrites,
		w.registerWorkflowWIP,
	} {
		if err := register(registry); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (w *WorkflowSession) registerWorkflowPredicates(registry *engine.Registry) error {
	if err := registry.RegisterPredicate(engine.Predicate{
		Doc: engine.FuncDoc{Name: "sessionQuiescent", Doc: "Nothing but the session shell is running — no open move instances in this session."},
		Fn: func(*engine.Context) (bool, error) {
			for _, instance := range w.session.Instances() {
				if instance.ID != w.shell && instance.Status == engine.StatusRunning {
					return false, nil
				}
			}
			return true, nil
		},
		FailMessage: "the session still has open move instances",
	}); err != nil {
		return err
	}
	return nil
}

func (w *WorkflowSession) registerWorkflowQueries(registry *engine.Registry) error {
	if err := registry.RegisterQuery(engine.Query{
		Doc:       engine.FuncDoc{Name: "sessionInfo", Doc: "Session framing: local participant, configured language, and available search modes."},
		ServeSafe: true,
		Fn: func(*engine.Context, map[string]any) (any, error) {
			info, err := w.readInfo()
			if err != nil {
				return nil, err
			}
			return map[string]any{"participant": info.Participant, "language": info.Language, "search": info.Search}, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.RegisterQuery(engine.Query{
		Doc:       engine.FuncDoc{Name: "participantNames", Doc: "The names a capture's participants are chosen from: the local participant, then each active actor's canonical with its aliases, one actor per line."},
		ServeSafe: true,
		Bound:     engine.QueryBound{Part: serveview.PartLineList, Cap: serveview.Default().Cap(serveview.PartLineList)},
		Fn: func(ctx *engine.Context, _ map[string]any) (any, error) {
			info, err := w.readInfo()
			if err != nil {
				return nil, err
			}
			return renderParticipantNames(info.Participant, ctx.Graph), nil
		},
	}); err != nil {
		return err
	}
	if err := registry.RegisterQuery(engine.Query{
		Doc:       engine.FuncDoc{Name: "factIndex", Doc: "Active indexed facts from the current project graph, ordered for session discovery."},
		ServeSafe: true,
		Fn: func(ctx *engine.Context, _ map[string]any) (any, error) {
			rows := ctx.Graph.IndexedFacts()
			result := make([]FactIndexRow, len(rows))
			for i, row := range rows {
				result[i] = FactIndexRow{ID: row.ID, Title: row.Title, Topic: row.Topic.String()}
			}
			return result, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.RegisterQuery(engine.Query{
		Doc:       engine.FuncDoc{Name: "viewLayout", Doc: "Rendered `sdd view` pipeline result. Arg layout: the full pipeline syntax; may be a Go template over the store. The engine bounds the result at the inject's declared maxBytes (line-boundary cut, honest notice; the cut's pull is the lane's own layout)."},
		ServeSafe: true,
		Bound: engine.QueryBound{
			Part: serveview.PartText,
			Cap:  serveview.Default().Cap(serveview.PartText),
			Pull: func(args map[string]any, _ truncate.Cut) string {
				layout, _ := args["layout"].(string)
				return layout
			},
		},
		Fn: func(ctx *engine.Context, args map[string]any) (any, error) {
			layout, _ := args["layout"].(string)
			if strings.TrimSpace(layout) == "" {
				return nil, fmt.Errorf("viewLayout needs arg layout")
			}
			view, err := w.graphs.viewFor(ctx.Store)
			if err != nil {
				return nil, err
			}
			result, err := w.app.viewFromSnapshot(w.ctx, w.identity, view.runtime, view.snapshot, ViewRequest{Layout: layout, Budget: servedViewBudget})
			if err != nil {
				return nil, err
			}
			return result.Sections, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.RegisterQuery(engine.Query{
		// Not serve-safe: it calls LogRead, so it writes a read event. It may
		// inject into a step unit, but never as a framing lane (a serve must not
		// write — I7); the spec loader enforces that.
		Doc:   engine.FuncDoc{Name: "entryChains", Doc: "Entries with upstream/downstream chains for an explicit id arg or the store's anchor/targets. Args up, down: expansion depths; onceFull: serve nothing when every entry was already served in full this session; requireBody: fail when a primary resolves to an empty body. An explicit id resolves to its live supersession head before serving, so a project override wins and the read logs under the same ID the dedup checks. Chains are bounded per direction by the engine's entry-list budget; the cut renders as the same honest frontier the depth limit uses.", Reads: []string{"anchor", "targets"}},
		Bound: engine.QueryBound{Part: serveview.PartEntryList},
		Fn: func(ctx *engine.Context, args map[string]any) (any, error) {
			var ids []string
			if id, _ := args["id"].(string); strings.TrimSpace(id) != "" {
				ids = append(ids, workflowLiveHead(ctx.Graph, strings.TrimSpace(id)))
			} else {
				if id, ok := workflowStoreString(ctx.Store, "anchor"); ok {
					ids = append(ids, id)
				}
				ids = append(ids, workflowStoreStrings(ctx.Store, "targets")...)
			}
			if len(ids) == 0 {
				return nil, fmt.Errorf("entryChains: no explicit id and neither anchor nor targets is set in the store")
			}
			if onceFull, _ := args["onceFull"].(bool); onceFull {
				pending := ids[:0]
				for _, id := range ids {
					if ctx.Reads[workflowLiveHead(ctx.Graph, id)] != engine.ReadFull {
						pending = append(pending, id)
					}
				}
				ids = pending
				if len(ids) == 0 {
					return "", nil
				}
			}
			if requireBody, _ := args["requireBody"].(bool); requireBody {
				for _, id := range ids {
					if model.IsCrossRepoID(id) {
						return nil, fmt.Errorf("entryChains: requireBody does not support cross-repo primary %s", id)
					}
					if _, _, err := ctx.Graph.FactBody(id); err != nil {
						return nil, fmt.Errorf("entryChains: %w", err)
					}
				}
			}
			view, err := w.graphs.viewFor(ctx.Store)
			if err != nil {
				return nil, err
			}
			result, err := showFromSnapshot(view.runtime, view.snapshot, ShowRequest{
				IDs: ids, UpDepth: workflowIntArg(args, "up", query.DefaultUpDepth), DownDepth: workflowIntArg(args, "down", query.DefaultDownDepth),
				Budget: serveChainBudget,
			})
			if err != nil {
				return nil, err
			}
			w.session.LogRead("inject:entryChains", result.FullIDs, result.SummaryIDs)
			return result.Entries, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.RegisterQuery(engine.Query{
		Doc:       engine.FuncDoc{Name: "topicLabels", Doc: "The distinct topic labels in use across active entries — bare and sorted, one per line. Byte-stable across serves, unlike the count/heat table `as-counts` renders."},
		ServeSafe: true,
		Fn: func(ctx *engine.Context, _ map[string]any) (any, error) {
			labels := ctx.Graph.TopicLabels(ctx.Graph.Filter(model.GraphFilter{OpenOnly: true}))
			return strings.Join(labels, "\n"), nil
		},
	}); err != nil {
		return err
	}
	if err := registry.RegisterQuery(engine.Query{
		Doc:       engine.FuncDoc{Name: "procedureList", Doc: "The live playbook moves, one line each: canonical, a compact signature of its accepted start params, and the first sentence of the head entry's summary. Shell-class procedures are excluded — they enter through start_session."},
		ServeSafe: true,
		Fn: func(ctx *engine.Context, _ map[string]any) (any, error) {
			return renderProcedureList(ctx.Graph), nil
		},
	}); err != nil {
		return err
	}
	if err := registry.RegisterQuery(engine.Query{
		Doc:       engine.FuncDoc{Name: "entryHead", Doc: "The live supersession head of the entry named by arg id — a pointer, not a serve; an empty id yields an empty result. Arg requireBody: fail when the head has an empty body, so the pointer never aims at nothing."},
		ServeSafe: true,
		Fn: func(ctx *engine.Context, args map[string]any) (any, error) {
			id, _ := args["id"].(string)
			id = strings.TrimSpace(id)
			if id == "" {
				return "", nil
			}
			if model.IsCrossRepoID(id) {
				return id, nil
			}
			if requireBody, _ := args["requireBody"].(bool); requireBody {
				head, _, err := ctx.Graph.FactBody(id)
				if err != nil {
					return nil, fmt.Errorf("entryHead: %w", err)
				}
				return head, nil
			}
			return ctx.Graph.ResolveRef(id).Head(), nil
		},
	}); err != nil {
		return err
	}
	return registry.RegisterQuery(engine.Query{
		Doc:       engine.FuncDoc{Name: "generatedSummary", Doc: "The stored summary of the entry named by the store's entryId, for fidelity review.", Reads: []string{"entryId"}},
		ServeSafe: true,
		Fn: func(ctx *engine.Context, _ map[string]any) (any, error) {
			id, ok := workflowStoreString(ctx.Store, "entryId")
			if !ok {
				return nil, fmt.Errorf("generatedSummary: entryId is not set — the write gate has not created an entry")
			}
			entry, ok := ctx.Graph.ByID[id]
			if !ok {
				return nil, fmt.Errorf("generatedSummary: entry %s not found", id)
			}
			return entry.Summary, nil
		},
	})
}

func (w *WorkflowSession) registerWorkflowWrites(registry *engine.Registry) error {
	if err := registry.RegisterPredicate(engine.Predicate{
		Doc: engine.FuncDoc{Name: "capturePreflightCurrent", Doc: "The recorded preflight covers the current draft, staged attachments and target.", Reads: captureDraftFields},
		Fn:  w.capturePreflightCurrent,
	}); err != nil {
		return err
	}
	if err := registry.RegisterCommand(engine.Command{
		Doc: engine.FuncDoc{Name: "preflightEntry", Doc: "Checks the current capture draft before publication and records its findings.",
			Reads: captureDraftFields, Writes: []string{"findings", "resolvedCaptureBranch", "preflightOverride"}},
		GraphIndependent: true,
		Fn:               w.runWorkflowPreflight,
	}); err != nil {
		return err
	}
	if err := registry.RegisterCommand(engine.Command{
		Doc: engine.FuncDoc{
			Name: "newEntry", Doc: "Publishes the recorded entry identity from accepted capture state and immutable staged references.",
			Reads: []string{"body", "entryKind", "layer", "refs", "topics", "index", "confidence", "intent", "attachments", "participants", "supersedes", "closes", "canonical", "aliases", "roleActor", "involvement", "focusActors", "focusWhen", "preflightOverride"}, Writes: []string{"resolvedCaptureBranch", "entryId", "findings"},
		},
		MutatesGraph:     true,
		GraphIndependent: true,
		Prepare:          w.prepareWorkflowNewEntry,
		Fn:               w.runWorkflowNewEntry,
		Effects:          w.reportWorkflowNewEntryEffects,
	}); err != nil {
		return err
	}
	if err := registry.RegisterCommand(engine.Command{
		Doc: engine.FuncDoc{
			Name: "writingGuide", Doc: "Runs the writing guide against the draft in isolation (d-cpt-20r); findings are drafting input, never a gate. Runs once per capture — a recorded run is never repeated implicitly; requestGuideRecheck clears it for an explicit re-run.",
			Reads: []string{"body", "entryKind", "layer", "refs", "intent", "attachments", "closes", "supersedes"}, Writes: []string{"guideFindings"},
		},
		Fn: w.runWorkflowWritingGuide,
	}); err != nil {
		return err
	}
	return registry.RegisterCommand(engine.Command{
		Doc:              engine.FuncDoc{Name: "replaceSummary", Doc: "Writes the user-supplied corrected summary onto the entry named by entryId, conditioned on the document the correction was read from.", Reads: []string{"entryId", "correctedSummary"}},
		MutatesGraph:     true,
		GraphIndependent: true,
		Prepare:          w.prepareWorkflowReplaceSummary,
		Fn:               w.runWorkflowReplaceSummary,
		Effects:          w.reportWorkflowReplaceSummaryEffects,
	})
}

func (w *WorkflowSession) registerWorkflowWIP(registry *engine.Registry) error {
	if err := registry.RegisterCommand(engine.Command{
		Doc:              engine.FuncDoc{Name: "wipStart", Doc: "Creates an exclusive WIP marker for the store's anchor entry on the session's current branch, described by wipDescription.", Reads: []string{"anchor", "wipDescription", "participants"}, Writes: []string{"wipMarker"}},
		MutatesGraph:     true,
		GraphIndependent: true,
		Prepare:          w.prepareWorkflowWIPStart,
		Fn:               w.runWorkflowWIPStart,
		Effects:          w.reportWorkflowWIPEffects,
	}); err != nil {
		return err
	}
	if err := registry.RegisterCommand(engine.Command{
		Doc:              engine.FuncDoc{Name: "wipDone", Doc: "Removes the WIP marker named by the store's wipMarker field from the session's current branch.", Reads: []string{"wipMarker"}, Writes: []string{"wipMarker"}},
		MutatesGraph:     true,
		GraphIndependent: true,
		Prepare:          w.prepareWorkflowWIPDone,
		Fn:               w.runWorkflowWIPDone,
		Effects:          w.reportWorkflowWIPEffects,
	}); err != nil {
		return err
	}
	return registry.RegisterCommand(engine.Command{
		Doc:              engine.FuncDoc{Name: "wipRemove", Doc: "Removes the WIP marker named by the store's staleMarker field (groom's orphaned-marker cleanup).", Reads: []string{"staleMarker"}},
		MutatesGraph:     true,
		GraphIndependent: true,
		Prepare:          w.prepareWorkflowWIPRemove,
		Fn:               w.runWorkflowWIPRemove,
		Effects:          w.reportWorkflowWIPEffects,
	})
}

// runWorkflowWritingGuide runs the writing guide op between assemble and
// playback — once per capture. A recorded run (guideFindings present) is
// never repeated implicitly: revises and adjusts pass through, because a
// second pass judges the repairs instead of the draft and drifts toward
// find-something escalation; re-running is the agent's explicit choice
// (requestGuideRecheck). An LLM infrastructure failure returns loudly — the
// op re-runs on the next report rather than degrading to a silent pass.
func (w *WorkflowSession) runWorkflowWritingGuide(ctx *engine.Context) error {
	// A nil value is a cleared run (requestGuideRecheck), not a recorded one.
	if v, ok := ctx.Store.Get("guideFindings"); ok && v != nil {
		return nil
	}
	for _, field := range []string{"body", "entryKind", "layer"} {
		if _, ok := workflowStoreString(ctx.Store, field); !ok {
			return fmt.Errorf("writingGuide: %s is not set", field)
		}
	}
	findings, err := w.app.WritingGuideCheck(w.ctx, w.identity, w.instanceProject(ctx.Instance), w.draftFromStore(ctx.Store))
	if err != nil {
		return fmt.Errorf("writingGuide: %w", err)
	}
	guideFindings := make([]query.GuideFinding, 0, len(findings))
	for _, f := range findings {
		guideFindings = append(guideFindings, query.GuideFinding{Reasoning: f.Reasoning, Axis: f.Axis, Quote: f.Quote, Repair: f.Repair, Severity: query.GuideSeverity(f.Severity)})
	}
	if err := ctx.Store.WriteEngine("guideFindings", guideFindings); err != nil {
		return fmt.Errorf("writingGuide: %w", err)
	}
	return nil
}

func (w *WorkflowSession) draftFromStore(store *engine.Store) EntryDraft {
	draft := EntryDraft{
		Topics: workflowStoreStrings(store, "topics"),
		Closes: workflowStoreStrings(store, "closes"), Supersedes: workflowStoreStrings(store, "supersedes"),
		AttachmentHandles: workflowStoreStrings(store, "attachments"), Participants: workflowStoreStrings(store, "participants"),
	}
	draft.Kind, _ = workflowStoreString(store, "entryKind")
	draft.Layer, _ = workflowStoreString(store, "layer")
	draft.Body, _ = workflowStoreString(store, "body")
	draft.Confidence, _ = workflowStoreString(store, "confidence")
	draft.Intent, _ = workflowStoreString(store, "intent")
	if index, ok := workflowStoreDocument(store, "index"); ok {
		title, _ := index["title"].(string)
		topic, _ := index["topic"].(string)
		draft.Index = &FactIndex{Title: title, Topic: topic}
	}
	draft.Canonical, _ = workflowStoreString(store, "canonical")
	draft.Actor, _ = workflowStoreString(store, "roleActor")
	draft.Class, _ = workflowStoreString(store, "class")
	if spec, ok := workflowStoreDocument(store, "procedureSpec"); ok {
		draft.ProcedureSpec = spec
	}
	draft.Aliases = workflowStoreStrings(store, "aliases")
	draft.FocusActors = workflowStoreStrings(store, "focusActors")
	// Store values are normalized JSON documents, so focusWhen comes back as a
	// map keyed by its JSON field names and involvement as a list of such maps —
	// not as typed engine.When / engine.Involvement values.
	if when, ok := workflowStoreDocument(store, "focusWhen"); ok {
		draft.FocusWhen = focusWhenFromDocument(when)
	}
	for _, inv := range workflowStoreDocuments(store, "involvement") {
		target, _ := inv["target"].(string)
		involvement := types.Involvement{Target: target, When: focusWhenFromDocument(inv["when"])}
		// "actors" is present in the normalized document only when it was set
		// (engine.Involvement.MarshalJSON), so key presence reconstructs the
		// unset-vs-explicit-empty distinction the model relies on.
		if actors, ok := inv["actors"]; ok {
			involvement.ActorsSet = true
			involvement.Actors = documentStrings(actors)
		}
		draft.Involvement = append(draft.Involvement, involvement)
	}
	for _, ref := range workflowStoreDocuments(store, "refs") {
		id, _ := ref["id"].(string)
		kind, _ := ref["kind"].(string)
		desc, _ := ref["desc"].(string)
		draft.Refs = append(draft.Refs, EntryRef{ID: id, Kind: kind, Desc: desc})
	}
	return draft
}

func (w *WorkflowSession) runWorkflowNewEntry(ctx *engine.Context) error {
	if ctx.Intent == nil {
		return fmt.Errorf("newEntry requires a recorded invocation")
	}
	target := MutationTarget{Project: ProjectID(ctx.Intent.Values["project"]), Branch: ctx.Intent.Values["branch"]}
	if err := w.authorizeTarget(target.Project, AccessWrite); err != nil {
		return err
	}
	draft := w.draftFromStore(ctx.Store)
	draft.Target = target
	draft.EntryID = ctx.Intent.Values["entryId"]
	draft.Publication = w.newEntryPublicationKey(ctx.Intent)
	staged, err := w.stagedAt(ctx.Intent.Ref)
	if err != nil {
		return err
	}
	if err := captureAttachments(&draft, staged); err != nil {
		return err
	}
	if override, ok := ctx.Store.Get("preflightOverride"); ok {
		draft.SkipPreflight, _ = override.(bool)
	}
	result, err := w.app.CreateEntry(w.ctx, w.identity, target.Project, w.binding, draft)
	if err != nil {
		return fmt.Errorf("newEntry: %w", withTargetRemedy(ctx.Intent, err))
	}
	w.binding = result.Binding
	if err := ctx.Store.WriteEngine("entryId", result.EntryID); err != nil {
		return err
	}
	w.session.LogRead("newEntry", []string{result.EntryID}, nil)
	return w.session.SinkErr()
}

// wipTarget is the WIP marker's authority: the session's current branch,
// resolved to a concrete branch the intent records with its provenance
// (20260923-230855-d-cpt-34w).
func (w *WorkflowSession) wipTarget(ctx *engine.Context) (map[string]string, error) {
	target, source, err := w.concreteEffectiveTarget(ctx.Store)
	if err != nil {
		return nil, err
	}
	if err := w.authorizeTarget(target.Project, AccessWrite); err != nil {
		return nil, err
	}
	return map[string]string{"project": string(target.Project), "branch": target.Branch, "source": source}, nil
}

// workflowBranchFields is the application-owned registry of procedure state
// fields carrying branch authority, in precedence order: capture state names
// captureBranch, published entries use resolvedCaptureBranch. The
// implementation procedure holds none (20260923-230855-d-cpt-34w). A procedure
// that introduces another branch-bearing field must register it here, or its
// reads and writes silently fall back to the session binding.
var workflowBranchFields = [...]string{"captureBranch", "resolvedCaptureBranch"}

// targetSourceBinding names the durable session binding as the branch source
// in a target provenance, targetSourceBase the session's derived base; a state
// field names itself; empty means the branch is not yet resolved.
const (
	targetSourceBinding = "binding"
	targetSourceBase    = "base"
)

// effectiveTarget is the sole target precedence rule, shared by graph reads
// and graph writes: the project is the instance's (d-cpt-yjc); the branch is
// an explicit procedure-state branch, then the durable session binding —
// reported by the second result, and a fact about the home checkout only
// (20260722-112853-d-tac-ln1) — then empty, which lets the application resolve
// the target project's configured default_branch. No cwd or other ambient
// state participates, and the engine stays unaware of what these fields mean.
func (w *WorkflowSession) effectiveTarget(store *engine.Store) (MutationTarget, bool) {
	return w.effectiveTargetFor(w.projectFor(store), store)
}

func (w *WorkflowSession) effectiveTargetFor(project ProjectID, store *engine.Store) (MutationTarget, bool) {
	target, source := w.effectiveTargetSource(project, store)
	return target, source == targetSourceBinding
}

// effectiveTargetSource is effectiveTargetFor with the branch's provenance:
// the state field that chose it, targetSourceBinding for the session binding,
// or empty for the configured default. A failed read reports it, so an agent
// sees which field sent the read to a branch that has no checkout.
func (w *WorkflowSession) effectiveTargetSource(project ProjectID, store *engine.Store) (MutationTarget, string) {
	entryID, _ := workflowStoreString(store, "entryId")
	for _, field := range workflowBranchFields {
		// A preflight result does not pin the branch before publication.
		if field == "resolvedCaptureBranch" && entryID == "" {
			continue
		}
		if branch, _ := workflowStoreString(store, field); branch != "" {
			return MutationTarget{Project: project, Branch: branch}, field
		}
	}
	if w.branch != "" && project == w.project {
		return MutationTarget{Project: project, Branch: w.branch}, targetSourceBinding
	}
	return MutationTarget{Project: project}, ""
}

// concreteEffectiveTarget resolves an unbound target to the session's base
// without mutating procedure state, returning the branch's provenance as
// effectiveTargetSource names it, or targetSourceBase.
func (w *WorkflowSession) concreteEffectiveTarget(store *engine.Store) (MutationTarget, string, error) {
	project := w.projectFor(store)
	target, source := w.effectiveTargetSource(project, store)
	if target.Branch != "" {
		return target, source, nil
	}
	runtime, err := w.targetRuntime(target.Project, AccessRead)
	if err != nil {
		return MutationTarget{}, "", err
	}
	target, _, err = runtime.baseTarget(w.ctx)
	if err != nil {
		return MutationTarget{}, "", err
	}
	return target, targetSourceBase, nil
}

// withTargetRemedy names the remedy when a recorded write's branch cannot be
// acquired, by the provenance the intent recorded (20260923-233057-d-cpt-ekd).
func withTargetRemedy(intent *engine.MutationIntent, err error) error {
	branch := intent.Values["branch"]
	switch intent.Values["source"] {
	case targetSourceBinding:
		return withSessionBindingTargetError(branch, true, err)
	case targetSourceBase:
		return withBaseTargetError(branch, err)
	}
	return err
}

func (w *WorkflowSession) withSessionBindingTargetError(err error, fromBinding bool) error {
	return withSessionBindingTargetError(w.branch, fromBinding, err)
}

// focusWhenFromDocument builds a types.FocusWhen from a normalized store value.
// The value is a JSON document (map keyed by When's json field names) or absent;
// a range with neither end set collapses to nil.
func focusWhenFromDocument(v any) *types.FocusWhen {
	doc, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	from, _ := doc["from"].(string)
	to, _ := doc["to"].(string)
	if from == "" && to == "" {
		return nil
	}
	return &types.FocusWhen{From: from, To: to}
}

// documentStrings coerces a normalized list value ([]any of strings) to []string,
// skipping any non-string element.
func documentStrings(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func workflowStoreString(store *engine.Store, name string) (string, bool) {
	value, ok := store.Get(name)
	if !ok {
		return "", false
	}
	result, ok := value.(string)
	return result, ok && result != ""
}

// workflowStoreDocument reads a single object-shaped store value. Store values
// are normalized JSON documents, so a ref or fact-index comes back as a
// map[string]any keyed by its JSON field names, not as a typed struct.
func workflowStoreDocument(store *engine.Store, name string) (map[string]any, bool) {
	value, ok := store.Get(name)
	if !ok {
		return nil, false
	}
	doc, ok := value.(map[string]any)
	return doc, ok
}

// workflowStoreDocuments reads a list of object-shaped store values (e.g. refs),
// skipping any element that is not a JSON object.
func workflowStoreDocuments(store *engine.Store, name string) []map[string]any {
	value, ok := store.Get(name)
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	docs := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if doc, ok := item.(map[string]any); ok {
			docs = append(docs, doc)
		}
	}
	return docs
}

func workflowStoreStrings(store *engine.Store, name string) []string {
	value, ok := store.Get(name)
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok && text != "" {
			result = append(result, text)
		}
	}
	return result
}

func workflowIntArg(args map[string]any, name string, fallback int) int {
	switch value := args[name].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return fallback
	}
}

// ProcedureRegistry returns the engine registration procedure specs load and
// size against — the same bounds the runtime serves with (d-tac-rzi).
// Composition helper for the transitional CLI write path and `sdd lint`; the
// application's own flows wire it internally.
func ProcedureRegistry() (*engine.Registry, error) {
	return (&WorkflowSession{}).buildRegistry()
}

func WorkflowRegistryDocs(class string) ([]RegistryFunction, error) {
	switch engine.FuncClass(class) {
	case "", engine.ClassPredicate, engine.ClassQuery, engine.ClassCommand:
	default:
		return nil, fmt.Errorf("unknown registry class %q", class)
	}
	registry, err := (&WorkflowSession{}).buildRegistry()
	if err != nil {
		return nil, err
	}
	docs := registry.Docs(engine.FuncClass(class))
	result := make([]RegistryFunction, 0, len(docs))
	for _, doc := range docs {
		result = append(result, RegistryFunction{
			Name: doc.Name, Class: string(doc.Class), Doc: doc.Doc,
			Reads: append([]string(nil), doc.Reads...), Writes: append([]string(nil), doc.Writes...),
		})
	}
	return result, nil
}

// workflowLiveHead resolves a local ID to its supersession head. A cross-repo
// ID passes through unwalked — a deliberate deferral, not a missing capability:
// the context graph composes dependencies and ResolveAcross can walk the
// owning member's chain, but serve-time member graphs are best-effort
// (mirroring refsResolve's carve-out) and no shipped procedure serves a
// foreign ID yet. Whether the walk is wanted, and relative to which owning
// repo an ID resolves, is the open gap 20260819-171110-s-tac-4km.
func workflowLiveHead(graph *model.Graph, id string) string {
	if model.IsCrossRepoID(id) {
		return id
	}
	return graph.ResolveRef(id).Head()
}

// FactIndexRow is the application-boundary shape of an indexed fact: plain,
// serializable strings only. Topic carries the canonical slash-joined form
// (e.g. "cli/view"). ID and Title match the template keys the user-dialogue
// procedure renders from the factIndex inject result.
type FactIndexRow struct {
	ID    string
	Title string
	Topic string
}

type RegistryFunction struct {
	Name   string
	Class  string
	Doc    string
	Reads  []string
	Writes []string
}
