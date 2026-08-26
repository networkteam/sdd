package application

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

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
		Doc:       engine.FuncDoc{Name: "sessionInfo", Doc: "Session framing: local participant, configured language, available search modes, and actionable recovery notices."},
		ServeSafe: true,
		Fn: func(*engine.Context, map[string]any) (any, error) {
			info, err := w.app.Info(w.ctx, w.identity, w.project, InfoRequest{})
			if err != nil {
				return nil, err
			}
			return map[string]any{"participant": info.Participant, "language": info.Language, "search": info.Search, "recovery": info.Recovery}, nil
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
		Doc:       engine.FuncDoc{Name: "viewLayout", Doc: "Rendered `sdd view` pipeline result. Arg layout: the full pipeline syntax; may be a Go template over the store. Optional arg maxBytes: cap the rendered result on a line boundary (0 = uncapped), for a framing lane that must stay bounded."},
		ServeSafe: true,
		Fn: func(ctx *engine.Context, args map[string]any) (any, error) {
			layout, _ := args["layout"].(string)
			if strings.TrimSpace(layout) == "" {
				return nil, fmt.Errorf("viewLayout needs arg layout")
			}
			target, fromBinding := w.effectiveTarget(ctx.Store)
			result, err := w.app.View(w.ctx, w.identity, w.project, ViewRequest{Layout: layout, Branch: target.Branch})
			if err != nil {
				return nil, w.withSessionBindingTargetError(err, fromBinding)
			}
			return capOnLineBoundary(result.Sections, workflowIntArg(args, "maxBytes", 0)), nil
		},
	}); err != nil {
		return err
	}
	if err := registry.RegisterQuery(engine.Query{
		// Not serve-safe: it calls LogRead, so it writes a read event. It may
		// inject into a step unit, but never as a framing lane (a serve must not
		// write — I7); the spec loader enforces that.
		Doc: engine.FuncDoc{Name: "entryChains", Doc: "Entries with upstream/downstream chains for an explicit id arg or the store's anchor/targets. Args up, down: expansion depths; onceFull: serve nothing when every entry was already served in full this session; requireBody: fail when a primary resolves to an empty body. An explicit id resolves to its live supersession head before serving, so a project override wins and the read logs under the same ID the dedup checks.", Reads: []string{"anchor", "targets"}},
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
			target, fromBinding := w.effectiveTarget(ctx.Store)
			result, err := w.app.Show(w.ctx, w.identity, w.project, ShowRequest{
				IDs: ids, UpDepth: workflowIntArg(args, "up", query.DefaultUpDepth), DownDepth: workflowIntArg(args, "down", query.DefaultDownDepth),
				Branch: target.Branch,
			})
			if err != nil {
				return nil, w.withSessionBindingTargetError(err, fromBinding)
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
		Fn: func(*engine.Context, map[string]any) (any, error) {
			result, err := w.app.Procedures(w.ctx, w.identity, w.project, ProcedureListRequest{})
			if err != nil {
				return nil, err
			}
			return result.Procedures, nil
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
	if err := registry.RegisterCommand(engine.Command{
		Doc: engine.FuncDoc{
			Name: "newEntry", Doc: "Creates the entry from the capture state fields (pre-flight inside; staged attachments materialized from handles; a recorded override skips pre-flight, durably logged).",
			Reads: []string{"body", "entryKind", "layer", "refs", "topics", "index", "confidence", "intent", "attachments", "participants", "supersedes", "closes", "canonical", "aliases", "roleActor", "involvement", "focusActors", "focusWhen", "preflightOverride"}, Writes: []string{"resolvedCaptureBranch", "entryId", "findings"},
		},
		MutatesGraph: true,
		Fn:           w.runWorkflowNewEntry,
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
		Doc:          engine.FuncDoc{Name: "replaceSummary", Doc: "Writes the user-supplied corrected summary onto the entry named by entryId.", Reads: []string{"entryId", "correctedSummary"}},
		MutatesGraph: true,
		Fn: func(ctx *engine.Context) error {
			id, ok := workflowStoreString(ctx.Store, "entryId")
			if !ok {
				return fmt.Errorf("replaceSummary: entryId is not set")
			}
			text, ok := workflowStoreString(ctx.Store, "correctedSummary")
			if !ok {
				return fmt.Errorf("replaceSummary: correctedSummary is not set")
			}
			target, fromBinding := w.effectiveTarget(ctx.Store)
			result, err := w.app.ReplaceSummary(w.ctx, w.identity, w.project, w.binding, target, id, text)
			err = w.withSessionBindingTargetError(err, fromBinding)
			if err == nil {
				w.binding = result.Binding
			}
			return err
		},
	})
}

func (w *WorkflowSession) registerWorkflowWIP(registry *engine.Registry) error {
	if err := registry.RegisterCommand(engine.Command{
		Doc:          engine.FuncDoc{Name: "wipStart", Doc: "Creates an exclusive WIP marker for the store's anchor entry on baseBranch, described by wipDescription.", Reads: []string{"anchor", "baseBranch", "wipDescription", "participants"}, Writes: []string{"wipMarker"}},
		MutatesGraph: true,
		Fn: func(ctx *engine.Context) error {
			anchor, ok := workflowStoreString(ctx.Store, "anchor")
			if !ok {
				return fmt.Errorf("wipStart: anchor is not set")
			}
			description, _ := workflowStoreString(ctx.Store, "wipDescription")
			target := w.mutationTarget(ctx.Store, "baseBranch")
			if target == (MutationTarget{}) {
				return fmt.Errorf("WIP write requires an explicit baseBranch")
			}
			marker, result, err := w.app.StartWIP(w.ctx, w.identity, w.project, w.binding, target, anchor, description)
			if err != nil {
				return err
			}
			w.binding = result.Binding
			return ctx.Store.WriteEngine("wipMarker", marker)
		},
	}); err != nil {
		return err
	}
	if err := registry.RegisterCommand(engine.Command{
		Doc:          engine.FuncDoc{Name: "wipDone", Doc: "Removes the WIP marker named by the store's wipMarker field from baseBranch.", Reads: []string{"wipMarker", "baseBranch"}, Writes: []string{"wipMarker"}},
		MutatesGraph: true,
		Fn: func(ctx *engine.Context) error {
			marker, ok := workflowStoreString(ctx.Store, "wipMarker")
			if !ok {
				return fmt.Errorf("wipDone: wipMarker is not set")
			}
			target := w.mutationTarget(ctx.Store, "baseBranch")
			if target == (MutationTarget{}) {
				return fmt.Errorf("WIP write requires an explicit baseBranch")
			}
			result, err := w.app.FinishWIP(w.ctx, w.identity, w.project, w.binding, target, marker)
			if err != nil {
				return err
			}
			w.binding = result.Binding
			return ctx.Store.WriteEngine("wipMarker", nil)
		},
	}); err != nil {
		return err
	}
	return registry.RegisterCommand(engine.Command{
		Doc:          engine.FuncDoc{Name: "wipRemove", Doc: "Removes the WIP marker named by the store's staleMarker field (groom's orphaned-marker cleanup).", Reads: []string{"staleMarker"}},
		MutatesGraph: true,
		Fn: func(ctx *engine.Context) error {
			marker, ok := workflowStoreString(ctx.Store, "staleMarker")
			if !ok {
				return fmt.Errorf("wipRemove: staleMarker is not set")
			}
			result, err := w.app.FinishWIP(w.ctx, w.identity, w.project, w.binding, MutationTarget{}, marker)
			if err == nil {
				w.binding = result.Binding
			}
			return err
		},
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
	findings, err := w.app.WritingGuideCheck(w.ctx, w.identity, w.project, w.draftFromStore(ctx.Store))
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

// reportFieldForEntryField maps an entry-model field name to the capture
// store field the agent reports it under, so a served finding names the
// field the agent can actually fix. Inverse of draftFromStore's naming;
// fields absent here share their name on both sides.
var reportFieldForEntryField = func() func(string) string {
	names := map[string]string{
		"content": "body",
		"kind":    "entryKind",
		"actor":   "roleActor",
		"actors":  "focusActors",
		"when":    "focusWhen",
	}
	return func(field string) string {
		if mapped, ok := names[field]; ok {
			return mapped
		}
		return field
	}
}()

// draftFromStore reads the capture state fields into an EntryDraft — the one
// store-to-draft mapping, shared by the write op, the writing-guide op, and
// the draftValidates predicate so no surface restates the field set. Write-
// specific concerns (mutation target, staged-handle remap, pre-flight
// override) stay with the write op.
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
		involvement := model.Involvement{Target: target, When: focusWhenFromDocument(inv["when"])}
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
	for _, field := range []string{"entryKind", "layer", "body"} {
		if _, ok := workflowStoreString(ctx.Store, field); !ok {
			return fmt.Errorf("newEntry: %s is not set", field)
		}
	}
	target, fromBinding, resolvedDefault, err := w.concreteEffectiveTarget(ctx.Store)
	if err != nil {
		return err
	}
	draft := w.draftFromStore(ctx.Store)
	draft.Target = target
	for index, handle := range draft.AttachmentHandles {
		if blobID, ok := w.staged[handle]; ok {
			draft.AttachmentHandles[index] = blobID
		}
	}
	if override, ok := ctx.Store.Get("preflightOverride"); ok {
		draft.SkipPreflight, _ = override.(bool)
	}
	result, err := w.app.CreateEntry(w.ctx, w.identity, w.project, w.binding, draft)
	err = w.withSessionBindingTargetError(err, fromBinding)
	// A structural validation failure re-serves as high findings instead of
	// wedging the instance: the write step's `otherwise` routes to
	// reviseOrOverride, whose revise returns to assemble to fix the named rule
	// and field. No write happened, so the binding is left untouched.
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		findings := make([]query.Finding, 0, len(validationErr.Warnings))
		for _, warning := range validationErr.Warnings {
			observation := warning.Message
			if warning.Field != "" {
				observation = fmt.Sprintf("%s (field: %s)", warning.Message, reportFieldForEntryField(warning.Field))
			}
			findings = append(findings, query.Finding{
				Severity:    query.SeverityHigh,
				Category:    "validation",
				Observation: observation,
			})
		}
		if writeErr := ctx.Store.WriteEngine("findings", findings); writeErr != nil {
			return fmt.Errorf("newEntry: %w", writeErr)
		}
		return nil
	}
	if err == nil {
		w.binding = result.Binding
	}
	findings := make([]query.Finding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		findings = append(findings, query.Finding{Severity: query.Severity(finding.Severity), Category: finding.Category, Observation: finding.Observation})
	}
	if writeErr := ctx.Store.WriteEngine("findings", findings); writeErr != nil {
		return fmt.Errorf("newEntry: %w", writeErr)
	}
	if err != nil {
		return fmt.Errorf("newEntry: %w", err)
	}
	for _, finding := range findings {
		if finding.Severity == query.SeverityHigh {
			return nil
		}
	}
	if resolvedDefault {
		if err := ctx.Store.WriteEngine("resolvedCaptureBranch", target.Branch); err != nil {
			return fmt.Errorf("newEntry: pinning default capture branch: %w", err)
		}
	}
	if err := ctx.Store.WriteEngine("entryId", result.EntryID); err != nil {
		return fmt.Errorf("newEntry: %w", err)
	}
	w.session.LogRead("newEntry", []string{result.EntryID}, nil)
	return nil
}

func (w *WorkflowSession) mutationTarget(store *engine.Store, branchField string) MutationTarget {
	branch, _ := workflowStoreString(store, branchField)
	if branch == "" {
		return MutationTarget{}
	}
	return MutationTarget{Project: w.project, Branch: branch}
}

// workflowBranchFields is the application-owned registry of procedure state
// fields carrying branch authority, in precedence order: capture state names
// captureBranch, the write gate pins resolvedCaptureBranch, implementation
// state names workBranch. A procedure that introduces another branch-bearing
// field must register it here, or its reads and writes silently fall back to
// the session binding.
var workflowBranchFields = [...]string{"captureBranch", "resolvedCaptureBranch", "workBranch"}

// effectiveTarget is the sole branch precedence rule, shared by graph reads and
// graph writes (20260722-112853-d-tac-ln1): an explicit procedure-state branch
// wins, then the durable session binding — reported by the second result — then
// a zero target lets the application resolve configured default_branch. No cwd
// or other ambient state participates, and the engine stays unaware of what
// these fields mean.
func (w *WorkflowSession) effectiveTarget(store *engine.Store) (MutationTarget, bool) {
	for _, field := range workflowBranchFields {
		if target := w.mutationTarget(store, field); target != (MutationTarget{}) {
			return target, false
		}
	}
	if w.branch != "" {
		return MutationTarget{Project: w.project, Branch: w.branch}, true
	}
	return MutationTarget{}, false
}

// concreteEffectiveTarget resolves the configured default without mutating
// procedure state. The write gate records that default as an engine-owned
// resolvedCaptureBranch only after CreateEntry reports that an artifact was
// actually written.
func (w *WorkflowSession) concreteEffectiveTarget(store *engine.Store) (MutationTarget, bool, bool, error) {
	if target, fromBinding := w.effectiveTarget(store); target != (MutationTarget{}) {
		return target, fromBinding, false, nil
	}
	_, runtime, err := w.app.resolve(w.ctx, w.identity, w.project, AccessRead)
	if err != nil {
		return MutationTarget{}, false, false, err
	}
	target, err := runtime.defaultMutationTarget()
	if err != nil {
		return MutationTarget{}, false, false, err
	}
	return target, false, true, nil
}

func (w *WorkflowSession) withSessionBindingTargetError(err error, fromBinding bool) error {
	return withSessionBindingTargetError(w.branch, fromBinding, err)
}

// focusWhenFromDocument builds a model.FocusWhen from a normalized store value.
// The value is a JSON document (map keyed by When's json field names) or absent;
// a range with neither end set collapses to nil.
func focusWhenFromDocument(v any) *model.FocusWhen {
	doc, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	from, _ := doc["from"].(string)
	to, _ := doc["to"].(string)
	if from == "" && to == "" {
		return nil
	}
	return &model.FocusWhen{From: from, To: to}
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

// capOnLineBoundary truncates s to at most max bytes, preferring a line
// boundary and otherwise a UTF-8 rune boundary (never splitting a multi-byte
// rune), then appends a one-line elision notice. A non-positive max leaves s
// untouched.
func capOnLineBoundary(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	truncated := s[:max]
	if i := strings.LastIndexByte(truncated, '\n'); i > 0 {
		truncated = truncated[:i]
	} else {
		// No newline to cut on — back up to a rune boundary so the byte cap
		// never slices a multi-byte UTF-8 sequence in half.
		for len(truncated) > 0 && !utf8.RuneStart(truncated[len(truncated)-1]) {
			truncated = truncated[:len(truncated)-1]
		}
	}
	return strings.TrimRight(truncated, "\n") + "\n… (lane truncated to fit its byte cap)"
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
