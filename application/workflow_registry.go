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
	return registry.RegisterPredicate(engine.Predicate{
		Doc: engine.FuncDoc{Name: "sessionQuiescent", Doc: "Nothing but the session shell is running — no open move instances in this session."},
		Fn: func(*engine.Context) (bool, error) {
			for _, instance := range w.session.Instances() {
				if instance.ID != w.shell && instance.Status == engine.StatusRunning {
					return false, nil
				}
			}
			return true, nil
		},
		FailMessage: "open threads remain — settle each with the user (finish it, abandon it, or park the session)",
	})
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
		Doc:       engine.FuncDoc{Name: "viewLayout", Doc: "Rendered `sdd view` pipeline result. Arg layout: the full pipeline syntax; may be a Go template over the store. Optional arg maxBytes: cap the rendered result on a line boundary (0 = uncapped), for a framing lane that must stay bounded."},
		ServeSafe: true,
		Fn: func(_ *engine.Context, args map[string]any) (any, error) {
			layout, _ := args["layout"].(string)
			if strings.TrimSpace(layout) == "" {
				return nil, fmt.Errorf("viewLayout needs arg layout")
			}
			result, err := w.app.View(w.ctx, w.identity, w.project, ViewRequest{Layout: layout})
			if err != nil {
				return nil, err
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
		Doc: engine.FuncDoc{Name: "entryChains", Doc: "Entries with upstream/downstream chains for the store's anchor (or targets). Args up, down: expansion depths.", Reads: []string{"anchor", "targets"}},
		Fn: func(ctx *engine.Context, args map[string]any) (any, error) {
			var ids []string
			if id, ok := workflowStoreString(ctx.Store, "anchor"); ok {
				ids = append(ids, id)
			}
			ids = append(ids, workflowStoreStrings(ctx.Store, "targets")...)
			if len(ids) == 0 {
				return nil, fmt.Errorf("entryChains: neither anchor nor targets is set in the store")
			}
			result, err := w.app.Show(w.ctx, w.identity, w.project, ShowRequest{
				IDs: ids, UpDepth: workflowIntArg(args, "up", query.DefaultUpDepth), DownDepth: workflowIntArg(args, "down", query.DefaultDownDepth),
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
			Reads: []string{"body", "entryKind", "layer", "refs", "topics", "confidence", "intent", "attachments", "participants", "supersedes", "closes", "canonical", "aliases", "roleActor", "involvement", "focusActors", "focusWhen", "preflightOverride"}, Writes: []string{"entryId", "findings"},
		},
		MutatesGraph: true,
		Fn:           w.runWorkflowNewEntry,
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
			result, err := w.app.ReplaceSummary(w.ctx, w.identity, w.project, w.binding, w.mutationTarget(ctx.Store, "captureBranch"), id, text)
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
			marker, result, err := w.app.StartWIP(w.ctx, w.identity, w.project, w.binding, w.mutationTarget(ctx.Store, "baseBranch"), anchor, description)
			if err != nil {
				return err
			}
			w.binding = result.Binding
			ctx.Store.WriteEngine("wipMarker", marker)
			return nil
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
			result, err := w.app.FinishWIP(w.ctx, w.identity, w.project, w.binding, w.mutationTarget(ctx.Store, "baseBranch"), marker)
			if err != nil {
				return err
			}
			w.binding = result.Binding
			ctx.Store.WriteEngine("wipMarker", nil)
			return nil
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

func (w *WorkflowSession) runWorkflowNewEntry(ctx *engine.Context) error {
	entryKind, ok := workflowStoreString(ctx.Store, "entryKind")
	if !ok {
		return fmt.Errorf("newEntry: entryKind is not set")
	}
	layer, ok := workflowStoreString(ctx.Store, "layer")
	if !ok {
		return fmt.Errorf("newEntry: layer is not set")
	}
	body, ok := workflowStoreString(ctx.Store, "body")
	if !ok {
		return fmt.Errorf("newEntry: body is not set")
	}
	draft := EntryDraft{
		Target: w.mutationTarget(ctx.Store, "captureBranch"),
		Kind:   entryKind, Layer: layer, Body: body, Topics: workflowStoreStrings(ctx.Store, "topics"),
		Closes: workflowStoreStrings(ctx.Store, "closes"), Supersedes: workflowStoreStrings(ctx.Store, "supersedes"),
		AttachmentHandles: workflowStoreStrings(ctx.Store, "attachments"), Participants: workflowStoreStrings(ctx.Store, "participants"),
	}
	for index, handle := range draft.AttachmentHandles {
		if blobID, ok := w.staged[handle]; ok {
			draft.AttachmentHandles[index] = blobID
		}
	}
	draft.Confidence, _ = workflowStoreString(ctx.Store, "confidence")
	draft.Intent, _ = workflowStoreString(ctx.Store, "intent")
	draft.Canonical, _ = workflowStoreString(ctx.Store, "canonical")
	draft.Actor, _ = workflowStoreString(ctx.Store, "roleActor")
	draft.Aliases = workflowStoreStrings(ctx.Store, "aliases")
	draft.FocusActors = workflowStoreStrings(ctx.Store, "focusActors")
	if v, ok := ctx.Store.Get("focusWhen"); ok {
		if w, ok := v.(*engine.When); ok {
			draft.FocusWhen = modelFocusWhen(w)
		}
	}
	if v, ok := ctx.Store.Get("involvement"); ok {
		if items, ok := v.([]any); ok {
			for _, item := range items {
				if inv, ok := item.(engine.Involvement); ok {
					draft.Involvement = append(draft.Involvement, model.Involvement{
						Target:    inv.Target,
						Actors:    append([]string(nil), inv.Actors...),
						ActorsSet: inv.ActorsSet,
						When:      modelFocusWhen(inv.When),
					})
				}
			}
		}
	}
	if override, ok := ctx.Store.Get("preflightOverride"); ok {
		draft.SkipPreflight, _ = override.(bool)
	}
	if value, ok := ctx.Store.Get("refs"); ok {
		if refs, ok := value.([]any); ok {
			for _, item := range refs {
				if ref, ok := item.(engine.Ref); ok {
					draft.Refs = append(draft.Refs, EntryRef{ID: ref.ID, Kind: ref.Kind, Desc: ref.Desc})
				}
			}
		}
	}
	result, err := w.app.CreateEntry(w.ctx, w.identity, w.project, w.binding, draft)
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
				observation = fmt.Sprintf("%s (field: %s)", warning.Message, warning.Field)
			}
			findings = append(findings, query.Finding{
				Severity:    query.SeverityHigh,
				Category:    "validation",
				Observation: observation,
			})
		}
		ctx.Store.WriteEngine("findings", findings)
		return nil
	}
	if err == nil {
		w.binding = result.Binding
	}
	findings := make([]query.Finding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		findings = append(findings, query.Finding{Severity: query.Severity(finding.Severity), Category: finding.Category, Observation: finding.Observation})
	}
	ctx.Store.WriteEngine("findings", findings)
	if err != nil {
		return fmt.Errorf("newEntry: %w", err)
	}
	for _, finding := range findings {
		if finding.Severity == query.SeverityHigh {
			return nil
		}
	}
	ctx.Store.WriteEngine("entryId", result.EntryID)
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

func modelFocusWhen(w *engine.When) *model.FocusWhen {
	if w == nil {
		return nil
	}
	return &model.FocusWhen{From: w.From, To: w.To}
}

func workflowStoreString(store *engine.Store, name string) (string, bool) {
	value, ok := store.Get(name)
	if !ok {
		return "", false
	}
	result, ok := value.(string)
	return result, ok && result != ""
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

type RegistryFunction struct {
	Name   string
	Class  string
	Doc    string
	Reads  []string
	Writes []string
}
