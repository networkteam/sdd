package mcpapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/serveview"
	"github.com/networkteam/sdd/internal/textpatch"
	"github.com/networkteam/sdd/internal/truncate"
	sdd "github.com/networkteam/sdd/pkg/application"
)

// defaultSearchHits caps search responses when the caller sets no limit —
// the CLI's `--limit 8` drill behavior, adopted as the MCP default
// (d-tac-dbk serve sizes).
const defaultSearchHits = 8

// --- the loop -------------------------------------------------------------

type StartSessionArgs struct {
	Project string `json:"project,omitempty" jsonschema:"project to open the session in, by ID. Optional: a composition serving one project infers it; when several are accessible and none is passed, the response lists them under projects instead of starting a session — pick one and call again"`
	Shell   string `json:"shell,omitempty" jsonschema:"shell procedure to open the session with, by canonical; defaults to user-dialogue"`
	Label   string `json:"label,omitempty" jsonschema:"short single-line subject label for the session; set it early, update when the subject sharpens"`
}

// ProjectResult is one accessible project, served by start_session when the
// caller must choose before a session can open.
type ProjectResult struct {
	ID          string `json:"id" jsonschema:"project ID; pass as start_session's project"`
	DisplayName string `json:"display_name,omitempty"`
	CanWrite    bool   `json:"can_write"`
}

// StatusProjectRequired is the ServeResult status start_session returns when
// the composition has several projects and none was passed: no session
// opened, the accessible projects are listed instead.
const StatusProjectRequired = "project-required"

type StartProcedureArgs struct {
	Session   string         `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — carry it across context compaction"`
	Canonical string         `json:"canonical" jsonschema:"the procedure to start, by its stable name (e.g. capture)"`
	Params    map[string]any `json:"params,omitempty" jsonschema:"typed start inputs per the procedure's declaration: declared params, plus any declared state field the caller wants to seed at start (e.g. a known anchor)"`
	Label     string         `json:"label,omitempty" jsonschema:"short single-line subject label for the session (the dialogue); set it early, update when the subject sharpens"`
	Parent    string         `json:"parent,omitempty" jsonschema:"instance handle of the spawning instance, when this is a sub-move (e.g. a capture dispatched from an engage); records lineage in the session log"`
	Project   string         `json:"project,omitempty" jsonschema:"project the move works in, when not the session's home project: it must lie in the home project's declared dependency closure and be one the principal can read. Omitted, a foreign anchor selects its project, else the parent's, else the home project"`
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
	Session        string            `json:"session,omitempty" jsonschema:"session handle (the session ID); sessions survive restarts and resume via resume_session. Pass it to every other tool"`
	Project        string            `json:"project,omitempty" jsonschema:"the project this instance works in — the session's home project unless the move was started in a dependency"`
	Projects       []ProjectResult   `json:"projects,omitempty" jsonschema:"start_session only: the principal's accessible projects, served with status project-required when several exist and none was passed — no session opened yet"`
	Branch         string            `json:"branch,omitempty" jsonschema:"the session's declared branch binding"`
	Instance       string            `json:"instance,omitempty"`
	Procedure      string            `json:"procedure,omitempty"`
	Status         string            `json:"status" jsonschema:"running, completed, or abandoned — or project-required from start_session, when a project must be chosen first"`
	Step           string            `json:"step,omitempty"`
	Goal           string            `json:"goal" jsonschema:"one line: what advances the instance from here"`
	Instructions   string            `json:"instructions,omitempty"`
	Missing        []string          `json:"missing,omitempty" jsonschema:"required report fields not yet provided"`
	ReportSchema   map[string]any    `json:"report_schema,omitempty" jsonschema:"JSON Schema for the current step's report"`
	PendingChooser *ChooserResult    `json:"pending_chooser,omitempty"`
	Execution      string            `json:"execution,omitempty" jsonschema:"execution hint for this instance; fork-preferred means the procedure is a task best run in a disposable forked context"`
	Produced       map[string]string `json:"produced,omitempty" jsonschema:"engine-written results on completion (e.g. the created entry ID)"`
	Framing        string            `json:"framing,omitempty" jsonschema:"session framing (aspirations, directives, focus, participants); served when its content is new to this connection, omitted while unchanged"`
	Vocabulary     string            `json:"vocabulary,omitempty" jsonschema:"translation table for non-English graphs: canonical tokens stay English, user-facing narration renders in the configured language; served once per connection"`
	OpenThreads    string            `json:"open_threads,omitempty" jsonschema:"this dialogue's own open threads, carried on the session shell's serves. Other dialogues are never listed here"`
	Base           *BaseServe        `json:"base_junction,omitempty" jsonschema:"the session shell's current serve — where the dialogue lands now that this move has ended"`
	Collected      map[string]string `json:"collected,omitempty" jsonschema:"state already collected by this instance — values persist across handover; do not re-derive them"`
}

// BaseServe is the session shell's serve as nested into landing responses
// (move completion, abandon). Same shape as ServeResult minus the nesting
// field — the shell never lands on itself, and the omission keeps the JSON
// schema acyclic.
type BaseServe struct {
	Session        string         `json:"session"`
	Branch         string         `json:"branch,omitempty"`
	Instance       string         `json:"instance"`
	Procedure      string         `json:"procedure"`
	Status         string         `json:"status"`
	Step           string         `json:"step,omitempty"`
	Goal           string         `json:"goal"`
	Instructions   string         `json:"instructions,omitempty"`
	PendingChooser *ChooserResult `json:"pending_chooser,omitempty"`
	Framing        string         `json:"framing,omitempty"`
	OpenThreads    string         `json:"open_threads,omitempty" jsonschema:"this dialogue's own open threads; other dialogues are never listed here"`
}

// --- sessions & staging ----------------------------------------------------

type ResumeSessionArgs struct {
	Session string `json:"session" jsonschema:"the session handle (the session ID) whose current position to re-serve: its framing plus every running move at its current step with the schema to continue it. Presenting the handle is the whole authorization; it also declares that you need re-serving, so the served-once memory resets and the complete position serves in full"`
}

type ResumeSessionResult struct {
	Session      string        `json:"session" jsonschema:"session handle (the session ID)"`
	Project      string        `json:"project,omitempty" jsonschema:"the project this session is bound to"`
	Participant  string        `json:"participant,omitempty"`
	Label        string        `json:"label,omitempty" jsonschema:"the session's subject label, when one was recorded"`
	Branch       string        `json:"branch,omitempty" jsonschema:"the session's declared branch binding"`
	Open         []ServeResult `json:"open_instances" jsonschema:"current serve for every running instance; the session shell's serve carries the open-threads block"`
	Framing      string        `json:"framing,omitempty"`
	Instructions string        `json:"instructions,omitempty"`
}

type BindBranchArgs struct {
	Session string `json:"session" jsonschema:"session handle this connection is attached to"`
	Branch  string `json:"branch,omitempty" jsonschema:"branch to bind this session to"`
	Clear   bool   `json:"clear,omitempty" jsonschema:"clear the current branch binding"`
}

type BindBranchResult struct {
	Branch string `json:"branch,omitempty" jsonschema:"the now-effective branch binding; empty after clear"`
	Status string `json:"status"`
}

type StageAttachmentArgs struct {
	Session string             `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — carry it across context compaction"`
	Name    string             `json:"name" jsonschema:"target filename (plain name, no paths)"`
	Content string             `json:"content,omitempty" jsonschema:"file content to stage (UTF-8 text)"`
	Path    string             `json:"path,omitempty" jsonschema:"local file path to stage instead of inline content"`
	Patches []SearchReplaceArg `json:"patches,omitempty" jsonschema:"edit the already-staged file named by name in place: ordered exact search-replace pairs applied atomically — each old must match exactly once, a failing pair refuses the whole edit. Pass instead of content/path"`
}

// SearchReplaceArg is one exact edit inside a patches list.
type SearchReplaceArg struct {
	Old string `json:"old" jsonschema:"exact text to replace — must match exactly once in the file as it stands when this pair applies"`
	New string `json:"new" jsonschema:"replacement text; empty deletes the old text"`
}

type StageAttachmentResult struct {
	Handle string `json:"handle" jsonschema:"attachment handle; pass it in a report's attachments field"`
}

// --- free reads -------------------------------------------------------------

type SearchArgs struct {
	IncludesRevision  string   `json:"includes_revision,omitempty" jsonschema:"require a selected revision containing this successful write"`
	Session           string   `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — the read runs in that session's project and branch"`
	Project           string   `json:"project,omitempty" jsonschema:"project to read in; defaults to the session's home project. Another project must lie in the home project's declared dependency closure and be one the principal can read"`
	Terms             []string `json:"terms,omitempty" jsonschema:"text mode: regex terms combined with AND"`
	Query             string   `json:"query,omitempty" jsonschema:"vector mode: free-form phrase (requires a configured embedding provider); both together run hybrid"`
	Type              string   `json:"type,omitempty" jsonschema:"filter: s/signal or d/decision"`
	Layer             string   `json:"layer,omitempty" jsonschema:"filter: stg, cpt, tac, ops, prc"`
	Kind              string   `json:"kind,omitempty" jsonschema:"filter: entry kind"`
	IncludeSuperseded bool     `json:"include_superseded,omitempty"`
	Limit             int      `json:"limit,omitempty" jsonschema:"hit cap; default 8"`
	MaxCitations      *int     `json:"max_citations,omitempty" jsonschema:"citation snippet lines per entry; omitted = 1 (the strongest matching chunk, the match evidence), 0 = headers only"`
	Repos             []string `json:"repos,omitempty" jsonschema:"also search these connected repos by repo-id (additive to the local graph)"`
	AllRepos          bool     `json:"all_repos,omitempty" jsonschema:"also search every connected repo"`
}

type SearchResult struct {
	Coverage []sdd.SearchCoverage `json:"coverage,omitempty" jsonschema:"published entry coverage for each fixed search snapshot"`
	Notice   string               `json:"notice,omitempty" jsonschema:"readable incomplete-indexing notice"`
	Results  string               `json:"results" jsonschema:"matching entries with citations"`
}

type ViewArgs struct {
	Session  string   `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — the read runs in that session's project and branch"`
	Project  string   `json:"project,omitempty" jsonschema:"project to read in; defaults to the session's home project. Another project must lie in the home project's declared dependency closure and be one the principal can read"`
	Layout   string   `json:"layout" jsonschema:"sdd view layout pipeline, e.g. 'active:as-counts' or 'top(15)'"`
	Repos    []string `json:"repos,omitempty" jsonschema:"also render the layout over these connected repos' graphs (additive to the local graph)"`
	AllRepos bool     `json:"all_repos,omitempty" jsonschema:"also render the layout over every connected repo"`
}

type ViewResult struct {
	Sections string `json:"sections"`
	Hint     string `json:"hint,omitempty" jsonschema:"one-line pointer to the view layout grammar, served once per connection"`
}

type ShowArgs struct {
	Session string   `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — the read runs in that session's project and branch"`
	Project string   `json:"project,omitempty" jsonschema:"project to read in; defaults to the session's home project. Another project must lie in the home project's declared dependency closure and be one the principal can read"`
	IDs     []string `json:"ids" jsonschema:"entry IDs to show; accepts an unambiguous short ID ({type}-{layer}-{suffix}) and resolves it to the full entry"`
	Up      *int     `json:"up,omitempty" jsonschema:"upstream chain depth; omitted = default 2, 0 = no upstream"`
	Down    *int     `json:"down,omitempty" jsonschema:"downstream chain depth; omitted = default 1, 0 = no downstream"`
}

type ShowResult struct {
	Entries string `json:"entries"`
}

type ReadAttachmentArgs struct {
	Session  string `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — the read runs in that session's project, and a staged file is read from that session"`
	Project  string `json:"project,omitempty" jsonschema:"project to read the entry's attachment in; defaults to the session's home project. Another project must lie in the home project's declared dependency closure and be one the principal can read"`
	ID       string `json:"id,omitempty" jsonschema:"full ID of the entry whose attachment to read; omit when reading a staged file by handle"`
	Name     string `json:"name,omitempty" jsonschema:"attachment filename; optional when the entry has exactly one"`
	Offset   int64  `json:"offset,omitempty" jsonschema:"byte position to continue from (next_offset of the previous page)"`
	MaxBytes int    `json:"max_bytes,omitempty" jsonschema:"page size cap; default 65536"`
	Handle   string `json:"handle,omitempty" jsonschema:"read a file staged in this session (before any entry carries it) by its handle, in the same bounded pages; pass instead of id"`
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
}

type InfoArgs struct {
	Session string `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required — the header describes that session's project"`
	Project string `json:"project,omitempty" jsonschema:"project to describe; defaults to the session's home project. Another project must lie in the home project's declared dependency closure and be one the principal can read"`
}

type InfoResult struct {
	Project     string `json:"project,omitempty" jsonschema:"the session's project ID"`
	Participant string `json:"participant,omitempty" jsonschema:"configured local participant (canonical name)"`
	Language    string `json:"language,omitempty" jsonschema:"configured graph language; empty = English"`
	Search      string `json:"search" jsonschema:"available retrieval modes: text or vector,text"`
	Recovery    string `json:"recovery,omitempty" jsonschema:"host-neutral actionable recovery notices; empty when no write awaits explicit recovery"`
	Version     string `json:"version,omitempty"`
}

type RegistryArgs struct {
	Session string `json:"session,omitempty" jsonschema:"session handle this connection is attached to (from start_session or resume_session); required"`
	Class   string `json:"class,omitempty" jsonschema:"filter: predicate, query, or command"`
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
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "start_session",
		Description: "Open a fresh dialogue session — one of the two doors, and the only tool that takes a " +
			"project. Auto-starts the session shell (user-dialogue by default) and returns its opening " +
			"serve: your orientation, the available moves, and the returned session handle. That handle " +
			"is the dialogue's identity and binds the session to one project — retain it across context " +
			"compaction and pass it to every other tool, reads included. Pass project when the composition " +
			"serves several; omitted, a sole accessible project is inferred, and with several the response " +
			"lists them (status project-required) instead of opening a session. Every call opens a new " +
			"dialogue under a new handle; to re-serve an existing one, present its handle to resume_session.",
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
		Name: "resume_session",
		Description: "The second door, and the compaction escape: re-serve the current position of the " +
			"session whose handle you present — its framing plus every running move at its current step, " +
			"each with the report schema to continue it. Presenting the handle is the whole authorization " +
			"(a handle reaches you only by issuance or by the user handing it over), and it declares that " +
			"you need re-serving: the served-once memory resets, so the complete position serves in full. " +
			"Only recorded session state resumes — step position, collected fields, staged files — never " +
			"another conversation's context. A lost handle is the user's to recover, not yours to guess.",
	}, s.resumeSession)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "bind_branch",
		Description: "Declare or clear the attached session's durable branch binding. Pass exactly one of " +
			"branch or clear:true. Setting validates the live registered checkout before changing the " +
			"session; clearing needs no branch capability.",
	}, s.bindBranch)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "stage_attachment",
		Description: "Stage a file into the session scratch (session required) and get back a handle to pass " +
			"in a report's attachments field, or amend what is already staged: with patches, the file named " +
			"by name is edited in place through atomic search-replace pairs instead of re-staging it whole. " +
			"Never a graph write — the write gate materializes staged files with the entry.",
	}, s.stageAttachment)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "search",
		Description: "Search graph entries: terms (text/regex), query (semantic phrase), or both (hybrid). " +
			"A free read within the session named by session (required): no move needed, never blocked " +
			"by procedure state; it runs in that session's project and branch.",
	}, s.search)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "view",
		Description: "Run an sdd view layout pipeline over the graph — overview sections, topic counts, " +
			"ranked lists. A free read within the session named by session (required).",
	}, s.view)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "show",
		Description: "Read entries in full with their upstream and downstream reference chains, within the " +
			"session named by session (required). Use whenever the dialogue touches a specific entry — " +
			"summaries are pointers, not facts.",
	}, s.show)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "read_attachment",
		Description: "Read an attachment's content, paged, within the session named by session (required): " +
			"an entry's by ID and filename, or a file staged in the session (before any entry carries it) " +
			"by handle. Never derive storage paths.",
	}, s.readAttachment)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "info",
		Description: "Session framing header for the session named by session (required): project, local participant, configured language, available search modes, and actionable recovery notices.",
	}, s.info)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "registry",
		Description: "Engine function contracts (predicates, queries, commands) — what procedure spec authors consult. Carries the session handle like every other tool.",
	}, s.registryDocs)
}

// attachedSession resolves the session a tool names by its handle: cached
// when this server already holds it replayed, loaded from the store otherwise
// — a same-ID load that raced another keeps the winner. The handle is the
// dialogue's identity and its capability (d-cpt-aen); a missing one names the
// doors. The session comes back locked; the caller unlocks it.
func (s *Server) attachedSession(ctx context.Context, req *mcp.CallToolRequest, session string) (*shellSession, error) {
	id := sdd.SessionID(strings.TrimSpace(session))
	if id == "" {
		return nil, noHandleError()
	}
	ss := s.sessions.get(id)
	if ss == nil {
		workflow, err := s.app.LoadWorkflow(ctx, s.requestIdentity(req), sdd.WorkflowResumeRequest{
			SessionID: id, ClientName: mcpClientName(req.Session), ClientVersion: mcpClientVersion(req.Session),
		})
		if err != nil {
			return nil, err
		}
		ss = s.sessions.put(&shellSession{root: workflow})
	}
	ss.mu.Lock()
	if ss.root.Finished() {
		ss.mu.Unlock()
		s.sessions.evict(id)
		return nil, toolError("session %s has ended — %s", session, sdd.NewSessionNote)
	}
	return ss, nil
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

// noHandleError is the typed rejection for a tool called without a session
// handle. It names both doors and nothing else: handles are issued, never
// published, so no listing rides a rejection (d-cpt-aen).
func noHandleError() error {
	return toolError("no session handle — every tool carries one: start_session opens a fresh session and returns its handle, resume_session re-serves the session whose handle you present. A handle you no longer hold is the user's to recover.")
}

// startSession opens a fresh dialogue: a new durable session under a new
// handle, every call. Nothing is derived from the connection.
func (s *Server) startSession(ctx context.Context, req *mcp.CallToolRequest, args StartSessionArgs) (*mcp.CallToolResult, ServeResult, error) {
	identity := s.requestIdentity(req)
	// The door is the one tool that takes a project. Left empty, the
	// application infers a sole accessible project or reports the ambiguity,
	// which becomes the project listing (d-tac-1z6).
	project := sdd.ProjectID(strings.TrimSpace(args.Project))
	workflow, serve, err := s.app.OpenWorkflow(ctx, identity, project, sdd.WorkflowOpenRequest{
		ClientName: mcpClientName(req.Session), ClientVersion: mcpClientVersion(req.Session),
		Shell: args.Shell, Label: args.Label,
	})
	if err != nil {
		var appErr *sdd.ApplicationError
		if project == "" && errors.As(err, &appErr) && appErr.Code == sdd.ErrorProjectRequired {
			result, err := s.serveProjects(ctx, identity)
			return nil, result, err
		}
		return nil, ServeResult{}, err
	}
	ss := s.sessions.put(&shellSession{root: workflow})
	ss.mu.Lock()
	defer ss.mu.Unlock()
	result, err := s.toRootServeResult(ctx, req, ss, serve)
	return nil, result, err
}

// serveProjects is start_session's answer when the principal reaches several
// projects and named none: no session opens, the projects are listed and the
// caller picks one.
func (s *Server) serveProjects(ctx context.Context, identity sdd.RequestIdentity) (ServeResult, error) {
	list, err := s.app.Projects(ctx, identity)
	if err != nil {
		return ServeResult{}, err
	}
	result := ServeResult{
		Status: StatusProjectRequired,
		Goal:   "choose a project, then call start_session again with project set",
		Instructions: "This composition serves several projects and none was named, so no session opened. " +
			"Ask the user which project the dialogue is about when it is not evident, then call " +
			"start_session with project set to its ID.",
	}
	for _, candidate := range list.Projects {
		if candidate.State != sdd.ProjectReady || !candidate.CanRead {
			continue
		}
		result.Projects = append(result.Projects, ProjectResult{ID: string(candidate.ID), DisplayName: candidate.DisplayName, CanWrite: candidate.CanWrite})
	}
	return result, nil
}

func (s *Server) startProcedure(ctx context.Context, req *mcp.CallToolRequest, args StartProcedureArgs) (*mcp.CallToolResult, ServeResult, error) {
	if strings.TrimSpace(args.Canonical) == "" {
		return nil, ServeResult{}, toolError("canonical is required")
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, ServeResult{}, err
	}
	defer ss.mu.Unlock()
	identity := s.requestIdentity(req)
	serve, err := ss.root.Start(ctx, identity, sdd.WorkflowStartRequest{
		Canonical: args.Canonical, Params: args.Params, Label: args.Label, Parent: args.Parent,
		Project: sdd.ProjectID(strings.TrimSpace(args.Project)),
	})
	if err != nil {
		return nil, ServeResult{}, err
	}
	result, err := s.toRootServeResult(ctx, req, ss, serve)
	return nil, result, err
}

func (s *Server) next(ctx context.Context, req *mcp.CallToolRequest, args NextArgs) (*mcp.CallToolResult, ServeResult, error) {
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, ServeResult{}, err
	}
	defer ss.mu.Unlock()
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
	if ss.root.Finished() {
		// The conclude serve is the dialogue's last; a finished session is
		// never re-served, so its replay leaves the cache with it.
		defer s.sessions.evict(ss.root.ID())
	}
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
	defer ss.mu.Unlock()
	identity := s.requestIdentity(req)
	result, err := ss.root.Abandon(ctx, identity, args.Instance, args.Reason)
	if err != nil {
		return nil, AbandonResult{}, err
	}
	out := AbandonResult{Abandoned: result.Abandoned, HeldMarkers: result.HeldMarkers}
	if len(out.HeldMarkers) > 0 {
		out.Instructions = "The instance holds WIP markers, left standing by design. Tell the user: resume the work later or close the markers through grooming."
	}
	if result.Base != nil {
		base, err := s.toBaseServeResult(ctx, req, ss, result.Base)
		if err != nil {
			return nil, AbandonResult{}, err
		}
		out.Base = base
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
	defer ss.mu.Unlock()
	if args.Instance == "" {
		return nil, ParkResult{}, toolError("instance is required")
	}
	identity := s.requestIdentity(req)
	result, err := ss.root.Park(ctx, identity, args.Instance, args.Note)
	if err != nil {
		return nil, ParkResult{}, err
	}
	out := ParkResult{
		Parked: true, Instance: result.Instance, Procedure: result.Procedure, Step: result.Step,
		Instructions: "The move is parked with its state kept — it stays listed as an open thread and resumes through next. Tell the user it is recorded as open work, not forgotten.",
	}
	if result.Base != nil {
		base, err := s.toBaseServeResult(ctx, req, ss, result.Base)
		if err != nil {
			return nil, ParkResult{}, err
		}
		out.Base = base
	}
	return nil, out, nil
}

// abandonSession tears down a session by handle: no resume, no framing, one
// call (d-tac-dbk). The response names the label and every discarded thread —
// nothing vanishes silently. Holding the handle is the authorization
// (d-cpt-aen); a session this server holds replayed leaves the cache with it.
func (s *Server) abandonSession(ctx context.Context, req *mcp.CallToolRequest, args AbandonArgs) (*mcp.CallToolResult, AbandonResult, error) {
	identity := s.requestIdentity(req)
	id := sdd.SessionID(strings.TrimSpace(args.Session))
	if id == "" {
		return nil, AbandonResult{}, noHandleError()
	}
	if cached := s.sessions.get(id); cached != nil {
		cached.mu.Lock()
		defer cached.mu.Unlock()
		s.sessions.evict(id)
	}
	result, err := s.app.AbandonWorkflowSession(ctx, identity, sdd.WorkflowResumeRequest{
		SessionID: id, ClientName: mcpClientName(req.Session), ClientVersion: mcpClientVersion(req.Session),
	}, args.Reason)
	if err != nil {
		return nil, AbandonResult{}, err
	}
	out := AbandonResult{Abandoned: true, Session: string(result.Session), Label: result.Label, HeldMarkers: boundList(result.HeldMarkers, "held markers")}
	for _, instance := range result.Discarded {
		out.DiscardedThreads = append(out.DiscardedThreads, instance.Procedure+" at "+instance.Step)
	}
	out.DiscardedThreads = boundList(out.DiscardedThreads, "discarded threads")
	if len(out.HeldMarkers) > 0 {
		out.Instructions = "The discarded threads hold WIP markers, left standing by design. Tell the user: resume the work later or close the markers through grooming."
	}
	return nil, out, nil
}

// resumeSession re-serves the current position of the session whose handle the
// caller presents — the reorientation. Presenting the handle declares the
// caller needs re-serving, so the ledger-derived served-once memory resets
// there and the complete position serves in full (d-cpt-aen).
func (s *Server) resumeSession(ctx context.Context, req *mcp.CallToolRequest, args ResumeSessionArgs) (*mcp.CallToolResult, ResumeSessionResult, error) {
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, ResumeSessionResult{}, err
	}
	defer ss.mu.Unlock()
	identity := s.requestIdentity(req)
	if err := ss.root.Reorient(ctx, identity); err != nil {
		return nil, ResumeSessionResult{}, err
	}
	result, err := ss.root.ServeAll(ctx, identity)
	if err != nil {
		return nil, ResumeSessionResult{}, err
	}
	mapped, err := s.mapRootResume(ctx, req, ss, result)
	return nil, mapped, err
}

func (s *Server) bindBranch(ctx context.Context, req *mcp.CallToolRequest, args BindBranchArgs) (*mcp.CallToolResult, BindBranchResult, error) {
	if args.Branch != strings.TrimSpace(args.Branch) {
		return nil, BindBranchResult{}, toolError("branch must not have leading or trailing whitespace")
	}
	if (args.Branch != "") == args.Clear {
		return nil, BindBranchResult{}, toolError("pass exactly one of a nonblank branch or clear=true")
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, BindBranchResult{}, err
	}
	defer ss.mu.Unlock()
	identity := s.requestIdentity(req)
	if err := ss.root.BindBranch(ctx, identity, args.Branch, args.Clear); err != nil {
		return nil, BindBranchResult{}, err
	}
	status := "bound"
	if args.Clear {
		status = "cleared"
	}
	return nil, BindBranchResult{Branch: ss.root.Branch(), Status: status}, nil
}

// ensureShellInstance re-derives the session's shell instance after replay
// or rebind, auto-starting the default shell when none is running — a
// resumed pre-shell session gains a base to land on (d-tac-bfc).

func (s *Server) stageAttachment(ctx context.Context, req *mcp.CallToolRequest, args StageAttachmentArgs) (*mcp.CallToolResult, StageAttachmentResult, error) {
	if err := validAttachmentName(args.Name); err != nil {
		return nil, StageAttachmentResult{}, toolError("name: %v", err)
	}
	sources := 0
	for _, present := range []bool{args.Content != "", args.Path != "", len(args.Patches) > 0} {
		if present {
			sources++
		}
	}
	if sources != 1 {
		return nil, StageAttachmentResult{}, toolError("pass exactly one of content, path, or patches")
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, StageAttachmentResult{}, err
	}
	defer ss.mu.Unlock()
	identity := s.requestIdentity(req)
	if len(args.Patches) > 0 {
		pairs := make([]textpatch.Pair, 0, len(args.Patches))
		for _, p := range args.Patches {
			pairs = append(pairs, textpatch.Pair{Old: p.Old, New: p.New})
		}
		if err := ss.root.EditStagedAttachment(ctx, identity, args.Name, pairs); err != nil {
			return nil, StageAttachmentResult{}, err
		}
		return nil, StageAttachmentResult{Handle: args.Name}, nil
	}
	content := []byte(args.Content)
	if args.Path != "" {
		content, err = os.ReadFile(args.Path)
		if err != nil {
			return nil, StageAttachmentResult{}, toolError("reading %s: %v", args.Path, err)
		}
	}
	handle, err := ss.root.StageAttachment(ctx, identity, args.Name, content)
	if err != nil {
		return nil, StageAttachmentResult{}, err
	}
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

// readScope is what a free read takes from its session: the project it runs
// in — the home project with the session's branch binding, or a project the
// caller names that the session may work in (d-tac-1z6, d-cpt-yjc).
func (s *Server) readScope(ctx context.Context, req *mcp.CallToolRequest, ss *shellSession, project string) (sdd.ProjectID, string, bool, error) {
	return ss.root.ReadScope(ctx, s.requestIdentity(req), sdd.ProjectID(strings.TrimSpace(project)))
}

// viewHintServedKey is the served-once sentinel for the first-view breadcrumb,
// deduped per session through the same served memory as instruction blocks.
const viewHintServedKey = "view-layout-grammar-hint"

// viewHint is the single producer of the view tool's Hint field: a one-time
// pointer to the view-grammar fact on a session's first view call
// (s-prc-3kh). It never gates the read (s-cpt-1dz, d-cpt-h99).
func viewHint(ss *shellSession) string {
	if ss.servedBefore(viewHintServedKey) {
		return ""
	}
	return "view layout grammar: show " + basefacts.ViewGrammarFactID +
		" for the full filter/rank/macro vocabulary and the quoting rules"
}

func (s *Server) logRootRead(ctx context.Context, req *mcp.CallToolRequest, ss *shellSession, tool string, full, summary []string) error {
	return ss.root.LogRead(ctx, s.requestIdentity(req), tool, full, summary)
}

func (s *Server) search(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error) {
	if len(args.Terms) == 0 && args.Query == "" {
		return nil, SearchResult{}, toolError("pass terms (text mode), query (vector mode), or both (hybrid)")
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, SearchResult{}, err
	}
	defer ss.mu.Unlock()
	limit := args.Limit
	if limit == 0 {
		limit = defaultSearchHits
	}
	// Omitted means the shared default, so both surfaces show match evidence;
	// only an explicit 0 is headers-only (s-tac-rst).
	maxCitations := query.DefaultMaxCitationsPerEntry
	if args.MaxCitations != nil {
		maxCitations = *args.MaxCitations
	}
	project, branch, branchFromSession, err := s.readScope(ctx, req, ss, args.Project)
	if err != nil {
		return nil, SearchResult{}, err
	}
	result, err := s.app.Search(ctx, s.requestIdentity(req), project, sdd.SearchRequest{
		SyncMode: s.searchSyncMode, IncludesRevision: args.IncludesRevision,
		Terms: args.Terms, Phrase: args.Query, Type: args.Type, Layer: args.Layer, Kind: args.Kind,
		IncludeSuperseded: args.IncludeSuperseded, Limit: limit, MaxCitations: maxCitations,
		Branch: branch, BranchFromSession: branchFromSession, Repos: args.Repos, AllRepos: args.AllRepos,
	})
	if err != nil {
		return nil, SearchResult{}, toolError("searching: %v", err)
	}
	if err := s.logRootRead(ctx, req, ss, "search", nil, result.EntryIDs); err != nil {
		return nil, SearchResult{}, err
	}
	out := result.Results
	if strings.TrimSpace(out) == "" {
		out = "(no entries matched — try another phrasing, or proceed if the topic is genuinely new)"
	}
	return nil, SearchResult{Results: out, Coverage: result.Coverage, Notice: result.Notice}, nil

}

func (s *Server) view(ctx context.Context, req *mcp.CallToolRequest, args ViewArgs) (*mcp.CallToolResult, ViewResult, error) {
	if strings.TrimSpace(args.Layout) == "" {
		return nil, ViewResult{}, toolError("layout is required")
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, ViewResult{}, err
	}
	defer ss.mu.Unlock()
	project, branch, branchFromSession, err := s.readScope(ctx, req, ss, args.Project)
	if err != nil {
		return nil, ViewResult{}, err
	}
	result, err := s.app.View(ctx, s.requestIdentity(req), project, sdd.ViewRequest{
		Layout: args.Layout, Branch: branch, BranchFromSession: branchFromSession, Repos: args.Repos, AllRepos: args.AllRepos,
	})
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
	hint := viewHint(ss)
	if err := ss.flushServed(ctx, s.requestIdentity(req)); err != nil {
		return nil, ViewResult{}, err
	}
	return nil, ViewResult{Sections: sections, Hint: hint}, nil

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
	bounded := truncate.Bytes(s, maxViewResultBytes, "")
	if bounded.Cut.Clean() {
		return s
	}
	return fmt.Sprintf("%s\n\n[view output truncated at %d of %d bytes — narrow with n(K), tighter filters, or fewer sections]", bounded.Text, bounded.Cut.KeptBytes, bounded.Cut.TotalBytes)
}

// Value-projection caps, derived from the calibrated serveview budget so the
// wire surface and the engine share one upgrade knob (d-tac-qwc). Collected
// and produced values are the session store projected onto a serve, so the
// per-value cap is the store-value cap and the per-instance block cap is the
// engine-written values cap.
var (
	// maxCollectedValueBytes caps a single projected collected or produced
	// value. The values persist in full in the session store; the cap only
	// bounds what the serve carries so a large reported judgment cannot blow
	// the client's token budget.
	maxCollectedValueBytes = serveview.Default().Cap(serveview.PartStoreValue).MaxBytes

	// maxCollectedInstanceBytes caps the total value projection for one
	// instance. Per-value capping alone lets a field-heavy instance stack many
	// near-cap values into a payload that, across several resumed instances, can
	// reach the ~10K-token output ceiling where Codex CLI truncates a tool result
	// (s-tac-jom, s-tac-40d) — a silent cut is exactly what d-cpt-0tm forbids. The
	// budget keeps whole, high-signal values in sorted-key order and replaces the
	// rest with a labeled omission, so the resume serve stays comfortably under the
	// host ceiling while every dropped value is named, never silently gone.
	maxCollectedInstanceBytes = serveview.Default().Cap(serveview.PartProduced).MaxBytes

	// maxLineListBytes caps the whole-line lists MCP responses carry (open
	// threads, discarded threads, session rejections); cuts keep whole lines
	// and name the remainder.
	maxLineListBytes = serveview.Default().Cap(serveview.PartLineList).MaxBytes

	// resumeOpenBudgetBytes is the aggregate byte budget across a resume
	// projection's open instances — a reorientation serves everything in full,
	// and whole-instance omission is the only legitimate cut (d-tac-qwc).
	resumeOpenBudgetBytes = serveview.Default().Total
)

// maxResumeOpenInstances caps how many open instances a resume projection
// serves in full; the rest are named, each reachable through next.
const maxResumeOpenInstances = 8

// capValues renders each collected or produced value to a display string,
// truncates any over-cap value, and enforces a per-instance total budget by
// spending it on whole values in sorted-key order — anything past the budget
// is replaced with an explicit omission notice, so a resuming agent sees the
// compression rather than losing a field silently (d-cpt-0tm). Returns nil for
// an empty projection so the field is omitted.
func capValues(values map[string]any) map[string]string {
	if len(values) == 0 {
		return nil
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make(map[string]string, len(values))
	spent := 0
	for _, name := range names {
		value := capValue(collectedString(values[name]))
		if spent+len(value) > maxCollectedInstanceBytes {
			out[name] = "[collected value omitted here for size — persisted in full in the session store]"
			continue
		}
		spent += len(value)
		out[name] = value
	}
	return out
}

// collectedString renders a store value for the projection: strings pass
// through; everything else is JSON-encoded so lists and scalars stay legible.
// A marshal failure is stamped rather than silently rendered as Go syntax, so
// an encoding error is visible (mirrors StateSnapshot's "!err:" marker).
func collectedString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("[!json-encode-error: %v] %v", err, v)
	}
	return string(b)
}

// capValue truncates an over-cap value on a rune boundary and appends
// a notice naming the cap, mirroring guardViewSize.
func capValue(s string) string {
	bounded := truncate.Bytes(s, maxCollectedValueBytes, "")
	if bounded.Cut.Clean() {
		return s
	}
	return fmt.Sprintf("%s\n\n[collected value truncated at %d of %d bytes — persisted in full, compressed here]", bounded.Text, bounded.Cut.KeptBytes, bounded.Cut.TotalBytes)
}

func (s *Server) show(ctx context.Context, req *mcp.CallToolRequest, args ShowArgs) (*mcp.CallToolResult, ShowResult, error) {
	if len(args.IDs) == 0 {
		return nil, ShowResult{}, toolError("ids is required")
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, ShowResult{}, err
	}
	defer ss.mu.Unlock()
	up := sdd.DefaultShowUpDepth
	if args.Up != nil {
		up = *args.Up
	}
	down := sdd.DefaultShowDownDepth
	if args.Down != nil {
		down = *args.Down
	}
	project, branch, branchFromSession, err := s.readScope(ctx, req, ss, args.Project)
	if err != nil {
		return nil, ShowResult{}, err
	}
	result, err := s.app.Show(ctx, s.requestIdentity(req), project, sdd.ShowRequest{
		IDs: args.IDs, UpDepth: up, DownDepth: down, Branch: branch, BranchFromSession: branchFromSession,
	})
	if err != nil {
		return nil, ShowResult{}, err
	}
	if err := s.logRootRead(ctx, req, ss, "show", result.FullIDs, result.SummaryIDs); err != nil {
		return nil, ShowResult{}, err
	}
	return nil, ShowResult{Entries: result.Entries}, nil

}

func (s *Server) readAttachment(ctx context.Context, req *mcp.CallToolRequest, args ReadAttachmentArgs) (*mcp.CallToolResult, ReadAttachmentResult, error) {
	maxBytes := args.MaxBytes
	if maxBytes == 0 {
		maxBytes = 65536
	}
	if args.Handle != "" && args.ID != "" {
		return nil, ReadAttachmentResult{}, toolError("pass id or handle, not both")
	}
	if args.Handle == "" && args.ID == "" {
		return nil, ReadAttachmentResult{}, toolError("id is required (or handle, for a staged file)")
	}
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, ReadAttachmentResult{}, err
	}
	defer ss.mu.Unlock()
	if args.Handle != "" {
		page, staged, err := ss.root.ReadStagedAttachment(ctx, s.requestIdentity(req), args.Handle, args.Offset, maxBytes)
		if err != nil {
			return nil, ReadAttachmentResult{}, err
		}
		return nil, ReadAttachmentResult{
			Name: page.Filename, Content: string(page.Content), Offset: page.Offset, NextOffset: page.NextOffset,
			TotalBytes: page.TotalSize, More: page.More, Available: staged,
		}, nil
	}
	project, _, _, err := s.readScope(ctx, req, ss, args.Project)
	if err != nil {
		return nil, ReadAttachmentResult{}, err
	}
	result, err := s.app.ReadAttachment(ctx, s.requestIdentity(req), project, sdd.ReadAttachmentRequest{EntryID: args.ID, Filename: args.Name, Offset: args.Offset, MaxBytes: maxBytes})
	if err != nil {
		return nil, ReadAttachmentResult{}, err
	}
	if err := s.logRootRead(ctx, req, ss, "read_attachment", nil, []string{args.ID}); err != nil {
		return nil, ReadAttachmentResult{}, err
	}
	page := result.Page
	output := ReadAttachmentResult{
		Name: page.Filename, Content: string(page.Content), Offset: page.Offset, NextOffset: page.NextOffset,
		TotalBytes: page.TotalSize, More: page.More, Available: result.Available,
	}
	if s.local && s.localAttachmentPath != nil {
		output.Path, err = s.localAttachmentPath(args.ID, page.Filename)
		if err != nil {
			return nil, ReadAttachmentResult{}, err
		}
	}
	return nil, output, nil

}

func (s *Server) info(ctx context.Context, req *mcp.CallToolRequest, args InfoArgs) (*mcp.CallToolResult, InfoResult, error) {
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, InfoResult{}, err
	}
	defer ss.mu.Unlock()
	project, _, _, err := s.readScope(ctx, req, ss, args.Project)
	if err != nil {
		return nil, InfoResult{}, err
	}
	info, err := s.app.Info(ctx, s.requestIdentity(req), project, sdd.InfoRequest{})
	if err != nil {
		return nil, InfoResult{}, err
	}
	return nil, InfoResult{
		Project: string(info.Project.ID), Participant: info.Participant, Language: info.Language, Search: info.Search,
		Recovery: info.Recovery, Version: s.version,
	}, nil

}

func (s *Server) registryDocs(ctx context.Context, req *mcp.CallToolRequest, args RegistryArgs) (*mcp.CallToolResult, RegistryResult, error) {
	ss, err := s.attachedSession(ctx, req, args.Session)
	if err != nil {
		return nil, RegistryResult{}, err
	}
	ss.mu.Unlock()
	docs, err := sdd.WorkflowRegistryDocs(args.Class)
	if err != nil {
		return nil, RegistryResult{}, err
	}
	result := RegistryResult{}
	for _, doc := range docs {
		result.Functions = append(result.Functions, RegistryFuncResult{Name: doc.Name, Class: doc.Class, Doc: doc.Doc, Reads: doc.Reads, Writes: doc.Writes})
	}
	return nil, result, nil

}

// --- serve conversion --------------------------------------------------------

// Serve conversion applies the served-once memory: full text the first time
// the session's consumer sees these exact rendered bytes, a one-line stub
// after — identical bytes stub, changed content always serves in full
// (d-tac-dbk). The memory is derived from the session ledger and reset by a
// reorientation (d-cpt-aen).

// composeFraming renders the framing blocks with per-lane served-once dedup:
// each block the consumer already holds is omitted, so only blocks whose
// content is new serve in full. A graph write that changes one lane (the
// recent-moves lane) re-serves that lane alone, never the stable aspirations or
// directives — the per-lane dedup that keeps a reorientation from exceeding the
// original serve (I6).
func composeFraming(ss *shellSession, blocks []string) string {
	var served []string
	for _, block := range blocks {
		if block == "" || ss.servedBefore(block) {
			continue
		}
		served = append(served, block)
	}
	return strings.Join(served, "\n\n")
}

// serveResultBody converts the serve fields every landing shares —
// instructions with per-lane dedup, chooser, framing, open threads. Schema
// and vocabulary dedup live only in toRootServeResult: a base mapping drops
// those fields, and recording bytes never delivered would stub them for a
// later serve that does deliver.
func (s *Server) serveResultBody(ctx context.Context, req *mcp.CallToolRequest, ss *shellSession, serve *sdd.WorkflowServe) (ServeResult, error) {
	res := ServeResult{
		Session: string(serve.Session), Project: string(serve.Project),
		Branch: serve.Branch, Instance: serve.Instance, Procedure: serve.Procedure, Status: serve.Status,
		Step: serve.Step, Goal: serve.Goal, Instructions: composeInstructions(ss, serve), Missing: serve.Missing,
		ReportSchema: serve.ReportSchema, Produced: capValues(serve.Produced), Execution: serve.Execution,
		Collected: capValues(serve.Collected),
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
	res.Framing = composeFraming(ss, framing)
	if ss.root.IsShell(serve.Instance) {
		res.OpenThreads = openThreadsRoot(ss)
	}
	return res, nil
}

// composeInstructions applies the served-once memory at lane granularity:
// lanes new to the consumer serve in full, already-served lanes collapse into
// one line naming them, and a serve with no new lane falls back to the
// whole-step reminder (d-tac-87o).
func composeInstructions(ss *shellSession, serve *sdd.WorkflowServe) string {
	lanes := serve.InstructionLanes
	if len(lanes) == 0 {
		return serve.Instructions
	}
	var fresh, withheld []string
	for _, lane := range lanes {
		if ss.servedBefore(lane.Text) {
			withheld = append(withheld, lane.Name)
			continue
		}
		fresh = append(fresh, lane.Text)
	}
	if len(fresh) == 0 {
		return serve.ReminderInstructions()
	}
	text := strings.Join(fresh, "\n\n")
	if len(withheld) > 0 {
		text += fmt.Sprintf("\n\n(lanes served earlier this session: %s — resume_session with this session's handle re-serves this position in full)", strings.Join(withheld, ", "))
	}
	return serve.ComposeInstructions(text)
}

// reportSchemaStub replaces a schema the consumer already holds.
func reportSchemaStub(serve *sdd.WorkflowServe) map[string]any {
	return map[string]any{
		"served_earlier": fmt.Sprintf("identical to the %s/%s report schema served earlier this session — resume_session with this session's handle re-serves it in full", serve.Procedure, serve.Step),
	}
}

// toRootServeResult converts an engine serve into the tool response and records
// what it served in full into the session ledger. The shell's open-work block
// lists this dialogue's own threads only; other dialogues never appear.
func (s *Server) toRootServeResult(ctx context.Context, req *mcp.CallToolRequest, ss *shellSession, serve *sdd.WorkflowServe) (ServeResult, error) {
	res, err := s.serveResultBody(ctx, req, ss, serve)
	if err != nil {
		return ServeResult{}, err
	}
	// The schema is a derived lane: generated, static per procedure version,
	// deduped on its canonical bytes (Go maps marshal with sorted keys).
	if len(serve.ReportSchema) > 0 {
		if raw, err := json.Marshal(serve.ReportSchema); err == nil && ss.servedBefore("report_schema:"+string(raw)) {
			res.ReportSchema = reportSchemaStub(serve)
		}
	}
	vocabulary, err := s.app.Vocabulary(ctx, s.requestIdentity(req), ss.root.Project())
	if err != nil {
		return ServeResult{}, err
	}
	if vocabulary != "" && !ss.servedBefore(vocabulary) {
		res.Vocabulary = vocabulary
	}
	if serve.Base != nil {
		base, err := s.toBaseServeResult(ctx, req, ss, serve.Base)
		if err != nil {
			return ServeResult{}, err
		}
		res.Base = base
	}
	if err := ss.flushServed(ctx, s.requestIdentity(req)); err != nil {
		return ServeResult{}, err
	}
	return res, nil
}

// toBaseServeResult maps a serve onto the base_junction landing, which
// carries no schema or vocabulary — so neither is recorded as served here.
func (s *Server) toBaseServeResult(ctx context.Context, req *mcp.CallToolRequest, ss *shellSession, serve *sdd.WorkflowServe) (*BaseServe, error) {
	body, err := s.serveResultBody(ctx, req, ss, serve)
	if err != nil {
		return nil, err
	}
	if err := ss.flushServed(ctx, s.requestIdentity(req)); err != nil {
		return nil, err
	}
	return rootBaseServe(body), nil
}

func rootBaseServe(result ServeResult) *BaseServe {
	return &BaseServe{
		Session: result.Session, Branch: result.Branch, Instance: result.Instance, Procedure: result.Procedure, Status: result.Status,
		Step: result.Step, Goal: result.Goal, Instructions: result.Instructions, PendingChooser: result.PendingChooser,
		Framing: result.Framing, OpenThreads: result.OpenThreads,
	}
}

func (s *Server) mapRootResume(ctx context.Context, req *mcp.CallToolRequest, ss *shellSession, source sdd.WorkflowResumeResult) (ResumeSessionResult, error) {
	result := ResumeSessionResult{
		Session: string(source.Session), Project: string(ss.root.Project()),
		Participant: source.Participant, Label: source.Label, Branch: source.Branch,
		Instructions: resumeInstructions,
	}
	framing, err := ss.root.Framing(ctx, s.requestIdentity(req))
	if err != nil {
		return ResumeSessionResult{}, err
	}
	result.Framing = composeFraming(ss, framing)
	// Whole-instance granularity is the only legitimate cut on a resume
	// projection (d-tac-qwc): budget and count decisions run on the source
	// serve, before mapping, because mapping records served-once blocks and an
	// omitted instance must leave that ledger untouched for the serve that
	// does deliver it.
	var omitted []string
	spent := 0
	for i := range source.Open {
		serve := &source.Open[i]
		size := resumeServeBytes(serve)
		if len(result.Open) >= maxResumeOpenInstances || (len(result.Open) > 0 && spent+size > resumeOpenBudgetBytes) {
			omitted = append(omitted, fmt.Sprintf("%s (%s at %s)", serve.Instance, serve.Procedure, serve.Step))
			continue
		}
		mapped, err := s.toRootServeResult(ctx, req, ss, serve)
		if err != nil {
			return ResumeSessionResult{}, err
		}
		mapped.Framing = ""
		spent += size
		result.Open = append(result.Open, mapped)
	}
	if len(omitted) > 0 {
		result.Instructions += fmt.Sprintf("\n\n%d open moves omitted here for size — each resumes at its current step through next: %s", len(omitted), strings.Join(omitted, "; "))
	}
	if err := ss.flushServed(ctx, s.requestIdentity(req)); err != nil {
		return ResumeSessionResult{}, err
	}
	return result, nil
}

// resumeServeBytes estimates one open serve's projection weight from the
// fields the mapping actually carries (never InstructionLanes, which duplicate
// Instructions). Dedup can only shrink the mapped result, so the estimate errs
// toward omitting — never toward an oversized projection.
func resumeServeBytes(serve *sdd.WorkflowServe) int {
	n := len(serve.Instructions) + len(serve.Goal)
	if b, err := json.Marshal(serve.ReportSchema); err == nil {
		n += len(b)
	}
	for _, v := range capValues(serve.Collected) {
		n += len(v)
	}
	for _, v := range capValues(serve.Produced) {
		n += len(v)
	}
	return n
}

// openThreadsRoot renders the session shell's open-work block: this dialogue's
// own open threads only. Other dialogues are never pushed onto any serve —
// handles are issued, never published (d-cpt-aen).
func openThreadsRoot(ss *shellSession) string {
	var lines []string
	for _, instance := range ss.root.OpenInstances() {
		lines = append(lines, fmt.Sprintf("- %s: %s at %s", instance.Instance, instance.Procedure, instance.Step))
	}
	if len(lines) == 0 {
		return ""
	}
	body := boundLines(lines, "open threads")
	// The one serve that reports dropped work states it in full — never stubbed by
	// served-once dedup, and never framed as continuations it no longer offers.
	if ss.root.Finished() {
		return concludedThreadsIntro + "\n" + body
	}
	header := openThreadsReminder
	if !ss.servedBefore(openThreadsIntro) {
		header = openThreadsIntro
	}
	return header + "\n" + body
}

// boundList cuts a whole-item string list at the line-list cap, appending a
// remainder item naming the drop.
func boundList(items []string, what string) []string {
	bounded := truncate.Items(items, func(s string) int { return len(s) + 1 }, maxLineListBytes, "")
	if bounded.Cut.Dropped > 0 {
		return append(bounded.Items, fmt.Sprintf("+%d more %s", bounded.Cut.Dropped, what))
	}
	return items
}

// boundLines cuts a whole-line list at the line-list cap and names the
// remainder — the shared shape for every line list an MCP response carries.
func boundLines(lines []string, remainder string) string {
	bounded := truncate.Items(lines, func(s string) int { return len(s) + 1 }, maxLineListBytes, "")
	body := strings.Join(bounded.Items, "\n")
	if bounded.Cut.Dropped > 0 {
		body += fmt.Sprintf("\n(+%d more %s)", bounded.Cut.Dropped, remainder)
	}
	return body
}
