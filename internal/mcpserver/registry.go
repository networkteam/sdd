package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// preflightTimeout matches the CLI default for sdd new.
const preflightTimeout = 2 * time.Minute

// buildRegistry assembles the engine registry for one shell session: the
// dependency-free built-ins plus the graph-coupled queries and effectful
// commands that the shell owns (per the engine core's contract, side effects
// register with the shell that wires their dependencies). Command closures
// capture the session because staging and graph refresh are session-scoped.
func (s *Server) buildRegistry(ss *shellSession) (*engine.Registry, error) {
	r := engine.NewRegistry()
	for _, register := range []func(*engine.Registry, *shellSession) error{
		s.registerQueries,
		s.registerShellPredicates,
		s.registerWriteCommands,
		s.registerWipCommands,
	} {
		if err := register(r, ss); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// registerShellPredicates wires the session-scoped predicates the engine
// core cannot own: they read the shell session's instance set, not the
// store or graph.
func (s *Server) registerShellPredicates(r *engine.Registry, ss *shellSession) error {
	return r.RegisterPredicate(engine.Predicate{
		Doc: engine.FuncDoc{
			Name: "sessionQuiescent",
			Doc:  "Nothing but the session shell is running — no open move instances in this session.",
		},
		Fn: func(_ *engine.Context) (bool, error) {
			return sessionQuiescent(ss), nil
		},
		FailMessage: "open threads remain — settle each with the user (finish it, abandon it, or park the session)",
	})
}

func (s *Server) registerQueries(r *engine.Registry, ss *shellSession) error {
	if err := r.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{
			Name: "sessionInfo",
			Doc:  "Session framing: local participant, configured language, available search modes (the sdd info header).",
		},
		Fn: func(_ *engine.Context, _ map[string]any) (any, error) {
			info, err := s.finder.Info(query.InfoQuery{})
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"participant": info.LocalParticipant,
				"language":    info.Language,
				"search":      info.Search,
			}, nil
		},
	}); err != nil {
		return err
	}

	if err := r.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{
			Name: "viewLayout",
			Doc:  "Rendered `sdd view` pipeline result. Arg layout: the full pipeline syntax; may be a Go template over the store.",
		},
		Fn: func(ctx *engine.Context, args map[string]any) (any, error) {
			layout, _ := args["layout"].(string)
			if strings.TrimSpace(layout) == "" {
				return nil, fmt.Errorf("viewLayout needs arg layout")
			}
			return s.renderView(ctx.Graph, layout)
		},
	}); err != nil {
		return err
	}

	if err := r.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{
			Name:  "entryChains",
			Doc:   "Entries with upstream/downstream chains for the store's anchor (or targets). Args up, down: expansion depths.",
			Reads: []string{"anchor", "targets"},
		},
		Fn: func(ctx *engine.Context, args map[string]any) (any, error) {
			var ids []string
			if v, ok := ctx.Store.Get("anchor"); ok {
				if id, ok := v.(string); ok && id != "" {
					ids = append(ids, id)
				}
			}
			if v, ok := ctx.Store.Get("targets"); ok {
				if list, ok := v.([]any); ok {
					for _, item := range list {
						if id, ok := item.(string); ok && id != "" {
							ids = append(ids, id)
						}
					}
				}
			}
			if len(ids) == 0 {
				return nil, fmt.Errorf("entryChains: neither anchor nor targets is set in the store")
			}
			return s.renderShow(ctx.Graph, ids, intArg(args, "up", query.DefaultUpDepth), intArg(args, "down", query.DefaultDownDepth))
		},
	}); err != nil {
		return err
	}

	if err := r.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{
			Name: "procedureList",
			Doc:  "The live playbook moves, one line each: canonical plus the first sentence of the head entry's summary. Shell-class procedures are excluded — they enter through start_session.",
		},
		Fn: func(ctx *engine.Context, _ map[string]any) (any, error) {
			var sb strings.Builder
			for _, chain := range ctx.Graph.ProcedureChains() {
				head := chain.Head
				if head == nil || head.Canonical == "" || head.IsShellProcedure() || head.IsTaskProcedure() || len(chain.LiveHeads) == 0 {
					continue
				}
				fmt.Fprintf(&sb, "- %s — %s\n", head.Canonical, head.FirstSummarySentence())
			}
			return strings.TrimRight(sb.String(), "\n"), nil
		},
	}); err != nil {
		return err
	}

	return r.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{
			Name:  "generatedSummary",
			Doc:   "The stored summary of the entry named by the store's entryId, for fidelity review.",
			Reads: []string{"entryId"},
		},
		Fn: func(ctx *engine.Context, _ map[string]any) (any, error) {
			v, ok := ctx.Store.Get("entryId")
			if !ok {
				return nil, fmt.Errorf("generatedSummary: entryId is not set — the write gate has not created an entry")
			}
			id, _ := v.(string)
			entry, ok := ctx.Graph.ByID[id]
			if !ok {
				return nil, fmt.Errorf("generatedSummary: entry %s not found", id)
			}
			return entry.Summary, nil
		},
	})
}

// registerWriteCommands wires the graph write path: newEntry (the write gate
// — pre-flight inside, override honored, staged attachments materialized)
// and replaceSummary. These exist only as procedure ops and chooser calls;
// no MCP tool reaches them directly.
func (s *Server) registerWriteCommands(r *engine.Registry, ss *shellSession) error {
	if err := r.RegisterCommand(engine.Command{
		Doc: engine.FuncDoc{
			Name: "newEntry",
			Doc: "Creates the entry from the capture state fields (pre-flight inside; staged attachments " +
				"materialized from handles; a recorded override skips pre-flight, durably logged).",
			Reads:  []string{"body", "entryKind", "layer", "refs", "topics", "confidence", "intent", "attachments", "participants", "supersedes", "closes", "preflightOverride"},
			Writes: []string{"entryId", "findings"},
		},
		Fn: func(ctx *engine.Context) error { return s.runNewEntry(ctx, ss) },
	}); err != nil {
		return err
	}

	return r.RegisterCommand(engine.Command{
		Doc: engine.FuncDoc{
			Name:  "replaceSummary",
			Doc:   "Writes the user-supplied corrected summary onto the entry named by entryId.",
			Reads: []string{"entryId", "correctedSummary"},
		},
		Fn: func(ctx *engine.Context) error {
			id, ok := storeString(ctx.Store, "entryId")
			if !ok {
				return fmt.Errorf("replaceSummary: entryId is not set")
			}
			text, ok := storeString(ctx.Store, "correctedSummary")
			if !ok {
				return fmt.Errorf("replaceSummary: correctedSummary is not set")
			}
			if err := s.handler.Summarize(context.Background(), &command.SummarizeCmd{
				EntryIDs:     []string{id},
				ExplicitText: &text,
			}); err != nil {
				return err
			}
			return s.refreshGraph(ss)
		},
	})
}

// registerWipCommands wires the WIP marker lifecycle. Owned by procedures,
// never a free tool: the implementation procedure opens and closes markers;
// orphaned markers fall to groom.
func (s *Server) registerWipCommands(r *engine.Registry, ss *shellSession) error {
	if err := r.RegisterCommand(engine.Command{
		Doc: engine.FuncDoc{
			Name:   "wipStart",
			Doc:    "Creates an exclusive WIP marker for the store's anchor entry, described by wipDescription.",
			Reads:  []string{"anchor", "wipDescription", "participants"},
			Writes: []string{"wipMarker"},
		},
		Fn: func(ctx *engine.Context) error {
			anchor, ok := storeString(ctx.Store, "anchor")
			if !ok {
				return fmt.Errorf("wipStart: anchor is not set")
			}
			desc, _ := storeString(ctx.Store, "wipDescription")
			cmd := &command.StartWIPCmd{
				EntryID:     anchor,
				Description: desc,
				Participant: s.resolveParticipant(ctx.Store),
				Exclusive:   true,
				OnStarted: func(markerID, _ string) {
					ctx.Store.WriteEngine("wipMarker", markerID)
				},
			}
			return s.handler.StartWIP(context.Background(), cmd)
		},
	}); err != nil {
		return err
	}

	return r.RegisterCommand(engine.Command{
		Doc: engine.FuncDoc{
			Name:   "wipDone",
			Doc:    "Removes the WIP marker named by the store's wipMarker field.",
			Reads:  []string{"wipMarker"},
			Writes: []string{"wipMarker"},
		},
		Fn: func(ctx *engine.Context) error {
			markerID, ok := storeString(ctx.Store, "wipMarker")
			if !ok {
				return fmt.Errorf("wipDone: wipMarker is not set")
			}
			if err := s.handler.FinishWIP(context.Background(), &command.FinishWIPCmd{MarkerID: markerID}); err != nil {
				return err
			}
			ctx.Store.WriteEngine("wipMarker", nil)
			return nil
		},
	})
}

// runNewEntry is the write gate: it assembles a NewEntryCmd from the store,
// runs the handler (pre-flight inside), and writes entryId + findings back
// per its contract. High-severity findings are a routing outcome (the
// procedure's noHighFindings transition), not an error.
func (s *Server) runNewEntry(ctx *engine.Context, ss *shellSession) error {
	entryKind, ok := storeString(ctx.Store, "entryKind")
	if !ok {
		return fmt.Errorf("newEntry: entryKind is not set")
	}
	entryType, err := typeForKind(entryKind)
	if err != nil {
		return err
	}
	layerVal, ok := storeString(ctx.Store, "layer")
	if !ok {
		return fmt.Errorf("newEntry: layer is not set")
	}
	body, ok := storeString(ctx.Store, "body")
	if !ok {
		return fmt.Errorf("newEntry: body is not set")
	}

	var refs []model.Ref
	if v, ok := ctx.Store.Get("refs"); ok {
		if items, isList := v.([]any); isList {
			for _, item := range items {
				if r, isRef := item.(engine.Ref); isRef {
					refs = append(refs, model.Ref{ID: r.ID, Kind: model.RefKind(r.Kind), Desc: r.Desc})
				}
			}
		}
	}

	attachments, err := s.materializeAttachments(ctx.Store, ss)
	if err != nil {
		return err
	}

	confidence, _ := storeString(ctx.Store, "confidence")
	intent, _ := storeString(ctx.Store, "intent")
	override := false
	if v, ok := ctx.Store.Get("preflightOverride"); ok {
		override, _ = v.(bool)
	}

	var (
		createdID  string
		pfFindings []query.Finding
	)
	var participants []string
	if p := s.resolveParticipant(ctx.Store); p != "" {
		participants = []string{p}
	}
	cmd := &command.NewEntryCmd{
		Type:             entryType,
		Layer:            model.Layer(layerVal),
		Kind:             model.Kind(entryKind),
		Intent:           intent,
		Description:      body,
		Refs:             refs,
		Closes:           storeStrings(ctx.Store, "closes"),
		Supersedes:       storeStrings(ctx.Store, "supersedes"),
		Participants:     participants,
		Confidence:       confidence,
		TopicLabels:      storeStrings(ctx.Store, "topics"),
		Attachments:      attachments,
		SkipPreflight:    override,
		PreflightTimeout: preflightTimeout,
		OnPreflight: func(result *query.PreflightResult) {
			pfFindings = result.Findings
		},
		OnNewEntry: func(id, _ string) {
			createdID = id
		},
	}
	if list := storeStrings(ctx.Store, "participants"); len(list) > 0 {
		cmd.Participants = list
	}

	handlerErr := s.handler.NewEntry(context.Background(), cmd)

	// The findings contract: newEntry always writes findings after a gate
	// run — their absence means the gate never ran. An override run skips
	// pre-flight, so it writes the empty set.
	if pfFindings == nil {
		pfFindings = []query.Finding{}
	}
	ctx.Store.WriteEngine("findings", pfFindings)

	if handlerErr != nil {
		if hasHighFinding(pfFindings) {
			return nil // routed by the procedure's noHighFindings transition
		}
		return fmt.Errorf("newEntry: %w", handlerErr)
	}
	ctx.Store.WriteEngine("entryId", createdID)
	return s.refreshGraph(ss)
}

// materializeAttachments resolves attachment handles from the session's
// staging scratch into NewEntryCmd attachments.
func (s *Server) materializeAttachments(store *engine.Store, ss *shellSession) ([]command.Attachment, error) {
	handles := storeStrings(store, "attachments")
	if len(handles) == 0 {
		return nil, nil
	}
	dir, err := s.sessions.stagingDir(ss.id)
	if err != nil {
		return nil, err
	}
	attachments := make([]command.Attachment, 0, len(handles))
	for _, h := range handles {
		if err := validAttachmentName(h); err != nil {
			return nil, fmt.Errorf("attachment handle %q: %w", h, err)
		}
		source := filepath.Join(dir, h)
		if _, err := os.Stat(source); err != nil {
			return nil, fmt.Errorf("attachment %q is not staged in this session — call stage_attachment first", h)
		}
		attachments = append(attachments, command.Attachment{Source: source, Target: h})
	}
	return attachments, nil
}

// refreshGraph reloads the graph into the session's engine so later guard
// evaluations and queries see the write.
func (s *Server) refreshGraph(ss *shellSession) error {
	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		return fmt.Errorf("reloading graph: %w", err)
	}
	ss.engine.Graph = graph
	return nil
}

// resolveParticipant picks the acting participant: the store's first
// participants value when a procedure collected one, the configured local
// participant otherwise.
func (s *Server) resolveParticipant(store *engine.Store) string {
	if list := storeStrings(store, "participants"); len(list) > 0 {
		return list[0]
	}
	if info, err := s.finder.Info(query.InfoQuery{}); err == nil {
		return info.LocalParticipant
	}
	return ""
}

// typeForKind derives the entry type from a kind name — the two kind sets
// are disjoint, so the kind determines the type.
func typeForKind(kind string) (model.EntryType, error) {
	k := model.Kind(kind)
	switch {
	case model.IsValidKindForType(model.TypeSignal, k):
		return model.TypeSignal, nil
	case model.IsValidKindForType(model.TypeDecision, k):
		return model.TypeDecision, nil
	default:
		return "", fmt.Errorf("unknown entry kind %q", kind)
	}
}

func storeString(store *engine.Store, name string) (string, bool) {
	v, ok := store.Get(name)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

func storeStrings(store *engine.Store, name string) []string {
	v, ok := store.Get(name)
	if !ok {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func hasHighFinding(findings []query.Finding) bool {
	for _, f := range findings {
		if f.Severity == query.SeverityHigh {
			return true
		}
	}
	return false
}

func intArg(args map[string]any, name string, fallback int) int {
	switch v := args[name].(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return fallback
	}
}

// validAttachmentName rejects handles that could escape the staging dir.
func validAttachmentName(name string) error {
	if name == "" {
		return fmt.Errorf("empty name")
	}
	if name != filepath.Base(name) || name == "." || name == ".." {
		return fmt.Errorf("must be a plain filename without path separators")
	}
	return nil
}
