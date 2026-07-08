package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// framingLayout renders the session framing injected once per consumer:
// aspirations, guiding directives, active focus, and participants. Brief
// rendering plus heat-ranked caps keep the block a few KB — full summaries
// measured ~21KB, and framing is orientation, not decision material
// (d-tac-bfc's carve-out of the framing share of s-tac-4hh).
const framingLayout = `aspirations:rank(heat(exp-14d)):n(8):brief,` +
	`kind(directive):intent(guiding):active:rank(heat(exp-14d)):n(10):name("Guiding directives"):brief:as-list,` +
	`focus:brief,` +
	`participants:brief`

// defaultShellCanonical is the shell procedure start_session opens when the
// caller names none — the interactive user-dialogue base. Projects override
// conduct by superseding its chain; future shells (autonomous sessions)
// enter through the same door with an explicit shell param.
const defaultShellCanonical = "user-dialogue"

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
	Canonical string         `json:"canonical" jsonschema:"the procedure to start, by its stable name (e.g. capture)"`
	Params    map[string]any `json:"params,omitempty" jsonschema:"typed start inputs per the procedure's declaration: declared params, plus any declared state field the caller wants to seed at start (e.g. a known anchor)"`
	Label     string         `json:"label,omitempty" jsonschema:"short single-line subject label for the session (the dialogue); set it early, update when the subject sharpens"`
	Parent    string         `json:"parent,omitempty" jsonschema:"instance handle of the spawning instance, when this is a sub-move (e.g. a capture dispatched from an engage); records lineage in the session log"`
}

type NextArgs struct {
	Instance string `json:"instance" jsonschema:"instance handle from start_procedure"`
	// Report carries either state fields for the current step (per the served
	// report_schema) or a chooser answer {chooser, choice, userWords?, fields?}.
	Report map[string]any `json:"report" jsonschema:"state fields per the served report_schema, or a chooser answer object {chooser, choice, userWords?, fields?}"`
	Label  string         `json:"label,omitempty" jsonschema:"update the session's subject label when the dialogue's subject has sharpened"`
}

type AbandonArgs struct {
	Instance string `json:"instance,omitempty" jsonschema:"instance handle to abandon (a move in the bound session)"`
	Session  string `json:"session,omitempty" jsonschema:"parked session handle to tear down directly — no resume, no framing; pass exactly one of instance or session"`
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
	Session string `json:"session" jsonschema:"session handle from list_sessions"`
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
		Description: "Open the dialogue session — the door every session enters through. Auto-starts the " +
			"session shell (user-dialogue by default) and returns its opening serve: your orientation, " +
			"the available moves, and any parked work as continuation options. Call it once at the start; " +
			"call it again to have the orientation re-served.",
	}, s.startSession)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "start_procedure",
		Description: "Start a procedure instance (a playbook move such as capture) inside the open session. " +
			"Returns the current step's instructions, the report schema to answer with, and the goal that " +
			"advances it. This is the only path that leads to graph writes — writes happen inside " +
			"procedure transitions, never through a direct tool.",
	}, s.startProcedure)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "next",
		Description: "Advance a procedure instance: send state fields per the served report_schema, or " +
			"answer a pending chooser with {chooser, choice, userWords?, fields?}. User choosers must " +
			"carry the user's answer relayed verbatim in userWords. When a move ends, the response " +
			"carries the session shell's serve — where the dialogue lands.",
	}, s.next)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "abandon",
		Description: "Abandon a running move instance (instance), or tear down a parked session " +
			"directly by handle (session) — one call, no resume, no framing; the response names the " +
			"label and discarded threads. Nothing is cleaned up implicitly: held WIP markers are " +
			"surfaced and left standing for resume or grooming. The session shell concludes through " +
			"its own junction, never through abandon.",
	}, s.abandon)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "park",
		Description: "Park a running move back to the session junction: state and step position keep, " +
			"the move lists as an open thread (at junctions and on conclude), and next resumes it. " +
			"Use it when the user shelves work mid-dialogue — a seeded draft parks as a graph-visible " +
			"thread instead of living in conversation memory as an agent promise.",
	}, s.park)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "list_sessions",
		Description: "List parked dialogue sessions (open moves, not bound to a live connection) with participant, anchor, and last activity.",
	}, s.listSessions)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "resume_session",
		Description: "Switch this connection to a parked session: replays its log and returns the current " +
			"serve for every running instance. Step position and evidence persist across restarts. " +
			"Leaving the current session ends it when nothing is open, parks it when moves are open.",
	}, s.resumeSession)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "stage_attachment",
		Description: "Stage a file into the session scratch and get back a handle to pass in a report's " +
			"attachments field. Never a graph write — the write gate materializes staged files with the entry.",
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
		Description: "Session framing header: local participant, configured language, available search modes.",
	}, s.info)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "registry",
		Description: "Engine function contracts (predicates, queries, commands) — what procedure spec authors consult.",
	}, s.registryDocs)
}

// boundSession returns the SDD session bound to the calling connection, or
// a door-pointing error — every stateful tool requires the session that
// start_session opens. The rejection inlines the parked-sessions list
// (handle + label), so a fresh-context agent poking at a stale handle gets
// its bearings in the same round trip (d-tac-dbk).
func (s *Server) boundSession(ms *mcp.ServerSession) (*shellSession, error) {
	if ss := s.sessions.bound(ms); ss != nil {
		return ss, nil
	}
	msg := "no session is open — start_session is the door (reads stay free)"
	if descs, err := s.sessions.listOpenSessions(); err == nil && len(descs) > 0 {
		lines := make([]string, 0, len(descs))
		for _, d := range descs {
			line := d.Session
			if d.Label != "" {
				line += " " + strconv.Quote(d.Label)
			}
			lines = append(lines, line)
		}
		msg += "; parked sessions (resume_session picks one up): " + strings.Join(lines, ", ")
	}
	return nil, toolError("%s", msg)
}

// newShellSession creates a fresh SDD session (log, registry, engine),
// unbound — the door binds it after the shell procedure started.
func (s *Server) newShellSession() (*shellSession, error) {
	now := time.Now()
	id := newSessionID(now)
	logFile, err := s.sessions.openLog(id)
	if err != nil {
		return nil, err
	}

	participant := ""
	if info, err := s.finder.Info(query.InfoQuery{}); err == nil {
		participant = info.LocalParticipant
	}

	ss := &shellSession{
		id:           id,
		participant:  participant,
		logFile:      logFile,
		lastActivity: now,
	}
	registry, err := s.buildRegistry(ss)
	if err != nil {
		ss.close()
		return nil, err
	}
	ss.graphs = s.finder.NewGraphSource(s.graphDir)
	ss.engine = engine.New(registry, ss.graphs)
	ss.sess = ss.engine.NewSession(id, participant, engine.NewWriterSink(logFile))
	return ss, nil
}

func (s *Server) startSession(ctx context.Context, req *mcp.CallToolRequest, args StartSessionArgs) (*mcp.CallToolResult, ServeResult, error) {
	canonical := strings.TrimSpace(args.Shell)
	if canonical == "" {
		canonical = defaultShellCanonical
	}

	// Knocking on an already-open door re-serves the shell's orientation in
	// full — the "re-servable on demand" half of the tier-one contract. The
	// connection's served-block memory resets, so units and framing serve
	// full text again.
	if ss := s.sessions.bound(req.Session); ss != nil && ss.shellInstance != "" {
		if inst, ok := ss.sess.Instance(ss.shellInstance); ok && inst.Status == engine.StatusRunning {
			ss.touch(time.Now())
			if err := applyLabel(ss, args.Label); err != nil {
				return nil, ServeResult{}, err
			}
			s.forgetConnection(req.Session)
			serve, err := ss.sess.Serve(ss.shellInstance)
			if err != nil {
				return nil, ServeResult{}, err
			}
			return nil, s.toServeResult(req.Session, ss, serve), nil
		}
	}

	ss, err := s.newShellSession()
	if err != nil {
		return nil, ServeResult{}, err
	}
	spec, err := s.loadProcedure(ss, canonical)
	if err != nil {
		ss.close()
		return nil, ServeResult{}, err
	}
	if spec.Class != model.ProcedureClassShell {
		ss.close()
		return nil, ServeResult{}, toolError("%q is a move, not a session shell — open the session first, then start it with start_procedure", canonical)
	}
	serve, err := ss.sess.Start(spec, nil, "")
	if err != nil {
		ss.close()
		return nil, ServeResult{}, err
	}
	ss.shellInstance = serve.Instance
	// Label lands only now — after the canonical resolved and the shell
	// started (d-cpt-h99 rider: a rejected start must not label a session).
	if err := applyLabel(ss, args.Label); err != nil {
		return nil, ServeResult{}, err
	}
	prev := s.sessions.bind(req.Session, ss)
	s.watchDisconnect(req.Session)
	s.leaveSession(prev)
	return nil, s.toServeResult(req.Session, ss, serve), nil
}

func (s *Server) startProcedure(ctx context.Context, req *mcp.CallToolRequest, args StartProcedureArgs) (*mcp.CallToolResult, ServeResult, error) {
	if strings.TrimSpace(args.Canonical) == "" {
		return nil, ServeResult{}, toolError("canonical is required")
	}
	ss, err := s.boundSession(req.Session)
	if err != nil {
		return nil, ServeResult{}, err
	}
	ss.touch(time.Now())
	// Drop the memo so this advance loads fresh — it picks up writes from other
	// connections or the CLI since the last advance. Cheap and idempotent;
	// unlike the old refresh, forgetting it can only stale orientation, never
	// gate a write on a stale snapshot (the write path re-invalidates itself).
	ss.graphs.Invalidate()

	spec, err := s.loadProcedure(ss, args.Canonical)
	if err != nil {
		return nil, ServeResult{}, err
	}
	if spec.Class == model.ProcedureClassShell {
		return nil, ServeResult{}, toolError("%q is a session shell — sessions open through start_session, not start_procedure", args.Canonical)
	}
	// Label lands only after the canonical resolved (d-cpt-h99 rider): a
	// rejected start_procedure must not label the session.
	if err := applyLabel(ss, args.Label); err != nil {
		return nil, ServeResult{}, err
	}

	// Moves nest under the session shell by construction — an explicit
	// parent (a sub-move dispatched from a running move) wins.
	parent := args.Parent
	if parent == "" {
		parent = ss.shellInstance
	}
	serve, err := ss.sess.Start(spec, args.Params, parent)
	if err != nil {
		return nil, ServeResult{}, err
	}
	return nil, s.toServeResult(req.Session, ss, serve), nil
}

// maxLabelLen caps session labels — a label is a list row, not a summary.
const maxLabelLen = 120

// applyLabel validates and records an agent-supplied session label. Empty
// means "no update"; the engine skips appending an unchanged value.
func applyLabel(ss *shellSession, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	if strings.ContainsAny(label, "\n\r") {
		return toolError("label must be a single line")
	}
	if utf8.RuneCountInString(label) > maxLabelLen {
		return toolError("label exceeds %d characters — it is a list row, not a summary", maxLabelLen)
	}
	ss.sess.SetLabel(label)
	return nil
}

func (s *Server) next(ctx context.Context, req *mcp.CallToolRequest, args NextArgs) (*mcp.CallToolResult, ServeResult, error) {
	ss, err := s.boundSession(req.Session)
	if err != nil {
		return nil, ServeResult{}, err
	}
	if args.Instance == "" {
		return nil, ServeResult{}, toolError("instance is required")
	}
	if len(args.Report) == 0 {
		return nil, ServeResult{}, toolError("report is required: state fields per the served report_schema, or a chooser answer {chooser, choice, userWords?, fields?}")
	}
	ss.touch(time.Now())
	// Fresh load for this advance; see startProcedure for why this is safe as a
	// plain memo-drop rather than the old hand-maintained refresh.
	ss.graphs.Invalidate()
	if err := applyLabel(ss, args.Label); err != nil {
		return nil, ServeResult{}, err
	}

	var serve *engine.Serve
	if chooser, choice, ok := chooserAnswer(args.Report); ok {
		fields, _ := args.Report["fields"].(map[string]any)
		userWords, _ := args.Report["userWords"].(string)
		serve, err = ss.sess.Answer(args.Instance, chooser, choice, fields, userWords)
	} else {
		serve, err = ss.sess.Report(args.Instance, args.Report)
	}
	if err != nil {
		return nil, ServeResult{}, err
	}
	res := s.toServeResult(req.Session, ss, serve)
	// A move that ended lands the dialogue back on the session shell's
	// junction — told on every response, never left to agent memory.
	if serve.Status != engine.StatusRunning && serve.Instance != ss.shellInstance {
		res.Base = s.serveShell(req.Session, ss)
	}
	return nil, res, nil
}

// serveShell re-serves the session shell's current position — the landing
// every ended move returns the dialogue to. Nil when the session has no
// shell (should not happen behind the door; callers omit the field then).
func (s *Server) serveShell(ms *mcp.ServerSession, ss *shellSession) *BaseServe {
	if ss.shellInstance == "" || ss.sess == nil {
		return nil
	}
	serve, err := ss.sess.Serve(ss.shellInstance)
	if err != nil {
		return nil
	}
	res := s.toServeResult(ms, ss, serve)
	return &BaseServe{
		Session:        res.Session,
		Instance:       res.Instance,
		Procedure:      res.Procedure,
		Status:         res.Status,
		Step:           res.Step,
		Goal:           res.Goal,
		Instructions:   res.Instructions,
		PendingChooser: res.PendingChooser,
		Framing:        res.Framing,
		OpenThreads:    res.OpenThreads,
	}
}

// chooserAnswer detects the chooser-answer form of a report: an object
// carrying chooser + choice. The two key names are reserved — no procedure
// state field may use them (spec authors: name fields otherwise).
func chooserAnswer(report map[string]any) (chooser, choice string, ok bool) {
	chooser, hasChooser := report["chooser"].(string)
	choice, hasChoice := report["choice"].(string)
	return chooser, choice, hasChooser && hasChoice && chooser != "" && choice != ""
}

func (s *Server) abandon(ctx context.Context, req *mcp.CallToolRequest, args AbandonArgs) (*mcp.CallToolResult, AbandonResult, error) {
	if (args.Instance == "") == (args.Session == "") {
		return nil, AbandonResult{}, toolError("pass exactly one of instance (abandon a move in the bound session) or session (tear down a parked session)")
	}
	if args.Session != "" {
		return s.abandonSession(req, args)
	}
	ss, err := s.boundSession(req.Session)
	if err != nil {
		return nil, AbandonResult{}, err
	}
	if args.Instance == ss.shellInstance {
		return nil, AbandonResult{}, toolError("the session shell concludes through its own junction (answer conclude) — abandon is for moves")
	}
	ss.touch(time.Now())

	var held []string
	if inst, ok := ss.sess.Instance(args.Instance); ok {
		if marker, present := storeString(inst.Store, "wipMarker"); present {
			held = append(held, marker)
		}
	}
	if err := ss.sess.Abandon(args.Instance, args.Reason); err != nil {
		return nil, AbandonResult{}, err
	}
	res := AbandonResult{Abandoned: true, HeldMarkers: held}
	if len(held) > 0 {
		res.Instructions = "The instance holds WIP markers, left standing by design. Tell the user: resume the work later or close the markers through grooming."
	}
	// Abandoning a move lands the dialogue back on the shell junction.
	res.Base = s.serveShell(req.Session, ss)
	return nil, res, nil
}

// park shelves a running move back to the shell junction: nothing about the
// instance changes — state, step, and evidence keep — but the shelving is
// logged (legible to a resuming agent) and the response lands the dialogue
// on the shell, with the move now listed among the open threads
// (d-tac-dbk). Seeded drafts ride the normal dispatch path; park adds no
// side channel.
func (s *Server) park(ctx context.Context, req *mcp.CallToolRequest, args ParkArgs) (*mcp.CallToolResult, ParkResult, error) {
	ss, err := s.boundSession(req.Session)
	if err != nil {
		return nil, ParkResult{}, err
	}
	if args.Instance == "" {
		return nil, ParkResult{}, toolError("instance is required")
	}
	if args.Instance == ss.shellInstance {
		return nil, ParkResult{}, toolError("the session shell is the junction moves park back to — park is for moves")
	}
	ss.touch(time.Now())

	inst, ok := ss.sess.Instance(args.Instance)
	if !ok {
		return nil, ParkResult{}, toolError("instance %q not found in session", args.Instance)
	}
	if err := ss.sess.Park(args.Instance, args.Note); err != nil {
		return nil, ParkResult{}, err
	}
	res := ParkResult{
		Parked:    true,
		Instance:  inst.ID,
		Procedure: inst.Spec.Canonical,
		Step:      inst.Step,
		Instructions: "The move is parked with its state kept — it stays listed as an open thread " +
			"and resumes through next. Tell the user it is recorded as open work, not forgotten.",
		Base: s.serveShell(req.Session, ss),
	}
	return nil, res, nil
}

// abandonSession tears down a parked session by handle: no resume, no
// framing, one call (d-tac-dbk; the measured baseline was six calls and
// ~28KB framing per session). The response names the label and every
// discarded thread — nothing vanishes silently. Needs no bound session:
// teardown is maintenance, not dialogue.
func (s *Server) abandonSession(req *mcp.CallToolRequest, args AbandonArgs) (*mcp.CallToolResult, AbandonResult, error) {
	current := s.sessions.bound(req.Session)
	if current != nil && current.id == args.Session {
		return nil, AbandonResult{}, toolError("session %s is the one this connection is in — the session shell concludes through its own junction (answer conclude)", args.Session)
	}
	if s.sessions.liveIDs()[args.Session] {
		return nil, AbandonResult{}, toolError("session %s is live on another connection — only parked sessions tear down", args.Session)
	}

	res := AbandonResult{Abandoned: true, Session: args.Session}
	if live := s.sessions.lookupID(args.Session); live != nil && live.logFile != nil && live.sess != nil {
		// Parked in memory: end every running instance through the engine
		// session, whose log file is already open.
		res.Label = live.sess.Label
		for _, inst := range live.sess.Instances() {
			if inst.Status != engine.StatusRunning {
				continue
			}
			if inst.ID != live.shellInstance {
				res.DiscardedThreads = append(res.DiscardedThreads, inst.Spec.Canonical+" at "+inst.Step)
				if marker, ok := storeString(inst.Store, "wipMarker"); ok {
					res.HeldMarkers = append(res.HeldMarkers, marker)
				}
			}
			if err := live.sess.Abandon(inst.ID, args.Reason); err != nil {
				return nil, AbandonResult{}, err
			}
		}
		live.close()
		s.sessions.drop(args.Session)
	} else {
		// On disk: append terminal events straight to the log — no replay, so
		// even a version-skewed session tears down.
		events, err := s.sessions.readLog(args.Session)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, AbandonResult{}, toolError("unknown session %q — list_sessions shows what is parked", args.Session)
			}
			return nil, AbandonResult{}, err
		}
		desc := deriveDescriptor(args.Session, events)
		res.Label = desc.Label
		for _, inst := range desc.Open {
			res.DiscardedThreads = append(res.DiscardedThreads, inst.Procedure+" at "+inst.Step)
		}
		running, markers := teardownFold(events)
		res.HeldMarkers = markers
		if err := s.sessions.appendAbandons(args.Session, events, running, args.Reason); err != nil {
			return nil, AbandonResult{}, err
		}
	}
	if len(res.HeldMarkers) > 0 {
		res.Instructions = "The discarded threads hold WIP markers, left standing by design. Tell the user: resume the work later or close the markers through grooming."
	}
	if current != nil {
		res.Base = s.serveShell(req.Session, current)
	}
	return nil, res, nil
}

func (s *Server) listSessions(ctx context.Context, req *mcp.CallToolRequest, _ ListSessionsArgs) (*mcp.CallToolResult, ListSessionsResult, error) {
	if _, err := s.boundSession(req.Session); err != nil {
		return nil, ListSessionsResult{}, err
	}
	descs, err := s.sessions.listOpenSessions()
	if err != nil {
		return nil, ListSessionsResult{}, err
	}
	return nil, ListSessionsResult{Sessions: descs}, nil
}

func (s *Server) resumeSession(ctx context.Context, req *mcp.CallToolRequest, args ResumeSessionArgs) (*mcp.CallToolResult, ResumeSessionResult, error) {
	current, err := s.boundSession(req.Session)
	if err != nil {
		return nil, ResumeSessionResult{}, err
	}
	if args.Session == "" {
		return nil, ResumeSessionResult{}, toolError("session is required")
	}
	if args.Session == current.id {
		return nil, ResumeSessionResult{}, toolError("session %s is the one this connection is already in", args.Session)
	}

	if live := s.sessions.lookupID(args.Session); live != nil && live.logFile != nil {
		// The session is already in memory. Bound to another connection it
		// is live, not parked — refuse rather than yanking it over. Parked
		// in memory, rebind without replaying over the open log; served-once
		// memory is per connection, so a genuinely new consumer gets full
		// text while a same-connection switch never re-pays it.
		if s.sessions.liveIDs()[args.Session] {
			return nil, ResumeSessionResult{}, toolError("session %s is live on another connection — only parked sessions resume", args.Session)
		}
		if err := s.ensureShellInstance(live); err != nil {
			return nil, ResumeSessionResult{}, err
		}
		prev := s.sessions.bind(req.Session, live)
		s.leaveSession(prev)
		res, err := s.resumeResult(req.Session, live)
		return nil, res, err
	}

	events, err := s.sessions.readLog(args.Session)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ResumeSessionResult{}, toolError("unknown session %q — list_sessions shows what is resumable", args.Session)
		}
		return nil, ResumeSessionResult{}, err
	}
	desc := deriveDescriptor(args.Session, events)

	ss := &shellSession{
		id:           args.Session,
		participant:  desc.Participant,
		lastActivity: time.Now(),
	}
	registry, err := s.buildRegistry(ss)
	if err != nil {
		return nil, ResumeSessionResult{}, err
	}
	ss.graphs = s.finder.NewGraphSource(s.graphDir)
	ss.engine = engine.New(registry, ss.graphs)

	resolve := func(canonical string) (*engine.Spec, error) {
		return s.loadProcedure(ss, canonical)
	}
	logFile, err := s.sessions.openLog(args.Session)
	if err != nil {
		return nil, ResumeSessionResult{}, err
	}
	sess, err := ss.engine.ReplaySession(args.Session, desc.Participant, events, resolve, engine.NewWriterSink(logFile))
	if err != nil {
		_ = logFile.Close()
		return nil, ResumeSessionResult{}, fmt.Errorf("replaying session %s: %v", args.Session, err)
	}
	ss.logFile = logFile
	ss.sess = sess
	if err := s.ensureShellInstance(ss); err != nil {
		ss.close()
		return nil, ResumeSessionResult{}, err
	}
	prev := s.sessions.bind(req.Session, ss)
	s.leaveSession(prev)
	res, err := s.resumeResult(req.Session, ss)
	return nil, res, err
}

// ensureShellInstance re-derives the session's shell instance after replay
// or rebind, auto-starting the default shell when none is running — a
// resumed pre-shell session gains a base to land on (d-tac-bfc).
func (s *Server) ensureShellInstance(ss *shellSession) error {
	for _, inst := range ss.sess.Instances() {
		if inst.Status == engine.StatusRunning && inst.Spec.Class == model.ProcedureClassShell {
			ss.shellInstance = inst.ID
			return nil
		}
	}
	ss.shellInstance = ""
	spec, err := s.loadProcedure(ss, defaultShellCanonical)
	if err != nil {
		return fmt.Errorf("auto-starting session shell: %v", err)
	}
	serve, err := ss.sess.Start(spec, nil, "")
	if err != nil {
		return fmt.Errorf("auto-starting session shell: %v", err)
	}
	ss.shellInstance = serve.Instance
	return nil
}

func (s *Server) resumeResult(ms *mcp.ServerSession, ss *shellSession) (ResumeSessionResult, error) {
	res := ResumeSessionResult{
		Session:      ss.id,
		Participant:  ss.sess.Participant,
		Label:        ss.sess.Label,
		Instructions: resumeInstructions,
	}
	// Framing rides the resume result itself, not the per-instance serves —
	// computed first so the per-serve framing below dedups against it. A
	// same-connection resume after the door already paid orientation gets
	// nothing here (s-tac-w3v); a fresh connection gets it in full.
	res.Framing = s.framingBlock(ms, ss)
	for _, inst := range ss.sess.Instances() {
		if inst.Status != engine.StatusRunning {
			continue
		}
		serve, err := ss.sess.Serve(inst.ID)
		if err != nil {
			return ResumeSessionResult{}, err
		}
		res.Open = append(res.Open, s.toServeResult(ms, ss, serve))
	}
	// Open threads need no separate slot: the shell's serve in the Open
	// list carries the block.
	for i := range res.Open {
		res.Open[i].Framing = ""
	}
	return res, nil
}

func (s *Server) stageAttachment(ctx context.Context, req *mcp.CallToolRequest, args StageAttachmentArgs) (*mcp.CallToolResult, StageAttachmentResult, error) {
	if err := validAttachmentName(args.Name); err != nil {
		return nil, StageAttachmentResult{}, toolError("name: %v", err)
	}
	if (args.Content == "") == (args.Path == "") {
		return nil, StageAttachmentResult{}, toolError("pass exactly one of content or path")
	}
	ss, err := s.boundSession(req.Session)
	if err != nil {
		return nil, StageAttachmentResult{}, err
	}
	ss.touch(time.Now())

	dir, err := s.sessions.stagingDir(ss.id)
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
	if err := os.WriteFile(filepath.Join(dir, args.Name), content, 0o644); err != nil {
		return nil, StageAttachmentResult{}, fmt.Errorf("staging attachment: %w", err)
	}
	return nil, StageAttachmentResult{Handle: args.Name}, nil
}

// selectRepoIDs resolves a cross-repo selection through the injected
// registry. A selection without wiring is a server construction gap — fail
// loud, never silently narrow to local-only.
func (s *Server) selectRepoIDs(named []string, all bool) ([]string, error) {
	if !all && len(named) == 0 {
		return nil, nil
	}
	if s.repos == nil {
		return nil, fmt.Errorf("connected-repos support is not wired on this server")
	}
	return s.repos.SelectRepoIDs(named, all)
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

// logRead records a read event on the calling connection's bound session.
// Reads stay free and ungated — tracking is logging, not gating (d-tac-dbk);
// with no session bound there is nothing to log against.
func (s *Server) logRead(ms *mcp.ServerSession, tool string, full, summary []string) {
	if ss := s.sessions.bound(ms); ss != nil && ss.sess != nil {
		ss.sess.LogRead(tool, full, summary)
	}
}

// showReads extracts the read-event ID sets from a show result: primaries
// render their bodies (full depth), chain items render as summary bullets.
func showReads(res *query.ShowResult) (full, summary []string) {
	for _, g := range res.Groups {
		if g.Primary != nil {
			if g.PrimaryID != "" {
				full = append(full, g.PrimaryID)
			} else {
				full = append(full, g.Primary.ID)
			}
		}
		for _, items := range [][]model.ShowTreeItem{g.Upstream, g.Downstream} {
			for _, item := range items {
				if item.Entry != nil {
					summary = append(summary, item.NodeID())
				}
			}
		}
	}
	return full, summary
}

func (s *Server) search(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error) {
	if len(args.Terms) == 0 && args.Query == "" {
		return nil, SearchResult{}, toolError("pass terms (text mode), query (vector mode), or both (hybrid)")
	}
	if args.Query != "" && !s.vector {
		return nil, SearchResult{}, toolError("query needs a configured embedding provider — this server has text mode only; use terms")
	}
	if s.searcher == nil {
		return nil, SearchResult{}, toolError("search is not configured on this server")
	}
	graph, err := s.finder.CurrentGraph(s.graphDir)
	if err != nil {
		return nil, SearchResult{}, toolError("loading graph: %v", err)
	}

	// Drill-serve defaults (d-tac-dbk): header-only, hit-capped — the
	// measured 26.5KB drill was this tool's response with snippet defaults.
	// Snippets stay one explicit parameter away; depth flows through show,
	// where it is logged as inspection.
	limit := args.Limit
	if limit == 0 {
		limit = defaultSearchHits
	}
	sq := query.SearchQuery{
		Graph:                graph,
		Terms:                args.Terms,
		Phrase:               args.Query,
		IncludeSuperseded:    args.IncludeSuperseded,
		Limit:                limit,
		MaxCitationsPerEntry: 0,
		Repos:                args.Repos,
		AllRepos:             args.AllRepos,
	}
	if args.MaxCitations != nil {
		sq.MaxCitationsPerEntry = *args.MaxCitations
	}
	if args.Type != "" {
		t, err := parseEntryType(args.Type)
		if err != nil {
			return nil, SearchResult{}, err
		}
		sq.Filter.Type = t
	}
	if args.Layer != "" {
		l, err := parseLayer(args.Layer)
		if err != nil {
			return nil, SearchResult{}, err
		}
		sq.Filter.Layer = l
	}
	if args.Kind != "" {
		sq.Filter.Kind = model.Kind(args.Kind)
	}

	res, err := s.searcher.Search(ctx, sq)
	if err != nil {
		return nil, SearchResult{}, toolError("searching: %v", err)
	}
	var hits []string
	for _, e := range res.Entries {
		if e.Entry != nil {
			hits = append(hits, e.DisplayID())
		}
	}
	s.logRead(req.Session, "search", nil, hits)
	var sb strings.Builder
	presenters.RenderSearch(&sb, res, graph)
	out := sb.String()
	if strings.TrimSpace(out) == "" {
		out = "(no entries matched — try another phrasing, or proceed if the topic is genuinely new)"
	}
	return nil, SearchResult{Results: out, Hint: s.readHint(req.Session)}, nil
}

func (s *Server) view(ctx context.Context, req *mcp.CallToolRequest, args ViewArgs) (*mcp.CallToolResult, ViewResult, error) {
	if strings.TrimSpace(args.Layout) == "" {
		return nil, ViewResult{}, toolError("layout is required")
	}
	repoIDs, err := s.selectRepoIDs(args.Repos, args.AllRepos)
	if err != nil {
		return nil, ViewResult{}, toolError("%v", err)
	}
	if len(repoIDs) > 0 {
		if _, err := s.handler.EnsureReposFresh(ctx, repoIDs); err != nil {
			return nil, ViewResult{}, toolError("refreshing repo caches: %v", err)
		}
	}
	graph, err := s.finder.CurrentGraph(s.graphDir)
	if err != nil {
		return nil, ViewResult{}, toolError("loading graph: %v", err)
	}
	out, err := s.renderView(graph, args.Layout)
	if err != nil {
		return nil, ViewResult{}, err
	}
	// Selected repos render the same layout over their own graphs, each
	// under a repo heading — entry IDs inside a repo section are scoped by
	// that heading.
	var sb strings.Builder
	sb.WriteString(out)
	for _, repoID := range repoIDs {
		member, err := graph.MemberGraph(repoID)
		if err != nil {
			return nil, ViewResult{}, toolError("loading graph for %s: %v", repoID, err)
		}
		if member == nil {
			sb.WriteString("\n── repo: " + repoID + " (unavailable) ──\n")
			continue
		}
		mout, err := s.renderView(member, args.Layout)
		if err != nil {
			return nil, ViewResult{}, err
		}
		sb.WriteString("\n── repo: " + repoID + " ──\n")
		sb.WriteString(mout)
	}
	return nil, ViewResult{Sections: sb.String(), Hint: s.readHint(req.Session)}, nil
}

func (s *Server) show(ctx context.Context, req *mcp.CallToolRequest, args ShowArgs) (*mcp.CallToolResult, ShowResult, error) {
	if len(args.IDs) == 0 {
		return nil, ShowResult{}, toolError("ids is required")
	}
	// Cross-repo IDs read through the connected-repos caches — freshen the
	// named repos first (lazy clone + cooldown pull).
	if repoIDs := model.CrossRepoIDs(args.IDs); len(repoIDs) > 0 {
		if _, err := s.handler.EnsureReposFresh(ctx, repoIDs); err != nil {
			return nil, ShowResult{}, toolError("refreshing repo caches: %v", err)
		}
	}
	graph, err := s.finder.CurrentGraph(s.graphDir)
	if err != nil {
		return nil, ShowResult{}, toolError("loading graph: %v", err)
	}
	up := args.Up
	if up == 0 {
		up = query.DefaultUpDepth
	}
	down := args.Down
	if down == 0 {
		down = query.DefaultDownDepth
	}
	out, res, err := s.renderShow(graph, args.IDs, up, down)
	if err != nil {
		return nil, ShowResult{}, err
	}
	full, summary := showReads(res)
	s.logRead(req.Session, "show", full, summary)
	return nil, ShowResult{Entries: out, Hint: s.readHint(req.Session)}, nil
}

func (s *Server) readAttachment(ctx context.Context, req *mcp.CallToolRequest, args ReadAttachmentArgs) (*mcp.CallToolResult, ReadAttachmentResult, error) {
	if args.ID == "" {
		return nil, ReadAttachmentResult{}, toolError("id is required")
	}
	graph, err := s.finder.CurrentGraph(s.graphDir)
	if err != nil {
		return nil, ReadAttachmentResult{}, toolError("loading graph: %v", err)
	}
	res, err := s.finder.ReadAttachment(query.ReadAttachmentQuery{
		Graph:    graph,
		GraphDir: s.graphDir,
		EntryID:  args.ID,
		Name:     args.Name,
		Offset:   args.Offset,
		MaxBytes: args.MaxBytes,
	})
	if err != nil {
		return nil, ReadAttachmentResult{}, err
	}
	// An attachment read is neither a body serve nor a chain bullet — it logs
	// at summary depth under its own tool name, so the audit trail keeps the
	// distinction while the inspection gate stays body-strict.
	s.logRead(req.Session, "read_attachment", nil, []string{args.ID})
	out := ReadAttachmentResult{
		Name:       res.Name,
		Content:    res.Content,
		Offset:     res.Offset,
		NextOffset: res.NextOffset,
		TotalBytes: res.TotalBytes,
		More:       res.More,
		Available:  res.Available,
		Hint:       s.readHint(req.Session),
	}
	if s.local {
		out.Path = res.Path
	}
	return nil, out, nil
}

func (s *Server) info(ctx context.Context, req *mcp.CallToolRequest, _ InfoArgs) (*mcp.CallToolResult, InfoResult, error) {
	info, err := s.finder.Info(query.InfoQuery{})
	if err != nil {
		return nil, InfoResult{}, err
	}
	return nil, InfoResult{
		Participant: info.LocalParticipant,
		Language:    info.Language,
		Search:      info.Search,
		Version:     s.version,
		Hint:        s.readHint(req.Session),
	}, nil
}

func (s *Server) registryDocs(ctx context.Context, req *mcp.CallToolRequest, args RegistryArgs) (*mcp.CallToolResult, RegistryResult, error) {
	var class engine.FuncClass
	switch args.Class {
	case "", string(engine.ClassPredicate), string(engine.ClassQuery), string(engine.ClassCommand):
		class = engine.FuncClass(args.Class)
	default:
		return nil, RegistryResult{}, toolError("unknown class %q: predicate, query, or command", args.Class)
	}
	res := RegistryResult{Hint: s.readHint(req.Session)}
	for _, doc := range s.docsRegistry.Docs(class) {
		res.Functions = append(res.Functions, RegistryFuncResult{
			Name:   doc.Name,
			Class:  string(doc.Class),
			Doc:    doc.Doc,
			Reads:  doc.Reads,
			Writes: doc.Writes,
		})
	}
	return nil, res, nil
}

// --- serve conversion --------------------------------------------------------

// toServeResult converts an engine serve into the tool response, applying
// served-once memory: full text the first time this connection sees these
// exact rendered bytes, a one-line stub after — identical bytes stub,
// changed content always serves in full (d-tac-dbk).
func (s *Server) toServeResult(ms *mcp.ServerSession, ss *shellSession, serve *engine.Serve) ServeResult {
	res := ServeResult{
		Session:      ss.id,
		Instance:     serve.Instance,
		Procedure:    serve.Procedure,
		Status:       string(serve.Status),
		Step:         serve.Step,
		Goal:         serve.Goal,
		Instructions: serve.Instructions,
		Missing:      serve.Missing,
		ReportSchema: serve.ReportSchema,
		Produced:     serve.Produced,
	}
	if serve.Unit != "" && s.servedBefore(ms, serve.UnitText) {
		reminder := fmt.Sprintf("(step %s instructions were served earlier this session — follow them; goal: %s)", serve.Step, serve.Goal)
		res.Instructions = engine.ComposeInstructions(reminder, serve.Diagnostics)
	}
	// Open threads ride the session shell's serves only — the junction the
	// dialogue stands on. Mid-procedure serves never carry the block.
	if ss.shellInstance != "" && serve.Instance == ss.shellInstance {
		res.OpenThreads = s.openThreadsBlock(ms, ss, true)
	}
	if serve.Chooser != nil {
		ch := &ChooserResult{Chooser: serve.Chooser.Chooser, Kind: string(serve.Chooser.Kind)}
		for _, o := range serve.Chooser.Options {
			opt := ChooserOptionResult{Choice: o.Choice}
			for _, cf := range o.Collect {
				name := cf.Name
				if cf.Optional {
					name += "?"
				}
				opt.Collect = append(opt.Collect, name)
			}
			ch.Options = append(ch.Options, opt)
		}
		res.PendingChooser = ch
	}
	// A task prefers a disposable forked context — surface the hint so a
	// harness that automates the fork has a structured signal, not just prose.
	if inst, ok := ss.sess.Instance(serve.Instance); ok && inst.Spec.Class == model.ProcedureClassTask {
		res.Execution = executionForkPreferred
	}
	res.Framing = s.framingBlock(ms, ss)
	// The vocabulary block is static per process — the connection pays it
	// exactly once, on whatever serve it sees first.
	if s.vocabulary != "" && !s.servedBefore(ms, s.vocabulary) {
		res.Vocabulary = s.vocabulary
	}
	return res
}

// executionForkPreferred is the execution hint carried on task-class serves:
// the procedure is best dispatched to a disposable forked context, inline as a
// paid fallback (d-tac-tlo). Consuming it in harness automation stays deferred.
const executionForkPreferred = "fork-preferred"

// framingBlock returns the session framing for this serve: the full render
// the first time this connection sees these exact bytes, empty when
// unchanged since. Changed content — the graph moved mid-session, a focus
// was captured — always serves in full; there are no semantic skip rules
// (d-tac-dbk). Re-orientation on demand goes through start_session, which
// clears the connection's memory.
func (s *Server) framingBlock(ms *mcp.ServerSession, ss *shellSession) string {
	framing := s.renderFraming(ss)
	if s.servedBefore(ms, framing) {
		return ""
	}
	return framing
}

// renderFraming renders the session framing, cached per graph value — the
// source memoizes within one advance, so the cache mainly spares re-renders
// within one response (a resume rehydrating several serves). The pointer key
// still invalidates correctly: a reload after Invalidate hands back a new graph
// pointer, missing the cache and re-rendering.
func (s *Server) renderFraming(ss *shellSession) string {
	graph, err := ss.graphs.Current()
	if err == nil && ss.framingGraph == graph && ss.framingText != "" {
		return ss.framingText
	}
	var sb strings.Builder
	if info, ierr := s.finder.Info(query.InfoQuery{}); ierr == nil {
		fmt.Fprintf(&sb, "Local participant: %s\n", info.LocalParticipant)
		if info.Language != "" {
			fmt.Fprintf(&sb, "Language: %s\n", info.Language)
		}
		fmt.Fprintf(&sb, "Search: %s\n\n", info.Search)
	}
	text := sb.String()
	if err != nil {
		// The graph is unavailable — degrade to the info header and don't cache,
		// so a later successful load re-renders. In practice a load failure has
		// already surfaced on the advance that produced this serve.
		return text
	}
	// A failed view render degrades to the info header — framing is
	// orientation, not a gate.
	if framing, verr := s.renderView(graph, framingLayout); verr == nil {
		text += framing
	}
	ss.framingGraph = graph
	ss.framingText = text
	return text
}

// loadProcedure resolves a canonical to its execution head and loads the
// spec against the session's registry. The not-found error lists move
// canonicals only — shells enter through the door, not this path.
func (s *Server) loadProcedure(ss *shellSession, canonical string) (*engine.Spec, error) {
	graph, err := ss.graphs.Current()
	if err != nil {
		return nil, fmt.Errorf("loading graph: %w", err)
	}
	entry := graph.ResolveProcedure(canonical)
	if entry == nil {
		available := make([]string, 0)
		for _, chain := range graph.ProcedureChains() {
			if chain.Head != nil && chain.Head.Canonical != "" && !chain.Head.IsShellProcedure() && !chain.Head.IsTaskProcedure() {
				available = append(available, chain.Head.Canonical)
			}
		}
		return nil, toolError("no procedure %q (available: %s)", canonical, strings.Join(available, ", "))
	}
	return engine.LoadSpec(entry, ss.engine.Registry)
}

// renderView parses a layout, executes it against the graph, and renders
// the plain-text result — the same pipeline `sdd view --layout` drives.
func (s *Server) renderView(graph *model.Graph, layout string) (string, error) {
	parsed, err := query.ParseLayout(layout)
	if err != nil {
		return "", fmt.Errorf("parsing layout %q: %w", layout, err)
	}
	parsed, err = query.ExpandMacros(parsed)
	if err != nil {
		return "", fmt.Errorf("expanding layout %q: %w", layout, err)
	}
	res, err := s.finder.View(query.ViewQuery{
		Graph:    graph,
		Layout:   parsed,
		GraphDir: s.graphDir,
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	presenters.RenderView(&sb, res)
	return sb.String(), nil
}

// renderShow renders entries with chains as plain markdown, returning the
// structured result alongside so callers can log what was served.
func (s *Server) renderShow(graph *model.Graph, ids []string, up, down int) (string, *query.ShowResult, error) {
	res, err := s.finder.Show(query.ShowQuery{
		Graph:     graph,
		IDs:       ids,
		UpDepth:   up,
		DownDepth: down,
	})
	if err != nil {
		return "", nil, err
	}
	var sb strings.Builder
	presenters.RenderShow(&sb, res, presenters.ShowOptions{})
	return sb.String(), res, nil
}

func parseEntryType(in string) (model.EntryType, error) {
	if t, ok := model.TypeFromAbbrev[in]; ok {
		return t, nil
	}
	t := model.EntryType(in)
	if _, ok := model.TypeAbbrev[t]; ok {
		return t, nil
	}
	return "", toolError("invalid type %q: use s (signal) or d (decision)", in)
}

func parseLayer(in string) (model.Layer, error) {
	if l, ok := model.LayerFromAbbrev[in]; ok {
		return l, nil
	}
	l := model.Layer(in)
	if _, ok := model.LayerAbbrev[l]; ok {
		return l, nil
	}
	return "", toolError("invalid layer %q: use stg, cpt, tac, ops, or prc", in)
}
