package mcpapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/basefacts"
)

// defaultSearchHits caps search responses when the caller sets no limit —
// the CLI's `--limit 8` drill behavior, adopted as the MCP default
// (d-tac-dbk serve sizes).
const defaultSearchHits = 8

// --- the loop -------------------------------------------------------------

type StartSessionArgs struct {
	Shell string `json:"shell,omitempty" jsonschema:"shell procedure to open the session with, by canonical; defaults to user-dialogue"`
	Label string `json:"label,omitempty" jsonschema:"short single-line subject label for the session; set it early, update when the subject sharpens"`
}

type StartProcedureArgs struct {
	Session   string         `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — carry it across context compaction"`
	Canonical string         `json:"canonical" jsonschema:"the procedure to start, by its stable name (e.g. capture)"`
	Params    map[string]any `json:"params,omitempty" jsonschema:"typed start inputs per the procedure's declaration: declared params, plus any declared state field the caller wants to seed at start (e.g. a known anchor)"`
	Label     string         `json:"label,omitempty" jsonschema:"short single-line subject label for the session (the dialogue); set it early, update when the subject sharpens"`
	Parent    string         `json:"parent,omitempty" jsonschema:"instance handle of the spawning instance, when this is a sub-move (e.g. a capture dispatched from an engage); records lineage in the session log"`
}

type NextArgs struct {
	Session  string `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — carry it across context compaction"`
	Instance string `json:"instance" jsonschema:"instance handle from start_procedure"`
	// Report carries either state fields for the current step (per the served
	// report_schema) or a chooser answer {chooser, choice, userWords?, fields?}.
	Report map[string]any `json:"report" jsonschema:"state fields per the served report_schema, or a chooser answer object {chooser, choice, userWords?, fields?}"`
	Label  string         `json:"label,omitempty" jsonschema:"update the session's subject label when the dialogue's subject has sharpened"`
}

type AbandonArgs struct {
	Instance string `json:"instance,omitempty" jsonschema:"instance handle to abandon (a move); pass session too, naming the session the move belongs to"`
	Session  string `json:"session,omitempty" jsonschema:"with instance: the attached session the move belongs to (required). Alone: a session handle to tear down directly — no resume, no framing"`
	Reason   string `json:"reason,omitempty" jsonschema:"why the instance or session is being abandoned"`
}

type AbandonResult struct {
	Abandoned bool   `json:"abandoned"`
	Session   string `json:"session,omitempty" jsonschema:"the torn-down session, on teardown by handle"`
	Label     string `json:"label,omitempty" jsonschema:"the torn-down session's subject label"`
	// DiscardedThreads names what went down with a session teardown — nothing
	// vanishes silently (d-tac-dbk).
	DiscardedThreads []string   `json:"discarded_threads,omitempty" jsonschema:"open moves discarded with the session (procedure at step)"`
	HeldMarkers      []string   `json:"held_markers,omitempty" jsonschema:"WIP markers held; left standing — resume later or close via groom"`
	Instructions     string     `json:"instructions,omitempty"`
	Base             *BaseServe `json:"base_junction,omitempty" jsonschema:"the session shell's current serve — where the dialogue now stands"`
}

type ParkArgs struct {
	Session  string `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — carry it across context compaction"`
	Instance string `json:"instance" jsonschema:"running move instance to park"`
	Note     string `json:"note,omitempty" jsonschema:"one line on why the move is shelved — carried in the session log for whoever resumes it"`
}

type ParkResult struct {
	Parked       bool       `json:"parked"`
	Instance     string     `json:"instance"`
	Procedure    string     `json:"procedure"`
	Step         string     `json:"step"`
	Instructions string     `json:"instructions,omitempty"`
	Base         *BaseServe `json:"base_junction,omitempty" jsonschema:"the session shell's serve — where the dialogue lands"`
}

type ChooserOptionResult struct {
	Choice  string   `json:"choice"`
	Collect []string `json:"collect,omitempty" jsonschema:"state fields this option carries (suffix ? = optional)"`
}

type ChooserResult struct {
	Chooser string                `json:"chooser" jsonschema:"the pending chooser's step id — the value to put in a chooser answer's chooser field"`
	Kind    string                `json:"kind" jsonschema:"who answers: agent (advisory judgment) or user (relay their answer verbatim)"`
	Options []ChooserOptionResult `json:"options"`
}

// ServeResult is the loop's uniform response shape: where the instance
// stands, what advances it, and the material to work with.
type ServeResult struct {
	Session        string         `json:"session" jsonschema:"session handle; sessions survive restarts and resume via resume_session"`
	Instance       string         `json:"instance"`
	Procedure      string         `json:"procedure"`
	Status         string         `json:"status" jsonschema:"running, completed, or abandoned"`
	Step           string         `json:"step,omitempty"`
	Goal           string         `json:"goal" jsonschema:"one line: what advances the instance from here"`
	Instructions   string         `json:"instructions,omitempty"`
	Missing        []string       `json:"missing,omitempty" jsonschema:"required report fields not yet provided"`
	ReportSchema   map[string]any `json:"report_schema,omitempty" jsonschema:"JSON Schema for the current step's report"`
	PendingChooser *ChooserResult `json:"pending_chooser,omitempty"`
	Execution      string         `json:"execution,omitempty" jsonschema:"execution hint for this instance; fork-preferred means the procedure is a task best run in a disposable forked context"`
	Produced       map[string]any `json:"produced,omitempty" jsonschema:"engine-written results on completion (e.g. the created entry ID)"`
	Framing        string         `json:"framing,omitempty" jsonschema:"session framing (aspirations, directives, focus, participants); served when its content is new to this connection, omitted while unchanged"`
	Vocabulary     string         `json:"vocabulary,omitempty" jsonschema:"translation table for non-English graphs: canonical tokens stay English, user-facing narration renders in the configured language; served once per connection"`
	OpenThreads    string         `json:"open_threads,omitempty" jsonschema:"open work, carried on the session shell's serves only: this dialogue's other threads, then other parked dialogues"`
	Base           *BaseServe     `json:"base_junction,omitempty" jsonschema:"the session shell's current serve — where the dialogue lands now that this move has ended"`
}

// BaseServe is the session shell's serve as nested into landing responses
// (move completion, abandon). Same shape as ServeResult minus the nesting
// field — the shell never lands on itself, and the omission keeps the JSON
// schema acyclic.
type BaseServe struct {
	Session        string         `json:"session"`
	Instance       string         `json:"instance"`
	Procedure      string         `json:"procedure"`
	Status         string         `json:"status"`
	Step           string         `json:"step,omitempty"`
	Goal           string         `json:"goal"`
	Instructions   string         `json:"instructions,omitempty"`
	PendingChooser *ChooserResult `json:"pending_chooser,omitempty"`
	Framing        string         `json:"framing,omitempty"`
	OpenThreads    string         `json:"open_threads,omitempty" jsonschema:"open work: this dialogue's other threads, then other parked dialogues"`
}

// --- sessions & staging ----------------------------------------------------

type ListSessionsArgs struct{}

type ListSessionsResult struct {
	Sessions []sessionDescriptor `json:"sessions"`
}

type ResumeSessionArgs struct {
	Session string `json:"session,omitempty" jsonschema:"a session handle (from list_sessions) to attach this connection to — works on a fresh unbound connection, and switches away from a currently attached session (parking or concluding it per the leave rule). Omit to reorient the session this connection is already attached to; omit while unattached to list the sessions with open work"`
}

type ResumeSessionResult struct {
	Session      string        `json:"session"`
	Participant  string        `json:"participant,omitempty"`
	Label        string        `json:"label,omitempty" jsonschema:"the session's subject label, when one was recorded"`
	Open         []ServeResult `json:"open_instances" jsonschema:"current serve for every running instance; the session shell's serve carries the open-threads block"`
	Framing      string        `json:"framing,omitempty"`
	Instructions string        `json:"instructions,omitempty"`
}

type StageAttachmentArgs struct {
	Session string `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — carry it across context compaction"`
	Name    string `json:"name" jsonschema:"target filename (plain name, no paths)"`
	Content string `json:"content,omitempty" jsonschema:"file content to stage (UTF-8 text)"`
	Path    string `json:"path,omitempty" jsonschema:"local file path to stage instead of inline content"`
}

type StageAttachmentResult struct {
	Handle string `json:"handle" jsonschema:"attachment handle; pass it in a report's attachments field"`
}

// --- free reads -------------------------------------------------------------

type SearchArgs struct {
	Terms             []string `json:"terms,omitempty" jsonschema:"text mode: regex terms combined with AND"`
	Query             string   `json:"query,omitempty" jsonschema:"vector mode: free-form phrase (requires a configured embedding provider); both together run hybrid"`
	Type              string   `json:"type,omitempty" jsonschema:"filter: s/signal or d/decision"`
	Layer             string   `json:"layer,omitempty" jsonschema:"filter: stg, cpt, tac, ops, prc"`
	Kind              string   `json:"kind,omitempty" jsonschema:"filter: entry kind"`
	IncludeSuperseded bool     `json:"include_superseded,omitempty"`
	Limit             int      `json:"limit,omitempty" jsonschema:"hit cap; default 8"`
	MaxCitations      *int     `json:"max_citations,omitempty" jsonschema:"citation snippet lines per entry; default 0 = headers only — depth comes from show, not snippets"`
	Repos             []string `json:"repos,omitempty" jsonschema:"also search these connected repos by repo-id (additive to the local graph)"`
	AllRepos          bool     `json:"all_repos,omitempty" jsonschema:"also search every connected repo"`
}

type SearchResult struct {
	Results string `json:"results" jsonschema:"matching entries with citations"`
	Hint    string `json:"hint,omitempty" jsonschema:"one-line breadcrumb while no session runs"`
}

type ViewArgs struct {
	Layout   string   `json:"layout" jsonschema:"sdd view layout pipeline, e.g. 'active:as-counts' or 'top(15)'"`
	Repos    []string `json:"repos,omitempty" jsonschema:"also render the layout over these connected repos' graphs (additive to the local graph)"`
	AllRepos bool     `json:"all_repos,omitempty" jsonschema:"also render the layout over every connected repo"`
}

type ViewResult struct {
	Sections string `json:"sections"`
	Hint     string `json:"hint,omitempty" jsonschema:"one-line breadcrumb while no session runs"`
}

type ShowArgs struct {
	IDs  []string `json:"ids" jsonschema:"entry IDs to show; accepts an unambiguous short ID ({type}-{layer}-{suffix}) and resolves it to the full entry"`
	Up   int      `json:"up,omitempty" jsonschema:"upstream chain depth; default 2"`
	Down int      `json:"down,omitempty" jsonschema:"downstream chain depth; default 1"`
}

type ShowResult struct {
	Entries string `json:"entries"`
	Hint    string `json:"hint,omitempty" jsonschema:"one-line breadcrumb while no session runs"`
}

type ReadAttachmentArgs struct {
	ID       string `json:"id" jsonschema:"full ID of the entry whose attachment to read"`
	Name     string `json:"name,omitempty" jsonschema:"attachment filename; optional when the entry has exactly one"`
	Offset   int64  `json:"offset,omitempty" jsonschema:"byte position to continue from (next_offset of the previous page)"`
	MaxBytes int    `json:"max_bytes,omitempty" jsonschema:"page size cap; default 65536"`
}

type ReadAttachmentResult struct {
	Name       string   `json:"name"`
	Content    string   `json:"content"`
	Offset     int64    `json:"offset"`
	NextOffset int64    `json:"next_offset,omitempty"`
	TotalBytes int64    `json:"total_bytes"`
	More       bool     `json:"more"`
	Available  []string `json:"available" jsonschema:"the entry's attachment filenames"`
	Path       string   `json:"path,omitempty" jsonschema:"absolute filesystem path; present only for local (stdio) clients, which may read the file directly instead of paging"`
	Hint       string   `json:"hint,omitempty" jsonschema:"one-line breadcrumb while no session runs"`
}

type InfoArgs struct{}

type InfoResult struct {
	Participant string `json:"participant,omitempty" jsonschema:"configured local participant (canonical name)"`
	Language    string `json:"language,omitempty" jsonschema:"configured graph language; empty = English"`
	Search      string `json:"search" jsonschema:"available retrieval modes: text or vector,text"`
	Recovery    string `json:"recovery,omitempty" jsonschema:"host-neutral actionable recovery notices; empty when no write awaits explicit recovery"`
	Version     string `json:"version,omitempty"`
	Hint        string `json:"hint,omitempty" jsonschema:"one-line breadcrumb while no session runs"`
}

type RegistryArgs struct {
	Class string `json:"class,omitempty" jsonschema:"filter: predicate, query, or command"`
}

type RegistryFuncResult struct {
	Name   string   `json:"name"`
	Class  string   `json:"class"`
	Doc    string   `json:"doc"`
	Reads  []string `json:"reads,omitempty"`
	Writes []string `json:"writes,omitempty"`
}

type RegistryResult struct {
	Functions []RegistryFuncResult `json:"functions"`
	Hint      string               `json:"hint,omitempty" jsonschema:"one-line breadcrumb while no session runs"`
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "start_session",
		Description: "Open a fresh dialogue session — one of the two doors. Auto-starts the session shell " +
			"(user-dialogue by default) and returns its opening serve: your orientation, the available " +
			"moves, and the returned session handle. That handle is the dialogue's identity — retain it " +
			"across context compaction and pass it to every work tool. Call it again to re-serve the " +
			"orientation in full.",
	}, s.startSession)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "start_procedure",
		Description: "Start a procedure instance (a playbook move such as capture) in the session named by " +
			"session (required). Returns the current step's instructions, the report schema to answer " +
			"with, and the goal that advances it. This is the only path that leads to graph writes — " +
			"writes happen inside procedure transitions, never through a direct tool.",
	}, s.startProcedure)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "next",
		Description: "Advance a procedure instance in the session named by session (required): send state " +
			"fields per the served report_schema, or answer a pending chooser with {chooser, choice, " +
			"userWords?, fields?}. User choosers must carry the user's answer relayed verbatim in " +
			"userWords. When a move ends, the response carries the session shell's serve — where the " +
			"dialogue lands.",
	}, s.next)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "abandon",
		Description: "Abandon a running move instance (instance, plus session naming the session it belongs " +
			"to), or tear down a session directly by handle (session alone) — one call, no resume, no " +
			"framing; the response names the label and discarded threads. Nothing is cleaned up " +
			"implicitly: held WIP markers are surfaced and left standing for resume or grooming. The " +
			"session shell concludes through its own junction, never through abandon.",
	}, s.abandon)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "park",
		Description: "Park a running move back to the session junction (session required): state and step " +
			"position keep, the move lists as an open thread (at junctions and on conclude), and next " +
			"resumes it. Use it when the user shelves work mid-dialogue — a seeded draft parks as a " +
			"graph-visible thread instead of living in conversation memory as an agent promise.",
	}, s.park)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "list_sessions",
		Description: "List every session with open work — free discovery, no session needed. Each carries " +
			"participant, label, client name, last activity, and an active/idle tag; active sessions are " +
			"listed, never hidden. Attach to one with resume_session.",
	}, s.listSessions)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "resume_session",
		Description: "The attach door, and the compaction escape. Pass a session handle (from list_sessions) " +
			"to attach this connection to it — works on a fresh unbound connection, and switches away from " +
			"a currently attached session (parking it when moves are open, concluding it when quiescent). " +
			"Omit session to reorient the session you are already attached to: its framing plus every " +
			"running move at its current step, each with the report schema to continue it. Omit it while " +
			"unattached to list the sessions with open work to attach to.",
	}, s.resumeSession)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "stage_attachment",
		Description: "Stage a file into the session scratch (session required) and get back a handle to pass " +
			"in a report's attachments field. Never a graph write — the write gate materializes staged " +
			"files with the entry.",
	}, s.stageAttachment)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "search",
		Description: "Search graph entries: terms (text/regex), query (semantic phrase), or both (hybrid). " +
			"Free read, never gated.",
	}, s.search)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "view",
		Description: "Run an sdd view layout pipeline over the graph — overview sections, topic counts, " +
			"ranked lists. Free read, never gated.",
	}, s.view)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "show",
		Description: "Read entries in full with their upstream and downstream reference chains. Use " +
			"whenever the dialogue touches a specific entry — summaries are pointers, not facts.",
	}, s.show)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "read_attachment",
		Description: "Read an entry's attachment content, paged. Attachments are resolved by entry ID and " +
			"filename — never derive storage paths.",
	}, s.readAttachment)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "info",
		Description: "Session framing header: local participant, configured language, available search modes, and actionable recovery notices.",
	}, s.info)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "registry",
		Description: "Engine function contracts (predicates, queries, commands) — what procedure spec authors consult.",
	}, s.registryDocs)
}

// attachedSession resolves the session a work tool names, requiring that this
// connection is attached to it. The handle is the dialogue's identity — a
// missing one names both doors with the open-session list inlined; a handle
// this connection is not attached to funnels into resume_session, the single
// attach point (d-cpt-9of).
func (s *Server) attachedSession(ctx context.Context, req *mcp.CallToolRequest, session string) (*shellSession, error) {
	if strings.TrimSpace(session) == "" {
		return nil, s.noHandleError(ctx, req)
	}
	bound := s.sessions.bound(req.Session)
	if bound == nil || string(bound.root.ID()) != session {
		return nil, toolError("this connection is not attached to %s — attach with resume_session", session)
	}
	return bound, nil
}

// noHandleError is the typed rejection for a work tool called without a valid
// session handle. It names both doors — start_session for a fresh session,
// resume_session to attach to an existing one — and inlines the sessions with
// open work (handle + label), so the follow-up call can already be the attach
// (d-tac-dbk).
func (s *Server) noHandleError(ctx context.Context, req *mcp.CallToolRequest) error {
	msg := "no session handle — start_session opens a fresh session, resume_session attaches to an existing one (reads stay free)"
	items, err := s.app.ListWorkflowSessions(ctx, s.requestIdentity(req), s.project)
	if err != nil {
		return err
	}
	var lines []string
	for _, item := range items {
		if len(item.Open) == 0 {
			continue
		}
		line := string(item.Session)
		if item.Label != "" {
			line += " " + strconv.Quote(item.Label)
		}
		lines = append(lines, line)
	}
	if len(lines) > 0 {
		msg += "; sessions with open work (resume_session attaches): " + strings.Join(lines, ", ")
	}
	return toolError("%s", msg)
}

func mcpSessionID(session *mcp.ServerSession) string {
	if session == nil {
		return "stdio"
	}
	if id := session.ID(); id != "" {
		return id
	}
	return fmt.Sprintf("connection-%p", session)
}

func mcpClientName(session *mcp.ServerSession) string {
	if session != nil {
		if params := session.InitializeParams(); params != nil && params.ClientInfo != nil {
			return params.ClientInfo.Name
		}
	}
	return ""
}

func mcpClientVersion(session *mcp.ServerSession) string {
	if session != nil {
		if params := session.InitializeParams(); params != nil && params.ClientInfo != nil {
			return params.ClientInfo.Version
		}
	}
	return ""
}

// newShellSession creates a fresh SDD session (log, registry, engine),
// unbound — the door binds it after the shell procedure started.
func (s *Server) startSession(ctx context.Context, req *mcp.CallToolRequest, args StartSessionArgs) (*mcp.CallToolResult, ServeResult, error) {
	identity := s.requestIdentity(req)
	if current := s.sessions.bound(req.Session); current != nil {
		serve, err := current.root.Reopen(ctx, identity, args.Label)
		if err != nil {
			return nil, ServeResult{}, err
		}
		current.rootIdentity = identity
		s.forgetConnection(req.Session)
		result, err := s.toRootServeResult(ctx, req, current, serve)
		return nil, result, err
	}
	workflow, serve, err := s.app.OpenWorkflow(ctx, identity, s.project, sdd.WorkflowOpenRequest{
		MCPSessionID: mcpSessionID(req.Session), ClientName: mcpClientName(req.Session), ClientVersion: mcpClientVersion(req.Session),
		Shell: args.Shell, Label: args.Label,
	})
	if err != nil {
		return nil, ServeResult{}, err
	}
	ss := &shellSession{id: string(workflow.ID()), participant: "", root: workflow, rootIdentity: identity, lastActivity: time.Now()}
	prev := s.sessions.bind(req.Session, ss)
	if err := s.watchDisconnect(req.Session); err != nil {
		return nil, ServeResult{}, err
	}
	if err := s.leaveSession(ctx, prev); err != nil {
		return nil, ServeResult{}, err
	}
	result, err := s.toRootServeResult(ctx, req, ss, serve)
	return nil, result, err
}

func (s *Server) startProcedure(ctx context.Context, req *mcp.CallToolRequest, args StartProcedureArgs) (*mcp.CallToolResult, ServeResult, error) {
	if strings.TrimSpace(args.Canonical) == "" {
		return nil, ServeResult{}, toolError("canonical is required")
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, ServeResult{}, err
	}
	identity := s.requestIdentity(req)
	serve, err := ss.root.Start(ctx, identity, sdd.WorkflowStartRequest{Canonical: args.Canonical, Params: args.Params, Label: args.Label, Parent: args.Parent})
	if err != nil {
		return nil, ServeResult{}, err
	}
	ss.rootIdentity = identity
	result, err := s.toRootServeResult(ctx, req, ss, serve)
	return nil, result, err
}

func (s *Server) next(ctx context.Context, req *mcp.CallToolRequest, args NextArgs) (*mcp.CallToolResult, ServeResult, error) {
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, ServeResult{}, err
	}
	if args.Instance == "" {
		return nil, ServeResult{}, toolError("instance is required")
	}
	if len(args.Report) == 0 {
		return nil, ServeResult{}, toolError("report is required: state fields per the served report_schema, or a chooser answer {chooser, choice, userWords?, fields?}")
	}
	identity := s.requestIdentity(req)
	serve, err := ss.root.Advance(ctx, identity, sdd.WorkflowAdvanceRequest{Instance: args.Instance, Report: args.Report, Label: args.Label})
	if err != nil {
		return nil, ServeResult{}, err
	}
	ss.rootIdentity = identity
	result, err := s.toRootServeResult(ctx, req, ss, serve)
	return nil, result, err
}

func (s *Server) abandon(ctx context.Context, req *mcp.CallToolRequest, args AbandonArgs) (*mcp.CallToolResult, AbandonResult, error) {
	if args.Instance == "" {
		if args.Session == "" {
			return nil, AbandonResult{}, toolError("pass instance (a move to abandon, with its session) or session alone (tear down a session by handle)")
		}
		return s.abandonSession(ctx, req, args)
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, AbandonResult{}, err
	}
	identity := s.requestIdentity(req)
	result, err := ss.root.Abandon(ctx, identity, args.Instance, args.Reason)
	if err != nil {
		return nil, AbandonResult{}, err
	}
	ss.rootIdentity = identity
	out := AbandonResult{Abandoned: result.Abandoned, HeldMarkers: result.HeldMarkers}
	if len(out.HeldMarkers) > 0 {
		out.Instructions = "The instance holds WIP markers, left standing by design. Tell the user: resume the work later or close the markers through grooming."
	}
	if result.Base != nil {
		base, err := s.toRootServeResult(ctx, req, ss, result.Base)
		if err != nil {
			return nil, AbandonResult{}, err
		}
		out.Base = rootBaseServe(base)
	}
	return nil, out, nil
}

// park shelves a running move back to the shell junction: nothing about the
// instance changes — state, step, and evidence keep — but the shelving is
// logged (legible to a resuming agent) and the response lands the dialogue
// on the shell, with the move now listed among the open threads
// (d-tac-dbk). Seeded drafts ride the normal dispatch path; park adds no
// side channel.
func (s *Server) park(ctx context.Context, req *mcp.CallToolRequest, args ParkArgs) (*mcp.CallToolResult, ParkResult, error) {
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, ParkResult{}, err
	}
	if args.Instance == "" {
		return nil, ParkResult{}, toolError("instance is required")
	}
	identity := s.requestIdentity(req)
	result, err := ss.root.Park(ctx, identity, args.Instance, args.Note)
	if err != nil {
		return nil, ParkResult{}, err
	}
	ss.rootIdentity = identity
	out := ParkResult{
		Parked: true, Instance: result.Instance, Procedure: result.Procedure, Step: result.Step,
		Instructions: "The move is parked with its state kept — it stays listed as an open thread and resumes through next. Tell the user it is recorded as open work, not forgotten.",
	}
	if result.Base != nil {
		base, err := s.toRootServeResult(ctx, req, ss, result.Base)
		if err != nil {
			return nil, ParkResult{}, err
		}
		out.Base = rootBaseServe(base)
	}
	return nil, out, nil
}

// abandonSession tears down a parked session by handle: no resume, no
// framing, one call (d-tac-dbk; the measured baseline was six calls and
// ~28KB framing per session). The response names the label and every
// discarded thread — nothing vanishes silently. Needs no bound session:
// teardown is maintenance, not dialogue.
func (s *Server) abandonSession(ctx context.Context, req *mcp.CallToolRequest, args AbandonArgs) (*mcp.CallToolResult, AbandonResult, error) {
	identity := s.requestIdentity(req)
	current := s.sessions.bound(req.Session)
	if current != nil && current.root.ID() == sdd.SessionID(args.Session) {
		return nil, AbandonResult{}, toolError("session %s is the one this connection is in — the session shell concludes through its own junction (answer conclude)", args.Session)
	}
	result, err := s.app.AbandonWorkflowSession(ctx, identity, s.project, sdd.WorkflowResumeRequest{
		SessionID: sdd.SessionID(args.Session), MCPSessionID: mcpSessionID(req.Session), ClientName: mcpClientName(req.Session), ClientVersion: mcpClientVersion(req.Session),
	}, args.Reason)
	if err != nil {
		return nil, AbandonResult{}, err
	}
	out := AbandonResult{Abandoned: true, Session: string(result.Session), Label: result.Label, HeldMarkers: result.HeldMarkers}
	for _, instance := range result.Discarded {
		out.DiscardedThreads = append(out.DiscardedThreads, instance.Procedure+" at "+instance.Step)
	}
	if len(out.HeldMarkers) > 0 {
		out.Instructions = "The discarded threads hold WIP markers, left standing by design. Tell the user: resume the work later or close the markers through grooming."
	}
	if current != nil {
		serve, err := current.root.ServeShell(ctx, identity)
		if err != nil {
			return nil, AbandonResult{}, err
		}
		if serve != nil {
			mapped, err := s.toRootServeResult(ctx, req, current, serve)
			if err != nil {
				return nil, AbandonResult{}, err
			}
			out.Base = rootBaseServe(mapped)
		}
	}
	return nil, out, nil
}

// listSessions is free discovery — no attached session required. It lists
// every session with open work, active ones included (never hidden), each
// tagged active or idle so the caller can weigh attaching (d-cpt-9of).
func (s *Server) listSessions(ctx context.Context, req *mcp.CallToolRequest, _ ListSessionsArgs) (*mcp.CallToolResult, ListSessionsResult, error) {
	items, err := s.app.ListWorkflowSessions(ctx, s.requestIdentity(req), s.project)
	if err != nil {
		return nil, ListSessionsResult{}, err
	}
	var result ListSessionsResult
	for _, item := range items {
		if len(item.Open) == 0 {
			continue
		}
		desc := sessionDescriptor{
			Session: string(item.Session), Label: item.Label, Participant: item.Participant,
			Anchor: item.Anchor, Activity: activityTag(item.HolderLive),
		}
		if item.Holder != nil {
			desc.ClientName = item.Holder.ClientName
		}
		if !item.LastActivity.IsZero() {
			desc.LastActivity = item.LastActivity.Format(time.RFC3339)
		}
		for _, instance := range item.Open {
			desc.Open = append(desc.Open, instanceDescriptor{Instance: instance.Instance, Procedure: instance.Procedure, Step: instance.Step})
		}
		result.Sessions = append(result.Sessions, desc)
	}
	return nil, result, nil
}

// activityTag derives the listing label from whether a live client currently
// holds the session. Slice 3 swaps the source to attachment-stamp recency.
func activityTag(holderLive bool) string {
	if holderLive {
		return "active"
	}
	return "idle"
}

// resumeSession is the attach door and the compaction escape. Its session
// handle is the deliberate exception to the required-handle rule: omitting it
// reorients the currently attached session (or, unattached, lists what to
// attach to); a named handle attaches — working on a fresh unbound connection,
// and switching (leave rule on the previous) when already attached elsewhere
// (d-cpt-9of decisions 4, 6, 15).
func (s *Server) resumeSession(ctx context.Context, req *mcp.CallToolRequest, args ResumeSessionArgs) (*mcp.CallToolResult, ResumeSessionResult, error) {
	identity := s.requestIdentity(req)
	current := s.sessions.bound(req.Session)

	if args.Session != "" && (current == nil || args.Session != string(current.root.ID())) {
		workflow, result, err := s.app.ResumeWorkflow(ctx, identity, s.project, sdd.WorkflowResumeRequest{
			SessionID: sdd.SessionID(args.Session), MCPSessionID: mcpSessionID(req.Session), ClientName: mcpClientName(req.Session), ClientVersion: mcpClientVersion(req.Session),
		})
		if err != nil {
			return nil, ResumeSessionResult{}, err
		}
		ss := &shellSession{id: string(workflow.ID()), participant: result.Participant, root: workflow, rootIdentity: identity, lastActivity: time.Now()}
		prev := s.sessions.bind(req.Session, ss)
		if err := s.leaveSession(ctx, prev); err != nil {
			return nil, ResumeSessionResult{}, err
		}
		mapped, err := s.mapRootResume(ctx, req, ss, result)
		return nil, mapped, err
	}

	if current == nil {
		// Unattached with no handle: name the sessions to attach to — the
		// bootstrap for a connection with nothing to reorient back into.
		return nil, ResumeSessionResult{}, s.noHandleError(ctx, req)
	}
	s.forgetConnection(req.Session)
	result, err := current.root.ServeAll(ctx, identity)
	if err != nil {
		return nil, ResumeSessionResult{}, err
	}
	current.rootIdentity = identity
	mapped, err := s.mapRootResume(ctx, req, current, result)
	return nil, mapped, err
}

// ensureShellInstance re-derives the session's shell instance after replay
// or rebind, auto-starting the default shell when none is running — a
// resumed pre-shell session gains a base to land on (d-tac-bfc).

func (s *Server) stageAttachment(ctx context.Context, req *mcp.CallToolRequest, args StageAttachmentArgs) (*mcp.CallToolResult, StageAttachmentResult, error) {
	if err := validAttachmentName(args.Name); err != nil {
		return nil, StageAttachmentResult{}, toolError("name: %v", err)
	}
	if (args.Content == "") == (args.Path == "") {
		return nil, StageAttachmentResult{}, toolError("pass exactly one of content or path")
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, StageAttachmentResult{}, err
	}
	content := []byte(args.Content)
	if args.Path != "" {
		content, err = os.ReadFile(args.Path)
		if err != nil {
			return nil, StageAttachmentResult{}, toolError("reading %s: %v", args.Path, err)
		}
	}
	identity := s.requestIdentity(req)
	handle, err := ss.root.StageAttachment(ctx, identity, args.Name, content)
	if err != nil {
		return nil, StageAttachmentResult{}, err
	}
	ss.rootIdentity = identity
	return nil, StageAttachmentResult{Handle: handle}, nil
}

func validAttachmentName(name string) error {
	if name == "" {
		return fmt.Errorf("filename is required")
	}
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("must be a plain filename")
	}
	return nil
}

// readHint is the one-line breadcrumb every free read carries while no
// session is bound: an agent that enters through a pasted entry ID gets its
// data and the trail to the door. No path through the tool surface avoids a
// breadcrumb (d-cpt-h99).
func (s *Server) readHint(ms *mcp.ServerSession) string {
	if s.sessions.bound(ms) != nil {
		return ""
	}
	return "no dialogue session is open — start_session is the door; reads stay free"
}

// viewHintServedKey is the served-once sentinel for the first-view breadcrumb,
// deduped per connection through the same servedBefore memory as instruction
// blocks (cleared on disconnect and repeated start_session).
const viewHintServedKey = "view-layout-grammar-hint"

// viewHint is the single producer of the view tool's Hint field. It joins two
// breadcrumbs that are otherwise mutually exclusive by session-boundness: the
// door breadcrumb readHint serves while no session is bound, and a one-time
// pointer to the view-grammar fact served on the first view call of a
// connection. First-view is keyed to the connection (like readHint), so the
// unbound free-reader cohort the fact exists for (s-prc-3kh) gets it too. In
// the one overlapping cell — unbound and first view — both join, door first;
// every other cell carries a single breadcrumb or none. This strengthens the
// breadcrumb; it never gates the read (s-cpt-1dz, d-cpt-h99).
func (s *Server) viewHint(ms *mcp.ServerSession) string {
	door := s.readHint(ms)
	var fact string
	if !s.servedBefore(ms, viewHintServedKey) {
		fact = "view layout grammar: show " + basefacts.ViewGrammarFactID +
			" for the full filter/rank/macro vocabulary and the quoting rules"
	}
	switch {
	case door != "" && fact != "":
		return door + " · " + fact
	case fact != "":
		return fact
	default:
		return door
	}
}

func (s *Server) logRootRead(ctx context.Context, req *mcp.CallToolRequest, tool string, full, summary []string) error {
	ss := s.sessions.bound(req.Session)
	if ss == nil {
		return nil
	}
	identity := s.requestIdentity(req)
	if err := ss.root.LogRead(ctx, identity, tool, full, summary); err != nil {
		return err
	}
	ss.rootIdentity = identity
	return nil
}

func (s *Server) search(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error) {
	if len(args.Terms) == 0 && args.Query == "" {
		return nil, SearchResult{}, toolError("pass terms (text mode), query (vector mode), or both (hybrid)")
	}
	limit := args.Limit
	if limit == 0 {
		limit = defaultSearchHits
	}
	maxCitations := 0
	if args.MaxCitations != nil {
		maxCitations = *args.MaxCitations
	}
	result, err := s.app.Search(ctx, s.requestIdentity(req), s.project, sdd.SearchRequest{
		Terms: args.Terms, Phrase: args.Query, Type: args.Type, Layer: args.Layer, Kind: args.Kind,
		IncludeSuperseded: args.IncludeSuperseded, Limit: limit, MaxCitations: maxCitations,
		Repos: args.Repos, AllRepos: args.AllRepos,
	})
	if err != nil {
		return nil, SearchResult{}, toolError("searching: %v", err)
	}
	if err := s.logRootRead(ctx, req, "search", nil, result.EntryIDs); err != nil {
		return nil, SearchResult{}, err
	}
	out := result.Results
	if strings.TrimSpace(out) == "" {
		out = "(no entries matched — try another phrasing, or proceed if the topic is genuinely new)"
	}
	return nil, SearchResult{Results: out, Hint: s.readHint(req.Session)}, nil

}

func (s *Server) view(ctx context.Context, req *mcp.CallToolRequest, args ViewArgs) (*mcp.CallToolResult, ViewResult, error) {
	if strings.TrimSpace(args.Layout) == "" {
		return nil, ViewResult{}, toolError("layout is required")
	}
	result, err := s.app.View(ctx, s.requestIdentity(req), s.project, sdd.ViewRequest{Layout: args.Layout, Repos: args.Repos, AllRepos: args.AllRepos})
	if err != nil {
		return nil, ViewResult{}, toolError("viewing: %v", err)
	}
	sections := result.Sections
	// An empty result must never reach an agent as a blank string — it cannot
	// tell "matched nothing" from a broken call. Say so, and when a participant
	// filter came up empty (exact-match miss), name the participants the graph
	// knows. Any rendered content (e.g. recovery notices) is kept below.
	if result.MatchedCount == 0 {
		msg := emptyViewMessage(result.KnownParticipants)
		if strings.TrimSpace(sections) == "" {
			sections = msg
		} else {
			sections = msg + "\n\n" + sections
		}
	}
	sections = guardViewSize(sections)
	return nil, ViewResult{Sections: sections, Hint: s.viewHint(req.Session)}, nil

}

// emptyViewMessage is the explicit stand-in for an empty view result. It names
// the graph's known participants when a participant filter matched nothing so
// an exact-match miss reads as a wrong spelling, not absent data.
func emptyViewMessage(knownParticipants []string) string {
	msg := "0 entries matched the layout."
	if len(knownParticipants) > 0 {
		msg += " Known participants: " + strings.Join(knownParticipants, ", ") + "."
	}
	return msg
}

// maxViewResultBytes caps a view response over MCP. A pathological layout
// (e.g. active:as-list on a large graph) can otherwise blow the client's token
// budget in one call; the cap keeps the tool usable and points at paging.
const maxViewResultBytes = 40000

// guardViewSize truncates an over-cap view result on a line boundary and
// appends a notice naming n() paging as the recovery, so an agent that hit the
// cap knows how to narrow rather than seeing a silently cut result.
func guardViewSize(s string) string {
	if len(s) <= maxViewResultBytes {
		return s
	}
	truncated := s[:maxViewResultBytes]
	if i := strings.LastIndexByte(truncated, '\n'); i > 0 {
		truncated = truncated[:i]
	}
	return fmt.Sprintf("%s\n\n[view output truncated at %d of %d bytes — narrow with n(K), tighter filters, or fewer sections]", truncated, len(truncated), len(s))
}

func (s *Server) show(ctx context.Context, req *mcp.CallToolRequest, args ShowArgs) (*mcp.CallToolResult, ShowResult, error) {
	if len(args.IDs) == 0 {
		return nil, ShowResult{}, toolError("ids is required")
	}
	up := args.Up
	if up == 0 {
		up = sdd.DefaultShowUpDepth
	}
	down := args.Down
	if down == 0 {
		down = sdd.DefaultShowDownDepth
	}
	result, err := s.app.Show(ctx, s.requestIdentity(req), s.project, sdd.ShowRequest{IDs: args.IDs, UpDepth: up, DownDepth: down})
	if err != nil {
		return nil, ShowResult{}, err
	}
	if err := s.logRootRead(ctx, req, "show", result.FullIDs, result.SummaryIDs); err != nil {
		return nil, ShowResult{}, err
	}
	return nil, ShowResult{Entries: result.Entries, Hint: s.readHint(req.Session)}, nil

}

func (s *Server) readAttachment(ctx context.Context, req *mcp.CallToolRequest, args ReadAttachmentArgs) (*mcp.CallToolResult, ReadAttachmentResult, error) {
	if args.ID == "" {
		return nil, ReadAttachmentResult{}, toolError("id is required")
	}
	maxBytes := args.MaxBytes
	if maxBytes == 0 {
		maxBytes = 65536
	}
	result, err := s.app.ReadAttachment(ctx, s.requestIdentity(req), s.project, sdd.ReadAttachmentRequest{EntryID: args.ID, Filename: args.Name, Offset: args.Offset, MaxBytes: maxBytes})
	if err != nil {
		return nil, ReadAttachmentResult{}, err
	}
	if err := s.logRootRead(ctx, req, "read_attachment", nil, []string{args.ID}); err != nil {
		return nil, ReadAttachmentResult{}, err
	}
	page := result.Page
	output := ReadAttachmentResult{
		Name: page.Filename, Content: string(page.Content), Offset: page.Offset, NextOffset: page.NextOffset,
		TotalBytes: page.TotalSize, More: page.More, Available: result.Available, Hint: s.readHint(req.Session),
	}
	if s.local && s.localAttachmentPath != nil {
		output.Path, err = s.localAttachmentPath(args.ID, page.Filename)
		if err != nil {
			return nil, ReadAttachmentResult{}, err
		}
	}
	return nil, output, nil

}

func (s *Server) info(ctx context.Context, req *mcp.CallToolRequest, _ InfoArgs) (*mcp.CallToolResult, InfoResult, error) {
	info, err := s.app.Info(ctx, s.requestIdentity(req), s.project, sdd.InfoRequest{})
	if err != nil {
		return nil, InfoResult{}, err
	}
	return nil, InfoResult{Participant: info.Participant, Language: info.Language, Search: info.Search, Recovery: info.Recovery, Version: s.version, Hint: s.readHint(req.Session)}, nil

}

func (s *Server) registryDocs(ctx context.Context, req *mcp.CallToolRequest, args RegistryArgs) (*mcp.CallToolResult, RegistryResult, error) {
	docs, err := sdd.WorkflowRegistryDocs(args.Class)
	if err != nil {
		return nil, RegistryResult{}, err
	}
	result := RegistryResult{Hint: s.readHint(req.Session)}
	for _, doc := range docs {
		result.Functions = append(result.Functions, RegistryFuncResult{Name: doc.Name, Class: doc.Class, Doc: doc.Doc, Reads: doc.Reads, Writes: doc.Writes})
	}
	return nil, result, nil

}

// --- serve conversion --------------------------------------------------------

// toServeResult converts an engine serve into the tool response, applying
// served-once memory: full text the first time this connection sees these
// exact rendered bytes, a one-line stub after — identical bytes stub,
// changed content always serves in full (d-tac-dbk).

func (s *Server) toRootServeResult(ctx context.Context, req *mcp.CallToolRequest, ss *shellSession, serve *sdd.WorkflowServe) (ServeResult, error) {
	res := ServeResult{
		Session: string(serve.Session), Instance: serve.Instance, Procedure: serve.Procedure, Status: serve.Status,
		Step: serve.Step, Goal: serve.Goal, Instructions: serve.Instructions, Missing: serve.Missing,
		ReportSchema: serve.ReportSchema, Produced: serve.Produced, Execution: serve.Execution,
	}
	if serve.InstructionUnit != "" && s.servedBefore(req.Session, serve.InstructionUnit) {
		res.Instructions = serve.ReminderInstructions()
	}
	if serve.PendingChooser != nil {
		chooser := &ChooserResult{Chooser: serve.PendingChooser.Chooser, Kind: string(serve.PendingChooser.Kind)}
		for _, option := range serve.PendingChooser.Options {
			chooser.Options = append(chooser.Options, ChooserOptionResult{Choice: option.Choice, Collect: option.Collect})
		}
		res.PendingChooser = chooser
	}
	framing, err := ss.root.Framing(ctx, s.requestIdentity(req))
	if err != nil {
		return ServeResult{}, err
	}
	if !s.servedBefore(req.Session, framing) {
		res.Framing = framing
	}
	if ss.root.IsShell(serve.Instance) {
		res.OpenThreads, err = s.openThreadsRoot(ctx, req, ss)
		if err != nil {
			return ServeResult{}, err
		}
	}
	vocabulary, err := s.app.Vocabulary(ctx, s.requestIdentity(req), s.project)
	if err != nil {
		return ServeResult{}, err
	}
	if vocabulary != "" && !s.servedBefore(req.Session, vocabulary) {
		res.Vocabulary = vocabulary
	}
	if serve.Base != nil {
		base, err := s.toRootServeResult(ctx, req, ss, serve.Base)
		if err != nil {
			return ServeResult{}, err
		}
		res.Base = &BaseServe{
			Session: base.Session, Instance: base.Instance, Procedure: base.Procedure, Status: base.Status,
			Step: base.Step, Goal: base.Goal, Instructions: base.Instructions, PendingChooser: base.PendingChooser,
			Framing: base.Framing, OpenThreads: base.OpenThreads,
		}
	}
	return res, nil
}

func rootBaseServe(result ServeResult) *BaseServe {
	return &BaseServe{
		Session: result.Session, Instance: result.Instance, Procedure: result.Procedure, Status: result.Status,
		Step: result.Step, Goal: result.Goal, Instructions: result.Instructions, PendingChooser: result.PendingChooser,
		Framing: result.Framing, OpenThreads: result.OpenThreads,
	}
}

func (s *Server) mapRootResume(ctx context.Context, req *mcp.CallToolRequest, ss *shellSession, source sdd.WorkflowResumeResult) (ResumeSessionResult, error) {
	result := ResumeSessionResult{
		Session: string(source.Session), Participant: source.Participant, Label: source.Label,
		Instructions: resumeInstructions,
	}
	framing, err := ss.root.Framing(ctx, s.requestIdentity(req))
	if err != nil {
		return ResumeSessionResult{}, err
	}
	if !s.servedBefore(req.Session, framing) {
		result.Framing = framing
	}
	for i := range source.Open {
		mapped, err := s.toRootServeResult(ctx, req, ss, &source.Open[i])
		if err != nil {
			return ResumeSessionResult{}, err
		}
		mapped.Framing = ""
		result.Open = append(result.Open, mapped)
	}
	return result, nil
}

func (s *Server) openThreadsRoot(ctx context.Context, req *mcp.CallToolRequest, ss *shellSession) (string, error) {
	var lines []string
	for _, instance := range ss.root.OpenInstances() {
		lines = append(lines, fmt.Sprintf("- (this dialogue) %s: %s at %s", instance.Instance, instance.Procedure, instance.Step))
	}
	sessions, err := s.app.ListWorkflowSessions(ctx, s.requestIdentity(req), s.project)
	if err != nil {
		return "", err
	}
	for _, item := range sessions {
		if item.Session == ss.root.ID() || len(item.Open) == 0 {
			continue
		}
		var line strings.Builder
		fmt.Fprintf(&line, "- %s", item.Session)
		if item.Label != "" {
			fmt.Fprintf(&line, " %q", item.Label)
		}
		if item.Participant != "" {
			fmt.Fprintf(&line, " (%s)", item.Participant)
		}
		var open []string
		for _, instance := range item.Open {
			open = append(open, instance.Procedure+" at "+instance.Step)
		}
		fmt.Fprintf(&line, " — open: %s", strings.Join(open, ", "))
		if !item.LastActivity.IsZero() {
			fmt.Fprintf(&line, ", last active %s", item.LastActivity.Format(time.RFC3339))
		}
		lines = append(lines, line.String())
	}
	if len(lines) == 0 {
		return "", nil
	}
	header := openThreadsReminder
	if !s.servedBefore(req.Session, openThreadsIntro) {
		header = openThreadsIntro
	}
	return header + "\n" + strings.Join(lines, "\n"), nil
}
